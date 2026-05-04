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
	limiter := ratelimit.New(5.0)
	productService := productsvc.New(shopsRepo, productsRepo, importLogRepo, jobsRepo, cfg.AppSecretKey, map[string]productsvc.MarketplaceFactory{
		"wb": func(shopID string, b []byte) (integration.Marketplace, error) {
			return wildberries.NewClient(shopID, b, limiter)
		},
		"ozon": func(shopID string, b []byte) (integration.Marketplace, error) {
			return ozon.NewClient(shopID, b, limiter)
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("worker started", "worker_id", workerID, "concurrency", cfg.WorkerConcurrency)
	run(ctx, log, workerID, cfg, jobsRepo, productService)
	log.Info("worker stopped", "worker_id", workerID)
}

func run(ctx context.Context, log *slog.Logger, workerID string, cfg *config.Config, jobs repository.BackgroundJobsRepository, productService *productsvc.Service) {
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
			processJob(log, jobs, productService, job, cfg.WorkerJobTimeout)
		}(job)
	}
}

func processJob(log *slog.Logger, jobs repository.BackgroundJobsRepository, productService *productsvc.Service, job *domain.BackgroundJob, timeout time.Duration) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Info("worker: job claimed", "job_id", job.ID, "job_type", job.JobType, "attempt", job.Attempts)
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
