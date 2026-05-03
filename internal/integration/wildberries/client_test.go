package wildberries

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Beliashkoff/RepricerX/internal/integration"
)

// redirectTransport перенаправляет все запросы на тестовый сервер,
// сохраняя путь и тело оригинального запроса.
type redirectTransport struct {
	base string // "http://127.0.0.1:PORT"
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(t.base)
	clone := req.Clone(req.Context())
	clone.URL.Scheme = u.Scheme
	clone.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(clone)
}

// newTestClient создаёт WB-клиент, перенаправляя все запросы на serverURL.
func newTestClient(serverURL string) *Client {
	return &Client{
		apiKey: "test-api-key",
		http: &http.Client{
			Transport: &redirectTransport{base: serverURL},
		},
	}
}

// ---------------------------------------------------------------------------
// TestAuth
// ---------------------------------------------------------------------------

func TestWB_TestAuth_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/info/seller" {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("Authorization: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := newTestClient(ts.URL).TestAuth(t.Context())
	if err != nil {
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

			err := newTestClient(ts.URL).TestAuth(t.Context())
			if err != integration.ErrUnauthorized {
				t.Fatalf("HTTP %d: ожидали ErrUnauthorized, получили %v", code, err)
			}
		})
	}
}

func TestWB_TestAuth_UnexpectedStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	err := newTestClient(ts.URL).TestAuth(t.Context())
	if err == nil {
		t.Fatal("ожидали ошибку при 500, получили nil")
	}
}

// ---------------------------------------------------------------------------
// ListSKUs — конвертация копейки → рубли и базовый сценарий
// ---------------------------------------------------------------------------

func TestWB_ListSKUs_PriceConversionKopeksToRubles(t *testing.T) {
	// API возвращает total в копейках; клиент должен делить на 100.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"cards": []map[string]any{
				{
					"vendorCode": "SKU-001",
					"title":      "Кроссовки",
					"sizes": []map[string]any{
						{"price": map[string]any{"total": 125000}}, // 1250.00 ₽
					},
				},
			},
			"cursor": map[string]any{"total": 1}, // < 100 → последняя страница
		}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer ts.Close()

	skus, err := newTestClient(ts.URL).ListSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if len(skus) != 1 {
		t.Fatalf("ожидали 1 SKU, получили %d", len(skus))
	}
	if skus[0].ExternalSKU != "SKU-001" {
		t.Errorf("ExternalSKU: %q", skus[0].ExternalSKU)
	}
	if skus[0].CurrentPrice != 1250.0 {
		t.Errorf("CurrentPrice: %.2f, ожидали 1250.00 (125000 копеек ÷ 100)", skus[0].CurrentPrice)
	}
	if skus[0].Currency != "RUB" {
		t.Errorf("Currency: %q", skus[0].Currency)
	}
}

func TestWB_ListSKUs_NoSizes_PriceIsZero(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"cards": []map[string]any{
				{"vendorCode": "SKU-NOSIZE", "title": "Без размера", "sizes": []any{}},
			},
			"cursor": map[string]any{"total": 1},
		}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer ts.Close()

	skus, err := newTestClient(ts.URL).ListSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if len(skus) != 1 {
		t.Fatalf("ожидали 1 SKU, получили %d", len(skus))
	}
	if skus[0].CurrentPrice != 0.0 {
		t.Errorf("цена без размера должна быть 0, получили %.2f", skus[0].CurrentPrice)
	}
}

// ---------------------------------------------------------------------------
// ListSKUs — постраничная загрузка с курсором
// ---------------------------------------------------------------------------

func TestWB_ListSKUs_Pagination(t *testing.T) {
	callCount := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody) //nolint:errcheck

		settings, _ := reqBody["settings"].(map[string]any)
		cursor, _ := settings["cursor"].(map[string]any)

		// Первый запрос: курсора нет — возвращаем 100 карточек.
		// Второй запрос: с курсором — возвращаем 3 карточки (конец).
		var cards []map[string]any
		total := 0

		if callCount == 1 {
			// Убеждаемся что первый запрос без курсора
			if _, hasCursor := cursor["updatedAt"]; hasCursor {
				t.Error("первый запрос не должен содержать курсор")
			}
			for i := range 100 {
				cards = append(cards, map[string]any{
					"vendorCode": fmt.Sprintf("SKU-%03d", i),
					"title":      fmt.Sprintf("Товар %d", i),
					"sizes":      []map[string]any{{"price": map[string]any{"total": 10000}}},
				})
			}
			total = 100
		} else {
			// Второй запрос должен содержать курсор
			if _, hasCursor := cursor["updatedAt"]; !hasCursor {
				t.Error("второй запрос должен содержать cursor.updatedAt")
			}
			for i := range 3 {
				cards = append(cards, map[string]any{
					"vendorCode": fmt.Sprintf("SKU-NEXT-%d", i),
					"title":      fmt.Sprintf("Следующий %d", i),
					"sizes":      []map[string]any{{"price": map[string]any{"total": 5000}}},
				})
			}
			total = 3
		}

		resp := map[string]any{
			"cards":  cards,
			"cursor": map[string]any{"total": total, "updatedAt": "2024-01-01T00:00:00Z", "nmID": 12345},
		}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer ts.Close()

	skus, err := newTestClient(ts.URL).ListSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if callCount != 2 {
		t.Errorf("ожидали 2 HTTP-запроса, сделано %d", callCount)
	}
	if len(skus) != 103 {
		t.Errorf("ожидали 103 SKU (100 + 3), получили %d", len(skus))
	}
}

// ---------------------------------------------------------------------------
// UpdatePrices
// ---------------------------------------------------------------------------

func TestWB_UpdatePrices_Success(t *testing.T) {
	var captured map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/upload/task" {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&captured) //nolint:errcheck
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := newTestClient(ts.URL).UpdatePrices(t.Context(), []integration.PriceUpdate{
		{ExternalSKU: "SKU-001", NewPrice: 1500},
		{ExternalSKU: "SKU-002", NewPrice: 2750},
	})
	if err != nil {
		t.Fatalf("UpdatePrices: %v", err)
	}

	data, _ := captured["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("payload.data: ожидали 2 элемента, получили %d", len(data))
	}

	first := data[0].(map[string]any)
	if first["nmID"] != "SKU-001" {
		t.Errorf("nmID: %v", first["nmID"])
	}
	// Price в рублях, целое
	if price, ok := first["price"].(float64); !ok || price != 1500 {
		t.Errorf("price: %v, ожидали 1500", first["price"])
	}
}

func TestWB_UpdatePrices_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	err := newTestClient(ts.URL).UpdatePrices(t.Context(), []integration.PriceUpdate{
		{ExternalSKU: "X", NewPrice: 100},
	})
	if err == nil {
		t.Fatal("ожидали ошибку при 500, получили nil")
	}
}
