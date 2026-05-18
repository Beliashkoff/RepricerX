// Package wildberries реализует адаптер к Wildberries Seller API.
package wildberries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/pkg/ratelimit"
)

// Дефолтные базы WB-сервисов. Тесты конструируют Client с собственными URL-ами.
const (
	defaultCommonBase  = "https://common-api.wildberries.ru"
	defaultContentBase = "https://content-api.wildberries.ru"
	defaultPricesBase  = "https://discounts-prices-api.wildberries.ru"
)

// Client — адаптер Wildberries. Реализует integration.Marketplace.
type Client struct {
	shopID      string
	apiKey      string
	http        *http.Client
	limiter     *ratelimit.Registry
	commonBase  string
	contentBase string
	pricesBase  string
}

// NewClient создаёт клиент из JSON-сериализованных WBCredentials.
// shopID используется для per-shop rate limiting.
func NewClient(shopID string, credsJSON []byte, limiter *ratelimit.Registry) (*Client, error) {
	var creds domain.WBCredentials
	if err := json.Unmarshal(credsJSON, &creds); err != nil {
		return nil, fmt.Errorf("wb: parse credentials: %w", err)
	}
	if creds.APIKey == "" {
		return nil, errors.New("wb: api_key is required")
	}
	return &Client{
		shopID:      shopID,
		apiKey:      creds.APIKey,
		http:        &http.Client{Timeout: 15 * time.Second},
		limiter:     limiter,
		commonBase:  defaultCommonBase,
		contentBase: defaultContentBase,
		pricesBase:  defaultPricesBase,
	}, nil
}

// doWithRetry выполняет HTTP-запрос с тремя попытками и экспоненциальным backoff.
// Retry только при сетевых ошибках и HTTP 5xx — 4xx не ретраим.
func (c *Client) doWithRetry(ctx context.Context, buildReq func() (*http.Request, error)) (*http.Response, error) {
	delays := []time.Duration{time.Second, 3 * time.Second, 9 * time.Second}
	var lastErr error
	for i := 0; i <= len(delays); i++ {
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx, c.shopID); err != nil {
				return nil, err
			}
		}
		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
		} else if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			return nil, integration.ErrRateLimited
		} else if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("wb: server error %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
		} else {
			return resp, nil
		}
		if i < len(delays) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delays[i]):
			}
		}
	}
	return nil, lastErr
}

// TestAuth проверяет валидность API-ключа запросом к /ping на discounts-prices-api.
// У WB у каждого сервиса свой /ping, и он валидирует не только токен, но и совпадение
// его категории с сервисом. Мы пингуем именно Discounts & Prices, потому что без
// этой категории невозможен core-flow (UpdatePrices), а пинг Common API отказал бы
// токенам других категорий и сбивал бы с толку.
// Лимит — 3 запроса/30 сек. https://dev.wildberries.ru/openapi/api-information
func (c *Client) TestAuth(ctx context.Context) error {
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			c.pricesBase+"/ping", nil)
		if err != nil {
			return nil, fmt.Errorf("wb: build request: %w", err)
		}
		c.setAuth(req)
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("wb: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return integration.ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wb: unexpected status %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
	}
	return nil
}

// ListSKUs возвращает товарные карточки магазина.
// Идёт двумя шагами:
//  1. /content/v2/get/cards/list — пагинированный список карточек (nmID, vendorCode, title).
//  2. /api/v2/list/goods/filter — актуальные цены пачками по 1000 nmID.
//
// Эндпоинт cards/list документирован в https://dev.wildberries.ru/openapi/work-with-products;
// goods/filter — там же, в разделе Discounts & Prices API.
func (c *Client) ListSKUs(ctx context.Context) ([]integration.SKU, error) {
	type cardObject struct {
		NmID       int64  `json:"nmID"`
		VendorCode string `json:"vendorCode"`
		Title      string `json:"title"`
	}
	type cursorObj struct {
		UpdatedAt string `json:"updatedAt"`
		NmID      int64  `json:"nmID"`
		Total     int    `json:"total"`
	}
	type response struct {
		Cards  []cardObject `json:"cards"`
		Cursor cursorObj    `json:"cursor"`
	}

	body := `{"settings":{"cursor":{"limit":100},"filter":{"withPhoto":-1}}}`
	var result []integration.SKU

	for {
		currentBody := body
		resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				c.contentBase+"/content/v2/get/cards/list",
				strings.NewReader(currentBody))
			if err != nil {
				return nil, fmt.Errorf("wb: build list request: %w", err)
			}
			c.setAuth(req)
			req.Header.Set("Content-Type", "application/json")
			return req, nil
		})
		if err != nil {
			return nil, fmt.Errorf("wb: list request: %w", err)
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			_ = resp.Body.Close()
			return nil, integration.ErrUnauthorized
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("wb: list status %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
		}

		var page response
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("wb: decode list: %w", err)
		}
		_ = resp.Body.Close()

		for _, card := range page.Cards {
			if card.NmID == 0 {
				continue
			}
			name := card.Title
			if name == "" {
				name = card.VendorCode
			}
			result = append(result, integration.SKU{
				ExternalSKU: strconv.FormatInt(card.NmID, 10),
				VendorCode:  card.VendorCode,
				Name:        name,
				Currency:    "RUB",
			})
		}

		if page.Cursor.Total < 100 {
			break
		}
		body = fmt.Sprintf(
			`{"settings":{"cursor":{"limit":100,"updatedAt":%q,"nmID":%d},"filter":{"withPhoto":-1}}}`,
			page.Cursor.UpdatedAt, page.Cursor.NmID,
		)
	}

	if len(result) > 0 {
		if err := c.fillPrices(ctx, result); err != nil {
			return nil, fmt.Errorf("wb: fill prices: %w", err)
		}
	}
	return result, nil
}

// fillPrices мутирует skus, проставляя CurrentPrice из /api/v2/list/goods/filter.
// Запросы пачками по 1000 nmID (лимит WB).
// При любой не-2xx ошибке (после ErrUnauthorized/ErrRateLimited/ErrUnexpectedStatus)
// возвращает ошибку — импорт прерывается целиком.
func (c *Client) fillPrices(ctx context.Context, skus []integration.SKU) error {
	type sizeObj struct {
		Price int `json:"price"` // RUB, целое
	}
	type good struct {
		NmID  int64     `json:"nmID"`
		Sizes []sizeObj `json:"sizes"`
	}
	type respShape struct {
		Data struct {
			ListGoods []good `json:"listGoods"`
		} `json:"data"`
	}

	bySKU := make(map[string]*integration.SKU, len(skus))
	nmIDs := make([]int64, 0, len(skus))
	for i := range skus {
		bySKU[skus[i].ExternalSKU] = &skus[i]
		n, err := strconv.ParseInt(skus[i].ExternalSKU, 10, 64)
		if err != nil {
			continue
		}
		nmIDs = append(nmIDs, n)
	}

	const chunkSize = 1000
	for start := 0; start < len(nmIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(nmIDs) {
			end = len(nmIDs)
		}
		payload, err := json.Marshal(map[string]any{"nmList": nmIDs[start:end]})
		if err != nil {
			return fmt.Errorf("wb: marshal goods/filter: %w", err)
		}
		payloadStr := string(payload)

		resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				c.pricesBase+"/api/v2/list/goods/filter",
				strings.NewReader(payloadStr))
			if err != nil {
				return nil, fmt.Errorf("wb: build goods/filter request: %w", err)
			}
			c.setAuth(req)
			req.Header.Set("Content-Type", "application/json")
			return req, nil
		})
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			_ = resp.Body.Close()
			return integration.ErrUnauthorized
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return fmt.Errorf("wb: goods/filter status %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
		}
		var page respShape
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			_ = resp.Body.Close()
			return fmt.Errorf("wb: decode goods/filter: %w", err)
		}
		_ = resp.Body.Close()

		for _, g := range page.Data.ListGoods {
			key := strconv.FormatInt(g.NmID, 10)
			if sku, ok := bySKU[key]; ok && len(g.Sizes) > 0 {
				sku.CurrentPrice = float64(g.Sizes[0].Price)
			}
		}
	}
	return nil
}

// UpdatePrices отправляет обновлённые цены через Discounts & Prices API v2.
// Тело запроса по спецификации: {"data":[{"nmID":<int>,"price":<int>,"discount":<int>}]}.
// nmID — целое число (не строка); discount — обязательное поле, 0 означает "без скидки".
// https://dev.wildberries.ru/openapi/work-with-products → POST /api/v2/upload/task
func (c *Client) UpdatePrices(ctx context.Context, updates []integration.PriceUpdate) error {
	type item struct {
		NmID     int64 `json:"nmID"`
		Price    int   `json:"price"`
		Discount int   `json:"discount"`
	}

	items := make([]item, 0, len(updates))
	for _, u := range updates {
		nm, err := strconv.ParseInt(u.ExternalSKU, 10, 64)
		if err != nil {
			return fmt.Errorf("wb: external sku %q is not a valid nmID: %w", u.ExternalSKU, err)
		}
		items = append(items, item{
			NmID:     nm,
			Price:    int(u.NewPrice),
			Discount: u.Discount,
		})
	}

	payload, err := json.Marshal(map[string]any{"data": items})
	if err != nil {
		return fmt.Errorf("wb: marshal prices: %w", err)
	}

	payloadStr := string(payload)
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.pricesBase+"/api/v2/upload/task",
			strings.NewReader(payloadStr))
		if err != nil {
			return nil, fmt.Errorf("wb: build update request: %w", err)
		}
		c.setAuth(req)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("wb: update request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wb: update status %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
	}
	return nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}
