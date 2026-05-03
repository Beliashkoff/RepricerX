package ozon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Beliashkoff/RepricerX/internal/integration"
)

type redirectTransport struct {
	base string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(t.base)
	clone := req.Clone(req.Context())
	clone.URL.Scheme = u.Scheme
	clone.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func newTestClient(serverURL string) *Client {
	return &Client{
		clientID: "test-client-id",
		apiKey:   "test-api-key",
		http: &http.Client{
			Transport: &redirectTransport{base: serverURL},
		},
	}
}

func checkOzonAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Client-Id") != "test-client-id" {
		t.Errorf("Client-Id: %q", r.Header.Get("Client-Id"))
	}
	if r.Header.Get("Api-Key") != "test-api-key" {
		t.Errorf("Api-Key: %q", r.Header.Get("Api-Key"))
	}
}

// ---------------------------------------------------------------------------
// TestAuth
// ---------------------------------------------------------------------------

func TestOzon_TestAuth_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/product/list" {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		checkOzonAuth(t, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := newTestClient(ts.URL).TestAuth(t.Context())
	if err != nil {
		t.Fatalf("ожидали nil, получили: %v", err)
	}
}

func TestOzon_TestAuth_Unauthorized(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprintf("HTTP_%d", code), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			}))
			defer ts.Close()

			err := newTestClient(ts.URL).TestAuth(t.Context())
			if err != integration.ErrUnauthorized {
				t.Fatalf("HTTP %d: ожидали ErrUnauthorized, получили %v", code, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ListSKUs — двухфазный запрос
// ---------------------------------------------------------------------------

func TestOzon_ListSKUs_TwoPhase(t *testing.T) {
	listCalled := 0
	infoCalled := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/product/list":
			listCalled++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"result": map[string]any{
					"items": []map[string]any{
						{"product_id": 11, "offer_id": "SKU-A"},
						{"product_id": 22, "offer_id": "SKU-B"},
					},
					"last_id": "",
					"total":   2,
				},
			})

		case "/v2/product/info/list":
			infoCalled++
			// Проверяем, что клиент передал product_id из первой фазы.
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			ids, _ := body["product_id"].([]any)
			if len(ids) != 2 {
				t.Errorf("info/list: ожидали 2 product_id, получили %d", len(ids))
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"result": []map[string]any{
					{"product_id": 11, "offer_id": "SKU-A", "name": "Товар А", "price": "1299.00"},
					{"product_id": 22, "offer_id": "SKU-B", "name": "Товар Б", "price": "599.50"},
				},
			})

		default:
			t.Errorf("неожиданный путь: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	skus, err := newTestClient(ts.URL).ListSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if listCalled != 1 {
		t.Errorf("ожидали 1 запрос к /v2/product/list, сделано %d", listCalled)
	}
	if infoCalled != 1 {
		t.Errorf("ожидали 1 запрос к /v2/product/info/list, сделано %d", infoCalled)
	}
	if len(skus) != 2 {
		t.Fatalf("ожидали 2 SKU, получили %d", len(skus))
	}
}

// ---------------------------------------------------------------------------
// ListSKUs — парсинг цены из строки
// ---------------------------------------------------------------------------

func TestOzon_ListSKUs_PriceFromString(t *testing.T) {
	cases := []struct {
		raw  string
		want float64
	}{
		{"1299.00", 1299.0},
		{"599", 599.0},
		{"0.5", 0.5},
		{"100000.99", 100000.99},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v2/product/list":
					json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
						"result": map[string]any{
							"items": []map[string]any{{"product_id": 1, "offer_id": "SKU-1"}},
							"total": 1,
						},
					})
				case "/v2/product/info/list":
					json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
						"result": []map[string]any{
							{"product_id": 1, "offer_id": "SKU-1", "name": "Товар", "price": tc.raw},
						},
					})
				}
			}))
			defer ts.Close()

			skus, err := newTestClient(ts.URL).ListSKUs(t.Context())
			if err != nil {
				t.Fatalf("ListSKUs(%q): %v", tc.raw, err)
			}
			if len(skus) != 1 {
				t.Fatalf("ожидали 1 SKU, получили %d", len(skus))
			}
			if skus[0].CurrentPrice != tc.want {
				t.Errorf("price %q: получили %.4f, ожидали %.4f", tc.raw, skus[0].CurrentPrice, tc.want)
			}
		})
	}
}

func TestOzon_ListSKUs_InvalidPrice_ReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/product/list":
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"result": map[string]any{
					"items": []map[string]any{{"product_id": 1, "offer_id": "BAD-1"}},
					"total": 1,
				},
			})
		case "/v2/product/info/list":
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"result": []map[string]any{
					{"product_id": 1, "offer_id": "BAD-1", "name": "X", "price": "not-a-number"},
				},
			})
		}
	}))
	defer ts.Close()

	_, err := newTestClient(ts.URL).ListSKUs(t.Context())
	if err == nil {
		t.Fatal("ожидали ошибку при нечисловой цене, получили nil")
	}
	if !strings.Contains(err.Error(), "parse price") {
		t.Errorf("ошибка должна упоминать 'parse price': %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListSKUs — постраничная загрузка
// ---------------------------------------------------------------------------

func TestOzon_ListSKUs_Pagination(t *testing.T) {
	listCallCount := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/product/list":
			listCallCount++
			var reqBody map[string]any
			json.NewDecoder(r.Body).Decode(&reqBody) //nolint:errcheck

			if listCallCount == 1 {
				// Первая страница — 100 товаров, есть last_id
				items := make([]map[string]any, 100)
				for i := range items {
					items[i] = map[string]any{"product_id": i + 1, "offer_id": fmt.Sprintf("SKU-%d", i+1)}
				}
				json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
					"result": map[string]any{"items": items, "last_id": "cursor_abc", "total": 100},
				})
			} else {
				// Вторая страница — 5 товаров, last_id пустой
				lastID, _ := reqBody["last_id"].(string)
				if lastID != "cursor_abc" {
					t.Errorf("вторая страница: last_id=%q, ожидали cursor_abc", lastID)
				}
				items := make([]map[string]any, 5)
				for i := range items {
					items[i] = map[string]any{"product_id": 200 + i, "offer_id": fmt.Sprintf("SKU-NEXT-%d", i)}
				}
				json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
					"result": map[string]any{"items": items, "last_id": "", "total": 5},
				})
			}

		case "/v2/product/info/list":
			// Возвращаем детали для всех переданных ID
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			ids, _ := body["product_id"].([]any)
			result := make([]map[string]any, len(ids))
			for i, id := range ids {
				result[i] = map[string]any{
					"product_id": id, "offer_id": fmt.Sprintf("SKU-%v", id),
					"name": "Товар", "price": "100.00",
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"result": result}) //nolint:errcheck
		}
	}))
	defer ts.Close()

	skus, err := newTestClient(ts.URL).ListSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if listCallCount != 2 {
		t.Errorf("ожидали 2 запроса к /v2/product/list, сделано %d", listCallCount)
	}
	if len(skus) != 105 {
		t.Errorf("ожидали 105 SKU (100+5), получили %d", len(skus))
	}
}

// ---------------------------------------------------------------------------
// UpdatePrices
// ---------------------------------------------------------------------------

func TestOzon_UpdatePrices_Success(t *testing.T) {
	var captured map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/product/import/prices" {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		checkOzonAuth(t, r)
		json.NewDecoder(r.Body).Decode(&captured) //nolint:errcheck
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := newTestClient(ts.URL).UpdatePrices(t.Context(), []integration.PriceUpdate{
		{ExternalSKU: "OFFER-1", NewPrice: 1299},
		{ExternalSKU: "OFFER-2", NewPrice: 599},
	})
	if err != nil {
		t.Fatalf("UpdatePrices: %v", err)
	}

	prices, _ := captured["prices"].([]any)
	if len(prices) != 2 {
		t.Fatalf("payload.prices: ожидали 2, получили %d", len(prices))
	}

	first := prices[0].(map[string]any)
	if first["offer_id"] != "OFFER-1" {
		t.Errorf("offer_id: %v", first["offer_id"])
	}
	// Ozon принимает цену как строку
	if first["price"] != "1299" {
		t.Errorf("price: %v, ожидали \"1299\"", first["price"])
	}
	if first["vat"] != "0" {
		t.Errorf("vat: %v, ожидали \"0\"", first["vat"])
	}
}

func TestOzon_UpdatePrices_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	err := newTestClient(ts.URL).UpdatePrices(t.Context(), []integration.PriceUpdate{
		{ExternalSKU: "X", NewPrice: 100},
	})
	if err == nil {
		t.Fatal("ожидали ошибку при 400, получили nil")
	}
}
