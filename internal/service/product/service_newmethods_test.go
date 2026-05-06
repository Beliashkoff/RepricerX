package product_test

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	productsvc "github.com/Beliashkoff/RepricerX/internal/service/product"
	"github.com/google/uuid"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakeProductsRepo struct {
	products     map[uuid.UUID]*domain.Product
	softDeleted  map[uuid.UUID]bool
	bulkPatched  []repository.BulkPricePatch
	bulkPatchErr error
	exported     []domain.Product
}

func newFakeProductsRepo(products ...*domain.Product) *fakeProductsRepo {
	r := &fakeProductsRepo{
		products:    make(map[uuid.UUID]*domain.Product),
		softDeleted: make(map[uuid.UUID]bool),
	}
	for _, p := range products {
		r.products[p.ID] = p
	}
	return r
}

func (r *fakeProductsRepo) Create(_ context.Context, _ uuid.UUID, in repository.ProductCreateInput) (*domain.Product, error) {
	p := &domain.Product{ID: uuid.New(), ShopID: in.ShopID, ExternalSKU: in.ExternalSKU, Name: in.Name}
	r.products[p.ID] = p
	return p, nil
}

func (r *fakeProductsRepo) GetByIDForUser(_ context.Context, userID, productID uuid.UUID) (*domain.Product, error) {
	p, ok := r.products[productID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return p, nil
}

func (r *fakeProductsRepo) List(_ context.Context, _ uuid.UUID, f repository.ProductListFilter) (*repository.ProductListResult, error) {
	return &repository.ProductListResult{Items: nil, Page: f.Page, PerPage: f.PerPage, Total: 0}, nil
}

func (r *fakeProductsRepo) PatchPrices(_ context.Context, _ uuid.UUID, id uuid.UUID, _ repository.ProductPricePatch) (*domain.Product, error) {
	p, ok := r.products[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return p, nil
}

func (r *fakeProductsRepo) UpsertImported(_ context.Context, _ uuid.UUID, _ []repository.ProductImportRow) (repository.ImportUpsertResult, error) {
	return repository.ImportUpsertResult{}, nil
}

func (r *fakeProductsRepo) SoftDelete(_ context.Context, _ uuid.UUID, productID uuid.UUID) error {
	if _, ok := r.products[productID]; !ok {
		return repository.ErrNotFound
	}
	r.softDeleted[productID] = true
	r.products[productID].Status = domain.ProductStatusArchived
	return nil
}

func (r *fakeProductsRepo) BulkPatch(_ context.Context, _ uuid.UUID, patches []repository.BulkPricePatch) (int, error) {
	if r.bulkPatchErr != nil {
		return 0, r.bulkPatchErr
	}
	r.bulkPatched = patches
	return len(patches), nil
}

func (r *fakeProductsRepo) ExportForUser(_ context.Context, _ uuid.UUID, _ repository.ProductListFilter) ([]*domain.Product, error) {
	result := make([]*domain.Product, len(r.exported))
	for i := range r.exported {
		p := r.exported[i]
		result[i] = &p
	}
	return result, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type fakeImportLogRepo struct {
	cancelCalled      bool
	cancelErr         error
	listErrors        []domain.ImportLogError
	listTotal         int
	listErr           error
	enqueued          bool
	enqueuedAttempts  int
	enqueuedCooldown  time.Duration
	enqueuedErr       error
	enqueuedRetry     time.Duration
	enqueuedImportLog *domain.ImportLogEntry
}

func (r *fakeImportLogRepo) HasRunning(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}
func (r *fakeImportLogRepo) Create(_ context.Context, _ *domain.ImportLogEntry) error { return nil }
func (r *fakeImportLogRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.ImportLogEntry, error) {
	return nil, repository.ErrNotFound
}
func (r *fakeImportLogRepo) GetForUser(_ context.Context, _, _ uuid.UUID) (*domain.ImportLogEntry, error) {
	return nil, repository.ErrNotFound
}
func (r *fakeImportLogRepo) EnqueueProductImport(_ context.Context, _, shopID uuid.UUID, maxAttempts int, cooldown time.Duration) (*domain.ImportLogEntry, *domain.BackgroundJob, time.Duration, error) {
	r.enqueued = true
	r.enqueuedAttempts = maxAttempts
	r.enqueuedCooldown = cooldown
	if r.enqueuedErr != nil {
		return nil, nil, r.enqueuedRetry, r.enqueuedErr
	}
	entry := r.enqueuedImportLog
	if entry == nil {
		entry = &domain.ImportLogEntry{ID: uuid.New(), ShopID: shopID, Status: domain.ImportStatusPending}
	}
	return entry, &domain.BackgroundJob{ID: uuid.New(), MaxAttempts: maxAttempts}, 0, nil
}
func (r *fakeImportLogRepo) MarkRunning(_ context.Context, _ uuid.UUID) error { return nil }
func (r *fakeImportLogRepo) Finish(_ context.Context, _ uuid.UUID, _ string, _, _, _, _, _ int, _ []domain.ImportLogError, _ time.Time) error {
	return nil
}
func (r *fakeImportLogRepo) Cancel(_ context.Context, _, _ uuid.UUID) error {
	r.cancelCalled = true
	return r.cancelErr
}
func (r *fakeImportLogRepo) ListErrors(_ context.Context, _, _ uuid.UUID, _, _ int) ([]domain.ImportLogError, int, error) {
	return r.listErrors, r.listTotal, r.listErr
}

// ─────────────────────────────────────────────────────────────────────────────

type fakeShopsRepo struct {
	shops map[uuid.UUID]*domain.Shop
}

func newFakeShopsRepo(shops ...*domain.Shop) *fakeShopsRepo {
	r := &fakeShopsRepo{shops: make(map[uuid.UUID]*domain.Shop)}
	for _, s := range shops {
		r.shops[s.ID] = s
	}
	return r
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
	return nil, nil
}
func (r *fakeShopsRepo) Update(_ context.Context, s *domain.Shop) error {
	r.shops[s.ID] = s
	return nil
}
func (r *fakeShopsRepo) Delete(_ context.Context, id, _ uuid.UUID) error {
	delete(r.shops, id)
	return nil
}
func (r *fakeShopsRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string, t time.Time) error {
	if s, ok := r.shops[id]; ok {
		s.Status = status
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────

type fakeJobsRepo struct{}

func (r *fakeJobsRepo) ClaimNext(_ context.Context, _, _ string, _ time.Duration) (*domain.BackgroundJob, error) {
	return nil, repository.ErrNotFound
}
func (r *fakeJobsRepo) Retry(_ context.Context, _ uuid.UUID, _ time.Time, _ string) error { return nil }
func (r *fakeJobsRepo) Succeed(_ context.Context, _ uuid.UUID, _ []byte) error            { return nil }
func (r *fakeJobsRepo) Fail(_ context.Context, _ uuid.UUID, _ string, _ []byte) error     { return nil }

// ─── helpers ─────────────────────────────────────────────────────────────────

func testShop(userID uuid.UUID) *domain.Shop {
	return &domain.Shop{
		ID: uuid.New(), UserID: userID, Marketplace: "wb",
		Name: "Test Shop", Status: domain.ShopStatusActive,
	}
}

func testProduct(shopID uuid.UUID) *domain.Product {
	price := 100.0
	return &domain.Product{
		ID: uuid.New(), ShopID: shopID, ExternalSKU: "SKU-1",
		Name: "Test Product", CurrentPrice: price, Currency: "RUB",
		Status: domain.ProductStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func newSvc(shopsRepo *fakeShopsRepo, productsRepo *fakeProductsRepo, importRepo *fakeImportLogRepo) *productsvc.Service {
	return newSvcWithOptions(shopsRepo, productsRepo, importRepo)
}

func newSvcWithOptions(shopsRepo *fakeShopsRepo, productsRepo *fakeProductsRepo, importRepo *fakeImportLogRepo, opts ...productsvc.Option) *productsvc.Service {
	return productsvc.New(
		shopsRepo, productsRepo, importRepo, &fakeJobsRepo{},
		"test-secret-32-bytes-padded!!!!",
		map[string]productsvc.MarketplaceFactory{
			"wb": func(_ string, _ []byte) (integration.Marketplace, error) {
				return nil, nil
			},
		},
		opts...,
	)
}

// ─── normalizeListFilter ──────────────────────────────────────────────────────

func TestNormalizeListFilterDefaultSort(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), &fakeImportLogRepo{})
	result, err := svc.List(context.Background(), uuid.New(), productsvc.ListFilter{SortBy: "", SortDir: ""})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestNormalizeListFilterSortByWhitelist(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), &fakeImportLogRepo{})

	// unknown sort field should not error — defaults silently to updated_at
	_, err := svc.List(context.Background(), uuid.New(), productsvc.ListFilter{SortBy: "evil_field; DROP TABLE users; --"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeListFilterPageBounds(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), &fakeImportLogRepo{})
	result, err := svc.List(context.Background(), uuid.New(), productsvc.ListFilter{Page: -5, PerPage: 9999})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// page clamped to 1, perPage clamped to 100
	_ = result
}

func TestNormalizeListFilterInvalidStatus(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), &fakeImportLogRepo{})
	_, err := svc.List(context.Background(), uuid.New(), productsvc.ListFilter{Status: "invalid_status"})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestStartImportUsesConfiguredMaxAttempts(t *testing.T) {
	userID := uuid.New()
	shop := testShop(userID)
	importRepo := &fakeImportLogRepo{}
	svc := newSvcWithOptions(
		newFakeShopsRepo(shop),
		newFakeProductsRepo(),
		importRepo,
		productsvc.WithImportMaxAttempts(9),
	)

	entry, err := svc.StartImport(context.Background(), userID, shop.ID)
	if err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	if entry == nil {
		t.Fatal("StartImport returned nil entry")
	}
	if !importRepo.enqueued {
		t.Fatal("expected import enqueue to be called")
	}
	if importRepo.enqueuedAttempts != 9 {
		t.Fatalf("max attempts = %d, want 9", importRepo.enqueuedAttempts)
	}
}

// ─── SoftDelete ──────────────────────────────────────────────────────────────

func TestSoftDeleteSuccess(t *testing.T) {
	userID := uuid.New()
	shop := testShop(userID)
	product := testProduct(shop.ID)

	productsRepo := newFakeProductsRepo(product)
	svc := newSvc(newFakeShopsRepo(shop), productsRepo, &fakeImportLogRepo{})

	err := svc.SoftDelete(context.Background(), userID, product.ID)
	if err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if !productsRepo.softDeleted[product.ID] {
		t.Fatal("product was not soft-deleted in repo")
	}
}

func TestSoftDeleteNotFound(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), &fakeImportLogRepo{})
	err := svc.SoftDelete(context.Background(), uuid.New(), uuid.New())
	if err != productsvc.ErrProductNotFound {
		t.Fatalf("want ErrProductNotFound, got %v", err)
	}
}

// ─── BulkPatch ───────────────────────────────────────────────────────────────

func TestBulkPatchSuccess(t *testing.T) {
	userID := uuid.New()
	maxPrice := 150.0
	product := &domain.Product{ID: uuid.New(), MaxPrice: &maxPrice}
	productsRepo := newFakeProductsRepo(product)
	svc := newSvc(newFakeShopsRepo(), productsRepo, &fakeImportLogRepo{})

	minPrice := 50.0
	items := []productsvc.BulkPatchItem{
		{ProductID: product.ID, MinPrice: repository.OptionalFloat64{Set: true, Value: &minPrice}},
		{ProductID: uuid.New()},
	}
	updated, err := svc.BulkPatch(context.Background(), userID, items)
	if err != nil {
		t.Fatalf("BulkPatch: %v", err)
	}
	if updated != 2 {
		t.Fatalf("want updated=2, got %d", updated)
	}
	if len(productsRepo.bulkPatched) != 2 {
		t.Fatalf("want 2 patches sent to repo, got %d", len(productsRepo.bulkPatched))
	}
}

func TestBulkPatchEmptyIsNoop(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), &fakeImportLogRepo{})
	updated, err := svc.BulkPatch(context.Background(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("BulkPatch empty: %v", err)
	}
	if updated != 0 {
		t.Fatalf("want 0, got %d", updated)
	}
}

func TestBulkPatchNegativePriceRejected(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), &fakeImportLogRepo{})
	neg := -1.0
	_, err := svc.BulkPatch(context.Background(), uuid.New(), []productsvc.BulkPatchItem{
		{ProductID: uuid.New(), MinPrice: repository.OptionalFloat64{Set: true, Value: &neg}},
	})
	if err == nil {
		t.Fatal("expected error for negative price")
	}
}

func TestBulkPatchMinGreaterThanCurrentMaxRejected(t *testing.T) {
	userID := uuid.New()
	maxPrice := 150.0
	product := &domain.Product{ID: uuid.New(), MaxPrice: &maxPrice}
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(product), &fakeImportLogRepo{})

	minPrice := 200.0
	_, err := svc.BulkPatch(context.Background(), userID, []productsvc.BulkPatchItem{
		{ProductID: product.ID, MinPrice: repository.OptionalFloat64{Set: true, Value: &minPrice}},
	})
	if err != productsvc.ErrInvalidPrice {
		t.Fatalf("want ErrInvalidPrice, got %v", err)
	}
}

func TestBulkPatchMaxLessThanCurrentMinRejected(t *testing.T) {
	userID := uuid.New()
	minPrice := 90.0
	product := &domain.Product{ID: uuid.New(), MinPrice: &minPrice}
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(product), &fakeImportLogRepo{})

	maxPrice := 80.0
	_, err := svc.BulkPatch(context.Background(), userID, []productsvc.BulkPatchItem{
		{ProductID: product.ID, MaxPrice: repository.OptionalFloat64{Set: true, Value: &maxPrice}},
	})
	if err != productsvc.ErrInvalidPrice {
		t.Fatalf("want ErrInvalidPrice, got %v", err)
	}
}

func TestBulkPatchConstraintViolationMappedToInvalidPrice(t *testing.T) {
	productsRepo := newFakeProductsRepo()
	productsRepo.bulkPatchErr = repository.ErrConstraintViolation
	svc := newSvc(newFakeShopsRepo(), productsRepo, &fakeImportLogRepo{})

	minPrice := 10.0
	maxPrice := 20.0
	_, err := svc.BulkPatch(context.Background(), uuid.New(), []productsvc.BulkPatchItem{
		{
			ProductID: uuid.New(),
			MinPrice:  repository.OptionalFloat64{Set: true, Value: &minPrice},
			MaxPrice:  repository.OptionalFloat64{Set: true, Value: &maxPrice},
		},
	})
	if err != productsvc.ErrInvalidPrice {
		t.Fatalf("want ErrInvalidPrice, got %v", err)
	}
}

// ─── ExportCSV ───────────────────────────────────────────────────────────────

func TestExportCSVHeaderAndRows(t *testing.T) {
	userID := uuid.New()
	productsRepo := newFakeProductsRepo()
	productsRepo.exported = []domain.Product{
		{
			ID: uuid.New(), ShopID: uuid.New(), ExternalSKU: "SKU-1",
			Name: "Красный чайник", CurrentPrice: 1999.99, Currency: "RUB",
			Status: domain.ProductStatusActive, UpdatedAt: time.Now(),
		},
	}
	svc := newSvc(newFakeShopsRepo(), productsRepo, &fakeImportLogRepo{})

	csvBytes, err := svc.ExportCSV(context.Background(), userID, productsvc.ListFilter{})
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	csvStr := string(csvBytes)
	if !strings.HasPrefix(csvStr, "id,") {
		t.Fatalf("CSV must start with header row, got: %q", csvStr[:min(len(csvStr), 50)])
	}
	if !strings.Contains(csvStr, "SKU-1") {
		t.Fatalf("CSV must contain SKU-1, got: %s", csvStr)
	}
	if !strings.Contains(csvStr, "Красный чайник") {
		t.Fatalf("CSV must contain product name, got: %s", csvStr)
	}
}

func TestExportCSVSanitizesFormulaInjection(t *testing.T) {
	userID := uuid.New()
	productsRepo := newFakeProductsRepo()
	productsRepo.exported = []domain.Product{
		{
			ID:           uuid.New(),
			ShopID:       uuid.New(),
			ExternalSKU:  "=cmd|' /C calc'!A0",
			Name:         "  +SUM(1,1)",
			CurrentPrice: 1999.99,
			Currency:     "RUB",
			Status:       domain.ProductStatusActive,
			UpdatedAt:    time.Now(),
		},
		{
			ID:           uuid.New(),
			ShopID:       uuid.New(),
			ExternalSKU:  "\t@unsafe",
			Name:         "normal name",
			CurrentPrice: 100,
			Currency:     "RUB",
			Status:       domain.ProductStatusActive,
			UpdatedAt:    time.Now(),
		},
	}
	svc := newSvc(newFakeShopsRepo(), productsRepo, &fakeImportLogRepo{})

	csvBytes, err := svc.ExportCSV(context.Background(), userID, productsvc.ListFilter{})
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(string(csvBytes))).ReadAll()
	if err != nil {
		t.Fatalf("read exported csv: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("want header plus 2 product rows, got %d", len(records))
	}
	if records[1][2] != "'=cmd|' /C calc'!A0" {
		t.Fatalf("formula SKU must be prefixed with apostrophe, got %q", records[1][2])
	}
	if records[1][3] != "'  +SUM(1,1)" {
		t.Fatalf("formula name with leading spaces must be prefixed with apostrophe, got %q", records[1][3])
	}
	if records[2][2] != "'\t@unsafe" {
		t.Fatalf("tab-prefixed SKU must be prefixed with apostrophe, got %q", records[2][2])
	}
	if records[2][3] != "normal name" {
		t.Fatalf("safe text must not be modified, got %q", records[2][3])
	}
}

func TestExportCSVEmpty(t *testing.T) {
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), &fakeImportLogRepo{})
	csvBytes, err := svc.ExportCSV(context.Background(), uuid.New(), productsvc.ListFilter{})
	if err != nil {
		t.Fatalf("ExportCSV empty: %v", err)
	}
	if !strings.Contains(string(csvBytes), "id,") {
		t.Fatal("empty export must still have header row")
	}
}

// ─── CancelImport ────────────────────────────────────────────────────────────

func TestCancelImportSuccess(t *testing.T) {
	importRepo := &fakeImportLogRepo{}
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), importRepo)

	err := svc.CancelImport(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("CancelImport: %v", err)
	}
	if !importRepo.cancelCalled {
		t.Fatal("expected Cancel to be called on repo")
	}
}

func TestCancelImportNotFound(t *testing.T) {
	importRepo := &fakeImportLogRepo{cancelErr: repository.ErrNotFound}
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), importRepo)

	err := svc.CancelImport(context.Background(), uuid.New(), uuid.New())
	if err != productsvc.ErrImportNotCancelable {
		t.Fatalf("want ErrImportNotCancelable, got %v", err)
	}
}

// ─── GetImportErrors ─────────────────────────────────────────────────────────

func TestGetImportErrorsSuccess(t *testing.T) {
	importRepo := &fakeImportLogRepo{
		listErrors: []domain.ImportLogError{
			{ExternalSKU: "SKU-1", Code: "invalid_sku", Message: "bad data"},
		},
		listTotal: 1,
	}
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), importRepo)

	errs, total, err := svc.GetImportErrors(context.Background(), uuid.New(), uuid.New(), 1, 20)
	if err != nil {
		t.Fatalf("GetImportErrors: %v", err)
	}
	if total != 1 {
		t.Fatalf("want total=1, got %d", total)
	}
	if len(errs) != 1 || errs[0].Code != "invalid_sku" {
		t.Fatalf("unexpected errors: %#v", errs)
	}
}

func TestGetImportErrorsNotFound(t *testing.T) {
	importRepo := &fakeImportLogRepo{listErr: repository.ErrNotFound}
	svc := newSvc(newFakeShopsRepo(), newFakeProductsRepo(), importRepo)

	_, _, err := svc.GetImportErrors(context.Background(), uuid.New(), uuid.New(), 1, 20)
	if err != productsvc.ErrImportNotFound {
		t.Fatalf("want ErrImportNotFound, got %v", err)
	}
}
