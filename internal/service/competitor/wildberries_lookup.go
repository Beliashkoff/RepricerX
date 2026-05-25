package competitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

// WBPriceLookup — интерфейс для получения цены товара WB по NmID.
type WBPriceLookup interface {
	Lookup(ctx context.Context, nmID string) (LookupResult, error)
}

// HTTPBasedWBLookup — реализация через публичный API card.wb.ru.
type HTTPBasedWBLookup struct {
	http *http.Client
}

func NewHTTPBasedWBLookup() *HTTPBasedWBLookup {
	return &HTTPBasedWBLookup{http: &http.Client{
		Timeout: 10 * time.Second,
	}}
}

// cardDetailURL — endpoint публичного каталога WB.
// dest=-1257786 соответствует Москве (основной склад).
const cardDetailURL = "https://card.wb.ru/cards/v1/detail?appType=1&curr=rub&dest=-1257786&nm="

type wbCardResponse struct {
	Data struct {
		Products []struct {
			ID            int    `json:"id"`
			Name          string `json:"name"`
			SalePriceU    int    `json:"salePriceU"`    // цена со скидкой в копейках
			PriceU        int    `json:"priceU"`        // цена без скидки в копейках
			TotalQuantity int    `json:"totalQuantity"` // остаток (может отсутствовать)
		} `json:"products"`
	} `json:"data"`
}

func (l *HTTPBasedWBLookup) Lookup(ctx context.Context, nmID string) (LookupResult, error) {
	if nmID == "" {
		return LookupResult{}, ErrInvalidTarget
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardDetailURL+nmID, nil)
	if err != nil {
		return LookupResult{}, ErrInvalidTarget
	}
	req.Header.Set("User-Agent", "RepricerX/1.0 competitor-price-check")
	req.Header.Set("Accept", "application/json")

	resp, err := l.http.Do(req)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: request", ErrRefreshFailed)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusTooManyRequests {
		return LookupResult{}, fmt.Errorf("%w: rate_limited", ErrRefreshFailed)
	}
	if resp.StatusCode == http.StatusNotFound {
		return LookupResult{Availability: domain.CompetitorAvailabilityNotFound, Source: "public_wb"}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return LookupResult{}, fmt.Errorf("%w: status %d", ErrRefreshFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: read", ErrRefreshFailed)
	}

	return parseWBCardResponse(body)
}

func parseWBCardResponse(body []byte) (LookupResult, error) {
	var card wbCardResponse
	if err := json.Unmarshal(body, &card); err != nil {
		return LookupResult{}, fmt.Errorf("%w: parse json", ErrRefreshFailed)
	}

	if len(card.Data.Products) == 0 {
		// Пустой массив — товар не найден
		return LookupResult{Availability: domain.CompetitorAvailabilityNotFound, Source: "public_wb"}, nil
	}

	product := card.Data.Products[0]

	// salePriceU — цена в копейках × 100 (у WB своё соглашение: делим на 100)
	priceKopecks := product.SalePriceU
	if priceKopecks <= 0 {
		priceKopecks = product.PriceU
	}
	if priceKopecks <= 0 {
		return LookupResult{Availability: domain.CompetitorAvailabilityUnknown, Source: "public_wb"}, nil
	}

	priceRub := float64(priceKopecks) / 100.0

	availability := domain.CompetitorAvailabilityAvailable
	if product.TotalQuantity == 0 {
		// totalQuantity = 0 означает нет в наличии
		// Примечание: поле может быть 0 и при наличии если склад не отдаёт данные.
		// Оставляем availability=available если price валидна, чтобы не терять цены.
		availability = domain.CompetitorAvailabilityAvailable
	}

	return LookupResult{Price: &priceRub, Availability: availability, Source: "public_wb"}, nil
}

// WBNmIDFromURL — извлекает NmID из URL вида wildberries.ru/catalog/{nmID}/detail.aspx
// или возвращает исходную строку если это уже чистое число.
func WBNmIDFromURL(raw string) string {
	// Ищем числовой фрагмент в пути URL
	matches := productIDPattern.FindAllString(raw, -1)
	if len(matches) == 0 {
		return ""
	}
	// Берём первое совпадение из пути (для WB URL это nmID)
	return matches[0]
}

// IsValidWBNmID — проверяет, является ли строка корректным WB NmID (6–12 цифр).
func IsValidWBNmID(s string) bool {
	if len(s) < 6 || len(s) > 12 {
		return false
	}
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}
