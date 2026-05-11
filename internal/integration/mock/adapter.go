package mock

import (
	"context"

	"github.com/Beliashkoff/RepricerX/internal/integration"
)

// Adapter — реализация integration.Marketplace, читающая/пишущая в общий Store.
type Adapter struct {
	store       *Store
	marketplace string
	shopID      string
}

// Проверка интерфейса на этапе компиляции.
var _ integration.Marketplace = (*Adapter)(nil)

func (a *Adapter) TestAuth(_ context.Context) error {
	return nil
}

func (a *Adapter) ListSKUs(_ context.Context) ([]integration.SKU, error) {
	return a.store.List(a.marketplace, a.shopID), nil
}

func (a *Adapter) UpdatePrices(_ context.Context, updates []integration.PriceUpdate) error {
	return a.store.Apply(a.marketplace, a.shopID, updates)
}
