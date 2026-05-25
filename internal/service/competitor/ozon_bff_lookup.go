// Package competitor — BFF-based реализация OzonPriceLookup.
// Использует тот же endpoint что Ozon SPA (entrypoint-api.bx/page/json/v2)
// и возвращает структурированный JSON вместо HTML.
package competitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

// BFFBasedOzonLookup получает цену через BFF API Ozon.
// API endpoint: https://www.ozon.ru/api/entrypoint-api.bx/page/json/v2?url=/product/-{id}/
// Это тот же endpoint, который использует сам сайт Ozon при загрузке страницы товара.
// Возвращает JSON с widgetStates — надёжнее HTML-парсинга, который ломается на SPA.
type BFFBasedOzonLookup struct {
	http *http.Client
}

// NewBFFBasedOzonLookup создаёт новый BFF lookup с дефолтными параметрами.
func NewBFFBasedOzonLookup() *BFFBasedOzonLookup {
	return &BFFBasedOzonLookup{
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

const ozonBFFBaseURL = "https://www.ozon.ru/api/entrypoint-api.bx/page/json/v2"

// Lookup возвращает цену товара Ozon по его PublicProductID (или URL).
// Реализует интерфейс OzonPriceLookup.
func (l *BFFBasedOzonLookup) Lookup(ctx context.Context, target OzonTarget) (LookupResult, error) {
	productID := target.PublicProductID
	if productID == "" && target.URL != "" {
		productID = lastID(target.URL)
	}
	if productID == "" {
		return LookupResult{}, ErrInvalidTarget
	}

	// Ozon SPA использует формат /product/-{id}/ (дефис перед ID — часть формата)
	productPath := "/product/-" + productID + "/"
	bffURL := ozonBFFBaseURL + "?url=" + url.QueryEscape(productPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bffURL, nil)
	if err != nil {
		return LookupResult{}, ErrInvalidTarget
	}
	// Заголовки браузера — аналогично search.go; без них Ozon может отклонить запрос.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-o3-app-name", "ozonWebDesktop")

	resp, err := l.http.Do(req)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: request", ErrRefreshFailed)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusTooManyRequests {
		return LookupResult{}, fmt.Errorf("%w: rate_limited", ErrRefreshFailed)
	}
	if resp.StatusCode == http.StatusNotFound {
		return LookupResult{Availability: domain.CompetitorAvailabilityNotFound, Source: "public_ozon_bff"}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return LookupResult{}, fmt.Errorf("%w: status %d", ErrRefreshFailed, resp.StatusCode)
	}

	// BFF ответы крупнее HTML — разрешаем до 3 МБ.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: read", ErrRefreshFailed)
	}

	return parseBFFProductResponse(body)
}

// parseBFFProductResponse парсит ответ BFF API страницы товара Ozon.
//
// Формат ответа: {"widgetStates": {"widget-key": "json-string", ...}}
// Каждое значение — двойно-закодированный JSON: json.RawMessage содержит строку,
// которая в свою очередь является JSON-объектом с данными виджета.
// Паттерн такой же как в search.go (searchResultsV2), но для другой страницы.
func parseBFFProductResponse(body []byte) (LookupResult, error) {
	var raw struct {
		WidgetStates map[string]json.RawMessage `json:"widgetStates"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return LookupResult{}, fmt.Errorf("%w: parse bff json", ErrRefreshFailed)
	}
	if len(raw.WidgetStates) == 0 {
		// Ozon вернул пустой widgetStates — CAPTCHA или блокировка.
		return LookupResult{}, fmt.Errorf("%w: empty widgetStates (bot protection?)", ErrRefreshFailed)
	}

	// Ищем цену по убыванию приоритета ключей виджетов.
	// Ozon меняет точные названия, поэтому ищем по подстроке.
	priorityPrefixes := []string{
		"webPrice",
		"webSingleProduct",
		"webProductHeading",
		"webAddToCart",
		"webProductBuy",
	}

	for _, prefix := range priorityPrefixes {
		for key, val := range raw.WidgetStates {
			if !strings.Contains(key, prefix) {
				continue
			}
			if price := extractPriceFromWidget(val); price != nil {
				return LookupResult{
					Price:        price,
					Availability: domain.CompetitorAvailabilityAvailable,
					Source:       "public_ozon_bff",
				}, nil
			}
		}
	}

	// Fallback: любой виджет с ценой > 0.
	for _, val := range raw.WidgetStates {
		if price := extractPriceFromWidget(val); price != nil {
			return LookupResult{
				Price:        price,
				Availability: domain.CompetitorAvailabilityAvailable,
				Source:       "public_ozon_bff",
			}, nil
		}
	}

	return LookupResult{}, fmt.Errorf("%w: no price found in bff response", ErrRefreshFailed)
}

// extractPriceFromWidget декодирует виджет из widgetStates и ищет в нём цену.
// Ozon использует двойное кодирование: json.RawMessage → json-string → json-object.
func extractPriceFromWidget(val json.RawMessage) *float64 {
	// Шаг 1: val может быть строкой (двойной encode) или сразу объектом.
	var inner string
	if err := json.Unmarshal(val, &inner); err != nil {
		// val — уже объект, не строка.
		return findPriceInJSON(val)
	}
	if inner == "" {
		return nil
	}
	// Шаг 2: inner — JSON-строка с объектом виджета.
	return findPriceInJSON([]byte(inner))
}

// findPriceInJSON рекурсивно ищет числовую цену в произвольной JSON-структуре.
func findPriceInJSON(data []byte) *float64 {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil
	}
	return findPriceInAny(v, 0)
}

// bffPriceFields — имена полей, которые могут содержать актуальную цену товара.
// Поле "originalPrice" намеренно исключено — это цена без скидки, нам нужна итоговая.
var bffPriceFields = map[string]bool{
	"price":      true,
	"finalPrice": true,
	"cardPrice":  true,
	"salePrice":  true,
	"offerPrice": true,
}

// findPriceInAny рекурсивно обходит произвольную структуру в поисках поля цены.
// Глубина ограничена 6 уровнями чтобы избежать бесконечного обхода.
func findPriceInAny(v any, depth int) *float64 {
	if depth > 6 {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		// Сначала ищем по приоритетным именам полей.
		for fieldName := range bffPriceFields {
			if raw, ok := val[fieldName]; ok {
				if price := priceFromAnyBFF(raw); price != nil {
					return price
				}
			}
		}
		// Специальный случай: money.value (формат Ozon для денежных значений).
		if money, ok := val["money"].(map[string]any); ok {
			if price := priceFromAnyBFF(money["value"]); price != nil {
				return price
			}
		}
		// Рекурсивно в дочерних объектах (кроме веток с зачёркнутыми ценами).
		for key, child := range val {
			if key == "originalPrice" || key == "crossedPrice" || key == "strikethroughPrice" {
				continue // пропускаем старые цены
			}
			if price := findPriceInAny(child, depth+1); price != nil {
				return price
			}
		}
	case []any:
		for _, item := range val {
			if price := findPriceInAny(item, depth+1); price != nil {
				return price
			}
		}
	}
	return nil
}

// priceFromAnyBFF конвертирует значение в цену.
// Поддерживает: float64, string "1 490 ₽", string "1490.00", string "1490".
func priceFromAnyBFF(v any) *float64 {
	switch val := v.(type) {
	case float64:
		if val > 0 {
			return &val
		}
	case string:
		if p, ok := parsePriceStringBFF(val); ok && p > 0 {
			return &p
		}
	}
	return nil
}

// parsePriceStringBFF парсит строку цены в форматах Ozon:
//   - "1 490 ₽"  — пробел как разделитель тысяч (типичный формат Ozon)
//   - "1490.00"  — точка как дробный разделитель
//   - "1,490.00" — US-формат (запятая как тысячи, точка как дробный)
//   - "1490,00"  — EU-формат (запятая как дробный)
//   - "1490"     — целое число
func parsePriceStringBFF(s string) (float64, bool) {
	// Убираем символ рубля и все виды пробелов (обычный, NBSP, узкий NBSP, пробел в цифрах).
	clean := strings.TrimSpace(s)
	clean = strings.ReplaceAll(clean, "₽", "") // ₽
	clean = strings.ReplaceAll(clean, " ", "") // неразрывный пробел
	clean = strings.ReplaceAll(clean, " ", "") // узкий неразрывный пробел
	clean = strings.ReplaceAll(clean, " ", "") // пробел в группах цифр
	clean = strings.ReplaceAll(clean, " ", "")      // обычный пробел
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return 0, false
	}

	// Определяем формат разделителей.
	hasDot := strings.ContainsRune(clean, '.')
	hasComma := strings.ContainsRune(clean, ',')

	var normalized string
	switch {
	case hasDot && hasComma:
		// "1,490.00" → запятая разделитель тысяч, точка дробный.
		normalized = strings.ReplaceAll(clean, ",", "")
	case hasComma && !hasDot:
		// "1490,00" → запятая дробный разделитель (EU).
		normalized = strings.ReplaceAll(clean, ",", ".")
	default:
		normalized = clean
	}

	// Оставляем только цифры и первую точку.
	var b strings.Builder
	dotSeen := false
	for _, r := range normalized {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' && !dotSeen:
			b.WriteRune(r)
			dotSeen = true
		}
	}
	if b.Len() == 0 {
		return 0, false
	}

	f, err := strconv.ParseFloat(b.String(), 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return f, true
}
