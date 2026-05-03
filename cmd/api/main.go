// Package main — точка входа RepricerX API.
//
//	@title			RepricerX API
//	@version		1.0
//	@description	REST API сервиса автоматического управления ценами на маркетплейсах Wildberries и Ozon.
//	@description
//	@description	## Аутентификация
//	@description	После успешного входа (POST /api/auth/login) сервер устанавливает HttpOnly-cookie `rx_session`.
//	@description	Браузер отправляет его автоматически; при работе через curl/Postman передавайте `-b "rx_session=<token>"`.
//	@description
//	@description	## CSRF-защита
//	@description	Все мутирующие эндпоинты (POST/PATCH/DELETE), кроме `/api/auth/register`, `/api/auth/login`,
//	@description	`/api/auth/verification/resend`, `/api/auth/password/forgot` и `/api/auth/password/reset`, проверяют заголовок `Origin`.
//	@description	Он должен совпадать с одним из разрешённых источников (настраивается через `ALLOWED_ORIGINS`).
//	@description	Swagger UI выставляет `Origin` автоматически.
//
//	@contact.name	Akim Zuev
//	@contact.email	akim.zuev.86@gmail.com
//
//	@host		localhost:8080
//	@BasePath	/
//
//	@securityDefinitions.apikey	SessionCookie
//	@in							cookie
//	@name						rx_session
//	@description				Сессионный cookie, выставляемый при логине (POST /api/auth/login).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/Beliashkoff/RepricerX/docs"
	"github.com/Beliashkoff/RepricerX/internal/config"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/integration/ozon"
	"github.com/Beliashkoff/RepricerX/internal/integration/wildberries"
	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/logger"
	"github.com/Beliashkoff/RepricerX/internal/pkg/mailer"
	"github.com/Beliashkoff/RepricerX/internal/pkg/redischeck"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	authsvc "github.com/Beliashkoff/RepricerX/internal/service/auth"
	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
	transport "github.com/Beliashkoff/RepricerX/internal/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Логгер ещё не готов — пишем в stderr напрямую.
		slog.Error("ошибка загрузки конфига", "error", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Environment)

	if err := runMigrations(cfg.DatabaseURL, log); err != nil {
		log.Error("миграции не применились", "error", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("не удалось подключиться к Postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Error("Postgres недоступен", "error", err)
		os.Exit(1)
	}
	log.Info("Postgres подключён")

	if err := redischeck.Ping(context.Background(), cfg.RedisAddr); err != nil {
		log.Error("Redis недоступен", "addr", cfg.RedisAddr, "error", err)
		os.Exit(1)
	}
	log.Info("Redis подключён", "addr", cfg.RedisAddr)

	usersRepo := repository.NewUsersRepository(pool)
	sessionsRepo := repository.NewSessionsRepository(pool)
	verRepo := repository.NewEmailVerificationsRepository(pool)
	resetRepo := repository.NewPasswordResetTokensRepository(pool)
	shopsRepo := repository.NewShopsRepository(pool)
	intLogRepo := repository.NewIntegrationLogRepository(pool)

	audit := auditlog.New(log)

	var m mailer.Mailer
	if cfg.MailerMode == "smtp" {
		m = mailer.NewSmtpMailer(cfg.SMTPHost, fmt.Sprintf("%d", cfg.SMTPPort), cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom)
	} else {
		m = mailer.NewLogMailer(log)
	}

	shopService := shopsvc.New(shopsRepo, intLogRepo, cfg.AppSecretKey, map[string]shopsvc.MarketplaceFactory{
		"wb": func(b []byte) (integration.Marketplace, error) {
			return wildberries.NewClient(b)
		},
		"ozon": func(b []byte) (integration.Marketplace, error) {
			return ozon.NewClient(b)
		},
	})

	svc := authsvc.New(usersRepo, sessionsRepo, verRepo, resetRepo, m, audit, authsvc.Config{
		IdleTTL:          24 * time.Hour,
		AbsoluteTTL:      cfg.SessionAbsoluteTTL,
		TrustProxy:       cfg.TrustProxyHeaders,
		VerificationURL:  cfg.VerificationURLBase,
		PasswordResetURL: cfg.PasswordResetURLBase,
	})

	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	health := &healthHandlers{pool: pool, redisAddr: cfg.RedisAddr}
	r.GET("/healthz", health.healthz)
	r.GET("/ready", health.ready)

	transport.RegisterRoutes(r, transport.RouterConfig{
		AuthSvc:        svc,
		ShopSvc:        shopService,
		UsersRepo:      usersRepo,
		Audit:          audit,
		AllowedOrigins: cfg.AllowedOrigins,
		TrustProxy:     cfg.TrustProxyHeaders,
		SecureCookie:   cfg.IsProd(),
		FrontendURL:    cfg.VerificationURLBase,
	})

	// Cleanup горутина: каждый час удаляем протухшие сессии и одноразовые токены.
	// Этап 7: переедет в robfig/cron v3.
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			n, err := sessionsRepo.DeleteExpired(context.Background())
			if err != nil {
				log.Error("cleanup: expired sessions", "error", err)
			} else if n > 0 {
				log.Info("cleanup: удалены сессии", "count", n)
			}
			n, err = verRepo.DeleteExpired(context.Background())
			if err != nil {
				log.Error("cleanup: expired verifications", "error", err)
			} else if n > 0 {
				log.Info("cleanup: удалены токены верификации", "count", n)
			}
			n, err = resetRepo.DeleteExpired(context.Background())
			if err != nil {
				log.Error("cleanup: expired password resets", "error", err)
			} else if n > 0 {
				log.Info("cleanup: удалены токены сброса пароля", "count", n)
			}
			n, err = intLogRepo.DeleteOlderThan(context.Background(), time.Now().UTC().Add(-30*24*time.Hour))
			if err != nil {
				log.Error("cleanup: integration_log", "error", err)
			} else if n > 0 {
				log.Info("cleanup: удалены записи integration_log", "count", n)
			}
		}
	}()

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("HTTP сервер запущен", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("ListenAndServe", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	log.Info("завершение работы...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("принудительное завершение", "error", err)
	}
	log.Info("сервер остановлен")
}

// runMigrations применяет все pending-миграции из директории migrations/.
func runMigrations(databaseURL string, log *slog.Logger) error {
	m, err := migrate.New("file://migrations", databaseURL)
	if err != nil {
		return err
	}
	defer m.Close() //nolint:errcheck

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info("миграции: нет изменений")
			return nil
		}
		return err
	}

	log.Info("миграции применены")
	return nil
}
