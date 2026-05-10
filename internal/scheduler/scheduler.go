// Package scheduler — robfig/cron v3 обёртка для Этапа 7.
// Запускает 4 cron-задачи: scheduledRecalc (per-shop), competitorRefresh,
// cleanupHourly, stalePlanCleanup. Multi-replica safety:
//   - per-shop задачи защищены CAS-update на shops.last_recalc_at.
//   - глобальные — pg_advisory_lock через internal/pkg/dblock.
//
// Scheduler ТОЛЬКО enqueue-ит задачи в background_jobs (для recalc/competitor refresh)
// или вызывает cleanup напрямую. Реальное исполнение recalc-плана и
// competitor refresh — в worker.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/dblock"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

// PricingTrigger — минимальный интерфейс для scheduledRecalc.
// Реализуется pricing.Service.Recalculate.
// Здесь интерфейс — чтобы избежать импорта pricing-пакета (циклы).
type PricingTrigger interface {
	Recalculate(ctx context.Context, userID, shopID uuid.UUID, productIDs []uuid.UUID) (*domain.PricePlan, *domain.BackgroundJob, error)
}

// DigestFlusher — минимальный интерфейс для DigestFlushTick (Этап D).
// Реализуется notifier.Service. В отдельный интерфейс — чтобы scheduler
// не зависел от пакета notifier на уровне типов.
type DigestFlusher interface {
	FlushDigests(ctx context.Context, channel string, now time.Time) (int, error)
}

type Service struct {
	c       *cron.Cron
	log     *slog.Logger
	pool    *pgxpool.Pool

	shops        repository.ShopsRepository
	sessions     repository.SessionsRepository
	verRepo      repository.EmailVerificationsRepository
	resetRepo    repository.PasswordResetTokensRepository
	intLog       repository.IntegrationLogRepository
	priceChanges repository.PriceChangesRepository
	competitors  repository.ProductCompetitorsRepository
	jobs         repository.BackgroundJobsRepository
	pricing      PricingTrigger
	digest       DigestFlusher

	now func() time.Time

	// Cron-выражения (можно переопределить через опции для тестов).
	specScheduledRecalc   string // "* * * * *"
	specCompetitorRefresh string // "*/30 * * * *"
	specCleanupHourly     string // "0 * * * *"
	specStalePlan         string // "0 4 * * *"
	specDigestFlush       string // "*/5 * * * *"

	// Параметры
	competitorMaxAge      time.Duration // 30 минут
	competitorBatchSize   int           // 1000
	stalePlanMaxAge       time.Duration // 24 часа

	mu      sync.Mutex
	started bool
}

type Option func(*Service)

func WithNow(f func() time.Time) Option { return func(s *Service) { s.now = f } }
func WithSpecs(scheduledRecalc, competitorRefresh, cleanupHourly, stalePlan string) Option {
	return func(s *Service) {
		if scheduledRecalc != "" {
			s.specScheduledRecalc = scheduledRecalc
		}
		if competitorRefresh != "" {
			s.specCompetitorRefresh = competitorRefresh
		}
		if cleanupHourly != "" {
			s.specCleanupHourly = cleanupHourly
		}
		if stalePlan != "" {
			s.specStalePlan = stalePlan
		}
	}
}

type Deps struct {
	Pool         *pgxpool.Pool
	Shops        repository.ShopsRepository
	Sessions     repository.SessionsRepository
	Verifications repository.EmailVerificationsRepository
	PasswordResets repository.PasswordResetTokensRepository
	IntegrationLog repository.IntegrationLogRepository
	PriceChanges  repository.PriceChangesRepository
	Competitors   repository.ProductCompetitorsRepository
	Jobs          repository.BackgroundJobsRepository
	Pricing       PricingTrigger
	Digest        DigestFlusher // optional; nil → DigestFlushTick не регистрируется
	Log           *slog.Logger
}

func New(deps Deps, opts ...Option) *Service {
	c := cron.New(cron.WithLogger(cronLogAdapter{log: deps.Log}))
	s := &Service{
		c:                     c,
		log:                   deps.Log.With("module", "scheduler"),
		pool:                  deps.Pool,
		shops:                 deps.Shops,
		sessions:              deps.Sessions,
		verRepo:               deps.Verifications,
		resetRepo:             deps.PasswordResets,
		intLog:                deps.IntegrationLog,
		priceChanges:          deps.PriceChanges,
		competitors:           deps.Competitors,
		jobs:                  deps.Jobs,
		pricing:               deps.Pricing,
		digest:                deps.Digest,
		now:                   func() time.Time { return time.Now().UTC() },
		specScheduledRecalc:   "* * * * *",
		specCompetitorRefresh: "*/30 * * * *",
		specCleanupHourly:     "0 * * * *",
		specStalePlan:         "0 4 * * *",
		specDigestFlush:       "*/5 * * * *",
		competitorMaxAge:      30 * time.Minute,
		competitorBatchSize:   1000,
		stalePlanMaxAge:       24 * time.Hour,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start регистрирует cron-задачи и запускает планировщик.
// Не блокирует; cron работает в фоне до Stop.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}

	if _, err := s.c.AddFunc(s.specScheduledRecalc, func() { s.ScheduledRecalcTick(ctx) }); err != nil {
		return fmt.Errorf("scheduler: add scheduledRecalc: %w", err)
	}
	if _, err := s.c.AddFunc(s.specCompetitorRefresh, func() { s.CompetitorRefreshTick(ctx) }); err != nil {
		return fmt.Errorf("scheduler: add competitorRefresh: %w", err)
	}
	if _, err := s.c.AddFunc(s.specCleanupHourly, func() { s.CleanupHourlyTick(ctx) }); err != nil {
		return fmt.Errorf("scheduler: add cleanupHourly: %w", err)
	}
	if _, err := s.c.AddFunc(s.specStalePlan, func() { s.StalePlanCleanupTick(ctx) }); err != nil {
		return fmt.Errorf("scheduler: add stalePlanCleanup: %w", err)
	}
	if s.digest != nil {
		if _, err := s.c.AddFunc(s.specDigestFlush, func() { s.DigestFlushTick(ctx) }); err != nil {
			return fmt.Errorf("scheduler: add digestFlush: %w", err)
		}
	}

	s.c.Start()
	s.started = true
	s.log.Info("scheduler started",
		"scheduledRecalc", s.specScheduledRecalc,
		"competitorRefresh", s.specCompetitorRefresh,
		"cleanupHourly", s.specCleanupHourly,
		"stalePlan", s.specStalePlan,
		"digestFlush", boolToStr(s.digest != nil, s.specDigestFlush, "disabled"),
	)
	return nil
}

// Stop graceful shutdown: ждёт завершения running tick-handlers до timeout.
func (s *Service) Stop(timeout time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return nil
	}
	s.started = false
	stopCtx := s.c.Stop() // возвращает контекст ждущий завершения running jobs
	select {
	case <-stopCtx.Done():
	case <-time.After(timeout):
		s.log.Warn("scheduler: shutdown timeout")
	}
	return nil
}

// ─── Tick handlers (экспортированы для unit/integration тестов) ─────────────

// ScheduledRecalcTick — для каждого shops с непустым schedule_cron парсит выражение,
// проверяет наступление nextRun и через CAS-update enqueue-ит recalc-job.
func (s *Service) ScheduledRecalcTick(ctx context.Context) {
	tickCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	shops, err := s.shops.ListSchedulable(tickCtx)
	if err != nil {
		s.log.Error("scheduledRecalc: list shops", "error", err)
		return
	}
	if len(shops) == 0 {
		return
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	now := s.now()
	enqueued := 0

	for _, shop := range shops {
		spec, err := parser.Parse(shop.ScheduleCron)
		if err != nil {
			s.log.Warn("scheduledRecalc: invalid cron",
				"shop_id", shop.ID, "cron", shop.ScheduleCron, "error", err)
			continue
		}
		baseline := shop.LastRecalcAt
		if baseline == nil {
			baseline = &shop.CreatedAt
		}
		nextRun := spec.Next(*baseline)
		if nextRun.After(now) {
			continue
		}

		ok, err := s.shops.TouchLastRecalcAt(tickCtx, shop.ID, shop.LastRecalcAt)
		if err != nil {
			s.log.Error("scheduledRecalc: touch", "shop_id", shop.ID, "error", err)
			continue
		}
		if !ok {
			continue // другая реплика забрала
		}

		if _, _, err := s.pricing.Recalculate(tickCtx, shop.UserID, shop.ID, nil); err != nil {
			s.log.Error("scheduledRecalc: enqueue",
				"shop_id", shop.ID, "user_id", shop.UserID, "error", err)
			continue
		}
		enqueued++
	}

	if enqueued > 0 {
		s.log.Info("scheduledRecalc: enqueued", "count", enqueued)
	}
}

// CompetitorRefreshTick — раз в 30 минут (по дефолту) enqueue-ит refresh-jobs
// для всех product_competitors с устаревшим last_checked_at.
func (s *Service) CompetitorRefreshTick(ctx context.Context) {
	tickCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	got, release, err := dblock.TryAcquire(tickCtx, s.pool, dblock.LockIDCompetitorRefresh)
	if err != nil {
		s.log.Error("competitorRefresh: lock", "error", err)
		return
	}
	if !got {
		s.log.Debug("competitorRefresh: lock taken by another replica, skip")
		return
	}
	defer func() {
		if err := release(); err != nil {
			s.log.Error("competitorRefresh: release lock", "error", err)
		}
	}()

	since := s.now().Add(-s.competitorMaxAge)
	stale, err := s.competitors.ListStaleForRefresh(tickCtx, since, s.competitorBatchSize)
	if err != nil {
		s.log.Error("competitorRefresh: list stale", "error", err)
		return
	}
	if len(stale) == 0 {
		return
	}

	enqueued := 0
	for _, c := range stale {
		payload, _ := json.Marshal(domain.CompetitorRefreshJobPayload{
			CompetitorID: c.CompetitorID,
			UserID:       c.UserID,
		})
		if _, err := s.jobs.Enqueue(tickCtx, repository.BackgroundJobEnqueue{
			JobType:     domain.BackgroundJobTypeCompetitorRefresh,
			Queue:       "default",
			Priority:    50,
			Payload:     payload,
			MaxAttempts: 3,
		}); err != nil {
			s.log.Error("competitorRefresh: enqueue",
				"competitor_id", c.CompetitorID, "error", err)
			continue
		}
		enqueued++
	}
	s.log.Info("competitorRefresh: enqueued", "count", enqueued, "stale_total", len(stale))
}

// CleanupHourlyTick — раз в час: удаляет expired sessions/email_verifications/
// password_resets, integration_log >30d, price_change_log >180d.
// Перенесено сюда из cmd/api/main.go cleanup-горутины.
func (s *Service) CleanupHourlyTick(ctx context.Context) {
	tickCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	got, release, err := dblock.TryAcquire(tickCtx, s.pool, dblock.LockIDCleanupHourly)
	if err != nil {
		s.log.Error("cleanupHourly: lock", "error", err)
		return
	}
	if !got {
		s.log.Debug("cleanupHourly: lock taken by another replica, skip")
		return
	}
	defer func() {
		if err := release(); err != nil {
			s.log.Error("cleanupHourly: release lock", "error", err)
		}
	}()

	now := s.now()

	if n, err := s.sessions.DeleteExpired(tickCtx); err != nil {
		s.log.Error("cleanupHourly: sessions", "error", err)
	} else if n > 0 {
		s.log.Info("cleanupHourly: deleted sessions", "count", n)
	}
	if n, err := s.verRepo.DeleteExpired(tickCtx); err != nil {
		s.log.Error("cleanupHourly: email_verifications", "error", err)
	} else if n > 0 {
		s.log.Info("cleanupHourly: deleted email_verifications", "count", n)
	}
	if n, err := s.resetRepo.DeleteExpired(tickCtx); err != nil {
		s.log.Error("cleanupHourly: password_resets", "error", err)
	} else if n > 0 {
		s.log.Info("cleanupHourly: deleted password_resets", "count", n)
	}
	if n, err := s.intLog.DeleteOlderThan(tickCtx, now.Add(-30*24*time.Hour)); err != nil {
		s.log.Error("cleanupHourly: integration_log", "error", err)
	} else if n > 0 {
		s.log.Info("cleanupHourly: deleted integration_log", "count", n)
	}
	if n, err := s.priceChanges.DeleteOlderThan(tickCtx, now.Add(-180*24*time.Hour)); err != nil {
		s.log.Error("cleanupHourly: price_change_log", "error", err)
	} else if n > 0 {
		s.log.Info("cleanupHourly: deleted price_change_log", "count", n)
	}
}

// DigestFlushTick — раз в 5 минут (по умолчанию) собирает накопленные
// 'pending_digest' доставки и enqueue-ит digest-job. Логика дедупликации,
// окна и quiet-hours живёт в notifier.Service (FlushDigests).
func (s *Service) DigestFlushTick(ctx context.Context) {
	if s.digest == nil {
		return
	}
	tickCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, ch := range []string{
		"email", // только email пока поддерживает дайджест; telegram/webhook — instant
	} {
		got, release, err := dblock.TryAcquire(tickCtx, s.pool, dblock.LockIDDigestFlush)
		if err != nil {
			s.log.Error("digestFlush: lock", "channel", ch, "error", err)
			continue
		}
		if !got {
			continue
		}
		n, err := s.digest.FlushDigests(tickCtx, ch, s.now())
		_ = release()
		if err != nil {
			s.log.Error("digestFlush", "channel", ch, "error", err)
			continue
		}
		if n > 0 {
			s.log.Info("digestFlush: enqueued", "channel", ch, "count", n)
		}
	}
}

func boolToStr(cond bool, t, f string) string {
	if cond {
		return t
	}
	return f
}

// StalePlanCleanupTick — раз в сутки cancel-ит "зависшие" планы старше 24 часов.
// Защита от висящих planов если worker умер посреди обработки.
func (s *Service) StalePlanCleanupTick(ctx context.Context) {
	tickCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	got, release, err := dblock.TryAcquire(tickCtx, s.pool, dblock.LockIDStalePlanCleanup)
	if err != nil {
		s.log.Error("stalePlanCleanup: lock", "error", err)
		return
	}
	if !got {
		return
	}
	defer func() { _ = release() }()

	cutoff := s.now().Add(-s.stalePlanMaxAge)
	tag, err := s.pool.Exec(tickCtx, `
		UPDATE price_plans
		SET status = 'cancelled'::plan_status, updated_at = NOW()
		WHERE status IN ('pending'::plan_status, 'processing'::plan_status, 'dispatching'::plan_status)
		  AND created_at < $1`, cutoff)
	if err != nil {
		s.log.Error("stalePlanCleanup: update", "error", err)
		return
	}
	if rows := tag.RowsAffected(); rows > 0 {
		s.log.Info("stalePlanCleanup: cancelled stale plans", "count", rows)
	}
}

// ─── cron logger adapter ────────────────────────────────────────────────────

type cronLogAdapter struct{ log *slog.Logger }

func (a cronLogAdapter) Info(msg string, keysAndValues ...any) {
	a.log.Info("cron: "+msg, keysAndValues...)
}
func (a cronLogAdapter) Error(err error, msg string, keysAndValues ...any) {
	args := append([]any{"error", err}, keysAndValues...)
	a.log.Error("cron: "+msg, args...)
}
