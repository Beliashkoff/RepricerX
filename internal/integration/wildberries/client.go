// Package wildberries реализует адаптер к Wildberries Seller API.
package wildberries

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
)

const (
	commonBase  = "https://common-api.wildberries.ru"
	contentBase = "https://content-api.wildberries.ru"
	pricesBase  = "https://discounts-prices-api.wildberries.ru"
)

// Client — адаптер Wildberries. Реализует integration.Marketplace.
type Client struct {
	apiKey string
	http   *http.Client
}

// NewClient создаёт клиент из JSON-сериализованных WBCredentials.
func NewClient(credsJSON []byte) (*Client, error) {
	var creds domain.WBCredentials
	if err := json.Unmarshal(credsJSON, &creds); err != nil {
		return nil, fmt.Errorf("wb: parse credentials: %w", err)
	}
	if creds.APIKey == "" {
		return nil, errors.New("wb: api_key is required")
	}
	return &Client{
		apiKey: creds.APIKey,
		http:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// TestAuth проверяет валидность API-ключа запросом к /api/v1/info/seller.
func (c *Client) TestAuth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		commonBase+"/api/v1/info/seller", nil)
	if err != nil {
		return fmt.Errorf("wb: build request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wb: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return integration.ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wb: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ListSKUs возвращает товарные карточки магазина через Content API v2.
func (c *Client) ListSKUs(ctx context.Context) ([]integration.SKU, error) {
	type cardObject struct {
		VendorCode string `json:"vendorCode"`
		Title      string `json:"title"`
		Sizes      []struct {
			Price struct {
				Total int `json:"total"` // цена в копейках
			} `json:"price"`
		} `json:"sizes"`
	}
	type cursor struct {
		UpdatedAt string `json:"updatedAt"`
		NmID      int64  `json:"nmID"`
		Total     int    `json:"total"`
	}
	type response struct {
		Cards  []cardObject `json:"cards"`
		Cursor cursor       `json:"cursor"`
	}

	body := `{"settings":{"cursor":{"limit":100},"filter":{"withPhoto":-1}}}`
	var result []integration.SKU

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			contentBase+"/content/v2/get/cards/list",
			strings.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("wb: build list request: %w", err)
		}
		c.setAuth(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("wb: list request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("wb: list status %d", resp.StatusCode)
		}

		var page response
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("wb: decode list: %w", err)
		}
		_ = resp.Body.Close()

		for _, card := range page.Cards {
			price := 0.0
			if len(card.Sizes) > 0 {
				price = float64(card.Sizes[0].Price.Total) / 100
			}
			result = append(result, integration.SKU{
				ExternalSKU:  card.VendorCode,
				Name:         card.Title,
				CurrentPrice: price,
				Currency:     "RUB",
			})
		}

		if page.Cursor.Total < 100 {
			break
		}
		// следующая страница — передаём курсор
		body = fmt.Sprintf(
			`{"settings":{"cursor":{"limit":100,"updatedAt":%q,"nmID":%d},"filter":{"withPhoto":-1}}}`,
			page.Cursor.UpdatedAt, page.Cursor.NmID,
		)
	}
	return result, nil
}

// UpdatePrices отправляет обновлённые цены через Discounts & Prices API v2.
func (c *Client) UpdatePrices(ctx context.Context, updates []integration.PriceUpdate) error {
	type item struct {
		NmID  string `json:"nmID"`
		Price int    `json:"price"` // в рублях, целое
	}

	items := make([]item, 0, len(updates))
	for _, u := range updates {
		items = append(items, item{
			NmID:  u.ExternalSKU,
			Price: int(u.NewPrice),
		})
	}

	payload, err := json.Marshal(map[string]any{"data": items})
	if err != nil {
		return fmt.Errorf("wb: marshal prices: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		pricesBase+"/api/v2/upload/task",
		strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("wb: build update request: %w", err)
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wb: update request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wb: update status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}
