package strategy_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	strategysvc "github.com/Beliashkoff/RepricerX/internal/service/strategy"
	"github.com/google/uuid"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakeStrategiesRepo struct {
	items map[uuid.UUID]*domain.Strategy
}

func newFakeStrategiesRepo() *fakeStrategiesRepo {
	return &fakeStrategiesRepo{items: map[uuid.UUID]*domain.Strategy{}}
}

func (r *fakeStrategiesRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]*domain.Strategy, error) {
	var out []*domain.Strategy
	for _, s := range r.items {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *fakeStrategiesRepo) GetByIDForUser(_ context.Context, userID, id uuid.UUID) (*domain.Strategy, error) {
	s, ok := r.items[id]
	if !ok || s.UserID != userID {
		return nil, repository.ErrNotFound
	}
	return s, nil
}

func (r *fakeStrategiesRepo) Create(_ context.Context, userID uuid.UUID, in repository.StrategyCreateInput) (*domain.Strategy, error) {
	s := &domain.Strategy{
		ID:             uuid.New(),
		UserID:         userID,
		Name:           in.Name,
		Type:           in.Type,
		Params:         in.Params,
		Constraints:    in.Constraints,
		FallbackPolicy: in.FallbackPolicy,
		Priority:       in.Priority,
		Enabled:        in.Enabled,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	r.items[s.ID] = s
	return s, nil
}

func (r *fakeStrategiesRepo) Update(_ context.Context, userID, id uuid.UUID, in repository.StrategyUpdateInput) (*domain.Strategy, error) {
	s, ok := r.items[id]
	if !ok || s.UserID != userID {
		return nil, repository.ErrNotFound
	}
	if in.Name != nil {
		s.Name = *in.Name
	}
	if in.Type != nil {
		s.Type = *in.Type
	}
	if in.Params != nil {
		s.Params = in.Params
	}
	if in.Constraints != nil {
		s.Constraints = in.Constraints
	}
	if in.FallbackPolicy != nil {
		s.FallbackPolicy = *in.FallbackPolicy
	}
	if in.Priority != nil {
		s.Priority = *in.Priority
	}
	if in.Enabled != nil {
		s.Enabled = *in.Enabled
	}
	s.UpdatedAt = time.Now()
	return s, nil
}

func (r *fakeStrategiesRepo) Delete(_ context.Context, userID, id uuid.UUID) error {
	s, ok := r.items[id]
	if !ok || s.UserID != userID {
		return repository.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeStrategiesRepo) CountAssignments(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

type fakeAssignmentsRepo struct {
	assignments map[uuid.UUID]uuid.UUID // productID → strategyID
}

func newFakeAssignmentsRepo() *fakeAssignmentsRepo {
	return &fakeAssignmentsRepo{assignments: map[uuid.UUID]uuid.UUID{}}
}

func (r *fakeAssignmentsRepo) AssignToProducts(_ context.Context, _, strategyID uuid.UUID, productIDs []uuid.UUID) error {
	for _, pid := range productIDs {
		r.assignments[pid] = strategyID
	}
	return nil
}

func (r *fakeAssignmentsRepo) UnassignFromProducts(_ context.Context, _, strategyID uuid.UUID, productIDs []uuid.UUID) error {
	for _, pid := range productIDs {
		if r.assignments[pid] == strategyID {
			delete(r.assignments, pid)
		}
	}
	return nil
}

func (r *fakeAssignmentsRepo) ListProductIDsByStrategy(_ context.Context, _, strategyID uuid.UUID) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for pid, sid := range r.assignments {
		if sid == strategyID {
			out = append(out, pid)
		}
	}
	return out, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func newSvc() (*strategysvc.Service, *fakeStrategiesRepo, *fakeAssignmentsRepo) {
	str := newFakeStrategiesRepo()
	asg := newFakeAssignmentsRepo()
	return strategysvc.New(str, asg), str, asg
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestCreate_Fixed_HappyPath(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()
	userID := uuid.New()

	st, err := svc.Create(ctx, userID, strategysvc.CreateInput{
		Name:           "Фиксированная",
		Type:           domain.StrategyTypeFixed,
		Params:         mustJSON(map[string]any{"value": 999.99}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Type != domain.StrategyTypeFixed {
		t.Errorf("unexpected type: %s", st.Type)
	}
}

func TestCreate_InvalidType(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "bad",
		Type:           "unknown_type",
		Params:         mustJSON(map[string]any{"value": 100}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestCreate_Fixed_InvalidValue_Zero(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeFixed,
		Params:         mustJSON(map[string]any{"value": 0}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for value=0")
	}
}

func TestCreate_BelowMedianPct_OutOfRange(t *testing.T) {
	svc, _, _ := newSvc()
	// Лимит поднят до 100 — значение 101 должно давать ошибку
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeBelowMedianPct,
		Params:         mustJSON(map[string]any{"pct": 101}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for pct=101")
	}
}

func TestCreate_BelowMedianPct_HighValue_OK(t *testing.T) {
	svc, _, _ := newSvc()
	// Лимит снят — пct=35 должно проходить (ранее было ограничение 20)
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeBelowMedianPct,
		Params:         mustJSON(map[string]any{"pct": 35}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err != nil {
		t.Fatalf("unexpected error for pct=35: %v", err)
	}
}

func TestCreate_MinCompetitorStep_OutOfRange(t *testing.T) {
	svc, _, _ := newSvc()
	// Отрицательный шаг — единственное что запрещено
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeMinCompetitorPlusStep,
		Params:         mustJSON(map[string]any{"step": -1}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for step=-1")
	}
}

func TestCreate_MinCompetitorStep_HighValue_OK(t *testing.T) {
	svc, _, _ := newSvc()
	// Шаг 2000 ₽ — должно проходить (ранее ограничение 500)
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeMinCompetitorPlusStep,
		Params:         mustJSON(map[string]any{"step": 2000}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err != nil {
		t.Fatalf("unexpected error for step=2000: %v", err)
	}
}

func TestCreate_MinMarginPct_Negative(t *testing.T) {
	svc, _, _ := newSvc()
	// Отрицательная маржа — единственное что запрещено
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeMinMarginPct,
		Params:         mustJSON(map[string]any{"margin_pct": -1}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for margin_pct=-1")
	}
}

func TestCreate_MinMarginPct_AnyPositiveValue_OK(t *testing.T) {
	svc, _, _ := newSvc()
	// Верхнего лимита нет — любое неотрицательное значение разрешено
	for _, pct := range []float64{0, 30, 200, 500, 9999, 50000} {
		_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
			Name:           "x",
			Type:           domain.StrategyTypeMinMarginPct,
			Params:         mustJSON(map[string]any{"margin_pct": pct}),
			FallbackPolicy: domain.FallbackPolicyKeepCurrent,
		})
		if err != nil {
			t.Fatalf("unexpected error for margin_pct=%.0f: %v", pct, err)
		}
	}
}

func TestCreate_Constraints_MinProfitPct_Negative(t *testing.T) {
	svc, _, _ := newSvc()
	// Отрицательная min_profit_pct — единственное что запрещено
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeFixed,
		Params:         mustJSON(map[string]any{"value": 100}),
		Constraints:    mustJSON(map[string]any{"min_profit_pct": -1}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for min_profit_pct=-1")
	}
}

func TestCreate_Constraints_MinPriceGtMaxPrice(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeFixed,
		Params:         mustJSON(map[string]any{"value": 100}),
		Constraints:    mustJSON(map[string]any{"min_price": 500.0, "max_price": 100.0}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for min_price > max_price")
	}
}

func TestCreate_Constraints_MaxChangePct_Negative(t *testing.T) {
	svc, _, _ := newSvc()
	// Отрицательный max_change_pct — единственное что запрещено
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeFixed,
		Params:         mustJSON(map[string]any{"value": 100}),
		Constraints:    mustJSON(map[string]any{"max_change_pct": -1}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for max_change_pct=-1")
	}
}

func TestCreate_Constraints_MinIntervalMin_OutOfRange(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.Create(context.Background(), uuid.New(), strategysvc.CreateInput{
		Name:           "x",
		Type:           domain.StrategyTypeFixed,
		Params:         mustJSON(map[string]any{"value": 100}),
		Constraints:    mustJSON(map[string]any{"min_interval_minutes": 0}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
	})
	if err == nil {
		t.Fatal("expected error for min_interval_minutes=0")
	}
}

func TestGet_NotFound(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.Get(context.Background(), uuid.New(), uuid.New())
	if err != strategysvc.ErrStrategyNotFound {
		t.Fatalf("expected ErrStrategyNotFound, got: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc, _, _ := newSvc()
	err := svc.Delete(context.Background(), uuid.New(), uuid.New())
	if err != strategysvc.ErrStrategyNotFound {
		t.Fatalf("expected ErrStrategyNotFound, got: %v", err)
	}
}

func TestUpdate_ChangeEnabled(t *testing.T) {
	svc, repo, _ := newSvc()
	ctx := context.Background()
	userID := uuid.New()

	// Seed directly.
	id := uuid.New()
	enabled := true
	repo.items[id] = &domain.Strategy{
		ID:             id,
		UserID:         userID,
		Name:           "test",
		Type:           domain.StrategyTypeFixed,
		Params:         mustJSON(map[string]any{"value": 100}),
		Constraints:    mustJSON(map[string]any{}),
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
		Enabled:        enabled,
	}

	disabled := false
	st, err := svc.Update(ctx, userID, id, strategysvc.UpdatePatch{Enabled: &disabled})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Enabled {
		t.Error("expected Enabled=false after update")
	}
}

func TestAssign_UnassignRoundtrip(t *testing.T) {
	svc, repo, asgRepo := newSvc()
	ctx := context.Background()
	userID := uuid.New()

	id := uuid.New()
	repo.items[id] = &domain.Strategy{
		ID: id, UserID: userID, Type: domain.StrategyTypeFixed,
		FallbackPolicy: domain.FallbackPolicyKeepCurrent,
		Params:         mustJSON(map[string]any{"value": 1}),
	}

	p1, p2 := uuid.New(), uuid.New()
	if err := svc.AssignToProducts(ctx, userID, id, []uuid.UUID{p1, p2}); err != nil {
		t.Fatalf("assign error: %v", err)
	}
	if asgRepo.assignments[p1] != id {
		t.Error("p1 should be assigned")
	}

	if err := svc.UnassignFromProducts(ctx, userID, id, []uuid.UUID{p1}); err != nil {
		t.Fatalf("unassign error: %v", err)
	}
	if _, ok := asgRepo.assignments[p1]; ok {
		t.Error("p1 should be unassigned")
	}
	if asgRepo.assignments[p2] != id {
		t.Error("p2 should still be assigned")
	}
}
