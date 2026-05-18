package wildberries

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Beliashkoff/RepricerX/internal/integration"
)

// newTestClient собирает Client с переданными базами; пустые остаются дефолтными.
func newTestClient(common, content, prices string) *Client {
	if common == "" {
		common = defaultCommonBase
	}
	if content == "" {
		content = defaultContentBase
	}
	if prices == "" {
		prices = defaultPricesBase
	}
	return &Client{
		apiKey:      "test-api-key",
		http:        &http.Client{},
		commonBase:  common,
		contentBase: content,
		pricesBase:  prices,
	}
}

// ---------------------------------------------------------------------------
// TestAuth — пингует discounts-prices-api/ping
// ---------------------------------------------------------------------------

func TestWB_TestAuth_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" {
			t.Errorf("неожиданный путь: %s, ожидали /ping", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("Authorization: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if err := newTestClient("", "", ts.URL).TestAuth(t.Context()); err != nil {
		t.Fatalf("ожидали nil, получили: %v", err)
	}
}

func TestWB_TestAuth_Unauthorized(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		code := code
		t.Run(fmt.Sprintf("HTTP_%d", code), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			}))
			defer ts.Close()

			err := newTestClient("", "", ts.URL).TestAuth(t.Context())
			if !errors.Is(err, integration.ErrUnauthorized) {
				t.Fatalf("HTTP %d: ожидали ErrUnauthorized, получили %v", code, err)
			}
		})
	}
}

func TestWB_TestAuth_NotFoundWrapsUnexpectedStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	err := newTestClient("", "", ts.URL).TestAuth(t.Context())
	if err == nil {
		t.Fatal("ожидали ошибку при 404, получили nil")
	}
	if !errors.Is(err, integration.ErrUnexpectedStatus) {
		t.Fatalf("ожидали обёртку integration.ErrUnexpectedStatus, получили %v", err)
	}
}

func TestWB_TestAuth_ServerErrorAfterRetriesWrapsUnexpectedStatus(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	err := newTestClient("", "", ts.URL).TestAuth(t.Context())
	if err == nil {
		t.Fatal("ожидали ошибку при 500, получили nil")
	}
	if !errors.Is(err, integration.ErrUnexpectedStatus) {
		t.Fatalf("ожидали обёртку integration.ErrUnexpectedStatus, получили %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListSKUs — карточки + цены через goods/filter
// ---------------------------------------------------------------------------

func TestWB_ListSKUs_CardsAndPricesMerged(t *testing.T) {
	contentTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/content/v2/get/cards/list" {
			t.Errorf("неожиданный путь content-api: %s", r.URL.Path)
		}
		resp := map[string]any{
			"cards": []map[string]any{
				{"nmID": 12345, "vendorCode": "SKU-001", "title": "Кроссовки"},
			},
			"cursor": map[string]any{"total": 1},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer contentTS.Close()

	pricesTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/list/goods/filter" {
			t.Errorf("неожиданный путь prices-api: %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		nmList, _ := body["nmList"].([]any)
		if len(nmList) != 1 {
			t.Errorf("nmList: ожидали 1 элемент, получили %d", len(nmList))
		}
		resp := map[string]any{
			"data": map[string]any{
				"listGoods": []map[string]any{
					{"nmID": 12345, "sizes": []map[string]any{{"price": 1250}}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer pricesTS.Close()
	skus, err := newTestClient("", contentTS.URL, pricesTS.URL).ListSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if len(skus) != 1 {
		t.Fatalf("ожидали 1 SKU, получили %d", len(skus))
	}
	if skus[0].ExternalSKU != "12345" {
		t.Errorf("ExternalSKU: %q, ожидали \"12345\"", skus[0].ExternalSKU)
	}
	if skus[0].VendorCode != "SKU-001" {
		t.Errorf("VendorCode: %q, ожидали \"SKU-001\"", skus[0].VendorCode)
	}
	if skus[0].CurrentPrice != 1250.0 {
		t.Errorf("CurrentPrice: %.2f, ожидали 1250.00", skus[0].CurrentPrice)
	}
	if skus[0].Currency != "RUB" {
		t.Errorf("Currency: %q", skus[0].Currency)
	}
}

func TestWB_ListSKUs_GoodsFilterEmpty_PriceIsZero(t *testing.T) {
	contentTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"cards":  []map[string]any{{"nmID": 99, "vendorCode": "V", "title": "X"}},
			"cursor": map[string]any{"total": 1},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer contentTS.Close()

	pricesTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"listGoods": []map[string]any{}}})
	}))
	defer pricesTS.Close()
	skus, err := newTestClient("", contentTS.URL, pricesTS.URL).ListSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if len(skus) != 1 {
		t.Fatalf("ожидали 1 SKU, получили %d", len(skus))
	}
	if skus[0].CurrentPrice != 0 {
		t.Errorf("CurrentPrice: %.2f, ожидали 0", skus[0].CurrentPrice)
	}
}

func TestWB_ListSKUs_GoodsFilter4xx_HardFail(t *testing.T) {
	contentTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"cards":  []map[string]any{{"nmID": 99, "vendorCode": "V", "title": "X"}},
			"cursor": map[string]any{"total": 1},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer contentTS.Close()

	pricesTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer pricesTS.Close()
	_, err := newTestClient("", contentTS.URL, pricesTS.URL).ListSKUs(t.Context())
	if err == nil {
		t.Fatal("ожидали ошибку при 400 от goods/filter, получили nil")
	}
	if !errors.Is(err, integration.ErrUnexpectedStatus) {
		t.Fatalf("ожидали обёртку ErrUnexpectedStatus, получили %v", err)
	}
}

func TestWB_ListSKUs_Pagination(t *testing.T) {
	contentCalls := 0
	contentTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentCalls++
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		settings, _ := reqBody["settings"].(map[string]any)
		cursor, _ := settings["cursor"].(map[string]any)

		var cards []map[string]any
		total := 0
		if contentCalls == 1 {
			if _, has := cursor["updatedAt"]; has {
				t.Error("первый запрос не должен содержать cursor.updatedAt")
			}
			for i := range 100 {
				cards = append(cards, map[string]any{
					"nmID":       1000 + i,
					"vendorCode": fmt.Sprintf("V-%d", i),
					"title":      fmt.Sprintf("T-%d", i),
				})
			}
			total = 100
		} else {
			if _, has := cursor["updatedAt"]; !has {
				t.Error("второй запрос должен содержать cursor.updatedAt")
			}
			for i := range 3 {
				cards = append(cards, map[string]any{
					"nmID":       2000 + i,
					"vendorCode": fmt.Sprintf("V-NEXT-%d", i),
					"title":      fmt.Sprintf("T-NEXT-%d", i),
				})
			}
			total = 3
		}
		resp := map[string]any{
			"cards":  cards,
			"cursor": map[string]any{"total": total, "updatedAt": "2024-01-01T00:00:00Z", "nmID": 999},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer contentTS.Close()

	pricesCalls := 0
	pricesTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pricesCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"listGoods": []map[string]any{}}})
	}))
	defer pricesTS.Close()
	skus, err := newTestClient("", contentTS.URL, pricesTS.URL).ListSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if contentCalls != 2 {
		t.Errorf("cards/list: ожидали 2 вызова, получили %d", contentCalls)
	}
	if pricesCalls != 1 {
		t.Errorf("goods/filter: ожидали 1 вызов, получили %d", pricesCalls)
	}
	if len(skus) != 103 {
		t.Errorf("ожидали 103 SKU, получили %d", len(skus))
	}
}

// ---------------------------------------------------------------------------
// UpdatePrices
// ---------------------------------------------------------------------------

func TestWB_UpdatePrices_PayloadShape(t *testing.T) {
	var captured map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/upload/task" {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	err := newTestClient("", "", ts.URL).UpdatePrices(t.Context(), []integration.PriceUpdate{
		{ExternalSKU: "12345", NewPrice: 1500},
		{ExternalSKU: "67890", NewPrice: 2750, Discount: 10},
	})
	if err != nil {
		t.Fatalf("UpdatePrices: %v", err)
	}

	data, _ := captured["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("payload.data: ожидали 2 элемента, получили %d", len(data))
	}

	first := data[0].(map[string]any)
	if nm, ok := first["nmID"].(float64); !ok || nm != 12345 {
		t.Errorf("nmID должен быть числом 12345, получили %v (%T)", first["nmID"], first["nmID"])
	}
	if price, ok := first["price"].(float64); !ok || price != 1500 {
		t.Errorf("price: %v", first["price"])
	}
	if disc, ok := first["discount"].(float64); !ok || disc != 0 {
		t.Errorf("discount должен быть 0, получили %v", first["discount"])
	}

	second := data[1].(map[string]any)
	if disc, ok := second["discount"].(float64); !ok || disc != 10 {
		t.Errorf("discount второго элемента должен быть 10, получили %v", second["discount"])
	}
}

func TestWB_UpdatePrices_InvalidNmID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("сервер не должен быть вызван при невалидном nmID")
	}))
	defer ts.Close()
	err := newTestClient("", "", ts.URL).UpdatePrices(t.Context(), []integration.PriceUpdate{
		{ExternalSKU: "not-a-number", NewPrice: 100},
	})
	if err == nil {
		t.Fatal("ожидали ошибку при невалидном nmID")
	}
	if !strings.Contains(err.Error(), "not-a-number") {
		t.Errorf("ожидали упоминание невалидного значения в ошибке, получили: %v", err)
	}
}

func TestWB_UpdatePrices_ServerErrorWrapsUnexpectedStatus(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	err := newTestClient("", "", ts.URL).UpdatePrices(t.Context(), []integration.PriceUpdate{
		{ExternalSKU: "12345", NewPrice: 100},
	})
	if err == nil {
		t.Fatal("ожидали ошибку при 500, получили nil")
	}
	if !errors.Is(err, integration.ErrUnexpectedStatus) {
		t.Fatalf("ожидали обёртку ErrUnexpectedStatus, получили %v", err)
	}
}
