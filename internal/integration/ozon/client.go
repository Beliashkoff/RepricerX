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

type ozonStockEntry struct {
	Type     string `json:"type"`
	Present  int    `json:"present"`
	Reserved int    `json:"reserved"`
}

type ozonStockItem struct {
	ProductID    int64            `json:"product_id"`
	Stocks       []ozonStockEntry `json:"stocks"`
	FboPresent   int              `json:"fbo_present"`
	FboReserved  int              `json:"fbo_reserved"`
	FbsPresent   int              `json:"fbs_present"`
	FbsReserved  int              `json:"fbs_reserved"`
	RfbsPresent  int              `json:"rfbs_present"`
	RfbsReserved int              `json:"rfbs_reserved"`
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
		} else if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			return nil, integration.ErrRateLimited
		} else if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("ozon: server error %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
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

// TestAuth проверяет валидность ключей запросом к /v1/seller/info.
func (c *Client) TestAuth(ctx context.Context) error {
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			baseURL+"/v1/seller/info",
			strings.NewReader(`{}`))
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
		return fmt.Errorf("ozon: seller/info status %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
	}
	return nil
}

// ListSKUs возвращает товары через /v3/product/list + /v3/product/info/list.
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
		Items []priceInfo `json:"items"`
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
				baseURL+"/v3/product/list",
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
	productIDs := make([]int64, 0, len(allItems))
	for _, item := range allItems {
		productIDs = append(productIDs, item.ProductID)
	}
	stockByProductID, err := c.fetchStocks(ctx, productIDs)
	if err != nil {
		return nil, err
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
				baseURL+"/v3/product/info/list",
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

		for _, p := range infoResp.Items {
			price := 0.0
			if _, err := fmt.Sscanf(p.Price, "%f", &price); err != nil {
				return nil, fmt.Errorf("ozon: parse price %q for sku %s: %w", p.Price, p.OfferID, err)
			}
			skus = append(skus, integration.SKU{
				ExternalSKU:  p.OfferID,
				Name:         p.Name,
				CurrentPrice: price,
				Currency:     "RUB",
				StockCount:   stockByProductID[p.ProductID],
			})
		}
	}
	return skus, nil
}

func (c *Client) fetchStocks(ctx context.Context, productIDs []int64) (map[int64]int, error) {
	type stockResponse struct {
		Result struct {
			Items []ozonStockItem `json:"items"`
		} `json:"result"`
		Items []ozonStockItem `json:"items"`
	}

	stockByProductID := make(map[int64]int, len(productIDs))
	for i := 0; i < len(productIDs); i += 100 {
		end := i + 100
		if end > len(productIDs) {
			end = len(productIDs)
		}
		ids := productIDs[i:end]
		payloadBytes, _ := json.Marshal(map[string]any{
			"filter": map[string]any{"product_id": ids, "visibility": "ALL"},
			"limit":  100,
		})
		payloadStr := string(payloadBytes)
		resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				baseURL+"/v4/product/info/stocks",
				strings.NewReader(payloadStr))
			if err != nil {
				return nil, fmt.Errorf("ozon: build stocks request: %w", err)
			}
			c.setAuth(req)
			return req, nil
		})
		if err != nil {
			return nil, fmt.Errorf("ozon: stocks request: %w", err)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			_ = resp.Body.Close()
			return nil, integration.ErrUnauthorized
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("ozon: stocks status %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
		}
		var stocks stockResponse
		if err := json.NewDecoder(resp.Body).Decode(&stocks); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("ozon: decode stocks: %w", err)
		}
		_ = resp.Body.Close()
		pageItems := stocks.Result.Items
		if len(pageItems) == 0 {
			pageItems = stocks.Items
		}
		for _, item := range pageItems {
			stockByProductID[item.ProductID] = totalAvailableStock(item)
		}
	}
	return stockByProductID, nil
}

func totalAvailableStock(item ozonStockItem) int {
	total := 0
	for _, stock := range item.Stocks {
		total += maxInt(stock.Present-stock.Reserved, 0)
	}
	if total > 0 || len(item.Stocks) > 0 {
		return total
	}
	total += maxInt(item.FboPresent-item.FboReserved, 0)
	total += maxInt(item.FbsPresent-item.FbsReserved, 0)
	total += maxInt(item.RfbsPresent-item.RfbsReserved, 0)
	return total
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("ozon: update status %d: %w", resp.StatusCode, integration.ErrUnexpectedStatus)
	}

	type updateResultItem struct {
		OfferID string `json:"offer_id"`
		Updated bool   `json:"updated"`
		Errors  []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	type updateResponse struct {
		Result []updateResultItem `json:"result"`
	}

	var res updateResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("ozon: decode update response: %w", err)
	}

	var failed []string
	for _, item := range res.Result {
		if !item.Updated {
			msg := item.OfferID
			if len(item.Errors) > 0 {
				msg += ": " + item.Errors[0].Message
			}
			failed = append(failed, msg)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("ozon: %d price(s) not updated: %s", len(failed), strings.Join(failed, "; "))
	}
	return nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
}
