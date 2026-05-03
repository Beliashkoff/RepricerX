package shop_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
	"github.com/google/uuid"
)

// --- in-memory stubs ---

type fakeShopsRepo struct {
	shops map[uuid.UUID]*domain.Shop
}

func newFakeShopsRepo() *fakeShopsRepo {
	return &fakeShopsRepo{shops: make(map[uuid.UUID]*domain.Shop)}
}

func (r *fakeShopsRepo) Create(_ context.Context, s *domain.Shop) error {
	r.shops[s.ID] = s
	return nil
}

func (r *fakeShopsRepo) GetByID(_ context.Context, id, userID uuid.UUID) (*domain.Shop, error) {
	s, ok := r.shops[id]
	if !ok || s.UserID != userID {
		return nil, repository.ErrNotFound
	}
	return s, nil
}

func (r *fakeShopsRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]*domain.Shop, error) {
	var out []*domain.Shop
	for _, s := range r.shops {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *fakeShopsRepo) Update(_ context.Context, s *domain.Shop) error {
	if _, ok := r.shops[s.ID]; !ok {
		return repository.ErrNotFound
	}
	r.shops[s.ID] = s
	return nil
}

func (r *fakeShopsRepo) Delete(_ context.Context, id, userID uuid.UUID) error {
	s, ok := r.shops[id]
	if !ok || s.UserID != userID {
		return repository.ErrNotFound
	}
	delete(r.shops, id)
	return nil
}

func (r *fakeShopsRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string, checkedAt time.Time) error {
	s, ok := r.shops[id]
	if !ok {
		return repository.ErrNotFound
	}
	s.Status = status
	s.LastCheckedAt = &checkedAt
	return nil
}

type fakeIntLogRepo struct{}

func (r *fakeIntLogRepo) Create(_ context.Context, _ *domain.IntegrationLogEntry) error { return nil }
func (r *fakeIntLogRepo) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error)  { return 0, nil }

type fakeMarketplace struct{ authErr error }

func (f *fakeMarketplace) TestAuth(_ context.Context) error                              { return f.authErr }
func (f *fakeMarketplace) ListSKUs(_ context.Context) ([]integration.SKU, error)        { return nil, nil }
func (f *fakeMarketplace) UpdatePrices(_ context.Context, _ []integration.PriceUpdate) error {
	return nil
}

const testSecret = "test-secret-for-unit-tests"

func newSvc(repo *fakeShopsRepo, testAuthErr error) *shopsvc.Service {
	factory := func(authErr error) shopsvc.MarketplaceFactory {
		return func(_ []byte) (integration.Marketplace, error) {
			return &fakeMarketplace{authErr: authErr}, nil
		}
	}
	return shopsvc.New(
		repo,
		&fakeIntLogRepo{},
		testSecret,
		map[string]shopsvc.MarketplaceFactory{
			"wb":   factory(testAuthErr),
			"ozon": factory(testAuthErr),
		},
	)
}

// --- Create ---

func TestCreate_InvalidMarketplace(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), nil)
	_, err := svc.Create(context.Background(), uuid.New(), "invalid", "Shop", json.RawMessage(`{}`))
	if err != shopsvc.ErrInvalidMarketplace {
		t.Fatalf("want ErrInvalidMarketplace, got %v", err)
	}
}

func TestCreate_WB(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, nil)
	userID := uuid.New()

	shop, err := svc.Create(context.Background(), userID, "wb", "My WB Shop", json.RawMessage(`{"api_key":"abc"}`))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if shop.Marketplace != "wb" {
		t.Fatalf("want marketplace wb, got %q", shop.Marketplace)
	}
	if shop.Name != "My WB Shop" {
		t.Fatalf("want name %q, got %q", "My WB Shop", shop.Name)
	}
	if shop.Status != domain.ShopStatusDraft {
		t.Fatalf("want status draft, got %q", shop.Status)
	}
	if shop.UserID != userID {
		t.Fatalf("want userID %v, got %v", userID, shop.UserID)
	}
	// Credentials must be encrypted, not stored as raw JSON
	if string(shop.CredentialsEncrypted) == `{"api_key":"abc"}` {
		t.Fatal("credentials must be encrypted, not plaintext")
	}
}

func TestCreate_Ozon(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), nil)
	shop, err := svc.Create(context.Background(), uuid.New(), "ozon", "Ozon Shop",
		json.RawMessage(`{"client_id":"123","api_key":"xyz"}`))
	if err != nil {
		t.Fatalf("Create ozon: %v", err)
	}
	if shop.Marketplace != "ozon" {
		t.Fatalf("want ozon, got %q", shop.Marketplace)
	}
}

// --- List ---

func TestList_Empty(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), nil)
	shops, err := svc.List(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(shops) != 0 {
		t.Fatalf("want 0 shops, got %d", len(shops))
	}
}

func TestList_IsolatedByOwner(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, nil)

	user1, user2 := uuid.New(), uuid.New()
	creds := json.RawMessage(`{"api_key":"k"}`)

	_, _ = svc.Create(context.Background(), user1, "wb", "Shop A", creds)
	_, _ = svc.Create(context.Background(), user1, "ozon", "Shop B", creds)
	_, _ = svc.Create(context.Background(), user2, "wb", "Shop C", creds)

	shops1, _ := svc.List(context.Background(), user1)
	shops2, _ := svc.List(context.Background(), user2)

	if len(shops1) != 2 {
		t.Fatalf("user1: want 2, got %d", len(shops1))
	}
	if len(shops2) != 1 {
		t.Fatalf("user2: want 1, got %d", len(shops2))
	}
}

// --- Get ---

func TestGet_NotFound(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), nil)
	_, err := svc.Get(context.Background(), uuid.New(), uuid.New())
	if err != shopsvc.ErrShopNotFound {
		t.Fatalf("want ErrShopNotFound, got %v", err)
	}
}

func TestGet_WrongOwner(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, nil)

	owner := uuid.New()
	shop, _ := svc.Create(context.Background(), owner, "wb", "Shop", json.RawMessage(`{"api_key":"k"}`))

	_, err := svc.Get(context.Background(), uuid.New(), shop.ID)
	if err != shopsvc.ErrShopNotFound {
		t.Fatalf("want ErrShopNotFound for wrong owner, got %v", err)
	}
}

func TestGet_Success(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, nil)
	userID := uuid.New()

	created, _ := svc.Create(context.Background(), userID, "wb", "Shop", json.RawMessage(`{"api_key":"k"}`))
	got, err := svc.Get(context.Background(), userID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("want ID %v, got %v", created.ID, got.ID)
	}
}

// --- Update ---

func TestUpdate_Name(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, nil)
	userID := uuid.New()

	shop, _ := svc.Create(context.Background(), userID, "wb", "Old Name", json.RawMessage(`{"api_key":"k"}`))
	newName := "New Name"
	updated, err := svc.Update(context.Background(), userID, shop.ID, shopsvc.UpdatePatch{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("want name %q, got %q", newName, updated.Name)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), nil)
	name := "x"
	err := func() error {
		_, e := svc.Update(context.Background(), uuid.New(), uuid.New(), shopsvc.UpdatePatch{Name: &name})
		return e
	}()
	if err != shopsvc.ErrShopNotFound {
		t.Fatalf("want ErrShopNotFound, got %v", err)
	}
}

// --- Delete ---

func TestDelete_Success(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, nil)
	userID := uuid.New()

	shop, _ := svc.Create(context.Background(), userID, "wb", "Shop", json.RawMessage(`{"api_key":"k"}`))
	if err := svc.Delete(context.Background(), userID, shop.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	shops, _ := svc.List(context.Background(), userID)
	if len(shops) != 0 {
		t.Fatal("shop should be gone after Delete")
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), nil)
	if err := svc.Delete(context.Background(), uuid.New(), uuid.New()); err != shopsvc.ErrShopNotFound {
		t.Fatalf("want ErrShopNotFound, got %v", err)
	}
}

func TestDelete_WrongOwner(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, nil)
	owner := uuid.New()

	shop, _ := svc.Create(context.Background(), owner, "wb", "Shop", json.RawMessage(`{"api_key":"k"}`))
	if err := svc.Delete(context.Background(), uuid.New(), shop.ID); err != shopsvc.ErrShopNotFound {
		t.Fatalf("want ErrShopNotFound for wrong owner, got %v", err)
	}
}

// --- TestConnection ---

func TestTestConnection_NotFound(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), nil)
	err := svc.TestConnection(context.Background(), uuid.New(), uuid.New())
	if err != shopsvc.ErrShopNotFound {
		t.Fatalf("want ErrShopNotFound, got %v", err)
	}
}

func TestTestConnection_Success(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, nil) // nil = TestAuth succeeds
	userID := uuid.New()

	shop, _ := svc.Create(context.Background(), userID, "wb", "Shop", json.RawMessage(`{"api_key":"valid"}`))
	if err := svc.TestConnection(context.Background(), userID, shop.ID); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	got, _ := svc.Get(context.Background(), userID, shop.ID)
	if got.Status != domain.ShopStatusActive {
		t.Fatalf("want status active, got %q", got.Status)
	}
	if got.LastCheckedAt == nil {
		t.Fatal("LastCheckedAt must be set after successful test")
	}
}

func TestTestConnection_AuthFailed(t *testing.T) {
	repo := newFakeShopsRepo()
	svc := newSvc(repo, integration.ErrUnauthorized)
	userID := uuid.New()

	shop, _ := svc.Create(context.Background(), userID, "wb", "Shop", json.RawMessage(`{"api_key":"bad"}`))
	err := svc.TestConnection(context.Background(), userID, shop.ID)
	if err != shopsvc.ErrAuthFailed {
		t.Fatalf("want ErrAuthFailed, got %v", err)
	}

	got, _ := svc.Get(context.Background(), userID, shop.ID)
	if got.Status != domain.ShopStatusError {
		t.Fatalf("want status error, got %q", got.Status)
	}
}
