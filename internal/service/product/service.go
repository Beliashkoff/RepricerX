package product

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/pkg/crypto"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrShopNotFound         = errors.New("shop not found")
	ErrProductNotFound      = errors.New("product not found")
	ErrInvalidProduct       = errors.New("invalid product")
	ErrDuplicateSKU         = errors.New("duplicate sku")
	ErrInvalidPrice         = errors.New("invalid price")
	ErrImportAlreadyRunning = errors.New("import already running")
	ErrImportCooldown       = errors.New("import cooldown active")
	ErrImportNotFound       = errors.New("import not found")
	ErrInvalidMarketplace   = errors.New("invalid marketplace")
	ErrImportNotCancelable  = errors.New("import cannot be canceled")
)

// MarketplaceFactory создаёт клиент маркетплейса. shopID — ключ для per-shop rate limiting.
type MarketplaceFactory func(shopID string, credsJSON []byte) (integration.Marketplace, error)

type Service struct {
	shops      repository.ShopsRepository
	products   repository.ProductsRepository
	importLogs repository.ImportLogRepository
	jobs       repository.BackgroundJobsRepository
	secret     string
	factories  map[string]MarketplaceFactory
}

type CreateInput struct {
	ExternalSKU  string
	Name         string
	CurrentPrice float64
	Currency     string
	Status       string
	MinPrice     *float64
	MaxPrice     *float64
	CostPrice    *float64
}

type ListFilter struct {
	Query       string
	ShopID      *uuid.UUID
	Status      string
	HasStrategy *bool
	Page        int
	PerPage     int
	SortBy      string
	SortDir     string
	PriceFrom   *float64
	PriceTo     *float64
}

// BulkPatchItem описывает изменение цен одного товара в bulk-операции.
type BulkPatchItem struct {
	ProductID uuid.UUID
	MinPrice  repository.OptionalFloat64
	MaxPrice  repository.OptionalFloat64
	CostPrice repository.OptionalFloat64
}

type PricePatch struct {
	MinPrice  repository.OptionalFloat64
	MaxPrice  repository.OptionalFloat64
	CostPrice repository.OptionalFloat64
}

type ImportJobExecution struct {
	ImportID      uuid.UUID
	Status        string
	Retryable     bool
	PublicCode    string
	PublicMessage string
	InternalError string
	ResultJSON    []byte
}

type ImportCooldownError struct {
	RetryAfter time.Duration
}

func (e ImportCooldownError) Error() string { return ErrImportCooldown.Error() }

func (e ImportCooldownError) Is(target error) bool {
	return target == ErrImportCooldown
}

const (
	defaultImportMaxAttempts = 5
	defaultImportCooldown    = 15 * time.Minute
)

func New(
	shops repository.ShopsRepository,
	products repository.ProductsRepository,
	importLogs repository.ImportLogRepository,
	jobs repository.BackgroundJobsRepository,
	secret string,
	factories map[string]MarketplaceFactory,
) *Service {
	return &Service{
		shops:      shops,
		products:   products,
		importLogs: importLogs,
		jobs:       jobs,
		secret:     secret,
		factories:  factories,
	}
}

func (s *Service) CreateManual(ctx context.Context, userID, shopID uuid.UUID, input CreateInput) (*domain.Product, error) {
	if _, err := s.getShop(ctx, userID, shopID); err != nil {
		return nil, err
	}
	normalized, err := normalizeCreate(shopID, input)
	if err != nil {
		return nil, err
	}
	product, err := s.products.Create(ctx, userID, normalized)
	if errors.Is(err, repository.ErrDuplicate) {
		return nil, ErrDuplicateSKU
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrShopNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("product create: %w", err)
	}
	return product, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, filter ListFilter) (*repository.ProductListResult, error) {
	normalized, err := normalizeListFilter(filter)
	if err != nil {
		return nil, err
	}
	result, err := s.products.List(ctx, userID, normalized)
	if err != nil {
		return nil, fmt.Errorf("product list: %w", err)
	}
	return result, nil
}

func (s *Service) PatchPrices(ctx context.Context, userID, productID uuid.UUID, patch PricePatch) (*domain.Product, error) {
	current, err := s.products.GetByIDForUser(ctx, userID, productID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("product get for patch: %w", err)
	}
	if err := validatePatchPrices(current, patch); err != nil {
		return nil, err
	}
	product, err := s.products.PatchPrices(ctx, userID, productID, repository.ProductPricePatch{
		MinPrice: patch.MinPrice, MaxPrice: patch.MaxPrice, CostPrice: patch.CostPrice,
	})
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("product patch prices: %w", err)
	}
	return product, nil
}

func (s *Service) StartImport(ctx context.Context, userID, shopID uuid.UUID) (*domain.ImportLogEntry, error) {
	shop, err := s.getShop(ctx, userID, shopID)
	if err != nil {
		return nil, err
	}
	if _, ok := s.factories[shop.Marketplace]; !ok {
		return nil, ErrInvalidMarketplace
	}

	entry, _, retryAfter, err := s.importLogs.EnqueueProductImport(ctx, userID, shopID, defaultImportMaxAttempts, defaultImportCooldown)
	if errors.Is(err, repository.ErrDuplicate) {
		return nil, ErrImportAlreadyRunning
	}
	if errors.Is(err, repository.ErrCooldownActive) {
		return nil, ImportCooldownError{RetryAfter: retryAfter}
	}
	if err != nil {
		return nil, fmt.Errorf("product import: enqueue: %w", err)
	}
	return entry, nil
}

func (s *Service) GetImport(ctx context.Context, userID, importID uuid.UUID) (*domain.ImportLogEntry, error) {
	entry, err := s.importLogs.GetForUser(ctx, userID, importID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrImportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("product import get: %w", err)
	}
	entry.Errors = publicImportLogErrors(entry.Errors)
	return entry, nil
}

func (s *Service) getShop(ctx context.Context, userID, shopID uuid.UUID) (*domain.Shop, error) {
	shop, err := s.shops.GetByID(ctx, shopID, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrShopNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("product shop get: %w", err)
	}
	return shop, nil
}

func (s *Service) ExecuteImportJob(ctx context.Context, job *domain.BackgroundJob) ImportJobExecution {
	result := ImportJobExecution{Status: domain.ImportStatusFailed}
	if job.JobType != domain.BackgroundJobTypeSKUImport {
		result.PublicCode, result.PublicMessage = publicImportError(importErrorUnknown)
		result.InternalError = "unsupported job type"
		return result
	}

	var payload domain.SKUImportJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		result.PublicCode, result.PublicMessage = publicImportError(importErrorUnknown)
		result.InternalError = "invalid job payload"
		return result
	}
	result.ImportID = payload.ImportID

	if err := s.importLogs.MarkRunning(ctx, payload.ImportID); err != nil {
		result.PublicCode, result.PublicMessage = publicImportError(importErrorUnknown)
		result.InternalError = "cannot mark import running"
		return result
	}

	shop, err := s.getShop(ctx, payload.RequestedByUserID, payload.ShopID)
	if err != nil {
		return s.finishImportFailure(ctx, payload.ImportID, job, 0, importErrorUnknown, "shop not found", false)
	}

	factory, ok := s.factories[shop.Marketplace]
	if !ok {
		return s.finishImportFailure(ctx, payload.ImportID, job, 0, importErrorUnknown, "invalid marketplace", false)
	}

	credsJSON, err := crypto.Decrypt(shop.CredentialsEncrypted, s.secret)
	if err != nil {
		return s.finishImportFailure(ctx, payload.ImportID, job, 0, importErrorCredentials, "cannot decrypt credentials", false)
	}
	client, err := factory(payload.ShopID.String(), credsJSON)
	if err != nil {
		return s.finishImportFailure(ctx, payload.ImportID, job, 0, importErrorCredentials, "cannot build marketplace adapter", false)
	}

	skus, err := client.ListSKUs(ctx)
	if err != nil {
		return s.finishImportFailure(ctx, payload.ImportID, job, 0, importErrorAdapter, "adapter list skus: "+redactImportDiagnostic(err.Error()), true)
	}

	rows, skipped, validationErrors := normalizeImportRows(skus)
	upsertResult, err := s.products.UpsertImported(ctx, payload.ShopID, rows)
	if err != nil {
		return s.finishImportFailure(ctx, payload.ImportID, job, len(skus), importErrorUpsert, "cannot save imported products", true)
	}

	status := domain.ImportStatusSucceeded
	if len(validationErrors) > 0 {
		status = domain.ImportStatusPartial
	}
	validationErrors = publicImportLogErrors(validationErrors)
	failed := len(validationErrors)
	finishedAt := time.Now().UTC()
	if err := s.importLogs.Finish(ctx, payload.ImportID, status, len(skus), upsertResult.Added, upsertResult.Updated, skipped, failed, validationErrors, finishedAt); err != nil {
		return ImportJobExecution{
			ImportID:      payload.ImportID,
			Status:        domain.ImportStatusFailed,
			Retryable:     true,
			PublicCode:    importErrorUnknown,
			PublicMessage: publicImportErrors[importErrorUnknown],
			InternalError: "cannot finish import log",
		}
	}
	result.Status = status
	result.ResultJSON = importJobResultJSON(status, len(skus), upsertResult.Added, upsertResult.Updated, skipped, failed)
	return result
}

func (s *Service) finishImportFailure(ctx context.Context, importID uuid.UUID, job *domain.BackgroundJob, total int, code, message string, retryable bool) ImportJobExecution {
	publicCode, publicMessage := publicImportError(code)
	internalDiagnostic := redactImportDiagnostic(message)
	if retryable && job.Attempts < job.MaxAttempts {
		return ImportJobExecution{
			ImportID:      importID,
			Status:        domain.ImportStatusRunning,
			Retryable:     true,
			PublicCode:    publicCode,
			PublicMessage: publicMessage,
			InternalError: internalDiagnostic,
		}
	}
	errs := capImportErrors([]domain.ImportLogError{importError("", publicCode, publicMessage)})
	_ = s.importLogs.Finish(ctx, importID, domain.ImportStatusFailed, total, 0, 0, 0, len(errs), errs, time.Now().UTC())
	return ImportJobExecution{
		ImportID:      importID,
		Status:        domain.ImportStatusFailed,
		Retryable:     false,
		PublicCode:    publicCode,
		PublicMessage: publicMessage,
		InternalError: internalDiagnostic,
		ResultJSON:    importJobResultJSON(domain.ImportStatusFailed, total, 0, 0, 0, len(errs)),
	}
}

func normalizeCreate(shopID uuid.UUID, input CreateInput) (repository.ProductCreateInput, error) {
	externalSKU := strings.TrimSpace(input.ExternalSKU)
	name := strings.TrimSpace(input.Name)
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	status := strings.TrimSpace(input.Status)
	if currency == "" {
		currency = "RUB"
	}
	if status == "" {
		status = domain.ProductStatusActive
	}
	if err := validateText(externalSKU, 100, true); err != nil {
		return repository.ProductCreateInput{}, ErrInvalidProduct
	}
	if err := validateText(name, 255, true); err != nil {
		return repository.ProductCreateInput{}, ErrInvalidProduct
	}
	if !validCurrency(currency) || !validProductStatus(status) {
		return repository.ProductCreateInput{}, ErrInvalidProduct
	}
	if err := validateMoney(input.CurrentPrice); err != nil {
		return repository.ProductCreateInput{}, err
	}
	if err := validatePrices(input.MinPrice, input.MaxPrice, input.CostPrice); err != nil {
		return repository.ProductCreateInput{}, err
	}
	return repository.ProductCreateInput{
		ShopID: shopID, ExternalSKU: externalSKU, Name: name,
		CurrentPrice: input.CurrentPrice, Currency: currency, Status: status,
		MinPrice: input.MinPrice, MaxPrice: input.MaxPrice, CostPrice: input.CostPrice,
	}, nil
}

func normalizeListFilter(filter ListFilter) (repository.ProductListFilter, error) {
	query := strings.TrimSpace(filter.Query)
	status := strings.TrimSpace(filter.Status)
	if status != "" && !validProductStatus(status) {
		return repository.ProductListFilter{}, ErrInvalidProduct
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	perPage := filter.PerPage
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 100 {
		perPage = 100
	}
	sortBy := filter.SortBy
	switch sortBy {
	case "name", "current_price", "updated_at":
	default:
		sortBy = "updated_at"
	}
	sortDir := strings.ToLower(filter.SortDir)
	if sortDir != "asc" {
		sortDir = "desc"
	}
	return repository.ProductListFilter{
		Query: query, ShopID: filter.ShopID, Status: status,
		HasStrategy: filter.HasStrategy, Page: page, PerPage: perPage,
		SortBy: sortBy, SortDir: sortDir,
		PriceFrom: filter.PriceFrom, PriceTo: filter.PriceTo,
	}, nil
}

func (s *Service) SoftDelete(ctx context.Context, userID, productID uuid.UUID) error {
	err := s.products.SoftDelete(ctx, userID, productID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrProductNotFound
	}
	if err != nil {
		return fmt.Errorf("product soft-delete: %w", err)
	}
	return nil
}

func (s *Service) BulkPatch(ctx context.Context, userID uuid.UUID, items []BulkPatchItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	repoItems := make([]repository.BulkPricePatch, 0, len(items))
	for _, it := range items {
		if it.MinPrice.Set && it.MinPrice.Value != nil {
			if err := validateMoney(*it.MinPrice.Value); err != nil {
				return 0, err
			}
		}
		if it.MaxPrice.Set && it.MaxPrice.Value != nil {
			if err := validateMoney(*it.MaxPrice.Value); err != nil {
				return 0, err
			}
		}
		if it.CostPrice.Set && it.CostPrice.Value != nil {
			if err := validateMoney(*it.CostPrice.Value); err != nil {
				return 0, err
			}
		}
		repoItems = append(repoItems, repository.BulkPricePatch{
			ProductID: it.ProductID,
			MinPrice:  it.MinPrice,
			MaxPrice:  it.MaxPrice,
			CostPrice: it.CostPrice,
		})
	}
	updated, err := s.products.BulkPatch(ctx, userID, repoItems)
	if err != nil {
		return 0, fmt.Errorf("product bulk-patch: %w", err)
	}
	return updated, nil
}

func (s *Service) ExportCSV(ctx context.Context, userID uuid.UUID, filter ListFilter) ([]byte, error) {
	repoFilter, err := normalizeListFilter(filter)
	if err != nil {
		return nil, err
	}
	products, err := s.products.ExportForUser(ctx, userID, repoFilter)
	if err != nil {
		return nil, fmt.Errorf("product export: %w", err)
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"id", "shop_id", "external_sku", "name", "current_price", "currency", "status", "min_price", "max_price", "cost_price", "updated_at"})
	for _, p := range products {
		row := []string{
			p.ID.String(),
			p.ShopID.String(),
			p.ExternalSKU,
			p.Name,
			fmt.Sprintf("%.2f", p.CurrentPrice),
			p.Currency,
			p.Status,
			nullableFloat(p.MinPrice),
			nullableFloat(p.MaxPrice),
			nullableFloat(p.CostPrice),
			p.UpdatedAt.Format(time.RFC3339),
		}
		_ = w.Write(row)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("product export csv: %w", err)
	}
	return buf.Bytes(), nil
}

func nullableFloat(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *v)
}

func (s *Service) CancelImport(ctx context.Context, userID, importID uuid.UUID) error {
	err := s.importLogs.Cancel(ctx, userID, importID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrImportNotCancelable
	}
	if err != nil {
		return fmt.Errorf("import cancel: %w", err)
	}
	return nil
}

func (s *Service) GetImportErrors(ctx context.Context, userID, importID uuid.UUID, page, perPage int) ([]domain.ImportLogError, int, error) {
	errs, total, err := s.importLogs.ListErrors(ctx, userID, importID, page, perPage)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, 0, ErrImportNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("import errors: %w", err)
	}
	return errs, total, nil
}

func normalizeImportRows(skus []integration.SKU) ([]repository.ProductImportRow, int, []domain.ImportLogError) {
	const maxRows = 10000
	seen := make(map[string]struct{}, len(skus))
	rows := make([]repository.ProductImportRow, 0, len(skus))
	var errs []domain.ImportLogError
	skipped := 0
	for i, sku := range skus {
		if i >= maxRows {
			skipped++
			errs = append(errs, importError("", "limit_exceeded", "import row limit exceeded"))
			continue
		}
		externalSKU := strings.TrimSpace(sku.ExternalSKU)
		name := strings.TrimSpace(sku.Name)
		currency := strings.ToUpper(strings.TrimSpace(sku.Currency))
		if currency == "" {
			currency = "RUB"
		}
		if _, exists := seen[externalSKU]; exists {
			skipped++
			errs = append(errs, importError(externalSKU, "duplicate_sku", "duplicate SKU in import payload"))
			continue
		}
		if validateText(externalSKU, 100, true) != nil || validateText(name, 255, false) != nil ||
			!validCurrency(currency) || validateMoney(sku.CurrentPrice) != nil {
			skipped++
			errs = append(errs, importError(externalSKU, "invalid_sku", "invalid imported SKU data"))
			continue
		}
		if name == "" {
			name = externalSKU
		}
		seen[externalSKU] = struct{}{}
		rows = append(rows, repository.ProductImportRow{
			ExternalSKU: externalSKU,
			Name:        name, CurrentPrice: sku.CurrentPrice, Currency: currency,
			Status: domain.ProductStatusActive,
		})
	}
	return rows, skipped, capImportErrors(errs)
}

func validateText(value string, maxLen int, required bool) error {
	if required && value == "" {
		return ErrInvalidProduct
	}
	if len(value) > maxLen || !utf8.ValidString(value) {
		return ErrInvalidProduct
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ErrInvalidProduct
		}
	}
	return nil
}

func validatePrices(minPrice, maxPrice, costPrice *float64) error {
	for _, price := range []*float64{minPrice, maxPrice, costPrice} {
		if price != nil {
			if err := validateMoney(*price); err != nil {
				return err
			}
		}
	}
	if minPrice != nil && maxPrice != nil && *minPrice > *maxPrice {
		return ErrInvalidPrice
	}
	return nil
}

func validatePatchPrices(current *domain.Product, patch PricePatch) error {
	if patch.MinPrice.Set && patch.MinPrice.Value != nil {
		if err := validateMoney(*patch.MinPrice.Value); err != nil {
			return err
		}
	}
	if patch.MaxPrice.Set && patch.MaxPrice.Value != nil {
		if err := validateMoney(*patch.MaxPrice.Value); err != nil {
			return err
		}
	}
	if patch.CostPrice.Set && patch.CostPrice.Value != nil {
		if err := validateMoney(*patch.CostPrice.Value); err != nil {
			return err
		}
	}
	minPrice := current.MinPrice
	maxPrice := current.MaxPrice
	if patch.MinPrice.Set {
		minPrice = patch.MinPrice.Value
	}
	if patch.MaxPrice.Set {
		maxPrice = patch.MaxPrice.Value
	}
	if minPrice != nil && maxPrice != nil && *minPrice > *maxPrice {
		return ErrInvalidPrice
	}
	return nil
}

func validateMoney(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 9999999999.99 {
		return ErrInvalidPrice
	}
	return nil
}

func validCurrency(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func validProductStatus(status string) bool {
	switch status {
	case domain.ProductStatusActive, domain.ProductStatusArchived, domain.ProductStatusOutOfStock:
		return true
	default:
		return false
	}
}

func importError(externalSKU, code, message string) domain.ImportLogError {
	return domain.ImportLogError{ExternalSKU: externalSKU, Code: code, Message: message}
}

func capImportErrors(errs []domain.ImportLogError) []domain.ImportLogError {
	const maxErrors = 50
	if len(errs) <= maxErrors {
		return errs
	}
	return append(errs[:maxErrors], importError("", "too_many_errors", "additional import errors were omitted"))
}

func importJobResultJSON(status string, total, added, updated, skipped, failed int) []byte {
	payload, _ := json.Marshal(map[string]any{
		"status":  status,
		"total":   total,
		"added":   added,
		"updated": updated,
		"skipped": skipped,
		"failed":  failed,
	})
	return payload
}
