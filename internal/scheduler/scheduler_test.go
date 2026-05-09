package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakeShopsRepo struct {
	mu       sync.Mutex
	shops    []*domain.Shop
	touchOK  bool       // что вернёт TouchLastRecalcAt
	touchErr error
	touched  []uuid.UUID
}

func (r *fakeShopsRepo) Create(_ context.Context, _ *domain.Shop) error           { return nil }
func (r *fakeShopsRepo) GetByID(_ context.Context, _, _ uuid.UUID) (*domain.Shop, error) {
	return nil, nil
}
func (r *fakeShopsRepo) ListByUserID(_ context.Context, _ uuid.UUID) ([]*domain.Shop, error) {
	return nil, nil
}
func (r *fakeShopsRepo) Update(_ context.Context, _ *domain.Shop) error           { return nil }
func (r *fakeShopsRepo) Delete(_ context.Context, _, _ uuid.UUID) error           { return nil }
func (r *fakeShopsRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _ string, _ time.Time) error {
	return nil
}
func (r *fakeShopsRepo) ListSchedulable(_ context.Context) ([]*domain.Shop, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.Shop, len(r.shops))
	copy(out, r.shops)
	return out, nil
}
func (r *fakeShopsRepo) TouchLastRecalcAt(_ context.Context, shopID uuid.UUID, _ *time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.touchErr != nil {
		return false, r.touchErr
	}
	if r.touchOK {
		r.touched = append(r.touched, shopID)
	}
	return r.touchOK, nil
}

type fakePricing struct {
	mu       sync.Mutex
	calls    []uuid.UUID
	returnErr error
}

func (p *fakePricing) Recalculate(_ context.Context, _, shopID uuid.UUID, _ []uuid.UUID) (*domain.PricePlan, *domain.BackgroundJob, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, shopID)
	if p.returnErr != nil {
		return nil, nil, p.returnErr
	}
	return &domain.PricePlan{ID: uuid.New(), ShopID: shopID}, &domain.BackgroundJob{ID: uuid.New()}, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func newSvc(t *testing.T, shops *fakeShopsRepo, pricing *fakePricing) *Service {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return New(Deps{
		Shops:   shops,
		Pricing: pricing,
		Log:     log,
	})
}

func makeShop(id uuid.UUID, scheduleCron string, lastRecalc *time.Time, createdAt time.Time) *domain.Shop {
	return &domain.Shop{
		ID:           id,
		UserID:       uuid.New(),
		Name:         "test",
		Marketplace:  "wb",
		Status:       domain.ShopStatusActive,
		ScheduleCron: scheduleCron,
		LastRecalcAt: lastRecalc,
		CreatedAt:    createdAt,
	}
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestScheduledRecalc_FirstRun_EnqueuesPricing(t *testing.T) {
	// LastRecalcAt=nil → baseline=createdAt 10 минут назад → spec="* * * * *" → nextRun уже прошёл.
	shopID := uuid.New()
	tenMinAgo := time.Now().UTC().Add(-10 * time.Minute)
	shops := &fakeShopsRepo{
		shops:   []*domain.Shop{makeShop(shopID, "* * * * *", nil, tenMinAgo)},
		touchOK: true,
	}
	pricing := &fakePricing{}
	svc := newSvc(t, shops, pricing)

	svc.ScheduledRecalcTick(context.Background())

	if len(pricing.calls) != 1 || pricing.calls[0] != shopID {
		t.Errorf("expected 1 Recalculate call for shop %s, got %v", shopID, pricing.calls)
	}
}

func TestScheduledRecalc_NotYet_Skip(t *testing.T) {
	// schedule_cron="0 3 * * *" (каждый день в 3:00 UTC), LastRecalcAt=сейчас.
	// nextRun будет через сутки, не прошёл.
	now := time.Now().UTC()
	shops := &fakeShopsRepo{
		shops: []*domain.Shop{makeShop(uuid.New(), "0 3 * * *", &now, now.Add(-time.Hour))},
	}
	pricing := &fakePricing{}
	svc := newSvc(t, shops, pricing)

	svc.ScheduledRecalcTick(context.Background())

	if len(pricing.calls) != 0 {
		t.Errorf("expected 0 calls (next is far future), got %d", len(pricing.calls))
	}
}

func TestScheduledRecalc_TouchFails_Skip(t *testing.T) {
	tenMinAgo := time.Now().UTC().Add(-10 * time.Minute)
	shops := &fakeShopsRepo{
		shops:   []*domain.Shop{makeShop(uuid.New(), "* * * * *", nil, tenMinAgo)},
		touchOK: false, // CAS race — другая реплика забрала
	}
	pricing := &fakePricing{}
	svc := newSvc(t, shops, pricing)

	svc.ScheduledRecalcTick(context.Background())

	if len(pricing.calls) != 0 {
		t.Errorf("expected 0 calls (touch failed), got %d", len(pricing.calls))
	}
}

func TestScheduledRecalc_InvalidCron_Skip(t *testing.T) {
	tenMinAgo := time.Now().UTC().Add(-10 * time.Minute)
	shops := &fakeShopsRepo{
		shops:   []*domain.Shop{makeShop(uuid.New(), "this is not cron", nil, tenMinAgo)},
		touchOK: true,
	}
	pricing := &fakePricing{}
	svc := newSvc(t, shops, pricing)

	svc.ScheduledRecalcTick(context.Background())

	if len(pricing.calls) != 0 {
		t.Errorf("expected 0 calls (invalid cron), got %d", len(pricing.calls))
	}
}

func TestScheduledRecalc_PricingError_Continue(t *testing.T) {
	// Если pricing.Recalculate упал — не блокируем остальные shops.
	tenMinAgo := time.Now().UTC().Add(-10 * time.Minute)
	shop1ID, shop2ID := uuid.New(), uuid.New()
	shops := &fakeShopsRepo{
		shops: []*domain.Shop{
			makeShop(shop1ID, "* * * * *", nil, tenMinAgo),
			makeShop(shop2ID, "* * * * *", nil, tenMinAgo),
		},
		touchOK: true,
	}
	pricing := &fakePricing{returnErr: errors.New("boom")}
	svc := newSvc(t, shops, pricing)

	svc.ScheduledRecalcTick(context.Background())

	// Обе shops обработаны (вызов pricing для каждой), несмотря на ошибку.
	if len(pricing.calls) != 2 {
		t.Errorf("expected 2 calls (continue on error), got %d", len(pricing.calls))
	}
}

func TestScheduledRecalc_NoShops_NoOp(t *testing.T) {
	shops := &fakeShopsRepo{shops: nil}
	pricing := &fakePricing{}
	svc := newSvc(t, shops, pricing)

	svc.ScheduledRecalcTick(context.Background())

	if len(pricing.calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(pricing.calls))
	}
}

func TestScheduledRecalc_NotInShopList_Skip(t *testing.T) {
	// Shop с status='draft' (не active) — НЕ должен быть в ListSchedulable.
	// Здесь fake возвращает то что мы дали; но в реале этот shop не пришёл бы.
	// Тестируем поведение когда shops пустой.
	shops := &fakeShopsRepo{shops: []*domain.Shop{}}
	pricing := &fakePricing{}
	svc := newSvc(t, shops, pricing)

	svc.ScheduledRecalcTick(context.Background())

	if len(pricing.calls) != 0 {
		t.Errorf("got %d calls, want 0", len(pricing.calls))
	}
}

func TestScheduledRecalc_TouchError_Skip(t *testing.T) {
	tenMinAgo := time.Now().UTC().Add(-10 * time.Minute)
	shops := &fakeShopsRepo{
		shops:    []*domain.Shop{makeShop(uuid.New(), "* * * * *", nil, tenMinAgo)},
		touchErr: errors.New("db error"),
	}
	pricing := &fakePricing{}
	svc := newSvc(t, shops, pricing)

	svc.ScheduledRecalcTick(context.Background())

	if len(pricing.calls) != 0 {
		t.Errorf("got %d calls, want 0 (touch err)", len(pricing.calls))
	}
}

func TestNew_DefaultsApplied(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := New(Deps{Log: log})
	if svc.specScheduledRecalc != "* * * * *" {
		t.Errorf("default specScheduledRecalc not set")
	}
	if svc.competitorMaxAge != 30*time.Minute {
		t.Errorf("default competitorMaxAge not set")
	}
}

func TestWithSpecs_OverridesDefaults(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := New(Deps{Log: log}, WithSpecs("@every 1s", "@every 2s", "@every 3s", "@every 4s"))
	if svc.specScheduledRecalc != "@every 1s" {
		t.Errorf("override failed: %s", svc.specScheduledRecalc)
	}
}
