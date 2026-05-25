//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// loginUser registers + activates + logs in; returns a ready client.
func loginUser(t *testing.T, email, password string) *http.Client {
	t.Helper()
	client := newClient()
	doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": email, "password": password, "displayName": "Test",
	}, withOrigin())
	activateUser(t, email)
	resp := doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": email, "password": password,
	}, withOrigin())
	mustStatus(t, resp, http.StatusOK)
	return client
}

func TestStrategy_CRUD(t *testing.T) {
	truncate(t)
	client := loginUser(t, "strat@example.com", "ValidPass123!")

	// Create.
	createBody := map[string]any{
		"name":           "Фиксированная",
		"type":           "fixed",
		"params":         map[string]any{"value": 999.99},
		"constraints":    map[string]any{"min_profit_pct": 10.0},
		"fallbackPolicy": "keep_current",
		"priority":       1,
		"enabled":        true,
	}
	resp := doJSON(t, client, http.MethodPost, "/api/strategies", createBody, withOrigin())
	mustStatus(t, resp, http.StatusCreated)

	var created struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
	}
	mustDecode(t, resp, &created)
	if created.Name != "Фиксированная" {
		t.Errorf("unexpected name: %s", created.Name)
	}
	if created.Type != "fixed" {
		t.Errorf("unexpected type: %s", created.Type)
	}

	// List.
	resp = doJSON(t, client, http.MethodGet, "/api/strategies", nil)
	mustStatus(t, resp, http.StatusOK)
	var list []map[string]any
	mustDecode(t, resp, &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 strategy in list, got %d", len(list))
	}

	// Get by ID.
	resp = doJSON(t, client, http.MethodGet, "/api/strategies/"+created.ID, nil)
	mustStatus(t, resp, http.StatusOK)
	var got struct {
		ID                 string   `json:"id"`
		AssignedProductIDs []string `json:"assignedProductIds"`
	}
	mustDecode(t, resp, &got)
	if got.ID != created.ID {
		t.Errorf("id mismatch: want %s, got %s", created.ID, got.ID)
	}

	// Update — disable.
	disabled := false
	patchBody := map[string]any{"enabled": disabled}
	resp = doJSON(t, client, http.MethodPatch, "/api/strategies/"+created.ID, patchBody, withOrigin())
	mustStatus(t, resp, http.StatusOK)
	var updated struct{ Enabled bool `json:"enabled"` }
	mustDecode(t, resp, &updated)
	if updated.Enabled {
		t.Error("expected enabled=false after patch")
	}

	// Delete.
	resp = doJSON(t, client, http.MethodDelete, "/api/strategies/"+created.ID, nil, withOrigin())
	mustStatus(t, resp, http.StatusNoContent)

	// List after delete.
	resp = doJSON(t, client, http.MethodGet, "/api/strategies", nil)
	mustStatus(t, resp, http.StatusOK)
	var afterDelete []map[string]any
	mustDecode(t, resp, &afterDelete)
	if len(afterDelete) != 0 {
		t.Errorf("expected 0 strategies after delete, got %d", len(afterDelete))
	}
}

func TestStrategy_ValidationErrors(t *testing.T) {
	truncate(t)
	client := loginUser(t, "stratval@example.com", "ValidPass123!")

	cases := []struct {
		name       string
		body       map[string]any
		wantCode   string
	}{
		{
			"invalid type",
			map[string]any{
				"name": "x", "type": "unknown", "params": map[string]any{"value": 1},
				"fallbackPolicy": "keep_current",
			},
			"invalid_strategy_type",
		},
		{
			"fixed value=0",
			map[string]any{
				"name": "x", "type": "fixed", "params": map[string]any{"value": 0},
				"fallbackPolicy": "keep_current",
			},
			"invalid_strategy_params",
		},
		{
			// Лимит поднят до 100 — значения выше 100 должны отклоняться
			"below_median_pct out of range",
			map[string]any{
				"name": "x", "type": "below_median_pct", "params": map[string]any{"pct": 101},
				"fallbackPolicy": "keep_current",
			},
			"invalid_strategy_params",
		},
		{
			// Отрицательная min_profit_pct — единственное запрещённое значение
			"min_profit_pct negative",
			map[string]any{
				"name": "x", "type": "fixed", "params": map[string]any{"value": 100},
				"constraints": map[string]any{"min_profit_pct": -5},
				"fallbackPolicy": "keep_current",
			},
			"invalid_constraints",
		},
		{
			"min_price > max_price",
			map[string]any{
				"name": "x", "type": "fixed", "params": map[string]any{"value": 100},
				"constraints": map[string]any{"min_price": 500, "max_price": 100},
				"fallbackPolicy": "keep_current",
			},
			"invalid_constraints",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, client, http.MethodPost, "/api/strategies", tc.body, withOrigin())
			mustStatus(t, resp, http.StatusBadRequest)
			mustErrorCode(t, resp, tc.wantCode)
		})
	}
}

func TestStrategy_Assignments(t *testing.T) {
	truncate(t)
	client := loginUser(t, "strategya@example.com", "ValidPass123!")

	// Create a shop + product first.
	shopResp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"name": "Test Shop", "marketplace": "wb",
		"credentials": map[string]any{"api_key": "test"},
	}, withOrigin())
	mustStatus(t, shopResp, http.StatusCreated)
	var shop struct{ ID string `json:"id"` }
	mustDecode(t, shopResp, &shop)

	prodResp := doJSON(t, client, http.MethodPost, "/api/shops/"+shop.ID+"/products", map[string]any{
		"externalSku": "SKU-001",
		"name":        "Test Product",
		"price":       1000.0,
		"currency":    "RUB",
	}, withOrigin())
	mustStatus(t, prodResp, http.StatusCreated)
	var prod struct{ ID string `json:"id"` }
	mustDecode(t, prodResp, &prod)

	// Create strategy.
	stratResp := doJSON(t, client, http.MethodPost, "/api/strategies", map[string]any{
		"name": "S1", "type": "fixed", "params": map[string]any{"value": 500},
		"fallbackPolicy": "keep_current", "enabled": true,
	}, withOrigin())
	mustStatus(t, stratResp, http.StatusCreated)
	var strat struct {
		ID            string `json:"id"`
		AssignedCount int    `json:"assignedCount"`
	}
	mustDecode(t, stratResp, &strat)

	// Assign product.
	resp := doJSON(t, client, http.MethodPost, "/api/strategies/"+strat.ID+"/assignments",
		map[string]any{"productIds": []string{prod.ID}}, withOrigin())
	mustStatus(t, resp, http.StatusNoContent)

	// Get strategy — check assignedProductIds and assignedCount.
	resp = doJSON(t, client, http.MethodGet, "/api/strategies/"+strat.ID, nil)
	mustStatus(t, resp, http.StatusOK)
	var detail struct {
		AssignedCount      int             `json:"assignedCount"`
		AssignedProductIDs []string        `json:"assignedProductIds"`
		Constraints        json.RawMessage `json:"constraints"`
	}
	mustDecode(t, resp, &detail)
	if detail.AssignedCount != 1 {
		t.Errorf("expected assignedCount=1, got %d", detail.AssignedCount)
	}
	if len(detail.AssignedProductIDs) != 1 || detail.AssignedProductIDs[0] != prod.ID {
		t.Errorf("unexpected assignedProductIds: %v", detail.AssignedProductIDs)
	}

	// Unassign product.
	resp = doJSON(t, client, http.MethodDelete, "/api/strategies/"+strat.ID+"/assignments",
		map[string]any{"productIds": []string{prod.ID}}, withOrigin())
	mustStatus(t, resp, http.StatusNoContent)

	resp = doJSON(t, client, http.MethodGet, "/api/strategies/"+strat.ID, nil)
	mustStatus(t, resp, http.StatusOK)
	mustDecode(t, resp, &detail)
	if detail.AssignedCount != 0 {
		t.Errorf("expected assignedCount=0 after unassign, got %d", detail.AssignedCount)
	}
}

func TestStrategy_NotFound(t *testing.T) {
	truncate(t)
	client := loginUser(t, "stratnf@example.com", "ValidPass123!")

	nonExistent := uuid.New().String()
	resp := doJSON(t, client, http.MethodGet, "/api/strategies/"+nonExistent, nil)
	mustStatus(t, resp, http.StatusNotFound)
	mustErrorCode(t, resp, "strategy_not_found")
}

func TestStrategy_TenantIsolation(t *testing.T) {
	truncate(t)
	clientA := loginUser(t, "usera@example.com", "ValidPass123!")
	clientB := loginUser(t, "userb@example.com", "ValidPass123!")

	// User A creates strategy.
	resp := doJSON(t, clientA, http.MethodPost, "/api/strategies", map[string]any{
		"name": "A's strategy", "type": "fixed", "params": map[string]any{"value": 100},
		"fallbackPolicy": "keep_current", "enabled": true,
	}, withOrigin())
	mustStatus(t, resp, http.StatusCreated)
	var strat struct{ ID string `json:"id"` }
	mustDecode(t, resp, &strat)

	// User B cannot access it.
	resp = doJSON(t, clientB, http.MethodGet, "/api/strategies/"+strat.ID, nil)
	mustStatus(t, resp, http.StatusNotFound)

	// User B cannot delete it.
	resp = doJSON(t, clientB, http.MethodDelete, "/api/strategies/"+strat.ID, nil, withOrigin())
	mustStatus(t, resp, http.StatusNotFound)
}
