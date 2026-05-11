package mock

import (
	"sort"
	"sync"

	"github.com/Beliashkoff/RepricerX/internal/integration"
)

// Store — потокобезопасное in-memory состояние для мок-адаптеров.
// Состояние партиционируется по (marketplace, shopID); при первом обращении
// бакет лениво наполняется seed-фикстурой.
type Store struct {
	mu   sync.RWMutex
	data map[string]map[string]integration.SKU
}

func NewStore() *Store {
	return &Store{data: make(map[string]map[string]integration.SKU)}
}

func bucketKey(marketplace, shopID string) string {
	return marketplace + ":" + shopID
}

// ensureSeeded должен вызываться под удержанным mu (Lock, не RLock).
func (s *Store) ensureSeeded(marketplace, shopID string) {
	key := bucketKey(marketplace, shopID)
	if _, ok := s.data[key]; ok {
		return
	}
	skus := seedFor(marketplace)
	bucket := make(map[string]integration.SKU, len(skus))
	for _, sku := range skus {
		bucket[sku.ExternalSKU] = sku
	}
	s.data[key] = bucket
}

// List возвращает копию SKU магазина, упорядоченную по ExternalSKU.
func (s *Store) List(marketplace, shopID string) []integration.SKU {
	s.mu.Lock()
	s.ensureSeeded(marketplace, shopID)
	bucket := s.data[bucketKey(marketplace, shopID)]
	out := make([]integration.SKU, 0, len(bucket))
	for _, sku := range bucket {
		out = append(out, sku)
	}
	s.mu.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].ExternalSKU < out[j].ExternalSKU })
	return out
}

// Apply мутирует CurrentPrice указанных SKU. Неизвестные external_sku
// игнорируются — реальные адаптеры WB/Ozon тоже не падают на таких записях,
// они просто возвращают ошибки на пер-SKU уровне, а UpdatePrices() в интерфейсе
// возвращает агрегированную ошибку только для транспортных сбоев.
func (s *Store) Apply(marketplace, shopID string, updates []integration.PriceUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureSeeded(marketplace, shopID)
	bucket := s.data[bucketKey(marketplace, shopID)]
	for _, up := range updates {
		sku, ok := bucket[up.ExternalSKU]
		if !ok {
			continue
		}
		sku.CurrentPrice = up.NewPrice
		bucket[up.ExternalSKU] = sku
	}
	return nil
}
