// Package repository определяет интерфейсы доступа к данным.
// Все реализации живут рядом в *_pg.go файлах.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

// Sentinel-ошибки — используются сервисами для ветвления без type assertion.
var (
	ErrNotFound            = errors.New("not found")
	ErrEmailTaken          = errors.New("email already taken")
	ErrDuplicate           = errors.New("duplicate")
	ErrCooldownActive      = errors.New("cooldown active")
	ErrConstraintViolation = errors.New("constraint violation")
)

// UsersRepository — операции с таблицей users.
type UsersRepository interface {
	Create(ctx context.Context, u *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	UpdateDisplayName(ctx context.Context, id uuid.UUID, name string) error
	// UpdatePasswordHash нужен при смене пароля — вызывающий обязан инвалидировать сессии.
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error
	RegisterFailedLogin(ctx context.Context, id uuid.UUID, newCount int, lockoutUntil *time.Time) error
	ResetFailedLogin(ctx context.Context, id uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

// SessionsRepository — операции с таблицей sessions.
type SessionsRepository interface {
	Create(ctx context.Context, s *domain.Session) error
	// GetByTokenHash ищет активную сессию: token_hash=$1 AND idle_expires_at > now() AND absolute_expires_at > now().
	GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error)
	// TouchIdleIfNeeded условно продлевает idle TTL, если до истечения < 12 ч.
	// Возвращает новый idle_expires_at, если обновление произошло; nil — если не нужно.
	TouchIdleIfNeeded(ctx context.Context, id uuid.UUID, candidateIdle time.Time) (*time.Time, error)
	TouchLastSeen(ctx context.Context, id uuid.UUID, at time.Time) error
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

// EmailVerificationsRepository — операции с таблицей email_verifications.
type EmailVerificationsRepository interface {
	Create(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	// GetUnusedValid ищет токен: token_hash=$1 AND expires_at > now() AND used_at IS NULL.
	GetUnusedValid(ctx context.Context, tokenHash string) (id uuid.UUID, userID uuid.UUID, err error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
	// InvalidatePending помечает used_at=now() для всех неиспользованных токенов юзера (при resend).
	InvalidatePending(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}

// PasswordResetTokensRepository — операции с одноразовыми токенами сброса пароля.
type PasswordResetTokensRepository interface {
	// Issue инвалидирует старые ожидающие токены пользователя и создаёт новый.
	Issue(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	// Consume атомарно помечает валидный токен использованным, меняет пароль,
	// сбрасывает lockout/fail-счётчик и отзывает все сессии пользователя.
	Consume(ctx context.Context, tokenHash string, newPasswordHash string) (userID uuid.UUID, revokedSessions int64, err error)
	DeleteExpired(ctx context.Context) (int64, error)
}

// ShopsRepository — операции с таблицей shops.
type ShopsRepository interface {
	Create(ctx context.Context, shop *domain.Shop) error
	// GetByID возвращает магазин только если он принадлежит userID.
	GetByID(ctx context.Context, id, userID uuid.UUID) (*domain.Shop, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Shop, error)
	Update(ctx context.Context, shop *domain.Shop) error
	// Delete удаляет магазин только если он принадлежит userID.
	Delete(ctx context.Context, id, userID uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, checkedAt time.Time) error
}

// IntegrationLogRepository — операции с таблицей integration_log.
type IntegrationLogRepository interface {
	Create(ctx context.Context, e *domain.IntegrationLogEntry) error
	// DeleteOlderThan удаляет записи старше cutoff; возвращает число удалённых строк.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type ProductListFilter struct {
	Query       string
	ShopID      *uuid.UUID
	Status      string
	HasStrategy *bool
	Page        int
	PerPage     int
	SortBy      string   // "name" | "current_price" | "updated_at" (default)
	SortDir     string   // "asc" | "desc" (default "desc")
	PriceFrom   *float64 // фильтр current_price >=
	PriceTo     *float64 // фильтр current_price <=
}

type ProductListResult struct {
	Items   []*domain.Product
	Total   int
	Page    int
	PerPage int
}

type ProductCreateInput struct {
	ShopID       uuid.UUID
	ExternalSKU  string
	Name         string
	CurrentPrice float64
	Currency     string
	Status       string
	MinPrice     *float64
	MaxPrice     *float64
	CostPrice    *float64
}

type ProductPricePatch struct {
	MinPrice  OptionalFloat64
	MaxPrice  OptionalFloat64
	CostPrice OptionalFloat64
}

type OptionalFloat64 struct {
	Set   bool
	Value *float64
}

type ProductImportRow struct {
	ExternalSKU  string
	Name         string
	CurrentPrice float64
	Currency     string
	Status       string
	StockCount   int
}

type ImportUpsertResult struct {
	Added   int
	Updated int
}

// BulkPricePatch описывает патч цен для одного товара в bulk-операции.
type BulkPricePatch struct {
	ProductID uuid.UUID
	MinPrice  OptionalFloat64
	MaxPrice  OptionalFloat64
	CostPrice OptionalFloat64
}

type ProductsRepository interface {
	Create(ctx context.Context, userID uuid.UUID, input ProductCreateInput) (*domain.Product, error)
	List(ctx context.Context, userID uuid.UUID, filter ProductListFilter) (*ProductListResult, error)
	GetByIDForUser(ctx context.Context, userID, productID uuid.UUID) (*domain.Product, error)
	PatchPrices(ctx context.Context, userID, productID uuid.UUID, patch ProductPricePatch) (*domain.Product, error)
	UpsertImported(ctx context.Context, shopID uuid.UUID, rows []ProductImportRow) (ImportUpsertResult, error)
	// SoftDelete переводит товар в статус "archived" (обратимое удаление).
	SoftDelete(ctx context.Context, userID, productID uuid.UUID) error
	// BulkPatch атомарно применяет патчи цен к нескольким товарам. Возвращает кол-во затронутых строк.
	BulkPatch(ctx context.Context, userID uuid.UUID, patches []BulkPricePatch) (int, error)
	// ExportForUser возвращает до 10 000 товаров пользователя без LIMIT-пагинации (для CSV-экспорта).
	ExportForUser(ctx context.Context, userID uuid.UUID, filter ProductListFilter) ([]*domain.Product, error)
}

type ImportLogRepository interface {
	HasRunning(ctx context.Context, shopID uuid.UUID) (bool, error)
	Create(ctx context.Context, entry *domain.ImportLogEntry) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ImportLogEntry, error)
	GetForUser(ctx context.Context, userID, importID uuid.UUID) (*domain.ImportLogEntry, error)
	EnqueueProductImport(ctx context.Context, userID, shopID uuid.UUID, maxAttempts int, cooldown time.Duration) (*domain.ImportLogEntry, *domain.BackgroundJob, time.Duration, error)
	MarkRunning(ctx context.Context, id uuid.UUID) error
	Finish(ctx context.Context, id uuid.UUID, status string, total, added, updated, skipped, failed int, errors []domain.ImportLogError, finishedAt time.Time) error
	// Cancel отменяет pending/running импорт и связанный job.
	Cancel(ctx context.Context, userID, importID uuid.UUID) error
	// ListErrors возвращает постраничный список ошибок импорта.
	ListErrors(ctx context.Context, userID, importID uuid.UUID, page, perPage int) ([]domain.ImportLogError, int, error)
}

type BackgroundJobsRepository interface {
	ClaimNext(ctx context.Context, queue, workerID string, lockTTL time.Duration) (*domain.BackgroundJob, error)
	Succeed(ctx context.Context, id uuid.UUID, result []byte) error
	Retry(ctx context.Context, id uuid.UUID, runAt time.Time, lastError string) error
	Fail(ctx context.Context, id uuid.UUID, lastError string, result []byte) error
}
