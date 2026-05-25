package competitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

// SearchResult — результат поиска конкурента.
type SearchResult struct {
	Marketplace string  `json:"marketplace"`
	ExternalID  string  `json:"external_id"`
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	URL         string  `json:"url"`
}

// SearchMaxLimit — максимальное кол-во результатов на запрос.
const SearchMaxLimit = 20

// Search — ищет товары-конкуренты по ключевому слову на указанном маркетплейсе.
func (s *Service) Search(ctx context.Context, marketplace, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > SearchMaxLimit {
		limit = SearchMaxLimit
	}
	switch marketplace {
	case domain.MarketplaceOzon:
		return searchOzon(ctx, query, limit)
	case domain.MarketplaceWB:
		return searchWB(ctx, query, limit)
	default:
		return nil, fmt.Errorf("unsupported marketplace: %s", marketplace)
	}
}

// ─── Ozon search ──────────────────────────────────────────────────────────────

// searchOzon — ищет товары на Ozon через неофициальный поисковый API.
func searchOzon(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// Используем публичный entrypoint Ozon
	searchURL := "https://www.ozon.ru/api/entrypoint-api.bx/page/json/v2?url=" +
		url.QueryEscape("/search/?text="+query+"&layout_page_index=1")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ozon search: create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ozon search: request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("ozon search: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("ozon search: read: %w", err)
	}

	return parseOzonSearchResponse(body, limit)
}

func parseOzonSearchResponse(body []byte, limit int) ([]SearchResult, error) {
	// Ozon entrypoint API возвращает {widgetStates: {"searchResultsV2.{n}": "json-string"}}
	var raw struct {
		WidgetStates map[string]json.RawMessage `json:"widgetStates"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("ozon search: parse page: %w", err)
	}

	// Ищем ключ, содержащий "searchResultsV2" или "cms-search-result-v2"
	for key, val := range raw.WidgetStates {
		if !strings.Contains(key, "searchResultsV2") && !strings.Contains(key, "cms-search-result-v2") {
			continue
		}
		// val — это JSON-строка, содержащая ещё один уровень JSON
		var inner string
		if err := json.Unmarshal(val, &inner); err != nil {
			continue
		}
		results := extractOzonItems([]byte(inner), limit)
		if len(results) > 0 {
			return results, nil
		}
	}

	return []SearchResult{}, nil
}

type ozonSearchWidget struct {
	Items []struct {
		ID    any    `json:"id"`   // может быть string или number
		Title string `json:"title"`
		Price struct {
			Price float64 `json:"price"`
		} `json:"price"`
		Action struct {
			Link string `json:"link"`
		} `json:"action"`
	} `json:"items"`
}

func extractOzonItems(data []byte, limit int) []SearchResult {
	var widget ozonSearchWidget
	if err := json.Unmarshal(data, &widget); err != nil {
		return nil
	}

	results := make([]SearchResult, 0, limit)
	for _, item := range widget.Items {
		if len(results) >= limit {
			break
		}
		// Извлекаем ID из ссылки
		link := item.Action.Link
		if link == "" {
			continue
		}
		nmID := lastID(link)
		if nmID == "" {
			continue
		}
		price := item.Price.Price
		if price <= 0 {
			continue
		}
		results = append(results, SearchResult{
			Marketplace: domain.MarketplaceOzon,
			ExternalID:  nmID,
			Name:        item.Title,
			Price:       price,
			URL:         "https://www.ozon.ru" + link,
		})
	}
	return results
}

// ─── WB search ───────────────────────────────────────────────────────────────

// searchWB — ищет товары на Wildberries через публичный search API.
func searchWB(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	searchURL := "https://search.wb.ru/exactmatch/ru/male/v4/search?" +
		"resultset=catalog&limit=" + fmt.Sprint(limit) +
		"&curr=rub&dest=-1257786&query=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("wb search: create request: %w", err)
	}
	req.Header.Set("User-Agent", "RepricerX/1.0 competitor-search")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wb search: request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("wb search: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("wb search: read: %w", err)
	}

	return parseWBSearchResponse(body, limit)
}

type wbSearchResponse struct {
	Data struct {
		Products []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			SalePriceU int    `json:"salePriceU"`
			PriceU     int    `json:"priceU"`
		} `json:"products"`
	} `json:"data"`
}

func parseWBSearchResponse(body []byte, limit int) ([]SearchResult, error) {
	var resp wbSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("wb search: parse: %w", err)
	}

	results := make([]SearchResult, 0, limit)
	for _, p := range resp.Data.Products {
		if len(results) >= limit {
			break
		}
		priceU := p.SalePriceU
		if priceU <= 0 {
			priceU = p.PriceU
		}
		if priceU <= 0 || p.ID == 0 {
			continue
		}
		nmID := fmt.Sprint(p.ID)
		results = append(results, SearchResult{
			Marketplace: domain.MarketplaceWB,
			ExternalID:  nmID,
			Name:        p.Name,
			Price:       float64(priceU) / 100.0,
			URL:         "https://www.wildberries.ru/catalog/" + nmID + "/detail.aspx",
		})
	}
	return results, nil
}
