package notifier

import (
	"context"

	"github.com/google/uuid"
)

// Emitters — тонкие обёртки над Emit, удобные хуком вызывать из других
// сервисов. Каждый метод — одно понятное событие; внутри собирает Event
// через фабрику в events.go.
//
// Важно: ошибка из Emit логируется, но не пробрасывается наверх. Хук
// вызывается уже после успешного бизнес-действия (план обновлён, импорт
// завершён); терять основное действие из-за проблем в notifier нельзя.

func (s *Service) NotifyDispatchCompleted(ctx context.Context, userID, planID, shopID uuid.UUID, planStatus string, dispatched, failed, skipped, pending int) {
	e := NewDispatchCompletedEvent(planID, shopID, planStatus, DispatchCounts{
		Dispatched: dispatched, Failed: failed, Skipped: skipped, Pending: pending,
	})
	if err := s.Emit(ctx, userID, e); err != nil {
		s.deps.Log.Warn("notifier: emit dispatch_completed", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyIntegrationError(ctx context.Context, userID, shopID uuid.UUID, operation string, httpStatus int, errText string) {
	e := NewIntegrationErrorEvent(shopID, operation, httpStatus, errText)
	if err := s.Emit(ctx, userID, e); err != nil {
		s.deps.Log.Warn("notifier: emit integration_error", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyRecalcCompleted(ctx context.Context, userID, planID, shopID uuid.UUID, total, calculated, skipped, errs int) {
	e := NewRecalcCompletedEvent(planID, shopID, RecalcCounts{
		Total: total, Calculated: calculated, Skipped: skipped, Errors: errs,
	})
	if err := s.Emit(ctx, userID, e); err != nil {
		s.deps.Log.Warn("notifier: emit recalc_completed", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyImportCompleted(ctx context.Context, userID, importID, shopID uuid.UUID, total, added, updated, skipped, failed int) {
	e := NewImportCompletedEvent(importID, shopID, ImportCounts{
		Total: total, Added: added, Updated: updated, Skipped: skipped, Failed: failed,
	})
	if err := s.Emit(ctx, userID, e); err != nil {
		s.deps.Log.Warn("notifier: emit import_completed", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyConstraintHit(ctx context.Context, userID, planID, shopID uuid.UUID, minPrice, maxPrice, maxChangePct, other int) {
	if minPrice == 0 && maxPrice == 0 && maxChangePct == 0 && other == 0 {
		return
	}
	e := NewConstraintHitEvent(planID, shopID, ConstraintHits{
		MinPrice: minPrice, MaxPrice: maxPrice, MaxChangePct: maxChangePct, Other: other,
	})
	if err := s.Emit(ctx, userID, e); err != nil {
		s.deps.Log.Warn("notifier: emit constraint_hit", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyScheduledJobFailed(ctx context.Context, jobName, errText string) {
	e := NewScheduledJobFailedEvent(jobName, errText)
	s.EmitToAdmins(ctx, e)
}

func (s *Service) NotifyUserScheduledJobFailed(ctx context.Context, userID uuid.UUID, jobName, errText string) {
	if err := s.Emit(ctx, userID, NewScheduledJobFailedEvent(jobName, errText)); err != nil {
		s.deps.Log.Warn("notifier: emit scheduled_job_failed", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyBusinessWarningNoCost(ctx context.Context, userID, productID uuid.UUID, externalSKU, productName string) {
	if err := s.Emit(ctx, userID, NewBusinessWarningNoCostEvent(productID, externalSKU, productName)); err != nil {
		s.deps.Log.Warn("notifier: emit business_warning_no_cost", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyBusinessWarningNoCompetitors(ctx context.Context, userID, productID uuid.UUID, externalSKU, productName string) {
	if err := s.Emit(ctx, userID, NewBusinessWarningNoCompetitorsEvent(productID, externalSKU, productName)); err != nil {
		s.deps.Log.Warn("notifier: emit business_warning_no_competitors", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyBusinessWarningPriceDrift(ctx context.Context, userID, productID uuid.UUID, externalSKU string, expected, actual float64) {
	if err := s.Emit(ctx, userID, NewBusinessWarningPriceDriftEvent(productID, externalSKU, expected, actual)); err != nil {
		s.deps.Log.Warn("notifier: emit business_warning_price_drift", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyCompetitorPriceDropped(ctx context.Context, userID, productID uuid.UUID, externalSKU string, oldPrice, newPrice float64) {
	if err := s.Emit(ctx, userID, NewCompetitorPriceDroppedEvent(productID, externalSKU, oldPrice, newPrice)); err != nil {
		s.deps.Log.Warn("notifier: emit competitor_price_dropped", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyCompetitorAppeared(ctx context.Context, userID, productID uuid.UUID, externalSKU, competitorURL string, price float64) {
	if err := s.Emit(ctx, userID, NewCompetitorAppearedEvent(productID, externalSKU, competitorURL, price)); err != nil {
		s.deps.Log.Warn("notifier: emit competitor_appeared", "err", err, "user_id", userID)
	}
}

func (s *Service) NotifyMedianShifted(ctx context.Context, userID, productID uuid.UUID, externalSKU string, oldMedian, newMedian float64) {
	if err := s.Emit(ctx, userID, NewMedianShiftedEvent(productID, externalSKU, oldMedian, newMedian)); err != nil {
		s.deps.Log.Warn("notifier: emit median_shifted", "err", err, "user_id", userID)
	}
}
