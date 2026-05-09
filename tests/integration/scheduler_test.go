//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/dblock"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/Beliashkoff/RepricerX/internal/scheduler"
	"github.com/google/uuid"
)

// newTestScheduler — собирает scheduler.Service на test-pool с testPricingSvc.
func newTestScheduler(t *testing.T) *scheduler.Service {
	t.Helper()
	silent := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return scheduler.New(scheduler.Deps{
		Pool:           testPool,
		Shops:          repository.NewShopsRepository(testPool),
		Sessions:       repository.NewSessionsRepository(testPool),
		Verifications:  repository.NewEmailVerificationsRepository(testPool),
		PasswordResets: repository.NewPasswordResetTokensRepository(testPool),
		IntegrationLog: repository.NewIntegrationLogRepository(testPool),
		PriceChanges:   repository.NewPriceChangesRepository(testPool),
		Competitors:    repository.NewProductCompetitorsRepository(testPool),
		Jobs:           repository.NewBackgroundJobsRepository(testPool),
		Pricing:        testPricingSvc,
		Log:            silent,
	})
}

// ─── 1. /run-now happy path ─────────────────────────────────────────────────

func TestScheduler_RunNow_HappyPath(t *testing.T) {
	truncate(t)
	client := loginUser(t, "runnow1@example.com", "ValidPass123!")

	shopID, productID := createShopAndProduct(t, "runnow1@example.com", 900, nil)
	encryptShopCreds(t, shopID)
	stratID := createStrategyDirect(t, client, "rn-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, stratID, productID.String())

	resp := doJSON(t, client, http.MethodPost,
		"/api/shops/"+shopID.String()+"/run-now", nil, withOrigin())
	mustStatus(t, resp, http.StatusAccepted)

	var body struct {
		PlanID string `json:"plan_id"`
		JobID  string `json:"job_id"`
	}
	mustDecode(t, resp, &body)
	if body.PlanID == "" || body.JobID == "" {
		t.Errorf("expected non-empty plan_id and job_id, got %+v", body)
	}

	// Plan создан.
	var status string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", body.PlanID,
	).Scan(&status)
	if status != domain.PlanStatusPending {
		t.Errorf("plan status=%s, want pending", status)
	}
}

// ─── 2. /run-now tenant mismatch ────────────────────────────────────────────

func TestScheduler_RunNow_TenantMismatch(t *testing.T) {
	truncate(t)
	clientA := loginUser(t, "rn-tenant-a@example.com", "ValidPass123!")
	clientB := loginUser(t, "rn-tenant-b@example.com", "ValidPass123!")
	_ = clientA

	shopID, _ := createShopAndProduct(t, "rn-tenant-a@example.com", 900, nil)

	// User B пытается запустить пересчёт магазина user A.
	resp := doJSON(t, clientB, http.MethodPost,
		"/api/shops/"+shopID.String()+"/run-now", nil, withOrigin())
	mustStatus(t, resp, http.StatusNotFound)
}

// ─── 3. ScheduledRecalc atomicity (две горутины) ────────────────────────────

func TestScheduler_ScheduledRecalc_TouchAtomicity(t *testing.T) {
	truncate(t)
	client := loginUser(t, "atom@example.com", "ValidPass123!")

	shopID, productID := createShopAndProduct(t, "atom@example.com", 900, nil)
	stratID := createStrategyDirect(t, client, "atom-fix", "fixed",
		map[string]any{"value": 700}, map[string]any{})
	assignStrategy(t, client, stratID, productID.String())

	// schedule_cron='* * * * *', last_recalc_at IS NULL → cron должен сработать.
	// Сдвигаем created_at на 10 мин назад, чтобы spec.Next(created_at) дал прошедшее время.
	if _, err := testPool.Exec(context.Background(),
		"UPDATE shops SET schedule_cron='* * * * *', created_at=NOW() - INTERVAL '10 minutes' WHERE id=$1", shopID,
	); err != nil {
		t.Fatalf("set cron: %v", err)
	}

	sched := newTestScheduler(t)

	// Запускаем 5 параллельных тиков (имитация multi-replica).
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sched.ScheduledRecalcTick(context.Background())
		}()
	}
	wg.Wait()

	// Ровно один recalc-job должен быть enqueue-нут.
	var n int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM background_jobs WHERE job_type='price_recalculation'",
	).Scan(&n)
	if n != 1 {
		t.Errorf("dispatched=%d, want 1 (CAS-protected)", n)
	}

	// last_recalc_at установлен.
	var lastRecalc *time.Time
	_ = testPool.QueryRow(context.Background(),
		"SELECT last_recalc_at FROM shops WHERE id=$1", shopID,
	).Scan(&lastRecalc)
	if lastRecalc == nil {
		t.Error("last_recalc_at не установлен")
	}
}

// ─── 4. CleanupHourly: старые удаляются, свежие остаются ────────────────────

func TestScheduler_CleanupHourly_RealRun(t *testing.T) {
	truncate(t)
	client := loginUser(t, "cleanup@example.com", "ValidPass123!")
	_ = client

	shopID, productID := createShopAndProduct(t, "cleanup@example.com", 900, nil)

	// 2 старые price_change_log (200 дней назад) + 1 свежая.
	old := time.Now().UTC().Add(-200 * 24 * time.Hour)
	for i := 0; i < 2; i++ {
		_, _ = testPool.Exec(context.Background(), `
			INSERT INTO price_change_log (shop_id, product_id, old_price, new_price, target_price, status, correlation_id, created_at)
			VALUES ($1, $2, 100, 90, 90, 'applied'::plan_item_status, gen_random_uuid(), $3)`,
			shopID, productID, old)
	}
	_, _ = testPool.Exec(context.Background(), `
		INSERT INTO price_change_log (shop_id, product_id, old_price, new_price, target_price, status, correlation_id, created_at)
		VALUES ($1, $2, 100, 90, 90, 'applied'::plan_item_status, gen_random_uuid(), NOW())`,
		shopID, productID)

	// 2 старые integration_log (40 дней назад).
	for i := 0; i < 2; i++ {
		_, _ = testPool.Exec(context.Background(), `
			INSERT INTO integration_log (shop_id, operation, error_text, correlation_id, created_at)
			VALUES ($1, 'test_op', '', gen_random_uuid(), NOW() - INTERVAL '40 days')`,
			shopID)
	}

	sched := newTestScheduler(t)
	sched.CleanupHourlyTick(context.Background())

	// price_change_log: 1 осталась (свежая).
	var pcCount int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM price_change_log WHERE product_id=$1", productID,
	).Scan(&pcCount)
	if pcCount != 1 {
		t.Errorf("price_change_log remaining=%d, want 1", pcCount)
	}

	// integration_log: 0 осталось (все старше 30d).
	var ilCount int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM integration_log WHERE shop_id=$1", shopID,
	).Scan(&ilCount)
	if ilCount != 0 {
		t.Errorf("integration_log remaining=%d, want 0", ilCount)
	}
}

// ─── 5. CompetitorRefreshTick → enqueues jobs ───────────────────────────────

func TestScheduler_CompetitorRefreshTick_EnqueuesJobs(t *testing.T) {
	truncate(t)
	client := loginUser(t, "compref@example.com", "ValidPass123!")
	_ = client

	shopID, productID := createShopAndProduct(t, "compref@example.com", 900, nil)
	_ = shopID

	// 3 product_competitors (ozon) с last_checked_at=NULL → попадут в stale.
	for i := 0; i < 3; i++ {
		idStr := fmt.Sprintf("%d", i+1)
		if _, err := testPool.Exec(context.Background(), `
			INSERT INTO product_competitors (
				id, product_id, marketplace, source,
				competitor_url, normalized_competitor_url,
				ozon_public_product_id, last_status, created_at, updated_at
			) VALUES (
				gen_random_uuid(), $1, 'ozon', 'public_ozon',
				'https://ozon.ru/p/' || $2, 'p/' || $2,
				$2, 'pending', NOW(), NOW()
			)`,
			productID, idStr,
		); err != nil {
			t.Fatalf("insert competitor %d: %v", i, err)
		}
	}

	// Sanity check: записи действительно в БД.
	var preCount int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM product_competitors WHERE product_id=$1", productID,
	).Scan(&preCount)
	if preCount != 3 {
		t.Fatalf("seeded competitors=%d, want 3", preCount)
	}

	sched := newTestScheduler(t)
	sched.CompetitorRefreshTick(context.Background())

	var n int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM background_jobs WHERE job_type='competitor_refresh'",
	).Scan(&n)
	if n != 3 {
		t.Errorf("competitor_refresh jobs=%d, want 3", n)
	}
}

// ─── 6. Advisory lock освобождается ─────────────────────────────────────────

func TestScheduler_AdvisoryLockReleased(t *testing.T) {
	truncate(t)
	sched := newTestScheduler(t)

	// 1-й tick — lock берётся и освобождается.
	sched.CleanupHourlyTick(context.Background())

	// Сразу после — мы должны мочь снова взять lock с тем же ID.
	got, release, err := dblock.TryAcquire(context.Background(), testPool, dblock.LockIDCleanupHourly)
	if err != nil {
		t.Fatalf("TryAcquire after tick: %v", err)
	}
	defer func() { _ = release() }()
	if !got {
		t.Error("lock не освободился после tick")
	}
}

// ─── 7. CompetitorRefreshTick: lock taken → skip ─────────────────────────────

func TestScheduler_CompetitorRefresh_LockTaken_Skip(t *testing.T) {
	truncate(t)
	client := loginUser(t, "compref-lock@example.com", "ValidPass123!")
	_ = client

	shopID, productID := createShopAndProduct(t, "compref-lock@example.com", 900, nil)
	_ = shopID

	// 2 product_competitors.
	for i := 0; i < 2; i++ {
		idStr := fmt.Sprintf("%d", i+10)
		_, _ = testPool.Exec(context.Background(), `
			INSERT INTO product_competitors (
				id, product_id, marketplace, source,
				competitor_url, normalized_competitor_url,
				ozon_public_product_id, last_status, created_at, updated_at
			) VALUES (
				gen_random_uuid(), $1, 'ozon', 'public_ozon',
				'https://ozon.ru/p/' || $2, 'p/' || $2,
				$2, 'pending', NOW(), NOW()
			)`,
			productID, idStr)
	}

	// Берём lock в тестовом коде — scheduler не должен enqueue.
	got, release, err := dblock.TryAcquire(context.Background(), testPool, dblock.LockIDCompetitorRefresh)
	if err != nil || !got {
		t.Fatalf("test setup: cannot acquire lock: ok=%v err=%v", got, err)
	}
	defer func() { _ = release() }()

	sched := newTestScheduler(t)
	sched.CompetitorRefreshTick(context.Background())

	// Никаких jobs не должно быть.
	var n int
	_ = testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM background_jobs WHERE job_type='competitor_refresh'",
	).Scan(&n)
	if n != 0 {
		t.Errorf("competitor_refresh jobs=%d, want 0 (lock held)", n)
	}
}

// ─── 8. StalePlanCleanup ────────────────────────────────────────────────────

func TestScheduler_StalePlanCleanup(t *testing.T) {
	truncate(t)
	client := loginUser(t, "stale@example.com", "ValidPass123!")
	_ = client

	shopID, _ := createShopAndProduct(t, "stale@example.com", 900, nil)

	// Создаём план старше 24ч с status=pending.
	stalePlanID := uuid.New()
	old := time.Now().UTC().Add(-30 * time.Hour)
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO price_plans (id, shop_id, status, created_at, updated_at)
		VALUES ($1, $2, 'pending'::plan_status, $3, $3)`,
		stalePlanID, shopID, old,
	); err != nil {
		t.Fatalf("seed stale plan: %v", err)
	}

	sched := newTestScheduler(t)
	sched.StalePlanCleanupTick(context.Background())

	var status string
	_ = testPool.QueryRow(context.Background(),
		"SELECT status::text FROM price_plans WHERE id=$1", stalePlanID,
	).Scan(&status)
	if status != "cancelled" {
		t.Errorf("stale plan status=%s, want cancelled", status)
	}
}
