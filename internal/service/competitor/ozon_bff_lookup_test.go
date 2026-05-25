package competitor

import (
	"encoding/json"
	"math"
	"testing"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// buildBFFBody строит синтетический BFF-ответ с заданными widgetStates.
func buildBFFBody(widgetStates map[string]any) []byte {
	type bffResp struct {
		WidgetStates map[string]any `json:"widgetStates"`
	}
	b, _ := json.Marshal(bffResp{WidgetStates: widgetStates})
	return b
}

// doubleEncode кодирует объект как JSON, затем ещё раз как JSON-строку
// (двойное кодирование, которое использует Ozon BFF для значений widgetStates).
func doubleEncode(v any) any {
	inner, _ := json.Marshal(v)
	return string(inner)
}

// ─── parseBFFProductResponse ─────────────────────────────────────────────────

func TestParseBFFResponse_PriceInWebPriceWidget(t *testing.T) {
	body := buildBFFBody(map[string]any{
		"webPrice-123456-default-1": doubleEncode(map[string]any{
			"price": 1490.0,
		}),
	})
	got, err := parseBFFProductResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price == nil || *got.Price != 1490.0 {
		t.Errorf("want price=1490.0, got %v", got.Price)
	}
	if got.Source != "public_ozon_bff" {
		t.Errorf("want source=public_ozon_bff, got %q", got.Source)
	}
}

func TestParseBFFResponse_PriceAsStringRuble(t *testing.T) {
	// Ozon часто возвращает цену в виде строки "1 490 ₽"
	body := buildBFFBody(map[string]any{
		"webPrice-999-default-1": doubleEncode(map[string]any{
			"price": "1 490 ₽",
		}),
	})
	got, err := parseBFFProductResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price == nil || *got.Price != 1490.0 {
		t.Errorf("want price=1490.0, got %v", got.Price)
	}
}

func TestParseBFFResponse_PriceInFinalPriceField(t *testing.T) {
	body := buildBFFBody(map[string]any{
		"webPrice-777": doubleEncode(map[string]any{
			"finalPrice": 2500.0,
		}),
	})
	got, err := parseBFFProductResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price == nil || *got.Price != 2500.0 {
		t.Errorf("want price=2500.0, got %v", got.Price)
	}
}

func TestParseBFFResponse_PriceInWebSingleProductWidget(t *testing.T) {
	body := buildBFFBody(map[string]any{
		"webSingleProduct-42": doubleEncode(map[string]any{
			"id": 42,
			"price": map[string]any{
				"price": 3200.0,
			},
		}),
	})
	got, err := parseBFFProductResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price == nil || *got.Price != 3200.0 {
		t.Errorf("want price=3200.0, got %v", got.Price)
	}
}

func TestParseBFFResponse_PriceInMoneyValue(t *testing.T) {
	body := buildBFFBody(map[string]any{
		"webPrice-456": doubleEncode(map[string]any{
			"money": map[string]any{
				"value": 999.0,
			},
		}),
	})
	got, err := parseBFFProductResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price == nil || *got.Price != 999.0 {
		t.Errorf("want price=999.0, got %v", got.Price)
	}
}

func TestParseBFFResponse_SkipsOriginalPriceFindsCardPrice(t *testing.T) {
	// originalPrice — это цена без скидки; должна игнорироваться,
	// нам нужна cardPrice (итоговая цена).
	body := buildBFFBody(map[string]any{
		"webPrice-101": doubleEncode(map[string]any{
			"originalPrice": 5000.0, // должна быть проигнорирована
			"cardPrice":     3500.0, // должна быть выбрана
		}),
	})
	got, err := parseBFFProductResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price == nil || *got.Price != 3500.0 {
		t.Errorf("want cardPrice=3500.0, got %v", got.Price)
	}
}

func TestParseBFFResponse_FallbackToAnyWidget(t *testing.T) {
	// Нет webPrice — но в каком-то другом виджете есть цена
	body := buildBFFBody(map[string]any{
		"someOtherWidget-1": doubleEncode(map[string]any{
			"price": 750.0,
		}),
	})
	got, err := parseBFFProductResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price == nil || *got.Price != 750.0 {
		t.Errorf("want price=750.0, got %v", got.Price)
	}
}

func TestParseBFFResponse_EmptyWidgetStates(t *testing.T) {
	body := buildBFFBody(map[string]any{})
	_, err := parseBFFProductResponse(body)
	if err == nil {
		t.Fatal("expected error for empty widgetStates")
	}
}

func TestParseBFFResponse_NoPriceInAnyWidget(t *testing.T) {
	body := buildBFFBody(map[string]any{
		"webProductHeading-1": doubleEncode(map[string]any{
			"title": "Товар без цены",
			"id":    12345,
		}),
	})
	_, err := parseBFFProductResponse(body)
	if err == nil {
		t.Fatal("expected error when no price found")
	}
}

func TestParseBFFResponse_PriceZeroSkipped(t *testing.T) {
	body := buildBFFBody(map[string]any{
		"webPrice-1": doubleEncode(map[string]any{
			"price": 0.0, // нулевая цена должна быть проигнорирована
		}),
	})
	_, err := parseBFFProductResponse(body)
	if err == nil {
		t.Fatal("expected error when price=0")
	}
}

func TestParseBFFResponse_InvalidJSON(t *testing.T) {
	_, err := parseBFFProductResponse([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ─── parsePriceStringBFF ─────────────────────────────────────────────────────

func TestParsePriceStringBFF(t *testing.T) {
	cases := []struct {
		input   string
		want    float64
		wantOK  bool
	}{
		{"1 490 ₽", 1490.0, true},
		{"1 490 ₽", 1490.0, true}, // неразрывные пробелы
		{"1490.00", 1490.0, true},
		{"1490", 1490.0, true},
		{"1,490.00", 1490.0, true},
		{"99", 99.0, true},
		{"0", 0, false},
		{"0.00", 0, false},
		{"", 0, false},
		{"₽", 0, false},
		{"abc", 0, false},
	}
	for _, tc := range cases {
		got, ok := parsePriceStringBFF(tc.input)
		if ok != tc.wantOK {
			t.Errorf("parsePriceStringBFF(%q): ok=%v, want %v", tc.input, ok, tc.wantOK)
			continue
		}
		if tc.wantOK && math.Abs(got-tc.want) > 0.001 {
			t.Errorf("parsePriceStringBFF(%q)=%.2f, want %.2f", tc.input, got, tc.want)
		}
	}
}

// ─── extractPriceFromWidget ───────────────────────────────────────────────────

func TestExtractPriceFromWidget_DirectObject(t *testing.T) {
	// Значение виджета — уже объект (не строка)
	obj, _ := json.Marshal(map[string]any{"price": 500.0})
	val := json.RawMessage(obj)
	price := extractPriceFromWidget(val)
	if price == nil || *price != 500.0 {
		t.Errorf("want 500.0, got %v", price)
	}
}

func TestExtractPriceFromWidget_DoubleEncoded(t *testing.T) {
	// Стандартный формат Ozon BFF: строка с JSON внутри
	inner, _ := json.Marshal(map[string]any{"price": 1200.0})
	outer, _ := json.Marshal(string(inner))
	val := json.RawMessage(outer)
	price := extractPriceFromWidget(val)
	if price == nil || *price != 1200.0 {
		t.Errorf("want 1200.0, got %v", price)
	}
}

func TestExtractPriceFromWidget_NoPrice(t *testing.T) {
	obj, _ := json.Marshal(map[string]any{"title": "no price here"})
	val := json.RawMessage(obj)
	if price := extractPriceFromWidget(val); price != nil {
		t.Errorf("want nil, got %v", *price)
	}
}
