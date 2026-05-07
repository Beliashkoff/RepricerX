package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	PriceChangeStatusSuccess = "success"
	PriceChangeStatusFailed  = "failed"
	PriceChangeStatusSkipped = "skipped"
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
