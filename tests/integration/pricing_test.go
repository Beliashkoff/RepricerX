//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/pkg/crypto"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	pricingsvc "github.com/Beliashkoff/RepricerX/internal/service/pricing"
	"github.com/google/uuid"
)

type integrationSKU = integration.SKU

func testEncrypt(plaintext []byte, secret string) ([]byte, error) {
	return crypto.Encrypt(plaintext, secret)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// createShopAndProduct — создаёт магазин и товар напрямую в БД
// (минуя marketplace-импорт) для тестов pricing.
// Возвращает shopID и productID.
func createShopAndProduct(t *testing.T, userEmail string, currentPrice float64, costPrice *float64) (shopID, productID uuid.UUID) {
	t.Helper()
	userID := getUserID(t, userEmail)

	shopID = uuid.New()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO shops (id, user_id, marketplace, name, credentials_encrypted, status, created_at, updated_at, auto_update_enabled, schedule_cron)
		VALUES ($1, $2, 'wb', 'Test Shop', '\x00'::bytea, 'active', NOW(), NOW(), false, '0 0 * * *')`,
		shopID, userID,
	); err != nil {
		t.Fatalf("create shop: %v", err)
	}

	productID = uuid.New()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO products (id, shop_id, external_sku, name, current_price, currency, status, cost_price, created_at, updated_at)
		VALUES ($1, $2, $3, 'Test Product', $4, 'RUB', 'active', $5, NOW(), NOW())`,
		productID, shopID, fmt.Sprintf("SKU-%s", productID.String()[:8]), currentPrice, costPrice,
	); err != nil {
		t.Fatalf("create product: %v", err)
	}
	return shopID, productID
}

// createStrategyDirect — создаёт стратегию через HTTP (валидируется).
// Возвращает strategyID.
func createStrategyDirect(t *testing.T, client *http.Client, name, stratType string, params, constraints map[string]any) string {
	t.Helper()
	body := map[string]any{
		"name": name, "type": stratType,
		"params": params, "constraints": constraints,
		"fallbackPolicy": "keep_current", "enabled": true,
	}
	resp := doJSON(t, client, http.MethodPost, "/api/strategies", body, withOrigin())
	mustStatus(t, resp, http.StatusCreated)
	var s struct{ ID string `json:"id"` }
	mustDecode(t, resp, &s)
	return s.ID
}

func assignStrategy(t *testing.T, client *http.Client, strategyID, productID string) {
	t.Helper()
	resp := doJSON(t, client, http.MethodPost,
		fmt.Sprintf("/api/strategies/%s/assignments", strategyID),
		map[string]any{"productIds": []string{productID}}, withOrigin())
	mustStatus(t, resp, http.StatusNoContent)
}

// ─── Simulate ────────────────────────────────────────────────────────────────

func TestPricing_Simulate_Fixed_HappyPath(t *testing.T) {
	truncate(t)
	client := loginUser(t, "sim1@example.com", "ValidPass123!")

	_, productID := createShopAndProduct(t, "sim1@example.com", 900, nil)
	strategyID := createStrategyDirect(t, client, "fix1", "fixed",
		map[string]any{"value": 500}, map[string]any{})

	resp := doJSON(t, client, http.MethodPost, "/api/pricing/simulate", map[string]any{
		"product_id": productID.String(), "strategy_id": strategyID,
	}, withOrigin())
	mustStatus(t, resp, http.StatusOK)

	var r struct {
		TargetPrice   float64 `json:"target_price"`
		FinalPrice    float64 `json:"final_price"`
		Status        string  `json:"status"`
		ConstraintHit *string `json:"constraint_hit"`
	}
	mustDecode(t, resp, &r)
	if r.FinalPrice != 500 || r.Status != "pending" {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestPricing_Simulate_CostPriceFloor(t *testing.T) {
	truncate(t)
	client := loginUser(t, "sim2@example.com", "ValidPass123!")

	cost := 600.0
	_, productID := createShopAndProduct(t, "sim2@example.com", 900, &cost)
	strategyID := createStrategyDirect(t, client, "fix2", "fixed",
		map[string]any{"value": 500}, map[string]any{})

	resp := doJSON(t, client, http.MethodPost, "/api/pricing/simulate", map[string]any{
		"product_id": productID.String(), "strategy_id": strategyID,
	}, withOrigin())
	mustStatus(t, resp, http.StatusOK)

	var r struct {
		FinalPrice    float64 `json:"final_price"`
		ConstraintHit *string `json:"constraint_hit"`
		Status        string  `json:"status"`
	}
	mustDecode(t, resp, &r)
	if r.FinalPrice != 600 || r.ConstraintHit == nil || *r.ConstraintHit != "cost_price_floor" {
		t.Errorf("expected cost_price_floor at 600, got %+v", r)
	}
}

func TestPricing_Simulate_BelowMedianMultipleCompetitors(t *testing.T) {
	truncate(t)
	client := loginUser(t, "sim3@example.com", "ValidPass123!")

	_, productID := createShopAndProduct(t, "sim3@example.com", 900, nil)
	strategyID := createStrategyDirect(t, client, "med", "below_median_pct",
		map[string]any{"pct": 5}, map[string]any{})

	resp := doJSON(t, client, http.MethodPost, "/api/pricing/simulate", map[string]any{
		"product_id":        productID.String(),
		"strategy_id":       strategyID,
		"competitor_prices": []float64{800, 850, 900},
	}, withOrigin())
	mustStatus(t, resp, http.StatusOK)

	var r struct {
		TargetPrice float64 `json:"target_price"`
		FinalPrice  float64 `json:"final_price"`
		Status      string  `json:"status"`
	}
	mustDecode(t, resp, &r)
	// median([800,850,900])=850 → 850*0.95=807.5
	if r.FinalPrice < 807 || r.FinalPrice > 808 {
		t.Errorf("expected ~807.50, got %+v", r)
	}
}

func TestPricing_Simulate_NoCompetitors_Skipped(t *testing.T) {
	truncate(t)
	client := loginUser(t, "sim4@example.com", "ValidPass123!")

	_, productID := createShopAndProduct(t, "sim4@example.com", 900, nil)
	strategyID := createStrategyDirect(t, client, "med2", "below_median_pct",
		map[string]any{"pct": 5}, map[string]any{})

	resp := doJSON(t, client, http.MethodPost, "/api/pricing/simulate", map[string]any{
		"product_id": productID.String(), "strategy_id": strategyID,
	}, withOrigin())
	mustStatus(t, resp, http.StatusOK)

	var r struct {
		Status     string  `json:"status"`
		FinalPrice float64 `json:"final_price"`
	}
	mustDecode(t, resp, &r)
	if r.Status != "skipped" || r.FinalPrice != 900 {
		t.Errorf("expected skipped/keep_current=900, got %+v", r)
	}
}

func TestPricing_Simulate_MissingCost_Skipped(t *testing.T) {
	truncate(t)
	client := loginUser(t, "sim5@example.com", "ValidPass123!")

	_, productID := createShopAndProduct(t, "sim5@example.com", 900, nil)
	strategyID := createStrategyDirect(t, client, "marg", "min_margin_pct",
		map[string]any{"margin_pct": 30}, map[string]any{})

	resp := doJSON(t, client, http.MethodPost, "/api/pricing/simulate", map[string]any{
		"product_id": productID.String(), "strategy_id": strategyID,
	}, withOrigin())
	mustStatus(t, resp, http.StatusOK)

	var r struct{ Status string `json:"status"` }
	mustDecode(t, resp, &r)
	if r.Status != "skipped" {
		t.Errorf("expected skipped (missing_cost), got %+v", r)
	}
}

// ─── Recalculate (async) ─────────────────────────────────────────────────────

func TestPricing_Recalculate_CreatesPlanAndJob(t *testing.T) {
	truncate(t)
	client := loginUser(t, "rec1@example.com", "ValidPass123!")

	shopID, productID := createShopAndProduct(t, "rec1@example.com", 900, nil)
	strategyID := createStrategyDirect(t, client, "fix-rec", "fixed",
		map[string]any{"value": 500}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	resp := doJSON(t, client, http.MethodPost, "/api/pricing/recalculate", map[string]any{
		"shop_id":     shopID.String(),
		"product_ids": []string{productID.String()},
	}, withOrigin())
	mustStatus(t, resp, http.StatusAccepted)

	var r struct {
		Plan struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"plan"`
		JobID string `json:"job_id"`
	}
	mustDecode(t, resp, &r)
	if r.Plan.Status != "pending" {
		t.Errorf("expected plan status=pending, got %s", r.Plan.Status)
	}
	if r.JobID == "" {
		t.Error("expected non-empty job_id")
	}

	// Проверяем плана и job в БД.
	var planStatus string
	if err := testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", r.Plan.ID,
	).Scan(&planStatus); err != nil {
		t.Fatalf("plan select: %v", err)
	}
	if planStatus != "pending" {
		t.Errorf("plan status in DB = %s, want pending", planStatus)
	}

	var jobType string
	if err := testPool.QueryRow(context.Background(),
		"SELECT job_type FROM background_jobs WHERE id=$1", r.JobID,
	).Scan(&jobType); err != nil {
		t.Fatalf("job select: %v", err)
	}
	if jobType != "price_recalculation" {
		t.Errorf("job_type=%s, want price_recalculation", jobType)
	}
}

func TestPricing_Recalculate_WrongShop_404(t *testing.T) {
	truncate(t)
	client := loginUser(t, "rec2@example.com", "ValidPass123!")

	resp := doJSON(t, client, http.MethodPost, "/api/pricing/recalculate", map[string]any{
		"shop_id": uuid.New().String(),
	}, withOrigin())
	mustStatus(t, resp, http.StatusNotFound)
}

// ─── ExecuteRecalcJob (worker direct) ───────────────────────────────────────

func TestPricing_ExecuteRecalcJob_AppliesItemsAndStatus(t *testing.T) {
	truncate(t)
	client := loginUser(t, "exec1@example.com", "ValidPass123!")
	userID := getUserID(t, "exec1@example.com")

	cost := 500.0
	shopID, productID := createShopAndProduct(t, "exec1@example.com", 900, &cost)
	strategyID := createStrategyDirect(t, client, "fix-exec", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	// 1. Создаём plan через Recalculate.
	plan, job, err := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	if err != nil {
		t.Fatalf("Recalculate: %v", err)
	}

	// 2. Симулируем worker: вызываем ExecuteRecalcJob напрямую.
	if err := testPricingSvc.ExecuteRecalcJob(context.Background(), job); err != nil {
		t.Fatalf("ExecuteRecalcJob: %v", err)
	}

	// 3. Проверяем что план переведён в calculated (расчёт окончен; dispatch — Этап 6).
	var status string
	if err := testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", plan.ID,
	).Scan(&status); err != nil {
		t.Fatalf("plan status: %v", err)
	}
	if status != domain.PlanStatusCalculated {
		t.Errorf("plan status=%s, want calculated", status)
	}

	// 4. Проверяем что есть item с правильной final_price.
	var (
		finalPrice    float64
		itemStatus    string
		constraintHit string
	)
	if err := testPool.QueryRow(context.Background(),
		"SELECT final_price::float8, status::text, constraint_hit FROM price_plan_items WHERE plan_id=$1 AND product_id=$2",
		plan.ID, productID,
	).Scan(&finalPrice, &itemStatus, &constraintHit); err != nil {
		t.Fatalf("item select: %v", err)
	}
	// fixed=700, cost=500 → 700>=500, нет cost_floor; final=700.
	if finalPrice != 700 || itemStatus != "pending" || constraintHit != "" {
		t.Errorf("item: final=%v status=%s hit=%s; want final=700/pending/empty", finalPrice, itemStatus, constraintHit)
	}
}

func TestPricing_ExecuteRecalcJob_CostFloorApplied(t *testing.T) {
	truncate(t)
	client := loginUser(t, "exec2@example.com", "ValidPass123!")
	userID := getUserID(t, "exec2@example.com")

	cost := 600.0
	shopID, productID := createShopAndProduct(t, "exec2@example.com", 900, &cost)
	// fixed=400, cost=600 → cost_price_floor поднимет до 600
	strategyID := createStrategyDirect(t, client, "fix-floor", "fixed",
		map[string]any{"value": 400}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	plan, job, err := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	if err != nil {
		t.Fatalf("Recalculate: %v", err)
	}
	if err := testPricingSvc.ExecuteRecalcJob(context.Background(), job); err != nil {
		t.Fatalf("ExecuteRecalcJob: %v", err)
	}

	var (
		finalPrice    float64
		constraintHit string
	)
	if err := testPool.QueryRow(context.Background(),
		"SELECT final_price::float8, constraint_hit FROM price_plan_items WHERE plan_id=$1",
		plan.ID,
	).Scan(&finalPrice, &constraintHit); err != nil {
		t.Fatalf("item select: %v", err)
	}
	if finalPrice != 600 || constraintHit != "cost_price_floor" {
		t.Errorf("expected final=600/cost_price_floor, got %v/%s", finalPrice, constraintHit)
	}
}

// ─── Plans listing & tenant ─────────────────────────────────────────────────

func TestPricing_GetPlan_Tenant_404(t *testing.T) {
	truncate(t)
	clientA := loginUser(t, "ta@example.com", "ValidPass123!")
	clientB := loginUser(t, "tb@example.com", "ValidPass123!")

	shopID, productID := createShopAndProduct(t, "ta@example.com", 900, nil)
	strategyID := createStrategyDirect(t, clientA, "ta-fix", "fixed",
		map[string]any{"value": 500}, map[string]any{})
	assignStrategy(t, clientA, strategyID, productID.String())

	resp := doJSON(t, clientA, http.MethodPost, "/api/pricing/recalculate", map[string]any{
		"shop_id": shopID.String(),
	}, withOrigin())
	mustStatus(t, resp, http.StatusAccepted)
	var rr struct {
		Plan struct{ ID string `json:"id"` } `json:"plan"`
	}
	mustDecode(t, resp, &rr)

	// Чужой пользователь не должен видеть план.
	resp = doJSON(t, clientB, http.MethodGet, "/api/price-plans/"+rr.Plan.ID, nil)
	mustStatus(t, resp, http.StatusNotFound)
}

func TestPricing_ListPlans_OnlyOwn(t *testing.T) {
	truncate(t)
	clientA := loginUser(t, "la@example.com", "ValidPass123!")
	clientB := loginUser(t, "lb@example.com", "ValidPass123!")
	userIDA := getUserID(t, "la@example.com")

	// User A создаёт магазин + план.
	shopID, productID := createShopAndProduct(t, "la@example.com", 900, nil)
	strategyID := createStrategyDirect(t, clientA, "la-fix", "fixed",
		map[string]any{"value": 500}, map[string]any{})
	assignStrategy(t, clientA, strategyID, productID.String())

	if _, _, err := testPricingSvc.Recalculate(context.Background(), userIDA, shopID, nil); err != nil {
		t.Fatalf("Recalculate: %v", err)
	}

	// User B видит пустой список.
	resp := doJSON(t, clientB, http.MethodGet, "/api/price-plans", nil)
	mustStatus(t, resp, http.StatusOK)
	var listB struct{ Total int `json:"total"` }
	mustDecode(t, resp, &listB)
	if listB.Total != 0 {
		t.Errorf("user B sees %d plans, want 0", listB.Total)
	}

	// User A видит свой план.
	resp = doJSON(t, clientA, http.MethodGet, "/api/price-plans", nil)
	mustStatus(t, resp, http.StatusOK)
	var listA struct{ Total int `json:"total"` }
	mustDecode(t, resp, &listA)
	if listA.Total != 1 {
		t.Errorf("user A sees %d plans, want 1", listA.Total)
	}
}

func TestPricing_ExecuteRecalcJob_MinIntervalSkips(t *testing.T) {
	// Дважды подряд запускаем recalculate с min_interval_minutes=60.
	// Первый раз — товар обрабатывается, второй раз — skip(min_interval_not_elapsed).
	truncate(t)
	client := loginUser(t, "minint@example.com", "ValidPass123!")
	userID := getUserID(t, "minint@example.com")

	shopID, productID := createShopAndProduct(t, "minint@example.com", 900, nil)
	body := map[string]any{
		"name": "fix-interval", "type": "fixed",
		"params":         map[string]any{"value": 500},
		"constraints":    map[string]any{"min_interval_minutes": 60},
		"fallbackPolicy": "keep_current", "enabled": true,
	}
	resp := doJSON(t, client, http.MethodPost, "/api/strategies", body, withOrigin())
	mustStatus(t, resp, http.StatusCreated)
	var s struct{ ID string `json:"id"` }
	mustDecode(t, resp, &s)
	assignStrategy(t, client, s.ID, productID.String())

	// Первый recalc — товар должен быть посчитан.
	plan1, job1, err := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	if err != nil {
		t.Fatalf("Recalculate 1: %v", err)
	}
	if err := testPricingSvc.ExecuteRecalcJob(context.Background(), job1); err != nil {
		t.Fatalf("ExecuteRecalcJob 1: %v", err)
	}

	var first struct {
		FinalPrice float64
		Status     string
	}
	_ = testPool.QueryRow(context.Background(),
		"SELECT final_price::float8, status::text FROM price_plan_items WHERE plan_id=$1",
		plan1.ID,
	).Scan(&first.FinalPrice, &first.Status)
	if first.FinalPrice != 500 || first.Status != "pending" {
		t.Fatalf("первый recalc: got %+v, want final=500/pending", first)
	}

	// Второй recalc — должен пропустить.
	plan2, job2, err := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	if err != nil {
		t.Fatalf("Recalculate 2: %v", err)
	}
	if err := testPricingSvc.ExecuteRecalcJob(context.Background(), job2); err != nil {
		t.Fatalf("ExecuteRecalcJob 2: %v", err)
	}

	var second struct {
		Status        string
		ConstraintHit string
		ErrorText     string
	}
	if err := testPool.QueryRow(context.Background(),
		"SELECT status::text, constraint_hit, error FROM price_plan_items WHERE plan_id=$1",
		plan2.ID,
	).Scan(&second.Status, &second.ConstraintHit, &second.ErrorText); err != nil {
		t.Fatalf("второй recalc: %v", err)
	}
	if second.Status != "skipped" || second.ConstraintHit != "min_interval_minutes" {
		t.Errorf("второй recalc: got %+v, want skipped/min_interval_minutes", second)
	}
	if !contains(second.ErrorText, "min_interval_not_elapsed") {
		t.Errorf("error должен содержать min_interval_not_elapsed, got %s", second.ErrorText)
	}
}

func TestPricing_ExecuteRecalcJob_NoStrategyProductSkipped(t *testing.T) {
	// Товар без стратегии не должен попасть в plan items.
	truncate(t)
	loginUser(t, "noassign@example.com", "ValidPass123!")
	userID := getUserID(t, "noassign@example.com")

	shopID, _ := createShopAndProduct(t, "noassign@example.com", 900, nil)

	plan, job, err := testPricingSvc.Recalculate(context.Background(), userID, shopID, nil)
	if err != nil {
		t.Fatalf("Recalculate: %v", err)
	}
	if err := testPricingSvc.ExecuteRecalcJob(context.Background(), job); err != nil {
		t.Fatalf("ExecuteRecalcJob: %v", err)
	}

	var count int
	if err := testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM price_plan_items WHERE plan_id=$1", plan.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 items (no strategy), got %d", count)
	}

	// Plan всё равно должен стать calculated (нет items для отправки).
	var status string
	_ = testPool.QueryRow(context.Background(), "SELECT status::text FROM price_plans WHERE id=$1", plan.ID).Scan(&status)
	if status != domain.PlanStatusCalculated {
		t.Errorf("plan status=%s, want calculated", status)
	}
}

// ─── BackgroundJobs Enqueue (smoke) ─────────────────────────────────────────

// TestPricing_PriceSync — проверяет, что при stale last_synced_at
// worker вызывает ListSKUs через MarketplaceFactory и обновляет current_price.
//
// В integration tests testPricingSvc создаётся БЕЗ WithPriceSync — поэтому здесь
// мы создаём отдельный pricingSvc с фейковой factory, передающей stale продукты.
func TestPricing_PriceSync(t *testing.T) {
	truncate(t)
	client := loginUser(t, "sync@example.com", "ValidPass123!")
	userID := getUserID(t, "sync@example.com")

	shopID, productID := createShopAndProduct(t, "sync@example.com", 900, nil)
	// Делаем last_synced_at=2 часа назад → stale (cutoff 60 мин).
	if _, err := testPool.Exec(context.Background(),
		"UPDATE products SET last_synced_at = NOW() - INTERVAL '2 hours' WHERE id=$1", productID,
	); err != nil {
		t.Fatalf("set last_synced_at: %v", err)
	}
	// Также шифруем фейковые credentials в shop, чтобы decrypt работал.
	encrypted, err := testEncrypt([]byte(`{"api_key":"test"}`), testShopSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := testPool.Exec(context.Background(),
		"UPDATE shops SET credentials_encrypted=$1 WHERE id=$2", encrypted, shopID,
	); err != nil {
		t.Fatalf("update shop creds: %v", err)
	}

	strategyID := createStrategyDirect(t, client, "sync-fix", "fixed",
		map[string]any{"value": 500}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	// Подменяем тестовые SKUs которые fakeMarketplace вернёт.
	// SKU должен совпадать с external_sku товара, чтобы upsert обновил его.
	var existingSKU string
	_ = testPool.QueryRow(context.Background(),
		"SELECT external_sku FROM products WHERE id=$1", productID,
	).Scan(&existingSKU)
	testSKUs = []integrationSKU{
		{ExternalSKU: existingSKU, Name: "Test Product", CurrentPrice: 1234.56, Currency: "RUB"},
	}

	// Создаём pricingSvc с включённым sync через fakeMarketplace.
	syncedSvc := pricingsvc.New(
		repository.NewProductsRepository(testPool),
		repository.NewStrategiesRepository(testPool),
		pricingsvc.WithCompetitors(repository.NewProductCompetitorsRepository(testPool)),
		pricingsvc.WithPlans(repository.NewPricePlansRepository(testPool)),
		pricingsvc.WithJobs(repository.NewBackgroundJobsRepository(testPool)),
		pricingsvc.WithShops(repository.NewShopsRepository(testPool)),
		pricingsvc.WithAssignments(repository.NewStrategyAssignmentsRepository(testPool)),
		pricingsvc.WithPriceSync(testShopSecret, map[string]pricingsvc.MarketplaceFactory{
			"wb": func(_ string, _ []byte) (integration.Marketplace, error) {
				return &fakeMarketplace{}, nil
			},
		}, 60*time.Minute),
	)

	plan, job, err := syncedSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	if err != nil {
		t.Fatalf("Recalculate: %v", err)
	}
	if err := syncedSvc.ExecuteRecalcJob(context.Background(), job); err != nil {
		t.Fatalf("ExecuteRecalcJob: %v", err)
	}

	// Проверяем что current_price обновилась после sync.
	var newPrice float64
	if err := testPool.QueryRow(context.Background(),
		"SELECT current_price::float8 FROM products WHERE id=$1", productID,
	).Scan(&newPrice); err != nil {
		t.Fatalf("query new price: %v", err)
	}
	if newPrice != 1234.56 {
		t.Errorf("expected current_price=1234.56 after sync, got %v", newPrice)
	}

	// План должен быть calculated (Этап 6 не вызывался).
	var planStatus string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", plan.ID,
	).Scan(&planStatus)
	if planStatus != domain.PlanStatusCalculated {
		t.Errorf("plan status=%s, want calculated", planStatus)
	}
}

func TestPricing_BackgroundJobsEnqueueSmoke(t *testing.T) {
	truncate(t)
	jobsRepo := repository.NewBackgroundJobsRepository(testPool)

	job, err := jobsRepo.Enqueue(context.Background(), repository.BackgroundJobEnqueue{
		JobType: "test_smoke", Queue: "default", Priority: 5,
		Payload: []byte(`{"x":1}`), MaxAttempts: 2,
		RunAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if job.JobType != "test_smoke" || job.Status != domain.BackgroundJobStatusPending || job.MaxAttempts != 2 {
		t.Errorf("unexpected job: %+v", job)
	}
	var payload string
	_ = testPool.QueryRow(context.Background(),
		"SELECT payload::text FROM background_jobs WHERE id=$1", job.ID,
	).Scan(&payload)
	if !contains(payload, `"x"`) {
		t.Errorf("payload missing: %s", payload)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ensure unused imports are not flagged
var _ = json.RawMessage{}
