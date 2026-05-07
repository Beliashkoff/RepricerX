//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestCompetitorsAPI_CRUDRefreshAndPricingSimulation(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "competitors")
	shopID := createTestShop(t, client, "competitors")
	product := createTestProduct(t, client, shopID, "COMP-1", "Competitor Product")
	productID := product["id"].(string)

	createResp := doJSON(t, client, http.MethodPost, "/api/products/"+productID+"/competitors", map[string]any{
		"target": "https://www.ozon.ru/product/test-product-123456789/?utm_source=x",
	}, withOrigin())
	mustStatus(t, createResp, http.StatusCreated)
	var competitor map[string]any
	mustDecode(t, createResp, &competitor)
	if competitor["ozonPublicProductId"] != "123456789" {
		t.Fatalf("want normalized Ozon id, got %v", competitor["ozonPublicProductId"])
	}

	dupResp := doJSON(t, client, http.MethodPost, "/api/products/"+productID+"/competitors", map[string]any{
		"target": "123456789",
	}, withOrigin())
	mustStatus(t, dupResp, http.StatusConflict)
	mustErrorCode(t, dupResp, "duplicate_competitor")

	price := 80.0
	testCompetitorPrice = &price
	refreshResp := doJSON(t, client, http.MethodPost, "/api/competitors/"+competitor["id"].(string)+"/refresh", nil, withOrigin())
	mustStatus(t, refreshResp, http.StatusOK)
	var refreshed map[string]any
	mustDecode(t, refreshResp, &refreshed)
	if refreshed["lastPrice"] != 80.0 || refreshed["lastStatus"] != "ok" {
		t.Fatalf("unexpected refresh result: %#v", refreshed)
	}

	listResp := doJSON(t, client, http.MethodGet, "/api/products/"+productID+"/competitors", nil)
	mustStatus(t, listResp, http.StatusOK)
	var list []map[string]any
	mustDecode(t, listResp, &list)
	if len(list) != 1 || list[0]["lastPrice"] != 80.0 {
		t.Fatalf("unexpected competitors list: %#v", list)
	}

	strategyResp := doJSON(t, client, http.MethodPost, "/api/strategies", map[string]any{
		"name":           "Competitor + step",
		"type":           "min_competitor_plus_step",
		"params":         map[string]any{"step": 2},
		"fallbackPolicy": "keep_current",
		"enabled":        true,
	}, withOrigin())
	mustStatus(t, strategyResp, http.StatusCreated)
	var strategy struct {
		ID string `json:"id"`
	}
	mustDecode(t, strategyResp, &strategy)

	simResp := doJSON(t, client, http.MethodPost, "/api/pricing/simulate", map[string]any{
		"product_id":  productID,
		"strategy_id": strategy.ID,
	}, withOrigin())
	mustStatus(t, simResp, http.StatusOK)
	var sim map[string]any
	mustDecode(t, simResp, &sim)
	if sim["target_price"] != 82.0 || sim["competitor_source"] != "auto" {
		t.Fatalf("auto competitor price not used: %#v", sim)
	}

	overrideResp := doJSON(t, client, http.MethodPost, "/api/pricing/simulate", map[string]any{
		"product_id":       productID,
		"strategy_id":      strategy.ID,
		"competitor_price": 70,
	}, withOrigin())
	mustStatus(t, overrideResp, http.StatusOK)
	var override map[string]any
	mustDecode(t, overrideResp, &override)
	if override["target_price"] != 72.0 || override["competitor_source"] != "manual" {
		t.Fatalf("manual competitor override not used: %#v", override)
	}
}

func TestCompetitorsAPI_TenantIsolation(t *testing.T) {
	truncate(t)
	clientA := loginAsNewUser(t, "competitors_a")
	shopA := createTestShop(t, clientA, "competitors_a")
	productA := createTestProduct(t, clientA, shopA, "COMP-A", "A Product")
	productAID := productA["id"].(string)

	createResp := doJSON(t, clientA, http.MethodPost, "/api/products/"+productAID+"/competitors", map[string]any{
		"target": "123456789",
	}, withOrigin())
	mustStatus(t, createResp, http.StatusCreated)
	var competitor map[string]any
	mustDecode(t, createResp, &competitor)

	clientB := loginAsNewUser(t, "competitors_b")
	createOtherResp := doJSON(t, clientB, http.MethodPost, "/api/products/"+productAID+"/competitors", map[string]any{
		"target": "987654321",
	}, withOrigin())
	mustStatus(t, createOtherResp, http.StatusNotFound)

	deleteOtherResp := doJSON(t, clientB, http.MethodDelete, "/api/competitors/"+competitor["id"].(string), nil, withOrigin())
	mustStatus(t, deleteOtherResp, http.StatusNotFound)

	listOtherResp := doJSON(t, clientB, http.MethodGet, "/api/products/"+productAID+"/competitors", nil)
	mustStatus(t, listOtherResp, http.StatusOK)
	var list []map[string]any
	mustDecode(t, listOtherResp, &list)
	if len(list) != 0 {
		t.Fatalf("other tenant saw competitors: %#v", list)
	}
}
