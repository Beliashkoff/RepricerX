//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// setShopAutoUpdate переключает auto_update_enabled у магазина в БД.
func setShopAutoUpdate(t *testing.T, shopID uuid.UUID, enabled bool) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		"UPDATE shops SET auto_update_enabled=$1, updated_at=NOW() WHERE id=$2", enabled, shopID,
	); err != nil {
		t.Fatalf("set auto_update_enabled: %v", err)
	}
}

// encryptShopCreds пишет валидные encrypted credentials для магазина (нужно для dispatch).
func encryptShopCreds(t *testing.T, shopID uuid.UUID) {
	t.Helper()
	enc, err := testEncrypt([]byte(`{"api_key":"test"}`), testShopSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := testPool.Exec(context.Background(),
		"UPDATE shops SET credentials_encrypted=$1 WHERE id=$2", enc, shopID,
	); err != nil {
		t.Fatalf("set creds: %v", err)
	}
}

// pollPlanStatus ждёт пока план не достигнет ожидаемого статуса (max 5 секунд).
func pollPlanStatus(t *testing.T, planID uuid.UUID, expectedStatus string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var s string
		_ = testPool.QueryRow(context.Background(),
			"SELECT status::text FROM price_plans WHERE id=$1", planID,
		).Scan(&s)
		if s == expectedStatus {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	var s string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", planID,
	).Scan(&s)
	return s
}

// ─── Dispatcher integration tests ───────────────────────────────────────────

// I1: Auto-flow happy path.
// shop.auto_update_enabled=true → recalc job → calculated → auto-enqueue dispatch
// → ExecuteDispatchJob → applied; price_change_log заполнен; integration_log заполнен.
func TestDispatcher_AutoFlow_HappyPath(t *testing.T) {
	truncate(t)
	client := loginUser(t, "auto1@example.com", "ValidPass123!")
	userID := getUserID(t, "auto1@example.com")

	shopID, productID := createShopAndProduct(t, "auto1@example.com", 900, nil)
	setShopAutoUpdate(t, shopID, true)
	encryptShopCreds(t, shopID)

	strategyID := createStrategyDirect(t, client, "auto-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	// Recalc.
	plan, recalcJob, err := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	if err != nil {
		t.Fatalf("Recalculate: %v", err)
	}
	if err := testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob); err != nil {
		t.Fatalf("ExecuteRecalcJob: %v", err)
	}

	// Auto-flow: после ExecuteRecalcJob должен появиться dispatch-job.
	var dispatchJobsCount int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM background_jobs WHERE job_type='price_dispatch'",
	).Scan(&dispatchJobsCount)
	if dispatchJobsCount != 1 {
		t.Fatalf("dispatch jobs=%d, want 1 (auto-enqueued)", dispatchJobsCount)
	}

	// Plan должен быть в dispatching после auto-enqueue.
	var status string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", plan.ID,
	).Scan(&status)
	if status != domain.PlanStatusDispatching {
		t.Errorf("after auto-enqueue status=%s, want dispatching", status)
	}

	// Симулируем worker: вызываем ExecuteDispatchJob напрямую с этим job-ом.
	var dispatchJob domain.BackgroundJob
	if err := testPool.QueryRow(context.Background(), `
		SELECT id, job_type, status::text, queue, priority, payload, result,
			attempts, max_attempts, run_at, locked_at, locked_by, lock_expires_at,
			last_error, created_at, updated_at, started_at, finished_at, canceled_at
		FROM background_jobs WHERE job_type='price_dispatch' LIMIT 1`,
	).Scan(&dispatchJob.ID, &dispatchJob.JobType, &dispatchJob.Status, &dispatchJob.Queue,
		&dispatchJob.Priority, &dispatchJob.Payload, &dispatchJob.Result,
		&dispatchJob.Attempts, &dispatchJob.MaxAttempts, &dispatchJob.RunAt,
		&dispatchJob.LockedAt, &dispatchJob.LockedBy, &dispatchJob.LockExpiresAt,
		&dispatchJob.LastError, &dispatchJob.CreatedAt, &dispatchJob.UpdatedAt,
		&dispatchJob.StartedAt, &dispatchJob.FinishedAt, &dispatchJob.CanceledAt); err != nil {
		t.Fatalf("load dispatch job: %v", err)
	}

	if err := testDispatcherSvc.ExecuteDispatchJob(context.Background(), &dispatchJob); err != nil {
		t.Fatalf("ExecuteDispatchJob: %v", err)
	}

	// Plan should be applied.
	if final := pollPlanStatus(t, plan.ID, domain.PlanStatusApplied); final != domain.PlanStatusApplied {
		t.Errorf("final plan status=%s, want applied", final)
	}

	// Item: dispatched, final_price=700.
	var (
		itemStatus string
		finalPrice float64
	)
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text, final_price::float8 FROM price_plan_items WHERE plan_id=$1",
		plan.ID,
	).Scan(&itemStatus, &finalPrice)
	if itemStatus != "dispatched" || finalPrice != 700 {
		t.Errorf("item status=%s final=%v; want dispatched/700", itemStatus, finalPrice)
	}

	// price_change_log.
	var pcCount int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM price_change_log WHERE product_id=$1", productID,
	).Scan(&pcCount)
	if pcCount != 1 {
		t.Errorf("price_change_log=%d, want 1", pcCount)
	}

	// integration_log: одна запись с http_status=200.
	var intCount int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM integration_log WHERE shop_id=$1 AND operation='price_dispatch' AND http_status=200",
		shopID,
	).Scan(&intCount)
	if intCount != 1 {
		t.Errorf("integration_log=%d, want 1 with http_status=200", intCount)
	}

	// fakeMarketplace получил правильный payload.
	testUpdatePricesMu.Lock()
	defer testUpdatePricesMu.Unlock()
	if len(testUpdatePricesCalls) != 1 || testUpdatePricesCalls[0].NewPrice != 700 {
		t.Errorf("fakeMarketplace updates: %+v", testUpdatePricesCalls)
	}
}

// I2: Manual-flow — нет auto, dispatch-job не enqueue.
func TestDispatcher_ManualFlow_NoAutoEnqueue(t *testing.T) {
	truncate(t)
	client := loginUser(t, "manual1@example.com", "ValidPass123!")
	userID := getUserID(t, "manual1@example.com")

	shopID, productID := createShopAndProduct(t, "manual1@example.com", 900, nil)
	setShopAutoUpdate(t, shopID, false) // явно выключаем
	encryptShopCreds(t, shopID)

	strategyID := createStrategyDirect(t, client, "man-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	plan, recalcJob, _ := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	if err := testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob); err != nil {
		t.Fatalf("ExecuteRecalcJob: %v", err)
	}

	// Plan: calculated (нет dispatch).
	var status string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", plan.ID,
	).Scan(&status)
	if status != domain.PlanStatusCalculated {
		t.Errorf("status=%s, want calculated", status)
	}

	// Нет dispatch-job в БД.
	var n int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM background_jobs WHERE job_type='price_dispatch'",
	).Scan(&n)
	if n != 0 {
		t.Errorf("dispatch jobs=%d, want 0", n)
	}
}

// I3: Manual dispatch через HTTP endpoint.
func TestDispatcher_ManualDispatchEndpoint(t *testing.T) {
	truncate(t)
	httpClient := loginUser(t, "manual2@example.com", "ValidPass123!")
	userID := getUserID(t, "manual2@example.com")

	shopID, productID := createShopAndProduct(t, "manual2@example.com", 900, nil)
	encryptShopCreds(t, shopID)

	strategyID := createStrategyDirect(t, httpClient, "man-disp", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, httpClient, strategyID, productID.String())

	plan, recalcJob, _ := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	_ = testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob)

	// POST /api/price-plans/:id/dispatch.
	resp := doJSON(t, httpClient, http.MethodPost,
		"/api/price-plans/"+plan.ID.String()+"/dispatch", nil, withOrigin())
	mustStatus(t, resp, http.StatusAccepted)

	// Status → dispatching.
	var status string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", plan.ID,
	).Scan(&status)
	if status != domain.PlanStatusDispatching {
		t.Errorf("status=%s, want dispatching", status)
	}

	// Job enqueued.
	var n int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM background_jobs WHERE job_type='price_dispatch'",
	).Scan(&n)
	if n != 1 {
		t.Errorf("dispatch jobs=%d, want 1", n)
	}
}

// I4: tenant — чужой пользователь не может dispatch.
func TestDispatcher_Dispatch_TenantMismatch(t *testing.T) {
	truncate(t)
	clientA := loginUser(t, "ta-disp@example.com", "ValidPass123!")
	clientB := loginUser(t, "tb-disp@example.com", "ValidPass123!")
	userIDA := getUserID(t, "ta-disp@example.com")

	shopID, productID := createShopAndProduct(t, "ta-disp@example.com", 900, nil)
	encryptShopCreds(t, shopID)

	strategyID := createStrategyDirect(t, clientA, "ta-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, clientA, strategyID, productID.String())

	plan, recalcJob, _ := testPricingSvc.Recalculate(context.Background(), userIDA, shopID, []uuid.UUID{productID})
	_ = testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob)

	// User B не должен видеть/диспатчить чужой план.
	resp := doJSON(t, clientB, http.MethodPost,
		"/api/price-plans/"+plan.ID.String()+"/dispatch", nil, withOrigin())
	mustStatus(t, resp, http.StatusNotFound)

	respCancel := doJSON(t, clientB, http.MethodPost,
		"/api/price-plans/"+plan.ID.String()+"/cancel", nil, withOrigin())
	mustStatus(t, respCancel, http.StatusNotFound)
}

// I5: Unauthorized → fail-fast, items=failed, integration_log с error="unauthorized".
func TestDispatcher_Unauthorized_FailsAll(t *testing.T) {
	truncate(t)
	client := loginUser(t, "unauth@example.com", "ValidPass123!")
	userID := getUserID(t, "unauth@example.com")

	shopID, productID := createShopAndProduct(t, "unauth@example.com", 900, nil)
	encryptShopCreds(t, shopID)

	strategyID := createStrategyDirect(t, client, "u-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	// Программируем fakeMarketplace.UpdatePrices → ErrUnauthorized.
	testUpdatePricesMu.Lock()
	testUpdatePricesResults = []error{integration.ErrUnauthorized}
	testUpdatePricesMu.Unlock()

	plan, recalcJob, _ := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	_ = testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob)

	// Dispatch через service напрямую.
	dispJob, err := testDispatcherSvc.EnqueueDispatch(context.Background(), userID, plan.ID)
	if err != nil {
		t.Fatalf("EnqueueDispatch: %v", err)
	}

	// Auto-flow ИЛИ manual: либо тут уже job в очереди, либо явно вызвали EnqueueDispatch.
	// Подгружаем job из БД и выполняем.
	dispatchJob := loadDispatchJob(t, dispJob.ID)
	err = testDispatcherSvc.ExecuteDispatchJob(context.Background(), dispatchJob)
	// Должна вернуться ErrUnauthorized.
	if err == nil {
		t.Fatal("expected error")
	}

	// Plan: failed.
	if status := pollPlanStatus(t, plan.ID, domain.PlanStatusFailed); status != domain.PlanStatusFailed {
		t.Errorf("plan status=%s, want failed", status)
	}

	// Items: failed.
	var itemStatus string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plan_items WHERE plan_id=$1", plan.ID,
	).Scan(&itemStatus)
	if itemStatus != "failed" {
		t.Errorf("item status=%s, want failed", itemStatus)
	}

	// integration_log: запись с http_status=401 + error=unauthorized.
	var (
		httpStatus *int
		errText    string
	)
	_ = testPool.QueryRow(context.Background(),
		"SELECT http_status, error_text FROM integration_log WHERE shop_id=$1 AND operation='price_dispatch'",
		shopID,
	).Scan(&httpStatus, &errText)
	if httpStatus == nil || *httpStatus != 401 || errText != "unauthorized" {
		t.Errorf("integration_log: status=%v error=%s", httpStatus, errText)
	}

	// price_change_log: запись со status=failed.
	var pcStatus string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_change_log WHERE product_id=$1", productID,
	).Scan(&pcStatus)
	if pcStatus != "failed" {
		t.Errorf("price_change.status=%s, want failed", pcStatus)
	}
}

// I6: rate-limited retry, потом success.
func TestDispatcher_RateLimited_RetryThenSuccess(t *testing.T) {
	truncate(t)
	client := loginUser(t, "rl@example.com", "ValidPass123!")
	userID := getUserID(t, "rl@example.com")

	shopID, productID := createShopAndProduct(t, "rl@example.com", 900, nil)
	encryptShopCreds(t, shopID)

	strategyID := createStrategyDirect(t, client, "rl-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	// Программируем: сначала ErrRateLimited, потом nil.
	testUpdatePricesMu.Lock()
	testUpdatePricesResults = []error{integration.ErrRateLimited, nil}
	testUpdatePricesMu.Unlock()

	plan, recalcJob, _ := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	_ = testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob)

	dispJob, _ := testDispatcherSvc.EnqueueDispatch(context.Background(), userID, plan.ID)
	dispatchJob := loadDispatchJob(t, dispJob.ID)

	// Первый attempt → должен вернуть retryable error.
	if err := testDispatcherSvc.ExecuteDispatchJob(context.Background(), dispatchJob); err == nil {
		t.Fatal("first attempt: expected error")
	}
	// Plan остался в dispatching.
	var status string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", plan.ID,
	).Scan(&status)
	if status != domain.PlanStatusDispatching {
		t.Errorf("after first attempt status=%s, want dispatching", status)
	}

	// Второй attempt — успех.
	dispatchJob.Attempts = 2
	if err := testDispatcherSvc.ExecuteDispatchJob(context.Background(), dispatchJob); err != nil {
		t.Fatalf("second attempt: %v", err)
	}

	if final := pollPlanStatus(t, plan.ID, domain.PlanStatusApplied); final != domain.PlanStatusApplied {
		t.Errorf("final status=%s, want applied", final)
	}
}

// I7: cancel calculated plan через API.
func TestDispatcher_CancelCalculatedPlan(t *testing.T) {
	truncate(t)
	client := loginUser(t, "cancel1@example.com", "ValidPass123!")
	userID := getUserID(t, "cancel1@example.com")

	shopID, productID := createShopAndProduct(t, "cancel1@example.com", 900, nil)
	encryptShopCreds(t, shopID)
	strategyID := createStrategyDirect(t, client, "c-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	plan, recalcJob, _ := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	_ = testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob)
	// Status: calculated.

	// POST /cancel.
	resp := doJSON(t, client, http.MethodPost,
		"/api/price-plans/"+plan.ID.String()+"/cancel", nil, withOrigin())
	mustStatus(t, resp, http.StatusNoContent)

	var status string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", plan.ID,
	).Scan(&status)
	if status != domain.PlanStatusCancelled {
		t.Errorf("status=%s, want cancelled", status)
	}

	// Попытка dispatch отменённого плана → 409 plan_terminal.
	resp = doJSON(t, client, http.MethodPost,
		"/api/price-plans/"+plan.ID.String()+"/dispatch", nil, withOrigin())
	mustStatus(t, resp, http.StatusConflict)
}

// I8: cancel applied plan → 409.
func TestDispatcher_CancelTerminalPlan_409(t *testing.T) {
	truncate(t)
	client := loginUser(t, "ct@example.com", "ValidPass123!")
	userID := getUserID(t, "ct@example.com")

	shopID, productID := createShopAndProduct(t, "ct@example.com", 900, nil)
	encryptShopCreds(t, shopID)
	strategyID := createStrategyDirect(t, client, "ct-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	plan, recalcJob, _ := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	_ = testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob)
	// Manually push to applied.
	_, _ = testPool.Exec(context.Background(),
		"UPDATE price_plans SET status='applied'::plan_status WHERE id=$1", plan.ID)

	resp := doJSON(t, client, http.MethodPost,
		"/api/price-plans/"+plan.ID.String()+"/cancel", nil, withOrigin())
	mustStatus(t, resp, http.StatusConflict)
}

// I9: retention — DeleteOlderThan(180d).
func TestDispatcher_PriceChangeRetention(t *testing.T) {
	truncate(t)
	client := loginUser(t, "ret@example.com", "ValidPass123!")
	_ = client

	shopID, productID := createShopAndProduct(t, "ret@example.com", 900, nil)
	encryptShopCreds(t, shopID)

	// Вставляем 2 старые записи и 2 свежие.
	repo := repository.NewPriceChangesRepository(testPool)
	old := time.Now().UTC().Add(-200 * 24 * time.Hour)
	fresh := time.Now().UTC().Add(-1 * time.Hour)

	for i := 0; i < 2; i++ {
		_, _ = testPool.Exec(context.Background(), `
			INSERT INTO price_change_log (shop_id, product_id, old_price, new_price, target_price, status, correlation_id, created_at)
			VALUES ($1, $2, 100, 90, 90, 'applied'::plan_item_status, gen_random_uuid(), $3)`,
			shopID, productID, old)
	}
	for i := 0; i < 2; i++ {
		_, _ = testPool.Exec(context.Background(), `
			INSERT INTO price_change_log (shop_id, product_id, old_price, new_price, target_price, status, correlation_id, created_at)
			VALUES ($1, $2, 100, 90, 90, 'applied'::plan_item_status, gen_random_uuid(), $3)`,
			shopID, productID, fresh)
	}

	cutoff := time.Now().UTC().Add(-180 * 24 * time.Hour)
	deleted, err := repo.DeleteOlderThan(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted=%d, want 2", deleted)
	}

	var remaining int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM price_change_log WHERE product_id=$1", productID,
	).Scan(&remaining)
	if remaining != 2 {
		t.Errorf("remaining=%d, want 2", remaining)
	}
}

// I10: idempotency — retry job-а не отправляет уже dispatched items.
func TestDispatcher_RetryIdempotent(t *testing.T) {
	truncate(t)
	client := loginUser(t, "idemp@example.com", "ValidPass123!")
	userID := getUserID(t, "idemp@example.com")

	shopID, productID := createShopAndProduct(t, "idemp@example.com", 900, nil)
	encryptShopCreds(t, shopID)
	strategyID := createStrategyDirect(t, client, "id-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, strategyID, productID.String())

	plan, recalcJob, _ := testPricingSvc.Recalculate(context.Background(), userID, shopID, []uuid.UUID{productID})
	_ = testPricingSvc.ExecuteRecalcJob(context.Background(), recalcJob)
	dispJob, _ := testDispatcherSvc.EnqueueDispatch(context.Background(), userID, plan.ID)
	dispatchJob := loadDispatchJob(t, dispJob.ID)

	// Первая отправка — успех.
	if err := testDispatcherSvc.ExecuteDispatchJob(context.Background(), dispatchJob); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}

	testUpdatePricesMu.Lock()
	callsAfterFirst := len(testUpdatePricesCalls)
	testUpdatePricesMu.Unlock()
	if callsAfterFirst != 1 {
		t.Errorf("first dispatch calls=%d, want 1", callsAfterFirst)
	}

	// Симулируем повторный вызов того же job-а (retry-сценарий).
	dispatchJob.Attempts = 2
	if err := testDispatcherSvc.ExecuteDispatchJob(context.Background(), dispatchJob); err != nil {
		t.Fatalf("retry dispatch: %v", err)
	}

	// fakeMarketplace НЕ должен быть вызван повторно — ListItemsForDispatch вернул 0.
	testUpdatePricesMu.Lock()
	defer testUpdatePricesMu.Unlock()
	if len(testUpdatePricesCalls) != 1 {
		t.Errorf("after retry calls=%d, want 1 (idempotent — no new calls)", len(testUpdatePricesCalls))
	}
}

// loadDispatchJob — загружает job из БД по ID.
func loadDispatchJob(t *testing.T, id uuid.UUID) *domain.BackgroundJob {
	t.Helper()
	var j domain.BackgroundJob
	if err := testPool.QueryRow(context.Background(), `
		SELECT id, job_type, status::text, queue, priority, payload, result,
			attempts, max_attempts, run_at, locked_at, locked_by, lock_expires_at,
			last_error, created_at, updated_at, started_at, finished_at, canceled_at
		FROM background_jobs WHERE id=$1`, id,
	).Scan(&j.ID, &j.JobType, &j.Status, &j.Queue, &j.Priority,
		&j.Payload, &j.Result, &j.Attempts, &j.MaxAttempts, &j.RunAt,
		&j.LockedAt, &j.LockedBy, &j.LockExpiresAt, &j.LastError,
		&j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.FinishedAt, &j.CanceledAt,
	); err != nil {
		t.Fatalf("load dispatch job: %v", err)
	}
	return &j
}
