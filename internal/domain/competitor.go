package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	CompetitorStatusPending     = "pending"
	CompetitorStatusOK          = "ok"
	CompetitorStatusFailed      = "failed"
	CompetitorStatusRateLimited = "rate_limited"
	CompetitorStatusBlocked     = "blocked"
	CompetitorStatusDisabled    = "disabled"

	CompetitorAvailabilityUnknown    = "unknown"
	CompetitorAvailabilityAvailable  = "available"
	CompetitorAvailabilityOutOfStock = "out_of_stock"
	CompetitorAvailabilityNotFound   = "not_found"

	CompetitorErrorInvalidTarget = "invalid_target"
	CompetitorErrorUnsupported   = "unsupported_source"
	CompetitorErrorUnavailable   = "source_unavailable"
	CompetitorErrorParseFailed   = "parse_failed"

	BackgroundJobTypeCompetitorRefresh = "competitor_refresh"
)

type ProductCompetitor struct {
	ID                      uuid.UUID
	ProductID               uuid.UUID
	Marketplace             string
	Source                  string
	CompetitorURL           string
	NormalizedCompetitorURL string
	OzonPublicProductID     *string
	OzonSKU                 *string
	LastPrice               *float64
	LastAvailability        string
	LastCheckedAt           *time.Time
	LastErrorCode           string
	LastStatus              string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type CompetitorPriceSnapshot struct {
	ID           uuid.UUID
	CompetitorID uuid.UUID
	Price        *float64
	Availability string
	CheckedAt    time.Time
	Status       string
	ErrorCode    string
	RawSource    string
}

type CompetitorRefreshJobPayload struct {
	CompetitorID uuid.UUID `json:"competitor_id"`
	UserID       uuid.UUID `json:"user_id"`
}
