//go:build integration

package integration

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPricingSimulate_UsesRealBackend(t *testing.T) {
	truncate(t)
	client := loginUser(t, "pricing@example.com", "ValidPass123!")
	shopID := createTestShop(t, client, "pricing")
	product := createTestProduct(t, client, shopID, "PR-SIM-1", "Pricing Product")

	strategyResp := doJSON(t, client, http.MethodPost, "/api/strategies", map[string]any{
		"name":           "Fixed",
		"type":           "fixed",
		"params":         map[string]any{"value": 1200},
		"constraints":    map[string]any{"max_change_pct": 10},
		"fallbackPolicy": "keep_current",
		"enabled":        true,
	}, withOrigin())
	mustStatus(t, strategyResp, http.StatusCreated)
	var strategy struct {
		ID string `json:"id"`
	}
	mustDecode(t, strategyResp, &strategy)

	resp := doJSON(t, client, http.MethodPost, "/api/pricing/simulate", map[string]any{
		"product_id":  product["id"],
		"strategy_id": strategy.ID,
	}, withOrigin())
	mustStatus(t, resp, http.StatusOK)
	var body struct {
		TargetPrice   float64 `json:"target_price"`
		FinalPrice    float64 `json:"final_price"`
		ConstraintHit *string `json:"constraint_hit"`
	}
	mustDecode(t, resp, &body)
	if body.TargetPrice != 1200 {
		t.Fatalf("target_price = %.2f, want 1200", body.TargetPrice)
	}
	if body.FinalPrice != 110.55 {
		t.Fatalf("final_price = %.2f, want max_change clamp to 110.55", body.FinalPrice)
	}
	if body.ConstraintHit == nil || *body.ConstraintHit != "max_change_pct" {
		t.Fatalf("constraint_hit = %v, want max_change_pct", body.ConstraintHit)
	}
}

// auditListResponse — DTO ответа /api/audit/price-changes (соответствует transport.priceChangeListResponse).
type auditListResponse struct {
	Items []struct {
		ID          string `json:"id"`
		ShopID      string `json:"shop_id"`
		ProductName string `json:"product_name"`
		OldPrice    float64
		NewPrice    float64
		Status      string `json:"status"`
		Reason      string `json:"reason"`
	} `json:"items"`
	Pagination struct {
		Page    int `json:"page"`
		PerPage int `json:"perPage"`
		Total   int `json:"total"`
	} `json:"pagination"`
}

func insertChange(t *testing.T, shopID, productID, strategyID uuid.UUID, status string, oldPrice, newPrice float64, createdAt time.Time) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO price_change_log
			(shop_id, product_id, strategy_id, old_price, new_price, target_price, reason, status, correlation_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $5, $6, $7::plan_item_status, $8, $9)`,
		shopID, productID, strategyID, oldPrice, newPrice, "test reason", status, uuid.New(), createdAt,
	)
	if err != nil {
		t.Fatalf("insert price_change_log: %v", err)
	}
}

func TestAuditEndpoints_UsePriceChangeLog(t *testing.T) {
	truncate(t)
	client := loginUser(t, "audit@example.com", "ValidPass123!")
	userID := getUserID(t, "audit@example.com")
	shopAID := createTestShop(t, client, "audit-a")
	shopBID := createTestShop(t, client, "audit-b")
	productA := createTestProduct(t, client, shopAID, "AUD-A", "Audit Product A")
	productB := createTestProduct(t, client, shopBID, "AUD-B", "Audit Product B")

	strategyID := createStrategy(t, userID)

	now := time.Now().UTC()
	week := now.Add(-7 * 24 * time.Hour)

	insertChange(t, uuid.MustParse(shopAID), uuid.MustParse(productA["id"].(string)), strategyID, "applied", 100, 90, now)
	insertChange(t, uuid.MustParse(shopAID), uuid.MustParse(productA["id"].(string)), strategyID, "failed", 100, 110, now.Add(-time.Minute))
	insertChange(t, uuid.MustParse(shopBID), uuid.MustParse(productB["id"].(string)), strategyID, "skipped", 100, 95, week)

	// 1) Базовый список (без фильтров) — все 3 записи в pagination wrapper.
	resp := doJSON(t, client, http.MethodGet, "/api/audit/price-changes", nil)
	mustStatus(t, resp, http.StatusOK)
	var page auditListResponse
	mustDecode(t, resp, &page)
	if len(page.Items) != 3 || page.Pagination.Total != 3 {
		t.Fatalf("expected 3 items+total, got items=%d total=%d", len(page.Items), page.Pagination.Total)
	}
	statuses := map[string]int{}
	for _, it := range page.Items {
		statuses[it.Status]++
	}
	if statuses["success"] != 1 || statuses["failed"] != 1 || statuses["skipped"] != 1 {
		t.Fatalf("expected 1/1/1 success/failed/skipped, got %v", statuses)
	}

	// 2) Фильтр по магазину A — должен отсечь магазин B.
	resp = doJSON(t, client, http.MethodGet, "/api/audit/price-changes?shop_id="+shopAID, nil)
	mustStatus(t, resp, http.StatusOK)
	mustDecode(t, resp, &page)
	if page.Pagination.Total != 2 {
		t.Fatalf("filter shop_id: total=%d, want 2", page.Pagination.Total)
	}
	for _, it := range page.Items {
		if it.ShopID != shopAID {
			t.Fatalf("filter shop_id leaked another shop: %s", it.ShopID)
		}
	}

	// 3) Фильтр по статусу.
	resp = doJSON(t, client, http.MethodGet, "/api/audit/price-changes?status=failed", nil)
	mustStatus(t, resp, http.StatusOK)
	mustDecode(t, resp, &page)
	if page.Pagination.Total != 1 || page.Items[0].Status != "failed" {
		t.Fatalf("filter status=failed: %#v", page)
	}

	// 4) Диапазон дат — окно вокруг now исключает «неделю назад».
	from := now.Add(-2 * time.Hour).Format(time.RFC3339)
	to := now.Add(time.Hour).Format(time.RFC3339)
	q := url.Values{"from": {from}, "to": {to}}.Encode()
	resp = doJSON(t, client, http.MethodGet, "/api/audit/price-changes?"+q, nil)
	mustStatus(t, resp, http.StatusOK)
	mustDecode(t, resp, &page)
	if page.Pagination.Total != 2 {
		t.Fatalf("date range filter: total=%d, want 2", page.Pagination.Total)
	}

	// 5) Поиск по external_sku — должен найти только товар A.
	resp = doJSON(t, client, http.MethodGet, "/api/audit/price-changes?external_sku=AUD-A", nil)
	mustStatus(t, resp, http.StatusOK)
	mustDecode(t, resp, &page)
	if page.Pagination.Total != 2 {
		t.Fatalf("filter external_sku: total=%d, want 2", page.Pagination.Total)
	}
	for _, it := range page.Items {
		if it.ProductName != "Audit Product A" {
			t.Fatalf("filter external_sku leaked: %s", it.ProductName)
		}
	}

	// 6) Пагинация per_page=1, page=2 — ровно 1 элемент, total остаётся 3.
	resp = doJSON(t, client, http.MethodGet, "/api/audit/price-changes?per_page=1&page=2", nil)
	mustStatus(t, resp, http.StatusOK)
	mustDecode(t, resp, &page)
	if len(page.Items) != 1 || page.Pagination.Total != 3 || page.Pagination.Page != 2 || page.Pagination.PerPage != 1 {
		t.Fatalf("pagination: %#v", page)
	}

	// 7) /api/reports/summary за умолчательный период — все 3 записи попадают (последние 30 дней).
	summaryResp := doJSON(t, client, http.MethodGet, "/api/reports/summary", nil)
	mustStatus(t, summaryResp, http.StatusOK)
	var summary struct {
		TotalUpdates      int `json:"total_updates"`
		SuccessfulUpdates int `json:"successful_updates"`
		FailedUpdates     int `json:"failed_updates"`
	}
	mustDecode(t, summaryResp, &summary)
	if summary.TotalUpdates != 3 || summary.SuccessfulUpdates != 1 || summary.FailedUpdates != 1 {
		t.Fatalf("summary: %#v", summary)
	}

	// 8) /api/reports/summary с явным окном вокруг now — только 2 записи.
	summaryResp = doJSON(t, client, http.MethodGet, "/api/reports/summary?"+q, nil)
	mustStatus(t, summaryResp, http.StatusOK)
	mustDecode(t, summaryResp, &summary)
	if summary.TotalUpdates != 2 {
		t.Fatalf("summary with date range: total=%d, want 2", summary.TotalUpdates)
	}

	// 9) CSV-экспорт по новому пути — 200, верный заголовок и парсимый CSV.
	exportResp := doJSON(t, client, http.MethodGet, "/api/audit/price-changes.csv", nil)
	mustStatus(t, exportResp, http.StatusOK)
	if cd := exportResp.Header.Get("Content-Disposition"); !strings.Contains(cd, "price-changes.csv") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	defer exportResp.Body.Close()
	rows, err := csv.NewReader(exportResp.Body).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	// header + 3 строки.
	if len(rows) != 4 {
		t.Fatalf("csv rows = %d, want 4", len(rows))
	}
	if rows[0][0] != "id" {
		t.Fatalf("csv header[0] = %q, want id", rows[0][0])
	}

	// 10) CSV с фильтром только по status=failed — header + 1 строка.
	exportResp = doJSON(t, client, http.MethodGet, "/api/audit/price-changes.csv?status=failed", nil)
	mustStatus(t, exportResp, http.StatusOK)
	defer exportResp.Body.Close()
	rows, err = csv.NewReader(exportResp.Body).ReadAll()
	if err != nil {
		t.Fatalf("parse filtered csv: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("csv rows with status=failed = %d, want 2", len(rows))
	}

	// 11) Старые роуты должны исчезнуть (404).
	for _, oldPath := range []string{"/api/audit/export", "/api/audit/summary"} {
		old := doJSON(t, client, http.MethodGet, oldPath, nil)
		if old.StatusCode != http.StatusNotFound {
			t.Fatalf("legacy route %s: status=%d, want 404", oldPath, old.StatusCode)
		}
	}

	// 12) Невалидный статус → 400.
	bad := doJSON(t, client, http.MethodGet, "/api/audit/price-changes?status=bogus", nil)
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad status code = %d, want 400", bad.StatusCode)
	}
}

// TestAuditList_Performance1000Rows — smoke-проверка acceptance-критерия ТЗ §8:
// отклик /api/audit/price-changes ≤ 4 с на 1000 записей.
func TestAuditList_Performance1000Rows(t *testing.T) {
	truncate(t)
	client := loginUser(t, "audit-perf@example.com", "ValidPass123!")
	userID := getUserID(t, "audit-perf@example.com")
	shopID := createTestShop(t, client, "audit-perf")
	product := createTestProduct(t, client, shopID, "PERF-1", "Perf Product")
	strategyID := createStrategy(t, userID)

	// Bulk-вставка 1000 записей одной командой через UNNEST — быстрее, чем 1000 отдельных INSERT.
	const n = 1000
	ids := make([]uuid.UUID, n)
	corrIDs := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		ids[i] = uuid.New()
		corrIDs[i] = uuid.New()
	}
	shopUUID := uuid.MustParse(shopID)
	productUUID := uuid.MustParse(product["id"].(string))
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO price_change_log
			(id, shop_id, product_id, strategy_id, old_price, new_price, target_price, reason, status, correlation_id)
		SELECT u.id, $1, $2, $3, 100, 95, 95, 'perf', 'applied'::plan_item_status, u.cid
		FROM unnest($4::uuid[], $5::uuid[]) AS u(id, cid)`,
		shopUUID, productUUID, strategyID, ids, corrIDs,
	)
	if err != nil {
		t.Fatalf("bulk insert: %v", err)
	}

	start := time.Now()
	resp := doJSON(t, client, http.MethodGet, "/api/audit/price-changes?per_page=200", nil)
	elapsed := time.Since(start)
	mustStatus(t, resp, http.StatusOK)

	if elapsed > 4*time.Second {
		t.Fatalf("list took %v, want < 4s (ТЗ §8)", elapsed)
	}

	var page auditListResponse
	mustDecode(t, resp, &page)
	if page.Pagination.Total != n || len(page.Items) != 200 {
		t.Fatalf("perf list: total=%d items=%d, want total=%d items=200", page.Pagination.Total, len(page.Items), n)
	}
}
