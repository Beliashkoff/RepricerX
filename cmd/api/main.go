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

	svc := authsvc.New(usersRepo, sessionsRepo, verRepo, m, audit, authsvc.Config{
		IdleTTL:         24 * time.Hour,
		AbsoluteTTL:     cfg.SessionAbsoluteTTL,
		TrustProxy:      cfg.TrustProxyHeaders,
		VerificationURL: cfg.VerificationURLBase,
	})

	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/ready", func(c *gin.Context) {
		if err := pool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "db unavailable"})
			return
		}
		if err := redischeck.Ping(c.Request.Context(), cfg.RedisAddr); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis unavailable"})
			return
		}
		c.Status(http.StatusOK)
	})

	transport.RegisterRoutes(r, transport.RouterConfig{
		AuthSvc:        svc,
		ShopSvc:        shopService,
		Audit:          audit,
		AllowedOrigins: cfg.AllowedOrigins,
		TrustProxy:     cfg.TrustProxyHeaders,
		SecureCookie:   cfg.IsProd(),
		FrontendURL:    cfg.VerificationURLBase,
	})

	// Cleanup горутина: каждый час удаляем протухшие сессии и токены верификации.
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
	defer m.Close()

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
