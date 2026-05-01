package domain

import (
	"time"

	"github.com/google/uuid"
)

type Shop struct {
	ID                   uuid.UUID
	UserID               uuid.UUID
	Marketplace          string
	Name                 string
	CredentialsEncrypted []byte
	Status               string
	AutoUpdateEnabled    bool
	ScheduleCron         string
	LastCheckedAt        *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

const (
	MarketplaceWB   = "wb"
	MarketplaceOzon = "ozon"

	ShopStatusDraft    = "draft"
	ShopStatusActive   = "active"
	ShopStatusError    = "error"
	ShopStatusDisabled = "disabled"
)

// WBCredentials — учётные данные Wildberries (один API-токен).
type WBCredentials struct {
	APIKey string `json:"api_key"`
}

// OzonCredentials — учётные данные Ozon (client_id + api_key).
type OzonCredentials struct {
	ClientID string `json:"client_id"`
	APIKey   string `json:"api_key"`
}
