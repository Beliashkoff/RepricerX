package mock

import (
	"bytes"

	"github.com/Beliashkoff/RepricerX/internal/integration"
)

// invalidCredsMarker — magic-значение в JSON credentials, которое заставляет
// мок-адаптер вернуть integration.ErrUnauthorized. Нужно для прогона негативной
// ветки `POST /api/shops/:id/test` без поднятия реальных API.
const invalidCredsMarker = `"invalid"`

// NewBuilder возвращает фабричную функцию мок-адаптера. Возвращаемый тип —
// нетипизированная `func(...) (...)`, что позволяет напрямую присваивать её
// любому named MarketplaceFactory (shopsvc, productsvc, pricingsvc, dispatchersvc).
func NewBuilder(store *Store, marketplace string) func(shopID string, credsJSON []byte) (integration.Marketplace, error) {
	return func(shopID string, credsJSON []byte) (integration.Marketplace, error) {
		if bytes.Contains(credsJSON, []byte(invalidCredsMarker)) {
			return nil, integration.ErrUnauthorized
		}
		return &Adapter{store: store, marketplace: marketplace, shopID: shopID}, nil
	}
}
