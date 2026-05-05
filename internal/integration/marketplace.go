package integration

import (
	"context"
	"errors"
)

// ErrUnauthorized возвращается адаптером если API-ключ отклонён маркетплейсом.
var ErrUnauthorized = errors.New("marketplace: unauthorized")

// ErrRateLimited возвращается адаптером при превышении лимита запросов (HTTP 429).
var ErrRateLimited = errors.New("marketplace: rate limited")

// SKU — товарная позиция, полученная от маркетплейса.
type SKU struct {
	ExternalSKU  string
	Name         string
	CurrentPrice float64
	Currency     string
}

// PriceUpdate — задание на обновление цены одного SKU.
type PriceUpdate struct {
	ExternalSKU string
	NewPrice    float64
}

// Marketplace — единый контракт для всех адаптеров маркетплейсов.
// Каждый адаптер создаётся с уже расшифрованными credentials.
type Marketplace interface {
	// TestAuth проверяет валидность API-ключей.
	TestAuth(ctx context.Context) error
	// ListSKUs возвращает все товары магазина.
	ListSKUs(ctx context.Context) ([]SKU, error)
	// UpdatePrices отправляет обновлённые цены в маркетплейс.
	UpdatePrices(ctx context.Context, updates []PriceUpdate) error
}
