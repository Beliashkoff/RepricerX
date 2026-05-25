// Package competitor — skeleton для будущей интеграции с MPStats API.
//
// MPStats — платный российский агрегатор данных маркетплейсов (mpstats.io).
// Используется большинством серьёзных SaaS-инструментов для WB/Ozon в России.
//
// Подключение: установить env OZON_PRICE_SOURCE=mpstats и MPSTATS_API_KEY=<token>.
// Документация API: https://mpstats.io/api
//
// TODO: реализовать Lookup когда появятся платящие пользователи.
package competitor

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// MPStatsOzonLookup получает цены через платный агрегатор MPStats.
// Реализует интерфейс OzonPriceLookup.
//
// Преимущества перед BFF-парсингом:
//   - Стабильный REST API с SLA
//   - История цен и аналитика
//   - Не зависит от антибот-защиты Ozon
//   - Юридически чистое решение (MPStats берёт риски на себя)
type MPStatsOzonLookup struct {
	apiKey string
	http   *http.Client
}

// NewMPStatsOzonLookup создаёт MPStats lookup с заданным API-ключом.
// Получить ключ: https://mpstats.io → Настройки → API.
func NewMPStatsOzonLookup(apiKey string) *MPStatsOzonLookup {
	return &MPStatsOzonLookup{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Lookup возвращает цену товара Ozon через MPStats API.
//
// TODO: реализовать:
//
//	GET https://mpstats.io/api/oz/get/item/{target.PublicProductID}
//	Header: X-Mpstats-TOKEN: {apiKey}
//	Response: {"price": 1490.0, "price_with_sale": 1200.0, "name": "...", ...}
//
// Маппинг ответа → LookupResult:
//   - price_with_sale (или price если скидки нет) → Price
//   - is_in_stock → Availability (available / out_of_stock)
//   - Source: "mpstats"
func (l *MPStatsOzonLookup) Lookup(_ context.Context, target OzonTarget) (LookupResult, error) {
	// Заглушка — не должна вызываться пока не реализована.
	// SelectOzonLookup не вернёт MPStatsOzonLookup без MPSTATS_API_KEY,
	// поэтому это паника маловероятна, но явная ошибка лучше чем тихий провал.
	return LookupResult{}, fmt.Errorf("mpstats lookup not implemented (product_id=%s): set OZON_PRICE_SOURCE=bff or implement Lookup()", target.PublicProductID)
}

// Проверка что MPStatsOzonLookup реализует интерфейс OzonPriceLookup.
var _ OzonPriceLookup = (*MPStatsOzonLookup)(nil)
