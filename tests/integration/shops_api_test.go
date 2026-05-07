//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// loginAsNewUser регистрирует нового пользователя, активирует его и выполняет вход.
// Возвращает готовый HTTP-клиент с установленной сессией.
func loginAsNewUser(t *testing.T, suffix string) *http.Client {
	t.Helper()
	email := fmt.Sprintf("shopuser%s@example.com", suffix)
	client := newClient()

	mustStatus(t, doJSON(t, client, http.MethodPost, "/api/auth/register", map[string]any{
		"email": email, "password": testPassword, "displayName": testName,
	}), http.StatusCreated)

	activateUser(t, email)

	mustStatus(t, doJSON(t, client, http.MethodPost, "/api/auth/login", map[string]any{
		"email": email, "password": testPassword,
	}), http.StatusOK)

	return client
}

func wbCreds() json.RawMessage {
	return json.RawMessage(`{"api_key":"test-wb-token"}`)
}

func ozonCreds() json.RawMessage {
	return json.RawMessage(`{"client_id":"123","api_key":"test-ozon-key"}`)
}

// --- GET /api/shops ---

func TestShopList_Unauthorized(t *testing.T) {
	truncate(t)
	mustStatus(t, doJSON(t, newClient(), http.MethodGet, "/api/shops", nil), http.StatusUnauthorized)
}

func TestShopList_Empty(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "list_empty")

	resp := doJSON(t, client, http.MethodGet, "/api/shops", nil)
	mustStatus(t, resp, http.StatusOK)

	var shops []map[string]any
	mustDecode(t, resp, &shops)
	if len(shops) != 0 {
		t.Fatalf("want 0 shops, got %d", len(shops))
	}
}

// --- POST /api/shops ---

func TestShopCreate_NoCSRF(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "create_nocsrf")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb",
		"name":        "My Shop",
		"credentials": wbCreds(),
	}) // no Origin header
	mustStatus(t, resp, http.StatusForbidden)
	mustErrorCode(t, resp, "csrf_blocked")
}

func TestShopCreate_Unauthorized(t *testing.T) {
	truncate(t)
	resp := doJSON(t, newClient(), http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "S", "credentials": wbCreds(),
	}, withOrigin())
	mustStatus(t, resp, http.StatusUnauthorized)
}

func TestShopCreate_InvalidMarketplace(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "create_badmp")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "unknown",
		"name":        "Shop",
		"credentials": json.RawMessage(`{}`),
	}, withOrigin())
	mustStatus(t, resp, http.StatusBadRequest)
	mustErrorCode(t, resp, "invalid_marketplace")
}

func TestShopCreate_MissingFields(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "create_missing")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb",
		// name and credentials missing
	}, withOrigin())
	mustStatus(t, resp, http.StatusBadRequest)
}

func TestShopCreate_InvalidCredentials(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "create_bad_creds")

	cases := []struct {
		name        string
		marketplace string
		credentials json.RawMessage
	}{
		{name: "wb token field", marketplace: "wb", credentials: json.RawMessage(`{"token":"test-wb-token"}`)},
		{name: "wb extra field", marketplace: "wb", credentials: json.RawMessage(`{"api_key":"test-wb-token","extra":"x"}`)},
		{name: "ozon missing client id", marketplace: "ozon", credentials: json.RawMessage(`{"api_key":"test-ozon-key"}`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
				"marketplace": tc.marketplace,
				"name":        "Shop",
				"credentials": tc.credentials,
			}, withOrigin())
			mustStatus(t, resp, http.StatusBadRequest)
			mustErrorCode(t, resp, "invalid_credentials")
		})
	}
}

func TestShopCreate_CredentialsTooLarge(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "create_large_creds")
	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb",
		"name":        "Shop",
		"credentials": map[string]any{"api_key": strings.Repeat("x", 4097)},
	}, withOrigin())
	mustStatus(t, resp, http.StatusBadRequest)
	mustErrorCode(t, resp, "invalid_credentials")
}

func TestShopCreate_WB_Success(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "create_wb")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb",
		"name":        "My WB Shop",
		"credentials": wbCreds(),
	}, withOrigin())
	mustStatus(t, resp, http.StatusCreated)

	var shop map[string]any
	mustDecode(t, resp, &shop)
	if shop["marketplace"] != "wb" {
		t.Fatalf("want marketplace wb, got %v", shop["marketplace"])
	}
	if shop["name"] != "My WB Shop" {
		t.Fatalf("want name, got %v", shop["name"])
	}
	if shop["status"] != "draft" {
		t.Fatalf("want status draft, got %v", shop["status"])
	}
	if shop["id"] == nil || shop["id"] == "" {
		t.Fatal("want non-empty id")
	}
	for _, key := range []string{"autoUpdateEnabled", "scheduleCron", "lastCheckedAt", "createdAt"} {
		if _, ok := shop[key]; !ok {
			t.Fatalf("response must contain camelCase key %q", key)
		}
	}
	for _, key := range []string{"auto_update_enabled", "schedule_cron", "last_checked_at", "created_at"} {
		if _, ok := shop[key]; ok {
			t.Fatalf("response must not contain snake_case key %q", key)
		}
	}
}

func TestShopCreate_Ozon_Success(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "create_ozon")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "ozon",
		"name":        "My Ozon Shop",
		"credentials": ozonCreds(),
	}, withOrigin())
	mustStatus(t, resp, http.StatusCreated)

	var shop map[string]any
	mustDecode(t, resp, &shop)
	if shop["marketplace"] != "ozon" {
		t.Fatalf("want marketplace ozon, got %v", shop["marketplace"])
	}
}

// --- GET /api/shops/:id ---

func TestShopGet_NotFound(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "get_notfound")

	resp := doJSON(t, client, http.MethodGet, "/api/shops/00000000-0000-0000-0000-000000000001", nil)
	mustStatus(t, resp, http.StatusNotFound)
	mustErrorCode(t, resp, "shop_not_found")
}

func TestShopGet_WrongOwner(t *testing.T) {
	truncate(t)
	// user1 creates shop
	client1 := loginAsNewUser(t, "get_owner1")
	resp := doJSON(t, client1, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	mustStatus(t, resp, http.StatusCreated)
	var shop map[string]any
	mustDecode(t, resp, &shop)
	shopID := shop["id"].(string)

	// user2 tries to get it
	client2 := loginAsNewUser(t, "get_owner2")
	resp2 := doJSON(t, client2, http.MethodGet, "/api/shops/"+shopID, nil)
	mustStatus(t, resp2, http.StatusNotFound)
}

func TestShopGet_Success(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "get_success")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	got := doJSON(t, client, http.MethodGet, "/api/shops/"+shopID, nil)
	mustStatus(t, got, http.StatusOK)

	var fetched map[string]any
	mustDecode(t, got, &fetched)
	if fetched["id"] != shopID {
		t.Fatalf("want id %q, got %v", shopID, fetched["id"])
	}
}

// --- PATCH /api/shops/:id ---

func TestShopUpdate_Name(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "update_name")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Old Name", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	updated := doJSON(t, client, http.MethodPatch, "/api/shops/"+shopID, map[string]any{
		"name": "New Name",
	}, withOrigin())
	mustStatus(t, updated, http.StatusOK)

	var body map[string]any
	mustDecode(t, updated, &body)
	if body["name"] != "New Name" {
		t.Fatalf("want name %q, got %v", "New Name", body["name"])
	}
}

func TestShopUpdate_CamelCaseSettings(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "update_settings")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	updated := doJSON(t, client, http.MethodPatch, "/api/shops/"+shopID, map[string]any{
		"autoUpdateEnabled": true,
		"scheduleCron":      "0 */4 * * *",
	}, withOrigin())
	mustStatus(t, updated, http.StatusOK)

	var body map[string]any
	mustDecode(t, updated, &body)
	if body["autoUpdateEnabled"] != true {
		t.Fatalf("want autoUpdateEnabled=true, got %v", body["autoUpdateEnabled"])
	}
	if body["scheduleCron"] != "0 */4 * * *" {
		t.Fatalf("want scheduleCron updated, got %v", body["scheduleCron"])
	}
	if _, ok := body["auto_update_enabled"]; ok {
		t.Fatal("response must not contain auto_update_enabled")
	}
	if _, ok := body["schedule_cron"]; ok {
		t.Fatal("response must not contain schedule_cron")
	}
}

func TestShopUpdate_NoCSRF(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "update_nocsrf")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	noOrigin := doJSON(t, client, http.MethodPatch, "/api/shops/"+shopID, map[string]any{"name": "X"})
	mustStatus(t, noOrigin, http.StatusForbidden)
}

// --- DELETE /api/shops/:id ---

func TestShopDelete_Success(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "delete_success")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	del := doJSON(t, client, http.MethodDelete, "/api/shops/"+shopID, nil, withOrigin())
	mustStatus(t, del, http.StatusNoContent)

	// Verify it's gone
	gone := doJSON(t, client, http.MethodGet, "/api/shops/"+shopID, nil)
	mustStatus(t, gone, http.StatusNotFound)
}

func TestShopDelete_WrongOwner(t *testing.T) {
	truncate(t)
	client1 := loginAsNewUser(t, "del_owner1")
	resp := doJSON(t, client1, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	client2 := loginAsNewUser(t, "del_owner2")
	del := doJSON(t, client2, http.MethodDelete, "/api/shops/"+shopID, nil, withOrigin())
	mustStatus(t, del, http.StatusNotFound)
}

func TestShopDelete_NoCSRF(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "delete_nocsrf")
	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	noOrigin := doJSON(t, client, http.MethodDelete, "/api/shops/"+shopID, nil)
	mustStatus(t, noOrigin, http.StatusForbidden)
}

// --- POST /api/shops/:id/test ---

func TestShopTestConnection_Success(t *testing.T) {
	truncate(t)
	testShopAuthFail = false
	client := loginAsNewUser(t, "test_ok")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	testResp := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/test", nil, withOrigin())
	mustStatus(t, testResp, http.StatusOK)

	var body map[string]any
	mustDecode(t, testResp, &body)
	if body["status"] != "active" {
		t.Fatalf("want status active, got %v", body["status"])
	}

	// Status should be updated in DB
	shopResp := doJSON(t, client, http.MethodGet, "/api/shops/"+shopID, nil)
	var shopData map[string]any
	mustDecode(t, shopResp, &shopData)
	if shopData["status"] != "active" {
		t.Fatalf("shop status should be active after successful test, got %v", shopData["status"])
	}
	if shopData["lastCheckedAt"] == nil || shopData["lastCheckedAt"] == "" {
		t.Fatalf("lastCheckedAt must be set after successful test, got %v", shopData["lastCheckedAt"])
	}
	if _, ok := shopData["last_checked_at"]; ok {
		t.Fatal("response must not contain last_checked_at")
	}
}

func TestShopTestConnection_AuthFailed(t *testing.T) {
	truncate(t)
	testShopAuthFail = true
	defer func() { testShopAuthFail = false }()

	client := loginAsNewUser(t, "test_fail")

	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "Shop", "credentials": wbCreds(),
	}, withOrigin())
	var created map[string]any
	mustDecode(t, resp, &created)
	shopID := created["id"].(string)

	testResp := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/test", nil, withOrigin())
	mustStatus(t, testResp, http.StatusUnprocessableEntity)
	mustErrorCode(t, testResp, "auth_failed")

	// Status should be error
	shopResp := doJSON(t, client, http.MethodGet, "/api/shops/"+shopID, nil)
	var shopData map[string]any
	mustDecode(t, shopResp, &shopData)
	if shopData["status"] != "error" {
		t.Fatalf("shop status should be error after failed test, got %v", shopData["status"])
	}
}

func TestShopTestConnection_NotFound(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "test_notfound")

	resp := doJSON(t, client, http.MethodPost, "/api/shops/00000000-0000-0000-0000-000000000002/test", nil, withOrigin())
	mustStatus(t, resp, http.StatusNotFound)
}

// --- Multiple shops ---

func TestShopList_MultipleShops(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "multilist")

	for _, name := range []string{"Shop A", "Shop B", "Shop C"} {
		doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
			"marketplace": "wb", "name": name, "credentials": wbCreds(),
		}, withOrigin())
	}

	resp := doJSON(t, client, http.MethodGet, "/api/shops", nil)
	mustStatus(t, resp, http.StatusOK)

	var shops []map[string]any
	mustDecode(t, resp, &shops)
	if len(shops) != 3 {
		t.Fatalf("want 3 shops, got %d", len(shops))
	}
}

func TestShopList_IsolatedByUser(t *testing.T) {
	truncate(t)
	client1 := loginAsNewUser(t, "iso1")
	client2 := loginAsNewUser(t, "iso2")

	doJSON(t, client1, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb", "name": "User1 Shop", "credentials": wbCreds(),
	}, withOrigin())
	doJSON(t, client2, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "ozon", "name": "User2 Shop", "credentials": ozonCreds(),
	}, withOrigin())

	resp1 := doJSON(t, client1, http.MethodGet, "/api/shops", nil)
	var shops1 []map[string]any
	mustDecode(t, resp1, &shops1)

	resp2 := doJSON(t, client2, http.MethodGet, "/api/shops", nil)
	var shops2 []map[string]any
	mustDecode(t, resp2, &shops2)

	if len(shops1) != 1 {
		t.Fatalf("user1: want 1, got %d", len(shops1))
	}
	if len(shops2) != 1 {
		t.Fatalf("user2: want 1, got %d", len(shops2))
	}
	if shops1[0]["marketplace"] != "wb" {
		t.Fatalf("user1 shop should be wb, got %v", shops1[0]["marketplace"])
	}
	if shops2[0]["marketplace"] != "ozon" {
		t.Fatalf("user2 shop should be ozon, got %v", shops2[0]["marketplace"])
	}
}
