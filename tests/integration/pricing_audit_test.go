//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

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

func TestAuditEndpoints_UsePriceChangeLog(t *testing.T) {
	truncate(t)
	client := loginUser(t, "audit@example.com", "ValidPass123!")
	userID := getUserID(t, "audit@example.com")
	shopID := createTestShop(t, client, "audit")
	product := createTestProduct(t, client, shopID, "AUD-1", "Audit Product")

	strategyID := createStrategy(t, userID)
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO price_change_log
			(shop_id, product_id, strategy_id, old_price, new_price, target_price, reason, status, correlation_id)
		VALUES ($1, $2, $3, 100, 90, 90, 'test', 'applied', $4)`,
		shopID, product["id"], strategyID, uuid.New(),
	)
	if err != nil {
		t.Fatalf("insert price_change_log: %v", err)
	}

	resp := doJSON(t, client, http.MethodGet, "/api/audit/price-changes", nil)
	mustStatus(t, resp, http.StatusOK)
	var changes []struct {
		ProductName string `json:"product_name"`
		Status      string `json:"status"`
	}
	mustDecode(t, resp, &changes)
	if len(changes) != 1 {
		t.Fatalf("want 1 price change, got %d", len(changes))
	}
	if changes[0].ProductName != "Audit Product" || changes[0].Status != "success" {
		t.Fatalf("unexpected audit change: %#v", changes[0])
	}

	summaryResp := doJSON(t, client, http.MethodGet, "/api/audit/summary", nil)
	mustStatus(t, summaryResp, http.StatusOK)
	var summary struct {
		TotalUpdates      int `json:"total_updates"`
		SuccessfulUpdates int `json:"successful_updates"`
	}
	mustDecode(t, summaryResp, &summary)
	if summary.TotalUpdates != 1 || summary.SuccessfulUpdates != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	exportResp := doJSON(t, client, http.MethodGet, "/api/audit/export", nil)
	mustStatus(t, exportResp, http.StatusOK)
	if ct := exportResp.Header.Get("Content-Type"); ct == "" {
		t.Fatal("export response must set content type")
	}
}
