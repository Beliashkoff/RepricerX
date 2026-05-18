package integration

import (
	"context"
	"errors"
)

// ErrUnauthorized возвращается адаптером если API-ключ отклонён маркетплейсом.
var ErrUnauthorized = errors.New("marketplace: unauthorized")

// ErrRateLimited возвращается адаптером при превышении лимита запросов (HTTP 429).
var ErrRateLimited = errors.New("marketplace: rate limited")

// ErrUnexpectedStatus возвращается, когда маркетплейс отдал не-2xx ответ,
// который мы не умеем классифицировать (не 401/403/429). Сервис мапит его в
// ErrMarketplaceUnavailable → HTTP 502.
var ErrUnexpectedStatus = errors.New("marketplace: unexpected status")

// SKU — товарная позиция, полученная от маркетплейса.
type SKU struct {
	ExternalSKU  string // WB: nmID десятичной строкой; Ozon: offer_id.
	VendorCode   string // WB: vendorCode (артикул продавца). Ozon: пусто.
	Name         string
	CurrentPrice float64
	Currency     string
	StockCount   int
}

// PriceUpdate — задание на обновление цены одного SKU.
type PriceUpdate struct {
	ExternalSKU string
	NewPrice    float64
	Discount    int // WB: обязательное поле в /api/v2/upload/task (0 допустимо). Ozon игнорирует.
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
