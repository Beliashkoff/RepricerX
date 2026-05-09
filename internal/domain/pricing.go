package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	PriceChangeStatusSuccess = "success"
	PriceChangeStatusFailed  = "failed"
	PriceChangeStatusSkipped = "skipped"

	PlanStatusPending     = "pending"
	PlanStatusProcessing  = "processing"
	PlanStatusCalculated  = "calculated"  // расчёт завершён, ждёт отправки (Этап 6)
	PlanStatusDispatching = "dispatching" // идёт отправка в МП (Этап 6)
	PlanStatusApplied     = "applied"     // отправка завершена успешно
	PlanStatusPartial     = "partial"     // часть отправлена, часть с ошибкой (Этап 6)
	PlanStatusFailed      = "failed"
	PlanStatusCancelled   = "cancelled"

	PlanItemStatusPending    = "pending"
	PlanItemStatusApplied    = "applied"    // legacy: использовалось в Этапе 5 как "рассчитан"
	PlanItemStatusDispatched = "dispatched" // цена отправлена в МП и принята (Этап 6)
	PlanItemStatusSkipped    = "skipped"
	PlanItemStatusFailed     = "failed"

	ConstraintMinPrice       = "min_price"
	ConstraintMaxPrice       = "max_price"
	ConstraintMinProfitPct   = "min_profit_pct"
	ConstraintMinProfitAbs   = "min_profit_abs"
	ConstraintMaxChangePct   = "max_change_pct"
	ConstraintCostPriceFloor = "cost_price_floor"

	ReasonMissingCost         = "missing_cost_price"
	ReasonNoCompetitors       = "no_competitors"
	ReasonFallbackKeepCurrent = "fallback_keep_current"
	ReasonFallbackSetMin      = "fallback_set_min"
	ReasonUnsupportedCurrency = "unsupported_currency"
	ReasonInvalidCurrent      = "invalid_current_price"
	ReasonProductArchived       = "product_archived"
	ReasonStrategyDisabled      = "strategy_disabled"
	ReasonMinIntervalNotElapsed = "min_interval_not_elapsed"

	// Reasons для Этапа 6 (отправка)
	ReasonDispatchUnauthorized     = "marketplace_unauthorized"
	ReasonDispatchRateLimited      = "marketplace_rate_limited"
	ReasonDispatchRetriesExhausted = "dispatch_retries_exhausted"
	ReasonDispatchNetworkError     = "marketplace_network_error"
	ReasonDispatchPartialReject    = "marketplace_partial_reject"

	ConstraintMinInterval = "min_interval_minutes"

	BackgroundJobTypePriceRecalculation = "price_recalculation"
	BackgroundJobTypePriceDispatch      = "price_dispatch"

	// Operations для integration_log (вызовы маркетплейса при dispatch)
	IntegrationOpPriceDispatch = "price_dispatch"
)

type PriceChange struct {
	ID            uuid.UUID
	ShopID        uuid.UUID
	ProductID     uuid.UUID
	ProductName   string
	StrategyID    *uuid.UUID
	OldPrice      float64
	NewPrice      float64
	TargetPrice   float64
	Reason        string
	ConstraintHit *string
	Status        string
	CreatedAt     time.Time
}

type PriceChangeSummary struct {
	TotalUpdates      int
	SuccessfulUpdates int
	FailedUpdates     int
	AvgChangePct      float64
	PeriodStart       time.Time
	PeriodEnd         time.Time
}

type PricePlan struct {
	ID        uuid.UUID
	ShopID    uuid.UUID
	Status    string
	Total     int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PricePlanItem struct {
	ID            uuid.UUID
	PlanID        uuid.UUID
	ProductID     uuid.UUID
	ProductName   string
	StrategyID    *uuid.UUID
	CurrentPrice  float64
	TargetPrice   float64
	FinalPrice    float64
	ConstraintHit string
	Status        string
	Error         string
	Reason        string
	CorrelationID uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PriceRecalculationJobPayload struct {
	PlanID            uuid.UUID   `json:"plan_id"`
	ShopID            uuid.UUID   `json:"shop_id"`
	ProductIDs        []uuid.UUID `json:"product_ids,omitempty"`
	RequestedByUserID uuid.UUID   `json:"requested_by_user_id"`
}

// PriceDispatchJobPayload — задача отправки уже рассчитанного плана в МП.
type PriceDispatchJobPayload struct {
	PlanID            uuid.UUID `json:"plan_id"`
	ShopID            uuid.UUID `json:"shop_id"`
	RequestedByUserID uuid.UUID `json:"requested_by_user_id"`
}
