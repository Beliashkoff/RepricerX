// cmd/scheduler — Этап 7. Бинарь robfig/cron v3 для системных периодических задач:
//   - scheduledRecalc: per-shop пересчёт цен по shop.schedule_cron
//   - competitorRefresh: обновление цен конкурентов раз в 30 мин
//   - cleanupHourly: удаление старых сессий, токенов, логов
//   - stalePlanCleanup: cancel зависших price_plans старше 24ч
//
// Multi-replica safety:
//   - per-shop через CAS на shops.last_recalc_at
//   - global через pg_advisory_lock (internal/pkg/dblock)
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/config"
	"github.com/Beliashkoff/RepricerX/internal/pkg/logger"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/Beliashkoff/RepricerX/internal/scheduler"
	pricingsvc "github.com/Beliashkoff/RepricerX/internal/service/pricing"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("scheduler: config", "error", err)
		os.Exit(1)
	}
	log := logger.New(cfg.Environment)

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("scheduler: postgres connect", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		log.Error("scheduler: postgres ping", "error", err)
		os.Exit(1)
	}

	// Минимальный pricingService: scheduler только enqueue-ит recalc-job.
	// Для Recalculate нужны: products, strategies, plans, jobs, shops.
	// WithDispatcher НЕ нужен — auto-dispatch произойдёт в worker (там dispatcher wired).
	pricingSvc := pricingsvc.New(
		repository.NewProductsRepository(pool),
		repository.NewStrategiesRepository(pool),
		pricingsvc.WithPlans(repository.NewPricePlansRepository(pool)),
		pricingsvc.WithJobs(repository.NewBackgroundJobsRepository(pool)),
		pricingsvc.WithShops(repository.NewShopsRepository(pool)),
		pricingsvc.WithAssignments(repository.NewStrategyAssignmentsRepository(pool)),
	)

	schedSvc := scheduler.New(scheduler.Deps{
		Pool:           pool,
		Shops:          repository.NewShopsRepository(pool),
		Sessions:       repository.NewSessionsRepository(pool),
		Verifications:  repository.NewEmailVerificationsRepository(pool),
		PasswordResets: repository.NewPasswordResetTokensRepository(pool),
		IntegrationLog: repository.NewIntegrationLogRepository(pool),
		PriceChanges:   repository.NewPriceChangesRepository(pool),
		Competitors:    repository.NewProductCompetitorsRepository(pool),
		Jobs:           repository.NewBackgroundJobsRepository(pool),
		Pricing:        pricingSvc,
		Log:            log,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := schedSvc.Start(ctx); err != nil {
		log.Error("scheduler: start", "error", err)
		os.Exit(1)
	}
	log.Info("scheduler running")

	<-ctx.Done()
	log.Info("scheduler: shutdown initiated")

	shutdownTimeout := 30 * time.Second
	if err := schedSvc.Stop(shutdownTimeout); err != nil {
		log.Error("scheduler: stop", "error", err)
	}
	log.Info("scheduler stopped")
}
