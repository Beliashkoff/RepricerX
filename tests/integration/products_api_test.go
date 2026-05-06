//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	mp "github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

func createTestShop(t *testing.T, client *http.Client, suffix string) string {
	t.Helper()
	resp := doJSON(t, client, http.MethodPost, "/api/shops", map[string]any{
		"marketplace": "wb",
		"name":        "Shop " + suffix,
		"credentials": wbCreds(),
	}, withOrigin())
	mustStatus(t, resp, http.StatusCreated)
	var body map[string]any
	mustDecode(t, resp, &body)
	return body["id"].(string)
}

func createTestProduct(t *testing.T, client *http.Client, shopID, sku, name string) map[string]any {
	t.Helper()
	resp := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products", map[string]any{
		"externalSku":  sku,
		"name":         name,
		"currentPrice": 100.50,
		"currency":     "RUB",
		"minPrice":     90,
		"maxPrice":     150,
		"costPrice":    70,
	}, withOrigin())
	mustStatus(t, resp, http.StatusCreated)
	var body map[string]any
	mustDecode(t, resp, &body)
	return body
}

func TestProductCreateAndDuplicateSKU(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "product_create")
	shopID := createTestShop(t, client, "product_create")

	product := createTestProduct(t, client, shopID, "SKU-1", "Test Product")
	if product["externalSku"] != "SKU-1" {
		t.Fatalf("want SKU-1, got %v", product["externalSku"])
	}
	// createTestProduct sends costPrice=70 — verify the RETURNING clause echoes it correctly.
	if cp, ok := product["costPrice"].(float64); !ok || cp != 70 {
		t.Fatalf("want costPrice=70, got %v", product["costPrice"])
	}

	dup := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products", map[string]any{
		"externalSku":  "SKU-1",
		"name":         "Duplicate",
		"currentPrice": 120,
	}, withOrigin())
	mustStatus(t, dup, http.StatusConflict)
	mustErrorCode(t, dup, "duplicate_sku")
}

func TestProductCreate_CostPrice(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "product_costprice")
	shopID := createTestShop(t, client, "product_costprice")

	// with explicit costPrice
	resp := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products", map[string]any{
		"externalSku":  "COST-001",
		"name":         "Cost Test",
		"currentPrice": 200,
		"currency":     "RUB",
		"costPrice":    55.5,
	}, withOrigin())
	mustStatus(t, resp, http.StatusCreated)
	var body map[string]any
	mustDecode(t, resp, &body)
	if cp, ok := body["costPrice"].(float64); !ok || cp != 55.5 {
		t.Fatalf("want costPrice=55.5, got %v", body["costPrice"])
	}

	// without costPrice — must be null / absent, not a SQL error
	resp2 := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products", map[string]any{
		"externalSku":  "COST-002",
		"name":         "No Cost",
		"currentPrice": 100,
		"currency":     "RUB",
	}, withOrigin())
	mustStatus(t, resp2, http.StatusCreated)
	var body2 map[string]any
	mustDecode(t, resp2, &body2)
	if body2["costPrice"] != nil {
		t.Fatalf("want costPrice=null when omitted, got %v", body2["costPrice"])
	}
}

func TestProductSameSKUAcrossShopsAndWrongOwner(t *testing.T) {
	truncate(t)
	client1 := loginAsNewUser(t, "product_owner1")
	client2 := loginAsNewUser(t, "product_owner2")
	shop1 := createTestShop(t, client1, "owner1")
	shop2 := createTestShop(t, client2, "owner2")

	p1 := createTestProduct(t, client1, shop1, "SHARED-SKU", "Owner 1 Product")
	p2 := createTestProduct(t, client2, shop2, "SHARED-SKU", "Owner 2 Product")
	if p1["id"] == p2["id"] {
		t.Fatal("same externalSku across shops should create different products")
	}

	wrongOwnerPatch := doJSON(t, client2, http.MethodPatch, "/api/products/"+p1["id"].(string), map[string]any{
		"minPrice": 10,
	}, withOrigin())
	mustStatus(t, wrongOwnerPatch, http.StatusNotFound)

	list := doJSON(t, client2, http.MethodGet, "/api/products?q=Owner+1", nil)
	mustStatus(t, list, http.StatusOK)
	var body struct {
		Items []map[string]any `json:"items"`
	}
	mustDecode(t, list, &body)
	if len(body.Items) != 0 {
		t.Fatalf("wrong owner product leaked in list: %#v", body.Items)
	}
}

func TestProductListSearchFilterAndPatch(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "product_list")
	shopID := createTestShop(t, client, "list")
	first := createTestProduct(t, client, shopID, "ABC-001", "Красный чайник")
	createTestProduct(t, client, shopID, "XYZ-002", "Blue kettle")

	search := doJSON(t, client, http.MethodGet, "/api/products?q=чайник&shop_id="+shopID+"&status=active&page=1&per_page=1", nil)
	mustStatus(t, search, http.StatusOK)
	var listBody struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			Page    int `json:"page"`
			PerPage int `json:"perPage"`
			Total   int `json:"total"`
		} `json:"pagination"`
	}
	mustDecode(t, search, &listBody)
	if len(listBody.Items) != 1 || listBody.Pagination.Total != 1 {
		t.Fatalf("unexpected search result: %#v", listBody)
	}

	patch := doJSON(t, client, http.MethodPatch, "/api/products/"+first["id"].(string), map[string]any{
		"minPrice":  80,
		"costPrice": nil,
	}, withOrigin())
	mustStatus(t, patch, http.StatusOK)
	var patched map[string]any
	mustDecode(t, patch, &patched)
	if patched["minPrice"].(float64) != 80 {
		t.Fatalf("want minPrice 80, got %v", patched["minPrice"])
	}
	if patched["costPrice"] != nil {
		t.Fatalf("want costPrice null, got %v", patched["costPrice"])
	}

	badPatch := doJSON(t, client, http.MethodPatch, "/api/products/"+first["id"].(string), map[string]any{
		"minPrice": 200,
		"maxPrice": 100,
	}, withOrigin())
	mustStatus(t, badPatch, http.StatusBadRequest)
	mustErrorCode(t, badPatch, "invalid_price")
}

func TestProductImportUpsertAndFailure(t *testing.T) {
	truncate(t)
	client := loginAsNewUser(t, "product_import")
	shopID := createTestShop(t, client, "import")
	createTestProduct(t, client, shopID, "SKU-EXISTING", "Old name")

	testSKUs = []mp.SKU{
		{ExternalSKU: "SKU-EXISTING", Name: "Updated name", CurrentPrice: 111, Currency: "RUB"},
		{ExternalSKU: "SKU-NEW", Name: "New product", CurrentPrice: 222, Currency: "RUB"},
		{ExternalSKU: "", Name: "Invalid product", CurrentPrice: 10, Currency: "RUB"},
	}

	start := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products/import", nil, withOrigin())
	mustStatus(t, start, http.StatusAccepted)
	var importResp map[string]any
	mustDecode(t, start, &importResp)
	importID := importResp["importId"].(string)
	if importResp["status"] != "pending" {
		t.Fatalf("want pending import, got %v", importResp["status"])
	}

	duplicateStart := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products/import", nil, withOrigin())
	mustStatus(t, duplicateStart, http.StatusConflict)
	mustErrorCode(t, duplicateStart, "import_already_running")

	poll := doJSON(t, client, http.MethodGet, "/api/imports/"+importID, nil)
	mustStatus(t, poll, http.StatusOK)
	otherClient := loginAsNewUser(t, "product_import_other")
	wrongOwnerPoll := doJSON(t, otherClient, http.MethodGet, "/api/imports/"+importID, nil)
	mustStatus(t, wrongOwnerPoll, http.StatusNotFound)

	runNextImportJob(t)
	waitImportStatus(t, importID, "partial")

	cooldownStart := doJSON(t, client, http.MethodPost, "/api/shops/"+shopID+"/products/import", nil, withOrigin())
	mustStatus(t, cooldownStart, http.StatusTooManyRequests)
	mustErrorCode(t, cooldownStart, "import_cooldown")
	if cooldownStart.Header.Get("Retry-After") == "" {
		t.Fatal("want Retry-After header for import cooldown")
	}

	list := doJSON(t, client, http.MethodGet, "/api/products?q=SKU-", nil)
	mustStatus(t, list, http.StatusOK)
	var body struct {
		Items []map[string]any `json:"items"`
	}
	mustDecode(t, list, &body)
	if len(body.Items) != 2 {
		t.Fatalf("want 2 imported/upserted products, got %d: %#v", len(body.Items), body.Items)
	}

	failShopID := createTestShop(t, client, "import_fail")
	maliciousAdapterError := `adapter failed: Authorization: Bearer SECRET-TOKEN api_key=AKIA123 password=hunter2 client_id=my-client Cookie: session=secretcookie url=https://example.test/import?token=querysecret#frag payload={"credential":"full-payload"}`
	testListSKUsErr = errors.New(maliciousAdapterError)
	startFail := doJSON(t, client, http.MethodPost, "/api/shops/"+failShopID+"/products/import", nil, withOrigin())
	mustStatus(t, startFail, http.StatusAccepted)
	var failResp map[string]any
	mustDecode(t, startFail, &failResp)
	failJobID := failResp["jobId"].(string)
	_, err := testPool.Exec(context.Background(),
		`UPDATE background_jobs SET max_attempts=1 WHERE id=$1`, failJobID)
	if err != nil {
		t.Fatalf("set max attempts: %v", err)
	}
	runNextImportJob(t)
	failImportID := failResp["importId"].(string)
	waitImportStatus(t, failImportID, "failed")

	failPoll := doJSON(t, client, http.MethodGet, "/api/imports/"+failImportID, nil)
	mustStatus(t, failPoll, http.StatusOK)
	failPollJSON, err := io.ReadAll(failPoll.Body)
	if err != nil {
		t.Fatalf("read fail poll response: %v", err)
	}
	var failStatus struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(failPollJSON, &failStatus); err != nil {
		t.Fatalf("decode fail poll response: %v", err)
	}
	if len(failStatus.Errors) != 1 {
		t.Fatalf("want one public import error, got %#v", failStatus.Errors)
	}
	if failStatus.Errors[0].Code != "adapter_error" {
		t.Fatalf("want adapter_error, got %q", failStatus.Errors[0].Code)
	}
	const publicAdapterMessage = "Import failed because the external shop adapter returned an error."
	if failStatus.Errors[0].Message != publicAdapterMessage {
		t.Fatalf("want public adapter message, got %q", failStatus.Errors[0].Message)
	}

	var importErrorsJSON []byte
	err = testPool.QueryRow(context.Background(),
		`SELECT errors FROM import_log WHERE id=$1`, failImportID).Scan(&importErrorsJSON)
	if err != nil {
		t.Fatalf("get import errors: %v", err)
	}
	var lastError string
	err = testPool.QueryRow(context.Background(),
		`SELECT last_error FROM background_jobs WHERE id=$1`, failJobID).Scan(&lastError)
	if err != nil {
		t.Fatalf("get background job last_error: %v", err)
	}
	for _, leak := range []string{
		"SECRET-TOKEN",
		"AKIA123",
		"hunter2",
		"my-client",
		"secretcookie",
		"querysecret",
		"full-payload",
	} {
		if strings.Contains(string(failPollJSON), leak) {
			t.Fatalf("poll response leaked adapter detail %q: %s", leak, failPollJSON)
		}
		if strings.Contains(string(importErrorsJSON), leak) {
			t.Fatalf("import_log errors leaked adapter detail %q: %s", leak, importErrorsJSON)
		}
		if strings.Contains(lastError, leak) {
			t.Fatalf("background_jobs last_error leaked adapter detail %q: %s", leak, lastError)
		}
	}
}

func runNextImportJob(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	jobs := repository.NewBackgroundJobsRepository(testPool)
	job, err := jobs.ClaimNext(ctx, "default", "test-worker", time.Minute)
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	result := testProductSvc.ExecuteImportJob(ctx, job)
	if result.Retryable {
		if err := jobs.Retry(ctx, job.ID, time.Now().UTC(), result.InternalError); err != nil {
			t.Fatalf("retry job: %v", err)
		}
		return
	}
	if result.Status == domain.ImportStatusSucceeded || result.Status == domain.ImportStatusPartial {
		if err := jobs.Succeed(ctx, job.ID, result.ResultJSON); err != nil {
			t.Fatalf("succeed job: %v", err)
		}
		return
	}
	if err := jobs.Fail(ctx, job.ID, result.InternalError, result.ResultJSON); err != nil {
		t.Fatalf("fail job: %v", err)
	}
}

func waitImportStatus(t *testing.T, importID, want string) {
	t.Helper()
	id := uuid.MustParse(importID)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err := testPool.QueryRow(context.Background(), `SELECT status FROM import_log WHERE id=$1`, id).Scan(&status)
		if err == nil && status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	var status string
	_ = testPool.QueryRow(context.Background(), `SELECT status FROM import_log WHERE id=$1`, id).Scan(&status)
	t.Fatalf("want import status %q, got %q", want, status)
}
