// Package dispatcher реализует Этап 6 ТЗ 4.1.1.7: отправка уже рассчитанного
// price_plan в маркетплейсы. Изолирован от pricing.Service: pricing формирует
// items со status='pending', dispatcher их отправляет и финализирует план.
package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrPlanNotFound = errors.New("price plan not found")
	ErrPlanNotReady = errors.New("plan not in calculated status")
	ErrPlanTerminal = errors.New("plan in terminal status")
	ErrShopNotFound = errors.New("shop not found")
	ErrUnauthorized = errors.New("marketplace unauthorized")
)

// MarketplaceFactory создаёт client маркетплейса. Те же сигнатуры, что в shop/pricing —
// дублируются здесь чтобы избежать перекрёстного импорта пакетов.
type MarketplaceFactory func(shopID string, credsJSON []byte) (integration.Marketplace, error)

type Service struct {
	plans          repository.PricePlansRepository
	products       repository.ProductsRepository
	priceChanges   repository.PriceChangesRepository
	intLog         repository.IntegrationLogRepository
	shops          repository.ShopsRepository
	jobs           repository.BackgroundJobsRepository
	secret         string
	factories      map[string]MarketplaceFactory
	chunkSize      int
	now            func() time.Time
	notifier       NotifierEmitter
}

// NotifierEmitter — минимальный интерфейс к notifier.Service (избегаем
// циклического импорта). Реализуется *notifier.Service.
type NotifierEmitter interface {
	NotifyDispatchCompleted(ctx context.Context, userID, planID, shopID uuid.UUID, planStatus string, dispatched, failed, skipped, pending int)
	NotifyIntegrationError(ctx context.Context, userID, shopID uuid.UUID, operation string, httpStatus int, errText string)
}

// WithNotifier подключает notifier для эмиссии событий. Если не задан —
// dispatcher работает молча (для тестов и чистого пути).
func WithNotifier(n NotifierEmitter) Option {
	return func(s *Service) { s.notifier = n }
}

type Option func(*Service)

func WithChunkSize(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.chunkSize = n
		}
	}
}

func WithNow(f func() time.Time) Option {
	return func(s *Service) { s.now = f }
}

func New(
	plans repository.PricePlansRepository,
	products repository.ProductsRepository,
	priceChanges repository.PriceChangesRepository,
	intLog repository.IntegrationLogRepository,
	shops repository.ShopsRepository,
	jobs repository.BackgroundJobsRepository,
	secret string,
	factories map[string]MarketplaceFactory,
	opts ...Option,
) *Service {
	s := &Service{
		plans:        plans,
		products:     products,
		priceChanges: priceChanges,
		intLog:       intLog,
		shops:        shops,
		jobs:         jobs,
		secret:       secret,
		factories:    factories,
		chunkSize:    100,
		now:          func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// EnqueueDispatch — manual trigger из HTTP. Атомарно переводит calculated→dispatching
// и enqueue-ит price_dispatch job. Защищает от двойного enqueue race-condition.
func (s *Service) EnqueueDispatch(ctx context.Context, userID, planID uuid.UUID) (*domain.BackgroundJob, error) {
	plan, _, err := s.plans.GetByIDForUser(ctx, userID, planID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("get plan: %w", err)
	}
	if isTerminalPlanStatus(plan.Status) {
		return nil, ErrPlanTerminal
	}
	if plan.Status != domain.PlanStatusCalculated {
		return nil, ErrPlanNotReady
	}

	// Атомарный переход calculated→dispatching защищает от двойного enqueue.
	ok, err := s.plans.TransitionStatus(ctx, planID, domain.PlanStatusCalculated, domain.PlanStatusDispatching)
	if err != nil {
		return nil, fmt.Errorf("transition: %w", err)
	}
	if !ok {
		// Кто-то другой уже забрал план (auto-flow или повторный клик).
		return nil, ErrPlanNotReady
	}

	payload, _ := json.Marshal(domain.PriceDispatchJobPayload{
		PlanID:            planID,
		ShopID:            plan.ShopID,
		RequestedByUserID: userID,
	})

	job, err := s.jobs.Enqueue(ctx, repository.BackgroundJobEnqueue{
		JobType:     domain.BackgroundJobTypePriceDispatch,
		Queue:       "default",
		Priority:    100,
		Payload:     payload,
		MaxAttempts: 3,
	})
	if err != nil {
		// Откатить статус плана обратно — иначе он застрянет.
		_, _ = s.plans.TransitionStatus(ctx, planID, domain.PlanStatusDispatching, domain.PlanStatusCalculated)
		return nil, fmt.Errorf("enqueue dispatch: %w", err)
	}
	return job, nil
}

// Cancel — отменяет план. Только из non-terminal статусов.
// Уже отправленные в МП цены остаются — отзвать HTTP-вызов нельзя.
func (s *Service) Cancel(ctx context.Context, userID, planID uuid.UUID) error {
	plan, _, err := s.plans.GetByIDForUser(ctx, userID, planID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrPlanNotFound
		}
		return fmt.Errorf("get plan: %w", err)
	}
	if isTerminalPlanStatus(plan.Status) {
		return ErrPlanTerminal
	}
	if err := s.plans.UpdateStatus(ctx, planID, domain.PlanStatusCancelled); err != nil {
		return fmt.Errorf("cancel: %w", err)
	}
	return nil
}

// MarkExhausted вызывается из worker при достижении max_attempts:
// помечает все ещё pending items как failed и финализирует статус плана.
func (s *Service) MarkExhausted(ctx context.Context, job *domain.BackgroundJob) error {
	var p domain.PriceDispatchJobPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	items, err := s.plans.ListItemsForDispatch(ctx, p.PlanID)
	if err != nil {
		return fmt.Errorf("list items: %w", err)
	}
	s.failAllItems(ctx, items, p.ShopID, domain.ReasonDispatchRetriesExhausted,
		fmt.Sprintf("after %d attempts", job.Attempts))
	return s.finalizePlanStatus(ctx, p.PlanID)
}

// failAllItems — переводит указанные items в failed + пишет price_change_log.
// Используется при ErrUnauthorized и при retries exhausted.
func (s *Service) failAllItems(ctx context.Context, items []*repository.PricePlanItemForDispatch, shopID uuid.UUID, reason, errText string) {
	errMsg := fmt.Sprintf("%s: %s", reason, errText)
	for _, it := range items {
		_ = s.plans.UpdateItemAfterDispatch(ctx, it.ItemID, domain.PlanItemStatusFailed, errMsg)
		_ = s.priceChanges.Create(ctx, repository.PriceChangeCreate{
			ShopID:        shopID,
			ProductID:     it.ProductID,
			StrategyID:    it.StrategyID,
			OldPrice:      it.CurrentPrice,
			NewPrice:      it.CurrentPrice, // цена не изменилась
			TargetPrice:   it.TargetPrice,
			Reason:        errMsg,
			ConstraintHit: it.ConstraintHit,
			Status:        domain.PlanItemStatusFailed,
			CorrelationID: it.CorrelationID,
		})
	}
}

// finalizePlanStatus решает финальный статус плана по составу items.
func (s *Service) finalizePlanStatus(ctx context.Context, planID uuid.UUID) error {
	counts, err := s.plans.CountItemsByStatus(ctx, planID)
	if err != nil {
		return fmt.Errorf("count items: %w", err)
	}
	dispatched := counts[domain.PlanItemStatusDispatched]
	failed := counts[domain.PlanItemStatusFailed]
	pending := counts[domain.PlanItemStatusPending]
	skipped := counts[domain.PlanItemStatusSkipped]

	var newStatus string
	switch {
	case pending > 0:
		// Не должно случаться при штатном финализе. Bail в dispatching, retry job вернёт.
		return nil
	case dispatched > 0 && failed > 0:
		newStatus = domain.PlanStatusPartial
	case dispatched > 0:
		newStatus = domain.PlanStatusApplied
	case failed > 0:
		newStatus = domain.PlanStatusFailed
	case skipped > 0:
		// Все skipped (нечего отправлять) — план обработан полностью.
		newStatus = domain.PlanStatusApplied
	default:
		// Пустой план.
		newStatus = domain.PlanStatusApplied
	}
	return s.plans.UpdateStatus(ctx, planID, newStatus)
}

func isTerminalPlanStatus(status string) bool {
	switch status {
	case domain.PlanStatusApplied, domain.PlanStatusPartial,
		domain.PlanStatusFailed, domain.PlanStatusCancelled:
		return true
	}
	return false
}
