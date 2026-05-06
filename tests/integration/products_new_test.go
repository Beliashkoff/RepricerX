//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"

	mp "github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/google/uuid"
)

// TestProductSoftDelete — DELETE /api/products/:id переводит товар в archived.
func TestProductSoftDelete(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "soft_delete")
	shopID := createTestShop(t, client, "soft_delete")
	product := createTestProduct(t, client, shopID, "DEL-001", "To be deleted")

	productID := product["id"].(string)

	// soft-delete
	resp := doJSON(t, client, http.MethodDelete, "/api/products/"+productID, nil, withOrigin())
	mustStatus(t, resp, http.StatusNoContent)

	// список с фильтром archived должен содержать удалённый товар
	listArchived := doJSON(t, client, http.MethodGet, "/api/products?status=archived", nil)
	mustStatus(t, listArchived, http.StatusOK)
	var archived struct {
		Items []map[string]any `json:"items"`
	}
	mustDecode(t, listArchived, &archived)
	found := false
	for _, item := range archived.Items {
		if item["id"] == productID {
			found = true
			if item["status"] != "archived" {
				t.Fatalf("want status archived, got %v", item["status"])
			}
		}
	}
	if !found {
		t.Fatalf("archived product %q not found in archived list", productID)
	}

	// список активных товаров не должен содержать удалённый
	listActive := doJSON(t, client, http.MethodGet, "/api/products?status=active", nil)
	mustStatus(t, listActive, http.StatusOK)
	var active struct {
		Items []map[string]any `json:"items"`
	}
	mustDecode(t, listActive, &active)
	for _, item := range active.Items {
		if item["id"] == productID {
			t.Fatalf("soft-deleted product appeared in active list")
		}
	}

	// повторный delete — 404 (продукт принадлежит пользователю, но уже archived; repo вернёт ErrNotFound)
	resp2 := doJSON(t, client, http.MethodDelete, "/api/products/"+productID, nil, withOrigin())
	mustStatus(t, resp2, http.StatusNotFound)

	// другой пользователь не может удалить
	other := loginAsNewUser(t, "soft_delete_other")
	resp3 := doJSON(t, other, http.MethodDelete, "/api/products/"+productID, nil, withOrigin())
	mustStatus(t, resp3, http.StatusNotFound)
}

// TestProductBulkPatch — POST /api/products/bulk-patch обновляет цены нескольких товаров.
func TestProductBulkPatch(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "bulk_patch")
	shopID := createTestShop(t, client, "bulk_patch")
	p1 := createTestProduct(t, client, shopID, "BULK-001", "Product 1")
	p2 := createTestProduct(t, client, shopID, "BULK-002", "Product 2")
	p3 := createTestProduct(t, client, shopID, "BULK-003", "Product 3")

	resp := doJSON(t, client, http.MethodPost, "/api/products/bulk-patch", map[string]any{
		"products": []map[string]any{
			{"id": p1["id"], "minPrice": 10.0, "maxPrice": 200.0},
			{"id": p2["id"], "minPrice": 20.0},
			{"id": p3["id"], "costPrice": 55.0},
		},
	}, withOrigin())
	mustStatus(t, resp, http.StatusOK)

	var body map[string]any
	mustDecode(t, resp, &body)
	if body["updated"].(float64) != 3 {
		t.Fatalf("want updated=3, got %v", body["updated"])
	}

	// проверяем обновление через GET списка с поиском
	list := doJSON(t, client, http.MethodGet, "/api/products?q=BULK-001&shopId="+shopID, nil)
	mustStatus(t, list, http.StatusOK)
	var listBody struct {
		Items []map[string]any `json:"items"`
	}
	mustDecode(t, list, &listBody)
	if len(listBody.Items) != 1 {
		t.Fatalf("want 1 product, got %d", len(listBody.Items))
	}
	if listBody.Items[0]["minPrice"].(float64) != 10.0 {
		t.Fatalf("want minPrice=10, got %v", listBody.Items[0]["minPrice"])
	}
	if listBody.Items[0]["maxPrice"].(float64) != 200.0 {
		t.Fatalf("want maxPrice=200, got %v", listBody.Items[0]["maxPrice"])
	}
}

// TestProductBulkPatchValidation — bulk-patch с невалидными данными возвращает 4xx.
func TestProductBulkPatchValidation(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "bulk_patch_val")

	// пустой список
	resp := doJSON(t, client, http.MethodPost, "/api/products/bulk-patch", map[string]any{
		"products": []any{},
	}, withOrigin())
	mustStatus(t, resp, http.StatusBadRequest)

	// нет поля products вообще
	resp2 := doJSON(t, client, http.MethodPost, "/api/products/bulk-patch", map[string]any{}, withOrigin())
	mustStatus(t, resp2, http.StatusBadRequest)

	// невалидный UUID
	resp3 := doJSON(t, client, http.MethodPost, "/api/products/bulk-patch", map[string]any{
		"products": []map[string]any{
			{"id": "not-a-uuid", "minPrice": 10.0},
		},
	}, withOrigin())
	mustStatus(t, resp3, http.StatusBadRequest)
}

func TestProductBulkPatchInvalidBoundsIsAtomic(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "bulk_patch_atomic")
	shopID := createTestShop(t, client, "bulk_patch_atomic")
	p1 := createTestProduct(t, client, shopID, "BULK-ATOMIC-001", "Product 1")
	p2 := createTestProduct(t, client, shopID, "BULK-ATOMIC-002", "Product 2")

	resp := doJSON(t, client, http.MethodPost, "/api/products/bulk-patch", map[string]any{
		"products": []map[string]any{
			{"id": p1["id"], "minPrice": 80.0},
			{"id": p2["id"], "minPrice": 200.0},
		},
	}, withOrigin())
	mustStatus(t, resp, http.StatusBadRequest)
	mustErrorCode(t, resp, "invalid_price")

	list := doJSON(t, client, http.MethodGet, "/api/products?q=BULK-ATOMIC-001&shopId="+shopID, nil)
	mustStatus(t, list, http.StatusOK)
	var listBody struct {
		Items []map[string]any `json:"items"`
	}
	mustDecode(t, list, &listBody)
	if len(listBody.Items) != 1 {
		t.Fatalf("want 1 product, got %d", len(listBody.Items))
	}
	if listBody.Items[0]["minPrice"].(float64) != 90.0 {
		t.Fatalf("bulk-patch must be atomic: want minPrice=90, got %v", listBody.Items[0]["minPrice"])
	}
}

// TestProductExportCSV — GET /api/products/export возвращает CSV с заголовком.
func TestProductExportCSV(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "export_csv")
	shopID := createTestShop(t, client, "export_csv")
	createTestProduct(t, client, shopID, "EXP-001", "Export Product 1")
	createTestProduct(t, client, shopID, "EXP-002", "Export Product 2")

	resp := doJSON(t, client, http.MethodGet, "/api/products/export?shopId="+shopID, nil)
	mustStatus(t, resp, http.StatusOK)

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Fatalf("want Content-Type text/csv, got %q", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Fatalf("want Content-Disposition attachment, got %q", cd)
	}

	var buf strings.Builder
	body := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(body)
		if n > 0 {
			buf.Write(body[:n])
		}
		if err != nil {
			break
		}
	}
	csv := buf.String()

	if !strings.HasPrefix(csv, "id,") {
		t.Fatalf("CSV must start with header row, got: %q", csv[:min(len(csv), 60)])
	}
	if !strings.Contains(csv, "EXP-001") {
		t.Fatalf("CSV must contain EXP-001, got: %s", csv)
	}
	if !strings.Contains(csv, "EXP-002") {
		t.Fatalf("CSV must contain EXP-002, got: %s", csv)
	}

	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("want 3 lines (header + 2 products), got %d", len(lines))
	}
}

// TestImportCancel — DELETE /api/imports/:id отменяет pending-импорт.
func TestImportCancel(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "cancel_import")
	shopID := createTestShop(t, client, "cancel_import")

	testSKUs = []mp.SKU{
		{ExternalSKU: "CANCEL-SKU", Name: "Cancel Me", CurrentPrice: 500, Currency: "RUB"},
	}

	// запускаем импорт
	start := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products/import", nil, withOrigin())
	mustStatus(t, start, http.StatusAccepted)
	var importResp map[string]any
	mustDecode(t, start, &importResp)
	importID := importResp["importId"].(string)

	// отменяем пока в статусе pending (воркер ещё не запускали)
	cancel := doJSON(t, client, http.MethodDelete, "/api/imports/"+importID, nil, withOrigin())
	mustStatus(t, cancel, http.StatusNoContent)

	// статус должен стать canceled
	waitImportStatus(t, importID, "canceled")

	poll := doJSON(t, client, http.MethodGet, "/api/imports/"+importID, nil)
	mustStatus(t, poll, http.StatusOK)
	var pollBody map[string]any
	mustDecode(t, poll, &pollBody)
	if pollBody["status"] != "canceled" {
		t.Fatalf("want status=canceled, got %v", pollBody["status"])
	}

	// повторная отмена — 409 (уже завершён)
	cancel2 := doJSON(t, client, http.MethodDelete, "/api/imports/"+importID, nil, withOrigin())
	mustStatus(t, cancel2, http.StatusConflict)
	mustErrorCode(t, cancel2, "import_not_cancelable")

	// чужой пользователь не может отменить
	other := loginAsNewUser(t, "cancel_import_other")
	otherShopID := createTestShop(t, other, "cancel_import_other")
	testSKUs = []mp.SKU{{ExternalSKU: "OTHER-SKU", Name: "Other", CurrentPrice: 100, Currency: "RUB"}}
	otherStart := doJSON(t, other, http.MethodPost, "/api/shops/"+otherShopID+"/products/import", nil, withOrigin())
	mustStatus(t, otherStart, http.StatusAccepted)
	var otherImport map[string]any
	mustDecode(t, otherStart, &otherImport)
	otherImportID := otherImport["importId"].(string)

	wrongCancel := doJSON(t, client, http.MethodDelete, "/api/imports/"+otherImportID, nil, withOrigin())
	mustStatus(t, wrongCancel, http.StatusConflict) // ErrNotFound → ErrImportNotCancelable → 409
}

// TestImportCancel_RunningImportNotCancelable — попытка отмены running-импорта возвращает 409.
// Воркер уже вызвал MarkRunning; отмена не должна прерывать выполнение.
func TestImportCancel_RunningImportNotCancelable(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "cancel_running")
	shopID := createTestShop(t, client, "cancel_running")

	testSKUs = []mp.SKU{
		{ExternalSKU: "RUN-SKU", Name: "Running", CurrentPrice: 100, Currency: "RUB"},
	}

	start := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products/import", nil, withOrigin())
	mustStatus(t, start, http.StatusAccepted)
	var importResp map[string]any
	mustDecode(t, start, &importResp)
	importID := importResp["importId"].(string)

	// Имитируем, что воркер перехватил джоб и перевёл импорт в running.
	_, err := testPool.Exec(context.Background(),
		`UPDATE import_log SET status = 'running' WHERE id = $1`,
		uuid.MustParse(importID),
	)
	if err != nil {
		t.Fatalf("force running: %v", err)
	}

	// Попытка отмены running-импорта должна вернуть 409.
	cancel := doJSON(t, client, http.MethodDelete, "/api/imports/"+importID, nil, withOrigin())
	mustStatus(t, cancel, http.StatusConflict)
	mustErrorCode(t, cancel, "import_not_cancelable")

	// Статус не изменился — всё ещё running.
	waitImportStatus(t, importID, "running")
}

// TestImportErrorsDrillDown — GET /api/imports/:id/errors возвращает постраничный список ошибок.
func TestImportErrorsDrillDown(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "import_errors")
	shopID := createTestShop(t, client, "import_errors")

	// импорт с валидным и невалидным SKU
	testSKUs = []mp.SKU{
		{ExternalSKU: "GOOD-SKU", Name: "Good Product", CurrentPrice: 100, Currency: "RUB"},
		{ExternalSKU: "", Name: "Invalid – empty SKU", CurrentPrice: 10, Currency: "RUB"},
		{ExternalSKU: "GOOD-SKU", Name: "Duplicate SKU", CurrentPrice: 200, Currency: "RUB"},
	}

	start := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products/import", nil, withOrigin())
	mustStatus(t, start, http.StatusAccepted)
	var importResp map[string]any
	mustDecode(t, start, &importResp)
	importID := importResp["importId"].(string)

	runNextImportJob(t)
	waitImportStatus(t, importID, "partial")

	// GET /imports/:id/errors
	errResp := doJSON(t, client, http.MethodGet, "/api/imports/"+importID+"/errors?page=1&perPage=10", nil)
	mustStatus(t, errResp, http.StatusOK)

	var errBody struct {
		Items []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"items"`
		Total   int `json:"total"`
		Page    int `json:"page"`
		PerPage int `json:"perPage"`
	}
	mustDecode(t, errResp, &errBody)

	if errBody.Total < 1 {
		t.Fatalf("want at least 1 error, got total=%d", errBody.Total)
	}
	if errBody.Page != 1 {
		t.Fatalf("want page=1, got %d", errBody.Page)
	}
	if errBody.PerPage != 10 {
		t.Fatalf("want perPage=10, got %d", errBody.PerPage)
	}
	if len(errBody.Items) != errBody.Total {
		t.Fatalf("items length %d != total %d", len(errBody.Items), errBody.Total)
	}

	// чужой пользователь не видит ошибки
	other := loginAsNewUser(t, "import_errors_other")
	wrongResp := doJSON(t, other, http.MethodGet, "/api/imports/"+importID+"/errors", nil)
	mustStatus(t, wrongResp, http.StatusNotFound)
}

// TestProductSortAndPriceFilter — GET /api/products с sortBy и priceFrom/priceTo.
func TestProductSortAndPriceFilter(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "sort_filter")
	shopID := createTestShop(t, client, "sort_filter")

	// создаём товары с разными ценами
	p1 := createTestProduct(t, client, shopID, "SORT-001", "Alfa")
	p2 := createTestProduct(t, client, shopID, "SORT-002", "Beta")
	_ = p1
	_ = p2

	// обновляем цены напрямую через patch
	doJSON(t, client, http.MethodPatch, "/api/products/"+p1["id"].(string), map[string]any{
		"minPrice": 10.0,
	}, withOrigin())
	doJSON(t, client, http.MethodPatch, "/api/products/"+p2["id"].(string), map[string]any{
		"minPrice": 500.0,
	}, withOrigin())

	// сортировка по имени asc
	listByName := doJSON(t, client, http.MethodGet, "/api/products?shopId="+shopID+"&sortBy=name&sortDir=asc", nil)
	mustStatus(t, listByName, http.StatusOK)
	var sortedBody struct {
		Items []map[string]any `json:"items"`
	}
	mustDecode(t, listByName, &sortedBody)
	if len(sortedBody.Items) >= 2 {
		name0 := sortedBody.Items[0]["name"].(string)
		name1 := sortedBody.Items[1]["name"].(string)
		if name0 > name1 {
			t.Fatalf("sort asc failed: %q should come before %q", name0, name1)
		}
	}

	// фильтр по currentPrice через priceFrom/priceTo (по imported current_price=100.5)
	listAll := doJSON(t, client, http.MethodGet, "/api/products?shopId="+shopID+"&priceFrom=50&priceTo=200", nil)
	mustStatus(t, listAll, http.StatusOK)
	var filteredBody struct {
		Items []map[string]any `json:"items"`
	}
	mustDecode(t, listAll, &filteredBody)
	for _, item := range filteredBody.Items {
		price := item["currentPrice"].(float64)
		if price < 50 || price > 200 {
			t.Fatalf("price filter failed: price %v out of range [50, 200]", price)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
