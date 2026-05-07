package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	StrategyTypeBelowMedianPct       = "below_median_pct"
	StrategyTypeMinCompetitorPlusStep = "min_competitor_plus_step"
	StrategyTypeMinMarginPct         = "min_margin_pct"
	StrategyTypeFixed                = "fixed"

	FallbackPolicyKeepCurrent = "keep_current"
	FallbackPolicySetFixed    = "set_fixed"
	FallbackPolicySetMin      = "set_min"
)

type Strategy struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Name           string
	Type           string
	Params         json.RawMessage
	Constraints    json.RawMessage
	FallbackPolicy string
	Priority       int
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

