package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/pkg/crypto"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// ExecuteDispatchJob — обработчик BackgroundJobTypePriceDispatch.
// Вызывается из cmd/worker/main.go в switch по job.JobType.
//
// Контракт возвращаемых ошибок:
//   - nil → worker вызывает Succeed(job)
//   - ErrUnauthorized → worker вызывает Fail(job) НЕМЕДЛЕННО, без retry
//   - любой другой error → worker вызывает Retry(job) с backoff,
//     либо Fail+MarkExhausted при достижении max_attempts
func (s *Service) ExecuteDispatchJob(ctx context.Context, job *domain.BackgroundJob) error {
	var p domain.PriceDispatchJobPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("dispatch: parse payload: %w", err)
	}

	log := slog.Default().With(
		"module", "dispatcher",
		"job_id", job.ID,
		"plan_id", p.PlanID,
		"shop_id", p.ShopID,
		"user_id", p.RequestedByUserID,
		"attempt", job.Attempts,
	)
	log.Info("dispatch: start")

	plan, _, err := s.plans.GetByIDForUser(ctx, p.RequestedByUserID, p.PlanID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrPlanNotFound
		}
		return fmt.Errorf("get plan: %w", err)
	}

	// Idempotency: terminal — успех без действий.
	if isTerminalPlanStatus(plan.Status) {
		log.Info("dispatch: plan already terminal", "status", plan.Status)
		return nil
	}

	// На первой попытке могли заехать с calculated (auto-flow вызвал EnqueueDispatch
	// который перевёл в dispatching). При retry уже dispatching — не трогаем.
	if plan.Status == domain.PlanStatusCalculated {
		ok, err := s.plans.TransitionStatus(ctx, p.PlanID, domain.PlanStatusCalculated, domain.PlanStatusDispatching)
		if err != nil {
			return fmt.Errorf("mark dispatching: %w", err)
		}
		if !ok {
			// Кто-то ещё забрал план; читаем актуальный статус.
			plan, _, _ = s.plans.GetByIDForUser(ctx, p.RequestedByUserID, p.PlanID)
			if isTerminalPlanStatus(plan.Status) {
				return nil
			}
		}
	}

	items, err := s.plans.ListItemsForDispatch(ctx, p.PlanID)
	if err != nil {
		return fmt.Errorf("list items: %w", err)
	}
	if len(items) == 0 {
		log.Info("dispatch: nothing to dispatch (all items already terminal)")
		return s.finalizePlanStatus(ctx, p.PlanID)
	}

	shop, err := s.shops.GetByID(ctx, p.ShopID, p.RequestedByUserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrShopNotFound
		}
		return fmt.Errorf("get shop: %w", err)
	}

	factory, ok := s.factories[shop.Marketplace]
	if !ok {
		return fmt.Errorf("dispatch: no factory for marketplace %q", shop.Marketplace)
	}
	creds, err := crypto.Decrypt(shop.CredentialsEncrypted, s.secret)
	if err != nil {
		// Credentials повреждены — fail-fast без retry.
		log.Error("dispatch: credentials decrypt failed", "error", err)
		s.failAllItems(ctx, items, p.ShopID, domain.ReasonDispatchUnauthorized, "credentials decrypt failed")
		_ = s.plans.UpdateStatus(ctx, p.PlanID, domain.PlanStatusFailed)
		return ErrUnauthorized
	}
	client, err := factory(p.ShopID.String(), creds)
	if err != nil {
		log.Error("dispatch: build adapter failed", "error", err)
		s.failAllItems(ctx, items, p.ShopID, domain.ReasonDispatchUnauthorized, "build adapter: "+err.Error())
		_ = s.plans.UpdateStatus(ctx, p.PlanID, domain.PlanStatusFailed)
		return ErrUnauthorized
	}

	chunks := chunkItems(items, s.chunkSize)
	log.Info("dispatch: processing", "items", len(items), "chunks", len(chunks))

	for chunkIdx, chunk := range chunks {
		// Re-check cancellation между chunks.
		cancelled, err := s.isPlanCancelled(ctx, p.RequestedByUserID, p.PlanID)
		if err != nil {
			return fmt.Errorf("check cancel: %w", err)
		}
		if cancelled {
			log.Info("dispatch: plan cancelled mid-run", "chunk", chunkIdx)
			return nil
		}

		updates := make([]integration.PriceUpdate, 0, len(chunk))
		for _, it := range chunk {
			updates = append(updates, integration.PriceUpdate{
				ExternalSKU: it.ExternalSKU,
				NewPrice:    it.FinalPrice,
			})
		}

		startedAt := s.now()
		callErr := client.UpdatePrices(ctx, updates)
		elapsed := time.Since(startedAt)

		// Лог попытки в integration_log независимо от исхода.
		s.logIntegration(ctx, p.ShopID, callErr, elapsed)

		switch {
		case errors.Is(callErr, integration.ErrUnauthorized):
			log.Warn("dispatch: unauthorized — fail-fast", "chunk", chunkIdx, "elapsed_ms", elapsed.Milliseconds())
			s.failAllItems(ctx, items, p.ShopID, domain.ReasonDispatchUnauthorized, "marketplace rejected credentials")
			_ = s.plans.UpdateStatus(ctx, p.PlanID, domain.PlanStatusFailed)
			return ErrUnauthorized

		case errors.Is(callErr, integration.ErrRateLimited):
			log.Warn("dispatch: rate limited — retry", "chunk", chunkIdx, "elapsed_ms", elapsed.Milliseconds())
			return fmt.Errorf("rate limited at chunk %d: %w", chunkIdx, callErr)

		case callErr != nil:
			log.Warn("dispatch: chunk failed — retry",
				"chunk", chunkIdx, "elapsed_ms", elapsed.Milliseconds(), "error", callErr)
			return fmt.Errorf("chunk %d: %w", chunkIdx, callErr)

		default:
			// Success: помечаем items как dispatched + price_change_log.
			for _, it := range chunk {
				if err := s.plans.UpdateItemAfterDispatch(ctx, it.ItemID, domain.PlanItemStatusDispatched, ""); err != nil {
					log.Error("mark dispatched failed", "item_id", it.ItemID, "error", err)
					continue
				}
				_ = s.priceChanges.Create(ctx, repository.PriceChangeCreate{
					ShopID:        p.ShopID,
					ProductID:     it.ProductID,
					StrategyID:    it.StrategyID,
					OldPrice:      it.CurrentPrice,
					NewPrice:      it.FinalPrice,
					TargetPrice:   it.TargetPrice,
					Reason:        fmt.Sprintf("dispatched to %s", shop.Marketplace),
					ConstraintHit: it.ConstraintHit,
					Status:        domain.PlanItemStatusDispatched,
					CorrelationID: it.CorrelationID,
				})
			}
			log.Info("dispatch: chunk applied",
				"chunk", chunkIdx, "items", len(chunk), "elapsed_ms", elapsed.Milliseconds())
		}
	}

	return s.finalizePlanStatus(ctx, p.PlanID)
}

// isPlanCancelled — лёгкий SELECT status; используется между chunks.
func (s *Service) isPlanCancelled(ctx context.Context, userID, planID uuid.UUID) (bool, error) {
	plan, _, err := s.plans.GetByIDForUser(ctx, userID, planID)
	if err != nil {
		return false, err
	}
	return plan.Status == domain.PlanStatusCancelled, nil
}

// logIntegration — одна запись в integration_log на chunk.
//
//nolint:unparam // elapsed зарезервирован для будущей метрики duration_ms
func (s *Service) logIntegration(ctx context.Context, shopID uuid.UUID, callErr error, _ time.Duration) {
	var (
		httpStatus *int
		errorText  string
	)
	switch {
	case callErr == nil:
		ok := 200
		httpStatus = &ok
	case errors.Is(callErr, integration.ErrUnauthorized):
		st := 401
		httpStatus = &st
		errorText = "unauthorized"
	case errors.Is(callErr, integration.ErrRateLimited):
		st := 429
		httpStatus = &st
		errorText = "rate_limited"
	default:
		errorText = truncate(callErr.Error(), 500)
	}
	corr := uuid.New()
	_ = s.intLog.Create(ctx, &domain.IntegrationLogEntry{
		ID:            uuid.New(),
		ShopID:        &shopID,
		Operation:     domain.IntegrationOpPriceDispatch,
		HTTPStatus:    httpStatus,
		ErrorText:     errorText,
		CorrelationID: corr,
		CreatedAt:     s.now(),
	})
}

// chunkItems — делит items по chunkSize.
func chunkItems(items []*repository.PricePlanItemForDispatch, size int) [][]*repository.PricePlanItemForDispatch {
	if size <= 0 {
		size = 100
	}
	var out [][]*repository.PricePlanItemForDispatch
	for i := 0; i < len(items); i += size {
		end := min(i+size, len(items))
		out = append(out, items[i:end])
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
