package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/config"
	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/logger"
	"github.com/Beliashkoff/RepricerX/internal/pkg/mailer"
	"github.com/Beliashkoff/RepricerX/internal/pkg/redischeck"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	authsvc "github.com/Beliashkoff/RepricerX/internal/service/auth"
	transport "github.com/Beliashkoff/RepricerX/internal/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("ошибка загрузки конфига: " + err.Error())
	}

	log, err := logger.New(cfg.Environment)
	if err != nil {
		panic("ошибка инициализации логгера: " + err.Error())
	}
	defer log.Sync() //nolint:errcheck

	// Применяем миграции до старта HTTP-сервера
	if err := runMigrations(cfg.DatabaseURL, log); err != nil {
		log.Fatal("миграции не применились", zap.Error(err))
	}

	// Подключаемся к Postgres через пул соединений
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal("не удалось подключиться к Postgres", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatal("Postgres недоступен", zap.Error(err))
	}
	log.Info("Postgres подключён")

	if err := redischeck.Ping(context.Background(), cfg.RedisAddr); err != nil {
		log.Fatal("Redis недоступен", zap.String("addr", cfg.RedisAddr), zap.Error(err))
	}
	log.Info("Redis подключён", zap.String("addr", cfg.RedisAddr))

	// Инициализируем зависимости сервисов.
	usersRepo := repository.NewUsersRepository(pool)
	sessionsRepo := repository.NewSessionsRepository(pool)
	verRepo := repository.NewEmailVerificationsRepository(pool)

	audit := auditlog.New(log)

	var m mailer.Mailer
	if cfg.MailerMode == "smtp" {
		m = mailer.NewSmtpMailer(cfg.SMTPHost, fmt.Sprintf("%d", cfg.SMTPPort), cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom)
	} else {
		m = mailer.NewLogMailer(log)
	}

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

	// Базовые эндпоинты готовности — нужны для docker healthcheck
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
				log.Error("cleanup expired sessions", zap.Error(err))
			} else if n > 0 {
				log.Info("cleanup: удалены сессии", zap.Int64("count", n))
			}
			n, err = verRepo.DeleteExpired(context.Background())
			if err != nil {
				log.Error("cleanup expired verifications", zap.Error(err))
			} else if n > 0 {
				log.Info("cleanup: удалены токены верификации", zap.Int64("count", n))
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

	// Graceful shutdown по SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("HTTP сервер запущен", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("ListenAndServe", zap.Error(err))
		}
	}()

	<-quit
	log.Info("завершение работы...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("принудительное завершение", zap.Error(err))
	}
	log.Info("сервер остановлен")
}

// runMigrations применяет все pending-миграции из директории migrations/.
func runMigrations(databaseURL string, log *zap.Logger) error {
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
