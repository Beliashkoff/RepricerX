package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/config"
	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/integration/ozon"
	"github.com/Beliashkoff/RepricerX/internal/integration/wildberries"
	"github.com/Beliashkoff/RepricerX/internal/pkg/logger"
	"github.com/Beliashkoff/RepricerX/internal/pkg/ratelimit"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	dispatchersvc "github.com/Beliashkoff/RepricerX/internal/service/dispatcher"
	pricingsvc "github.com/Beliashkoff/RepricerX/internal/service/pricing"
	productsvc "github.com/Beliashkoff/RepricerX/internal/service/product"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("ошибка загрузки конфига", "error", err)
		os.Exit(1)
	}
	log := logger.New(cfg.Environment)

	workerID := cfg.WorkerID
	if workerID == "" {
		host, _ := os.Hostname()
		workerID = host
	}
	if workerID == "" {
		workerID = fmt.Sprintf("worker-%d", os.Getpid())
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("worker: postgres connect", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		log.Error("worker: postgres ping", "error", err)
		os.Exit(1)
	}

	shopsRepo := repository.NewShopsRepository(pool)
	productsRepo := repository.NewProductsRepository(pool)
	importLogRepo := repository.NewImportLogRepository(pool)
	jobsRepo := repository.NewBackgroundJobsRepository(pool)
	strategiesRepo := repository.NewStrategiesRepository(pool)
	assignmentsRepo := repository.NewStrategyAssignmentsRepository(pool)
	plansRepo := repository.NewPricePlansRepository(pool)
	competitorsRepo := repository.NewProductCompetitorsRepository(pool)
	limiter := ratelimit.New(5.0)
	productService := productsvc.New(shopsRepo, productsRepo, importLogRepo, jobsRepo, cfg.AppSecretKey, map[string]productsvc.MarketplaceFactory{
		"wb": func(shopID string, b []byte) (integration.Marketplace, error) {
			return wildberries.NewClient(shopID, b, limiter)
		},
		"ozon": func(shopID string, b []byte) (integration.Marketplace, error) {
			return ozon.NewClient(shopID, b, limiter)
		},
	}, productsvc.WithImportMaxAttempts(cfg.WorkerMaxAttempts))
	pricingMarketplaceFactories := map[string]pricingsvc.MarketplaceFactory{
		"wb": func(shopID string, b []byte) (integration.Marketplace, error) {
			return wildberries.NewClient(shopID, b, limiter)
		},
		"ozon": func(shopID string, b []byte) (integration.Marketplace, error) {
			return ozon.NewClient(shopID, b, limiter)
		},
	}
	priceChangesRepo := repository.NewPriceChangesRepository(pool)
	dispatcherFactories := map[string]dispatchersvc.MarketplaceFactory{
		"wb": func(shopID string, b []byte) (integration.Marketplace, error) {
			return wildberries.NewClient(shopID, b, limiter)
		},
		"ozon": func(shopID string, b []byte) (integration.Marketplace, error) {
			return ozon.NewClient(shopID, b, limiter)
		},
	}
	intLogRepo := repository.NewIntegrationLogRepository(pool)
	dispatcherService := dispatchersvc.New(
		plansRepo, productsRepo, priceChangesRepo, intLogRepo,
		shopsRepo, jobsRepo,
		cfg.AppSecretKey, dispatcherFactories,
	)

	pricingService := pricingsvc.New(productsRepo, strategiesRepo,
		pricingsvc.WithCompetitors(competitorsRepo),
		pricingsvc.WithPlans(plansRepo),
		pricingsvc.WithJobs(jobsRepo),
		pricingsvc.WithShops(shopsRepo),
		pricingsvc.WithAssignments(assignmentsRepo),
		pricingsvc.WithPriceSync(cfg.AppSecretKey, pricingMarketplaceFactories, 60*time.Minute),
		pricingsvc.WithDispatcher(dispatcherService),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("worker started", "worker_id", workerID, "concurrency", cfg.WorkerConcurrency)
	run(ctx, log, workerID, cfg, jobsRepo, productService, pricingService, dispatcherService)
	log.Info("worker stopped", "worker_id", workerID)
}

func run(ctx context.Context, log *slog.Logger, workerID string, cfg *config.Config, jobs repository.BackgroundJobsRepository, productService *productsvc.Service, pricingService *pricingsvc.Service, dispatcherService *dispatchersvc.Service) {
	sem := make(chan struct{}, cfg.WorkerConcurrency)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			waitForShutdown(log, &wg, cfg.WorkerShutdownTimeout)
			return
		default:
		}

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			waitForShutdown(log, &wg, cfg.WorkerShutdownTimeout)
			return
		}

		job, err := jobs.ClaimNext(ctx, "default", workerID, cfg.WorkerLockTTL)
		if errors.Is(err, repository.ErrNotFound) {
			<-sem
			sleepOrDone(ctx, cfg.WorkerPollInterval)
			continue
		}
		if err != nil {
			<-sem
			log.Error("worker: claim job", "error", err)
			sleepOrDone(ctx, cfg.WorkerPollInterval)
			continue
		}

		wg.Add(1)
		go func(job *domain.BackgroundJob) {
			defer wg.Done()
			defer func() { <-sem }()
			processJob(log, jobs, productService, pricingService, dispatcherService, job, cfg.WorkerJobTimeout)
		}(job)
	}
}

func processJob(log *slog.Logger, jobs repository.BackgroundJobsRepository, productService *productsvc.Service, pricingService *pricingsvc.Service, dispatcherService *dispatchersvc.Service, job *domain.BackgroundJob, timeout time.Duration) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Info("worker: job claimed", "job_id", job.ID, "job_type", job.JobType, "attempt", job.Attempts)

	// Диспатч по типу джоба.
	if job.JobType == domain.BackgroundJobTypePriceRecalculation {
		processPricingJob(log, jobs, pricingService, job, started)
		return
	}
	if job.JobType == domain.BackgroundJobTypePriceDispatch {
		processDispatchJob(log, jobs, dispatcherService, job, started)
		return
	}

	result := productService.ExecuteImportJob(ctx, job)
	if result.Retryable {
		runAt := time.Now().UTC().Add(backoff(job.Attempts))
		if err := jobs.Retry(context.Background(), job.ID, runAt, result.InternalError); err != nil {
			log.Error("worker: retry job", "job_id", job.ID, "error", err)
			return
		}
		log.Warn("worker: job retry scheduled", "job_id", job.ID, "import_id", result.ImportID, "run_at", runAt, "public_code", result.PublicCode, "diagnostic", result.InternalError)
		return
	}

	if result.Status == domain.ImportStatusSucceeded || result.Status == domain.ImportStatusPartial {
		if err := jobs.Succeed(context.Background(), job.ID, result.ResultJSON); err != nil {
			log.Error("worker: succeed job", "job_id", job.ID, "error", err)
			return
		}
		log.Info("worker: job succeeded", "job_id", job.ID, "import_id", result.ImportID, "status", result.Status, "duration_ms", time.Since(started).Milliseconds())
		return
	}

	if err := jobs.Fail(context.Background(), job.ID, result.InternalError, result.ResultJSON); err != nil {
		log.Error("worker: fail job", "job_id", job.ID, "error", err)
		return
	}
	log.Warn("worker: job failed", "job_id", job.ID, "import_id", result.ImportID, "public_code", result.PublicCode, "diagnostic", result.InternalError, "duration_ms", time.Since(started).Milliseconds())
}

func processPricingJob(log *slog.Logger, jobs repository.BackgroundJobsRepository, pricingService *pricingsvc.Service, job *domain.BackgroundJob, started time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := pricingService.ExecuteRecalcJob(ctx, job); err != nil {
		// Ретрай по общему паттерну backoff.
		if job.Attempts < job.MaxAttempts {
			runAt := time.Now().UTC().Add(backoff(job.Attempts))
			if rerr := jobs.Retry(context.Background(), job.ID, runAt, err.Error()); rerr != nil {
				log.Error("worker: pricing retry", "job_id", job.ID, "error", rerr)
			} else {
				log.Warn("worker: pricing job retry", "job_id", job.ID, "run_at", runAt, "error", err)
			}
			return
		}
		if ferr := jobs.Fail(context.Background(), job.ID, err.Error(), nil); ferr != nil {
			log.Error("worker: pricing fail", "job_id", job.ID, "error", ferr)
		} else {
			log.Warn("worker: pricing job failed", "job_id", job.ID, "error", err)
		}
		return
	}
	if err := jobs.Succeed(context.Background(), job.ID, nil); err != nil {
		log.Error("worker: pricing succeed", "job_id", job.ID, "error", err)
		return
	}
	log.Info("worker: pricing job applied", "job_id", job.ID, "duration_ms", time.Since(started).Milliseconds())
}

// processDispatchJob — обработчик price_dispatch job-а.
// Контракт ошибок:
//   - dispatcher.ErrUnauthorized → Fail (no retry)
//   - любая другая ошибка → Retry если attempts < max_attempts; иначе MarkExhausted + Fail.
func processDispatchJob(log *slog.Logger, jobs repository.BackgroundJobsRepository, ds *dispatchersvc.Service, job *domain.BackgroundJob, started time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := ds.ExecuteDispatchJob(ctx, job)

	if errors.Is(err, dispatchersvc.ErrUnauthorized) {
		if ferr := jobs.Fail(context.Background(), job.ID, err.Error(), nil); ferr != nil {
			log.Error("dispatch fail save", "job_id", job.ID, "error", ferr)
		}
		log.Warn("dispatch job failed (unauthorized)",
			"job_id", job.ID, "duration_ms", time.Since(started).Milliseconds())
		return
	}

	if err != nil {
		if job.Attempts < job.MaxAttempts {
			runAt := time.Now().UTC().Add(dispatchBackoff(job.Attempts))
			if rerr := jobs.Retry(context.Background(), job.ID, runAt, err.Error()); rerr != nil {
				log.Error("dispatch retry save", "job_id", job.ID, "error", rerr)
				return
			}
			log.Warn("dispatch job retry scheduled",
				"job_id", job.ID, "run_at", runAt, "attempts", job.Attempts, "error", err)
			return
		}
		// attempts=max_attempts → fail-fast: помечаем оставшиеся pending items как failed.
		if mexErr := ds.MarkExhausted(context.Background(), job); mexErr != nil {
			log.Error("dispatch mark exhausted", "job_id", job.ID, "error", mexErr)
		}
		if ferr := jobs.Fail(context.Background(), job.ID, err.Error(), nil); ferr != nil {
			log.Error("dispatch fail save", "job_id", job.ID, "error", ferr)
		}
		log.Warn("dispatch job exhausted",
			"job_id", job.ID, "duration_ms", time.Since(started).Milliseconds())
		return
	}

	if err := jobs.Succeed(context.Background(), job.ID, nil); err != nil {
		log.Error("dispatch succeed save", "job_id", job.ID, "error", err)
		return
	}
	log.Info("dispatch job applied",
		"job_id", job.ID, "duration_ms", time.Since(started).Milliseconds())
}

// dispatchBackoff — schedule retry для price_dispatch (по ТЗ 4.1.1.7.4).
// 30s/60s/120s между попытками. После 3 попыток job завершается как failed.
func dispatchBackoff(attempt int) time.Duration {
	schedule := []time.Duration{
		30 * time.Second,
		60 * time.Second,
		120 * time.Second,
	}
	if attempt < 1 {
		return schedule[0]
	}
	idx := attempt - 1
	if idx >= len(schedule) {
		return schedule[len(schedule)-1]
	}
	return schedule[idx]
}

func backoff(attempt int) time.Duration {
	schedule := []time.Duration{
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
	}
	if attempt < 1 {
		return schedule[0]
	}
	idx := attempt - 1
	if idx >= len(schedule) {
		idx = len(schedule) - 1
	}
	return schedule[idx]
}

func sleepOrDone(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func waitForShutdown(log *slog.Logger, wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Warn("worker: shutdown timeout")
	}
}
