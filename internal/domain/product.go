package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	ProductStatusActive     = "active"
	ProductStatusArchived   = "archived"
	ProductStatusOutOfStock = "out_of_stock"

	ImportStatusPending   = "pending"
	ImportStatusRunning   = "running"
	ImportStatusSucceeded = "succeeded"
	ImportStatusFailed    = "failed"
	ImportStatusPartial   = "partial"
	ImportStatusCanceled  = "canceled"

	BackgroundJobTypeSKUImport = "sku_import"

	BackgroundJobStatusPending   = "pending"
	BackgroundJobStatusRunning   = "running"
	BackgroundJobStatusSucceeded = "succeeded"
	BackgroundJobStatusFailed    = "failed"
	BackgroundJobStatusCanceled  = "canceled"
	BackgroundJobStatusRetrying  = "retrying"
)

type Product struct {
	ID           uuid.UUID
	ShopID       uuid.UUID
	ExternalSKU  string
	Name         string
	CurrentPrice float64
	Currency     string
	Status       string
	MinPrice     *float64
	MaxPrice     *float64
	CostPrice    *float64
	StockCount   int
	Rating       *float64
	ReviewsCount int
	LastSyncedAt *time.Time
	HasStrategy  bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ImportLogEntry struct {
	ID          uuid.UUID
	ShopID      uuid.UUID
	JobID       *uuid.UUID
	UserID      *uuid.UUID
	Status      string
	StartedAt   time.Time
	RequestedAt time.Time
	FinishedAt  *time.Time
	JobStatus   string
	Total       int
	Added       int
	Updated     int
	Skipped     int
	Failed      int
	Errors      []ImportLogError
}

type ImportLogError struct {
	ExternalSKU string `json:"external_sku,omitempty"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

type BackgroundJob struct {
	ID            uuid.UUID
	JobType       string
	Status        string
	Queue         string
	Priority      int
	Payload       []byte
	Result        []byte
	Attempts      int
	MaxAttempts   int
	RunAt         time.Time
	LockedAt      *time.Time
	LockedBy      *string
	LockExpiresAt *time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
	CanceledAt    *time.Time
}

type SKUImportJobPayload struct {
	ImportID          uuid.UUID `json:"import_id"`
	ShopID            uuid.UUID `json:"shop_id"`
	RequestedByUserID uuid.UUID `json:"requested_by_user_id"`
}
