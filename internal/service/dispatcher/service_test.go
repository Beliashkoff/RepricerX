package dispatcher_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/pkg/crypto"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/Beliashkoff/RepricerX/internal/service/dispatcher"
	"github.com/google/uuid"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakePlansRepo struct {
	mu          sync.Mutex
	plans       map[uuid.UUID]*domain.PricePlan
	items       map[uuid.UUID][]*domain.PricePlanItem
	dispatchers map[uuid.UUID][]*repository.PricePlanItemForDispatch // by plan_id
	statuses    map[uuid.UUID]string
}

func newFakePlansRepo() *fakePlansRepo {
	return &fakePlansRepo{
		plans:       map[uuid.UUID]*domain.PricePlan{},
		items:       map[uuid.UUID][]*domain.PricePlanItem{},
		dispatchers: map[uuid.UUID][]*repository.PricePlanItemForDispatch{},
		statuses:    map[uuid.UUID]string{},
	}
}

func (r *fakePlansRepo) seedPlan(planID, userID, shopID uuid.UUID, status string, items []*repository.PricePlanItemForDispatch) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plans[planID] = &domain.PricePlan{ID: planID, ShopID: shopID, Status: status}
	r.dispatchers[planID] = items
	r.statuses[planID] = status
}

func (r *fakePlansRepo) Create(_ context.Context, shopID uuid.UUID) (*domain.PricePlan, error) {
	id := uuid.New()
	p := &domain.PricePlan{ID: id, ShopID: shopID, Status: domain.PlanStatusPending}
	r.plans[id] = p
	r.statuses[id] = p.Status
	return p, nil
}

func (r *fakePlansRepo) GetByIDForUser(_ context.Context, _, planID uuid.UUID) (*domain.PricePlan, []*domain.PricePlanItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[planID]
	if !ok {
		return nil, nil, repository.ErrNotFound
	}
	cp := *p
	cp.Status = r.statuses[planID]
	return &cp, r.items[planID], nil
}

func (r *fakePlansRepo) ListByUser(_ context.Context, _ uuid.UUID, _, _ int) ([]*domain.PricePlan, int, error) {
	return nil, 0, nil
}

func (r *fakePlansRepo) UpdateStatus(_ context.Context, planID uuid.UUID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.plans[planID]; !ok {
		return repository.ErrNotFound
	}
	r.statuses[planID] = status
	return nil
}

func (r *fakePlansRepo) InsertItems(_ context.Context, _ uuid.UUID, _ []*domain.PricePlanItem) error {
	return nil
}

func (r *fakePlansRepo) LatestItemCreatedAt(_ context.Context, _ uuid.UUID) (*time.Time, error) {
	return nil, nil
}

func (r *fakePlansRepo) ListItemsForDispatch(_ context.Context, planID uuid.UUID) ([]*repository.PricePlanItemForDispatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Возвращаем только items со status='pending'.
	all := r.dispatchers[planID]
	out := make([]*repository.PricePlanItemForDispatch, 0, len(all))
	for _, it := range all {
		if r.itemStatus(it.ItemID) == domain.PlanItemStatusPending {
			out = append(out, it)
		}
	}
	return out, nil
}

// itemStatuses + helper
var itemStatuses = sync.Map{}

func (r *fakePlansRepo) itemStatus(id uuid.UUID) string {
	if v, ok := itemStatuses.Load(id); ok {
		return v.(string)
	}
	return domain.PlanItemStatusPending
}

func (r *fakePlansRepo) UpdateItemAfterDispatch(_ context.Context, itemID uuid.UUID, status, _ string) error {
	itemStatuses.Store(itemID, status)
	return nil
}

func (r *fakePlansRepo) CountItemsByStatus(_ context.Context, planID uuid.UUID) (map[string]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	for _, it := range r.dispatchers[planID] {
		out[r.itemStatus(it.ItemID)]++
	}
	return out, nil
}

func (r *fakePlansRepo) ResolveOwnerAndShop(_ context.Context, _ uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	return uuid.Nil, uuid.Nil, nil
}

func (r *fakePlansRepo) TransitionStatus(_ context.Context, planID uuid.UUID, fromStatus, toStatus string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.statuses[planID] != fromStatus {
		return false, nil
	}
	r.statuses[planID] = toStatus
	return true, nil
}

// ──── другие fake repos ─────────────────────────────────────────────────────

type fakePriceChangesRepo struct {
	mu      sync.Mutex
	created []repository.PriceChangeCreate
}

func (r *fakePriceChangesRepo) ListForUser(_ context.Context, _ uuid.UUID, _ repository.PriceChangeFilter) ([]*domain.PriceChange, int, error) {
	return nil, 0, nil
}
func (r *fakePriceChangesRepo) ExportForUser(_ context.Context, _ uuid.UUID, _ repository.PriceChangeFilter) ([]*domain.PriceChange, error) {
	return nil, nil
}
func (r *fakePriceChangesRepo) SummaryForUser(_ context.Context, _ uuid.UUID, _ repository.PriceChangeFilter) (*domain.PriceChangeSummary, error) {
	return nil, nil
}
func (r *fakePriceChangesRepo) Create(_ context.Context, c repository.PriceChangeCreate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, c)
	return nil
}
func (r *fakePriceChangesRepo) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

type fakeIntLogRepo struct {
	mu      sync.Mutex
	entries []*domain.IntegrationLogEntry
}

func (r *fakeIntLogRepo) Create(_ context.Context, e *domain.IntegrationLogEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
	return nil
}
func (r *fakeIntLogRepo) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error) { return 0, nil }

type fakeShopsRepo struct {
	shops map[uuid.UUID]*domain.Shop
}

func (r *fakeShopsRepo) Create(_ context.Context, _ *domain.Shop) error { return nil }
func (r *fakeShopsRepo) GetByID(_ context.Context, id, _ uuid.UUID) (*domain.Shop, error) {
	s, ok := r.shops[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return s, nil
}
func (r *fakeShopsRepo) ListByUserID(_ context.Context, _ uuid.UUID) ([]*domain.Shop, error) {
	return nil, nil
}
func (r *fakeShopsRepo) Update(_ context.Context, _ *domain.Shop) error             { return nil }
func (r *fakeShopsRepo) Delete(_ context.Context, _, _ uuid.UUID) error             { return nil }
func (r *fakeShopsRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _ string, _ time.Time) error {
	return nil
}
func (r *fakeShopsRepo) ListSchedulable(_ context.Context) ([]*domain.Shop, error) { return nil, nil }
func (r *fakeShopsRepo) TouchLastRecalcAt(_ context.Context, _ uuid.UUID, _ *time.Time) (bool, error) {
	return true, nil
}

type fakeJobsRepo struct {
	enqueued []repository.BackgroundJobEnqueue
}

func (r *fakeJobsRepo) ClaimNext(_ context.Context, _, _ string, _ time.Duration) (*domain.BackgroundJob, error) {
	return nil, repository.ErrNotFound
}
func (r *fakeJobsRepo) Succeed(_ context.Context, _ uuid.UUID, _ []byte) error          { return nil }
func (r *fakeJobsRepo) Retry(_ context.Context, _ uuid.UUID, _ time.Time, _ string) error { return nil }
func (r *fakeJobsRepo) Fail(_ context.Context, _ uuid.UUID, _ string, _ []byte) error   { return nil }
func (r *fakeJobsRepo) Enqueue(_ context.Context, in repository.BackgroundJobEnqueue) (*domain.BackgroundJob, error) {
	r.enqueued = append(r.enqueued, in)
	return &domain.BackgroundJob{ID: uuid.New(), JobType: in.JobType, Status: domain.BackgroundJobStatusPending, Payload: in.Payload}, nil
}

// ──── fake marketplace ──────────────────────────────────────────────────────

type fakeMarketplace struct {
	mu       sync.Mutex
	updated  []integration.PriceUpdate
	results  []error // FIFO; nil = success; используется для последовательных вызовов
	resultIx int
}

func (m *fakeMarketplace) TestAuth(_ context.Context) error                   { return nil }
func (m *fakeMarketplace) ListSKUs(_ context.Context) ([]integration.SKU, error) { return nil, nil }
func (m *fakeMarketplace) UpdatePrices(_ context.Context, ups []integration.PriceUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updated = append(m.updated, ups...)
	if m.resultIx < len(m.results) {
		err := m.results[m.resultIx]
		m.resultIx++
		return err
	}
	return nil
}

// ──── helpers ───────────────────────────────────────────────────────────────

func resetItemStatuses() { itemStatuses = sync.Map{} }

func setupSvc(t *testing.T, fm *fakeMarketplace) (
	*dispatcher.Service,
	*fakePlansRepo,
	*fakePriceChangesRepo,
	*fakeIntLogRepo,
	*fakeShopsRepo,
	*fakeJobsRepo,
	uuid.UUID, // userID
	uuid.UUID, // shopID
	[]byte,    // creds (encrypted)
) {
	t.Helper()
	resetItemStatuses()

	plansR := newFakePlansRepo()
	pcR := &fakePriceChangesRepo{}
	intR := &fakeIntLogRepo{}
	jobsR := &fakeJobsRepo{}

	userID := uuid.New()
	shopID := uuid.New()
	secret := "test-secret-key-32-bytes-padding!"
	creds, err := crypto.Encrypt([]byte(`{"api_key":"x"}`), secret)
	if err != nil {
		t.Fatalf("encrypt creds: %v", err)
	}
	shopsR := &fakeShopsRepo{
		shops: map[uuid.UUID]*domain.Shop{
			shopID: {ID: shopID, UserID: userID, Marketplace: "wb",
				CredentialsEncrypted: creds, Status: domain.ShopStatusActive},
		},
	}

	factory := dispatcher.MarketplaceFactory(func(_ string, _ []byte) (integration.Marketplace, error) {
		return fm, nil
	})

	svc := dispatcher.New(plansR, nil, pcR, intR, shopsR, jobsR, secret,
		map[string]dispatcher.MarketplaceFactory{"wb": factory},
		dispatcher.WithChunkSize(100),
	)
	return svc, plansR, pcR, intR, shopsR, jobsR, userID, shopID, creds
}

func makeItems(n int) []*repository.PricePlanItemForDispatch {
	out := make([]*repository.PricePlanItemForDispatch, n)
	for i := 0; i < n; i++ {
		out[i] = &repository.PricePlanItemForDispatch{
			ItemID:        uuid.New(),
			ProductID:     uuid.New(),
			ExternalSKU:   "sku-" + uuid.NewString()[:8],
			CurrentPrice:  900,
			FinalPrice:    810 + float64(i),
			TargetPrice:   810,
			CorrelationID: uuid.New(),
		}
	}
	return out
}

func mkJob(planID, shopID, userID uuid.UUID, attempts int) *domain.BackgroundJob {
	payload, _ := json.Marshal(domain.PriceDispatchJobPayload{
		PlanID: planID, ShopID: shopID, RequestedByUserID: userID,
	})
	return &domain.BackgroundJob{
		ID: uuid.New(), JobType: domain.BackgroundJobTypePriceDispatch,
		Payload: payload, Attempts: attempts, MaxAttempts: 3,
	}
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestExecute_HappyPath(t *testing.T) {
	fm := &fakeMarketplace{} // success
	svc, plans, pc, intL, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	items := makeItems(5)
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, items)

	if err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plans.statuses[planID] != domain.PlanStatusApplied {
		t.Errorf("status=%s, want applied", plans.statuses[planID])
	}
	if len(fm.updated) != 5 {
		t.Errorf("updated=%d, want 5", len(fm.updated))
	}
	if len(pc.created) != 5 {
		t.Errorf("price_changes=%d, want 5", len(pc.created))
	}
	if len(intL.entries) != 1 {
		t.Errorf("integration_log entries=%d, want 1", len(intL.entries))
	}
	for _, c := range pc.created {
		if c.Status != domain.PlanItemStatusDispatched {
			t.Errorf("price_change.status=%s, want dispatched", c.Status)
		}
	}
}

func TestExecute_Unauthorized_FailFast(t *testing.T) {
	fm := &fakeMarketplace{results: []error{integration.ErrUnauthorized}}
	svc, plans, pc, intL, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	items := makeItems(5)
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, items)

	err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1))
	if !errors.Is(err, dispatcher.ErrUnauthorized) {
		t.Fatalf("err=%v, want ErrUnauthorized", err)
	}
	if plans.statuses[planID] != domain.PlanStatusFailed {
		t.Errorf("status=%s, want failed", plans.statuses[planID])
	}
	// Все 5 items должны быть failed.
	for _, it := range items {
		v, ok := itemStatuses.Load(it.ItemID)
		if !ok {
			t.Errorf("item %s not marked", it.ItemID)
		} else if v.(string) != domain.PlanItemStatusFailed {
			t.Errorf("item %s status=%v, want failed", it.ItemID, v)
		}
	}
	if len(pc.created) != 5 {
		t.Errorf("price_changes=%d, want 5 (all failed)", len(pc.created))
	}
	for _, c := range pc.created {
		if c.Status != domain.PlanItemStatusFailed {
			t.Errorf("price_change.status=%s, want failed", c.Status)
		}
	}
	if len(intL.entries) != 1 || intL.entries[0].ErrorText != "unauthorized" {
		t.Errorf("integration_log: %+v", intL.entries)
	}
}

func TestExecute_RateLimited_Retryable(t *testing.T) {
	fm := &fakeMarketplace{results: []error{integration.ErrRateLimited}}
	svc, plans, pc, intL, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	items := makeItems(5)
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, items)

	err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1))
	if err == nil || errors.Is(err, dispatcher.ErrUnauthorized) {
		t.Fatalf("err=%v, want retryable wrapped error (not Unauthorized)", err)
	}
	// Plan остаётся в dispatching (не финализирован).
	if plans.statuses[planID] != domain.PlanStatusDispatching {
		t.Errorf("status=%s, want dispatching", plans.statuses[planID])
	}
	// Items не трогали.
	if len(pc.created) != 0 {
		t.Errorf("price_changes=%d, want 0", len(pc.created))
	}
	if len(intL.entries) != 1 || intL.entries[0].ErrorText != "rate_limited" {
		t.Errorf("integration_log: %+v", intL.entries)
	}
}

func TestExecute_NetworkError_Retryable(t *testing.T) {
	fm := &fakeMarketplace{results: []error{errors.New("connection refused")}}
	svc, plans, _, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	items := makeItems(3)
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, items)

	err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1))
	if err == nil || errors.Is(err, dispatcher.ErrUnauthorized) {
		t.Fatalf("expected retryable error, got %v", err)
	}
	if plans.statuses[planID] != domain.PlanStatusDispatching {
		t.Errorf("status=%s, want dispatching (retryable)", plans.statuses[planID])
	}
}

func TestExecute_MultipleChunks_AllOk(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, intL, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	items := makeItems(250)
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, items)

	if err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1)); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(fm.updated) != 250 {
		t.Errorf("updated=%d, want 250", len(fm.updated))
	}
	if len(intL.entries) != 3 {
		t.Errorf("integration_log entries=%d, want 3", len(intL.entries))
	}
	if plans.statuses[planID] != domain.PlanStatusApplied {
		t.Errorf("status=%s, want applied", plans.statuses[planID])
	}
}

func TestExecute_FirstChunkOk_SecondRateLimited(t *testing.T) {
	fm := &fakeMarketplace{results: []error{nil, integration.ErrRateLimited}}
	svc, plans, pc, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	items := makeItems(200)
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, items)

	err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1))
	if err == nil {
		t.Fatal("expected retryable error")
	}
	if plans.statuses[planID] != domain.PlanStatusDispatching {
		t.Errorf("status=%s, want dispatching", plans.statuses[planID])
	}
	// Первый chunk должен быть dispatched (100 items + 100 price_change).
	if len(pc.created) != 100 {
		t.Errorf("price_changes=%d, want 100 (first chunk)", len(pc.created))
	}

	// Симулируем retry: вызываем ExecuteDispatchJob ещё раз — fakeMarketplace больше не вернёт ошибок.
	if err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 2)); err != nil {
		t.Fatalf("retry err: %v", err)
	}
	if plans.statuses[planID] != domain.PlanStatusApplied {
		t.Errorf("after retry status=%s, want applied", plans.statuses[planID])
	}
	if len(pc.created) != 200 {
		t.Errorf("price_changes after retry=%d, want 200", len(pc.created))
	}
}

func TestExecute_OnlySkipped_NoUpdatePrices(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, pc, intL, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	// Items со статусом skipped — не должны попасть в ListItemsForDispatch
	// (наш fake возвращает только pending). Создадим 0 pending → 0 для отправки.
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, nil)

	if err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1)); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(fm.updated) != 0 {
		t.Errorf("updated=%d, want 0", len(fm.updated))
	}
	if len(pc.created) != 0 {
		t.Errorf("price_changes=%d, want 0", len(pc.created))
	}
	if len(intL.entries) != 0 {
		t.Errorf("integration_log=%d, want 0", len(intL.entries))
	}
	if plans.statuses[planID] != domain.PlanStatusApplied {
		t.Errorf("status=%s, want applied", plans.statuses[planID])
	}
}

func TestExecute_TerminalPlan_NoOp(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusApplied, makeItems(5))

	if err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1)); err != nil {
		t.Fatalf("err: %v", err)
	}
	// fakeMarketplace не должен быть вызван.
	if len(fm.updated) != 0 {
		t.Errorf("updated=%d, want 0 (plan terminal)", len(fm.updated))
	}
}

func TestEnqueueDispatch_PlanNotReady(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusPending, nil)

	_, err := svc.EnqueueDispatch(context.Background(), userID, planID)
	if !errors.Is(err, dispatcher.ErrPlanNotReady) {
		t.Errorf("err=%v, want ErrPlanNotReady", err)
	}
}

func TestEnqueueDispatch_PlanTerminal(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusApplied, nil)

	_, err := svc.EnqueueDispatch(context.Background(), userID, planID)
	if !errors.Is(err, dispatcher.ErrPlanTerminal) {
		t.Errorf("err=%v, want ErrPlanTerminal", err)
	}
}

func TestEnqueueDispatch_HappyPath(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, _, _, jobsR, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, nil)

	job, err := svc.EnqueueDispatch(context.Background(), userID, planID)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if job.JobType != domain.BackgroundJobTypePriceDispatch {
		t.Errorf("job_type=%s", job.JobType)
	}
	if plans.statuses[planID] != domain.PlanStatusDispatching {
		t.Errorf("status=%s, want dispatching", plans.statuses[planID])
	}
	if len(jobsR.enqueued) != 1 {
		t.Errorf("enqueued=%d, want 1", len(jobsR.enqueued))
	}
}

func TestEnqueueDispatch_DoubleEnqueue_SecondFails(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, nil)

	if _, err := svc.EnqueueDispatch(context.Background(), userID, planID); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	// Второй enqueue должен упасть на TransitionStatus (статус уже dispatching).
	_, err := svc.EnqueueDispatch(context.Background(), userID, planID)
	if !errors.Is(err, dispatcher.ErrPlanNotReady) {
		t.Errorf("second enqueue err=%v, want ErrPlanNotReady", err)
	}
}

func TestCancel_HappyPath(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, nil)

	if err := svc.Cancel(context.Background(), userID, planID); err != nil {
		t.Fatalf("err: %v", err)
	}
	if plans.statuses[planID] != domain.PlanStatusCancelled {
		t.Errorf("status=%s, want cancelled", plans.statuses[planID])
	}
}

func TestCancel_TerminalPlan_Errors(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusApplied, nil)

	err := svc.Cancel(context.Background(), userID, planID)
	if !errors.Is(err, dispatcher.ErrPlanTerminal) {
		t.Errorf("err=%v, want ErrPlanTerminal", err)
	}
}

func TestExecute_CancelMidDispatch(t *testing.T) {
	// Детерминированный тест: после первого вызова UpdatePrices — отменяем план.
	// Второй chunk должен быть отменён по проверке между chunks.
	resetItemStatuses()
	plansR := newFakePlansRepo()
	pcR := &fakePriceChangesRepo{}
	intR := &fakeIntLogRepo{}
	jobsR := &fakeJobsRepo{}

	userID := uuid.New()
	shopID := uuid.New()
	secret := "test-secret-key-32-bytes-padding!"
	creds, _ := crypto.Encrypt([]byte(`{"api_key":"x"}`), secret)
	shopsR := &fakeShopsRepo{shops: map[uuid.UUID]*domain.Shop{
		shopID: {ID: shopID, UserID: userID, Marketplace: "wb", CredentialsEncrypted: creds, Status: "active"},
	}}

	planID := uuid.New()
	items := makeItems(200)
	plansR.seedPlan(planID, userID, shopID, domain.PlanStatusDispatching, items)

	hookFm := &fakeMarketplaceWithHook{}
	hookFm.afterCall = func(callIdx int) {
		if callIdx == 1 {
			_ = plansR.UpdateStatus(context.Background(), planID, domain.PlanStatusCancelled)
		}
	}
	factory := dispatcher.MarketplaceFactory(func(_ string, _ []byte) (integration.Marketplace, error) {
		return hookFm, nil
	})
	svc := dispatcher.New(plansR, nil, pcR, intR, shopsR, jobsR, secret,
		map[string]dispatcher.MarketplaceFactory{"wb": factory},
		dispatcher.WithChunkSize(100),
	)

	if err := svc.ExecuteDispatchJob(context.Background(), mkJob(planID, shopID, userID, 1)); err != nil {
		t.Fatalf("err: %v", err)
	}

	if plansR.statuses[planID] != domain.PlanStatusCancelled {
		t.Errorf("status=%s, want cancelled", plansR.statuses[planID])
	}
	if len(pcR.created) != 100 {
		t.Errorf("price_changes=%d, want 100 (only first chunk)", len(pcR.created))
	}
	if len(hookFm.updated) != 100 {
		t.Errorf("updated=%d, want 100", len(hookFm.updated))
	}
}

// fakeMarketplaceWithHook — расширение для cancel-теста.
type fakeMarketplaceWithHook struct {
	fakeMarketplace
	afterCall func(callIdx int)
	callIdx   int
}

func (m *fakeMarketplaceWithHook) UpdatePrices(ctx context.Context, ups []integration.PriceUpdate) error {
	err := m.fakeMarketplace.UpdatePrices(ctx, ups)
	m.callIdx++
	if m.afterCall != nil {
		m.afterCall(m.callIdx)
	}
	return err
}

func TestEnqueueDispatch_TenantMismatch(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, _, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusCalculated, nil)

	otherUserID := uuid.New()
	_, err := svc.EnqueueDispatch(context.Background(), otherUserID, planID)
	// Наш fake plansRepo не различает userID, но в реальности GetByIDForUser
	// JOIN-ится с shops и вернёт ErrNotFound. Здесь просто проверяем что не паникует.
	t.Logf("tenant mismatch err: %v", err)
}

func TestMarkExhausted(t *testing.T) {
	fm := &fakeMarketplace{}
	svc, plans, pc, _, _, _, userID, shopID, _ := setupSvc(t, fm)

	planID := uuid.New()
	items := makeItems(3)
	plans.seedPlan(planID, userID, shopID, domain.PlanStatusDispatching, items)

	job := mkJob(planID, shopID, userID, 3)
	if err := svc.MarkExhausted(context.Background(), job); err != nil {
		t.Fatalf("err: %v", err)
	}
	if plans.statuses[planID] != domain.PlanStatusFailed {
		t.Errorf("status=%s, want failed", plans.statuses[planID])
	}
	if len(pc.created) != 3 {
		t.Errorf("price_changes=%d, want 3 (all failed)", len(pc.created))
	}
}
