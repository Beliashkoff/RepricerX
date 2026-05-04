// Package ozon реализует адаптер к Ozon Seller API.
package ozon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/pkg/ratelimit"
)

const baseURL = "https://api-seller.ozon.ru"

// Client — адаптер Ozon. Реализует integration.Marketplace.
type Client struct {
	shopID   string
	clientID string
	apiKey   string
	http     *http.Client
	limiter  *ratelimit.Registry
}

// NewClient создаёт клиент из JSON-сериализованных OzonCredentials.
// shopID используется для per-shop rate limiting.
func NewClient(shopID string, credsJSON []byte, limiter *ratelimit.Registry) (*Client, error) {
	var creds domain.OzonCredentials
	if err := json.Unmarshal(credsJSON, &creds); err != nil {
		return nil, fmt.Errorf("ozon: parse credentials: %w", err)
	}
	if creds.ClientID == "" || creds.APIKey == "" {
		return nil, errors.New("ozon: client_id and api_key are required")
	}
	return &Client{
		shopID:   shopID,
		clientID: creds.ClientID,
		apiKey:   creds.APIKey,
		http:     &http.Client{Timeout: 15 * time.Second},
		limiter:  limiter,
	}, nil
}

// doWithRetry выполняет HTTP-запрос с тремя попытками и экспоненциальным backoff.
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
		} else if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("ozon: server error %d", resp.StatusCode)
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

// TestAuth проверяет валидность ключей запросом к /v1/product/list с limit=1.
func (c *Client) TestAuth(ctx context.Context) error {
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			baseURL+"/v1/product/list",
			strings.NewReader(`{"filter":{},"last_id":"","limit":1}`))
		if err != nil {
			return nil, fmt.Errorf("ozon: build request: %w", err)
		}
		c.setAuth(req)
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("ozon: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return integration.ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ozon: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ListSKUs возвращает товары через /v2/product/list.
func (c *Client) ListSKUs(ctx context.Context) ([]integration.SKU, error) {
	type productItem struct {
		ProductID int64  `json:"product_id"`
		OfferID   string `json:"offer_id"`
	}
	type listResponse struct {
		Result struct {
			Items  []productItem `json:"items"`
			LastID string        `json:"last_id"`
			Total  int           `json:"total"`
		} `json:"result"`
	}
	type priceInfo struct {
		ProductID int64  `json:"product_id"`
		OfferID   string `json:"offer_id"`
		Price     string `json:"price"`
		Name      string `json:"name"`
	}
	type infoResponse struct {
		Result []priceInfo `json:"result"`
	}

	var (
		allItems []productItem
		lastID   = ""
	)

	// 1. Получаем список product_id постранично
	for {
		currentLastID := lastID
		resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
			reqBody := fmt.Sprintf(`{"filter":{},"last_id":%q,"limit":100}`, currentLastID)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				baseURL+"/v2/product/list",
				strings.NewReader(reqBody))
			if err != nil {
				return nil, fmt.Errorf("ozon: build list request: %w", err)
			}
			c.setAuth(req)
			return req, nil
		})
		if err != nil {
			return nil, fmt.Errorf("ozon: list request: %w", err)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			_ = resp.Body.Close()
			return nil, integration.ErrUnauthorized
		}
		var page listResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("ozon: decode list: %w", err)
		}
		_ = resp.Body.Close()

		allItems = append(allItems, page.Result.Items...)
		if len(page.Result.Items) < 100 {
			break
		}
		lastID = page.Result.LastID
	}

	if len(allItems) == 0 {
		return nil, nil
	}

	// 2. Получаем детали (цену, название) батчами по 100
	var skus []integration.SKU
	for i := 0; i < len(allItems); i += 100 {
		end := i + 100
		if end > len(allItems) {
			end = len(allItems)
		}
		batch := allItems[i:end]

		ids := make([]int64, 0, len(batch))
		for _, it := range batch {
			ids = append(ids, it.ProductID)
		}

		payloadBytes, _ := json.Marshal(map[string]any{"product_id": ids})
		payloadStr := string(payloadBytes)
		resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				baseURL+"/v2/product/info/list",
				strings.NewReader(payloadStr))
			if err != nil {
				return nil, fmt.Errorf("ozon: build info request: %w", err)
			}
			c.setAuth(req)
			return req, nil
		})
		if err != nil {
			return nil, fmt.Errorf("ozon: info request: %w", err)
		}
		var infoResp infoResponse
		if err := json.NewDecoder(resp.Body).Decode(&infoResp); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("ozon: decode info: %w", err)
		}
		_ = resp.Body.Close()

		for _, p := range infoResp.Result {
			price := 0.0
			if _, err := fmt.Sscanf(p.Price, "%f", &price); err != nil {
				return nil, fmt.Errorf("ozon: parse price %q for sku %s: %w", p.Price, p.OfferID, err)
			}
			skus = append(skus, integration.SKU{
				ExternalSKU:  p.OfferID,
				Name:         p.Name,
				CurrentPrice: price,
				Currency:     "RUB",
			})
		}
	}
	return skus, nil
}

// UpdatePrices отправляет обновлённые цены через /v1/product/import/prices.
func (c *Client) UpdatePrices(ctx context.Context, updates []integration.PriceUpdate) error {
	type priceItem struct {
		OfferID  string `json:"offer_id"`
		Price    string `json:"price"`
		OldPrice string `json:"old_price"`
		Vat      string `json:"vat"`
	}

	items := make([]priceItem, 0, len(updates))
	for _, u := range updates {
		priceStr := fmt.Sprintf("%.0f", u.NewPrice)
		items = append(items, priceItem{
			OfferID:  u.ExternalSKU,
			Price:    priceStr,
			OldPrice: priceStr,
			Vat:      "0",
		})
	}

	payload, err := json.Marshal(map[string]any{"prices": items})
	if err != nil {
		return fmt.Errorf("ozon: marshal prices: %w", err)
	}

	payloadStr := string(payload)
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			baseURL+"/v1/product/import/prices",
			strings.NewReader(payloadStr))
		if err != nil {
			return nil, fmt.Errorf("ozon: build update request: %w", err)
		}
		c.setAuth(req)
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("ozon: update request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ozon: update status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
}
