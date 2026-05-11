// Package repository определяет интерфейсы доступа к данным.
// Все реализации живут рядом в *_pg.go файлах.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	// ListAdminIDs — для system-scoped уведомлений (упавший cron, глобальные ошибки).
	ListAdminIDs(ctx context.Context) ([]uuid.UUID, error)
	// SetAdmin вызывается из BootstrapAdmin при старте api по env BOOTSTRAP_ADMIN_EMAIL.
	SetAdmin(ctx context.Context, id uuid.UUID, isAdmin bool) error
	// SetTelegramMutedUntil — обновление окна mute из обработчика команды /mute в cmd/bot.
	SetTelegramMutedUntil(ctx context.Context, id uuid.UUID, until *time.Time) error
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
	// ConsumeAndActivate атомарно помечает токен использованным и переводит пользователя в 'active'
	// только если его текущий статус 'pending_verification'. Возвращает ErrNotFound если токен
	// невалиден/истёк/уже использован или пользователь не в статусе pending_verification.
	ConsumeAndActivate(ctx context.Context, tokenHash string) (userID uuid.UUID, err error)
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
	// ListSchedulable возвращает все active shops с непустым schedule_cron.
	// Используется scheduler-ом для итерации в cron-tick (Этап 7).
	// БЕЗ фильтра по auto_update_enabled — флаги независимы.
	ListSchedulable(ctx context.Context) ([]*domain.Shop, error)
	// TouchLastRecalcAt атомарно обновляет shops.last_recalc_at = NOW()
	// только если текущее значение равно expectedPrev (CAS-условие).
	// Возвращает (true, nil) если переход применился; (false, nil) если другая
	// реплика уже забрала. Защищает scheduledRecalc от двойного запуска
	// в multi-replica развёртывании.
	TouchLastRecalcAt(ctx context.Context, shopID uuid.UUID, expectedPrev *time.Time) (bool, error)
}

// IntegrationLogRepository — операции с таблицей integration_log.
type IntegrationLogRepository interface {
	Create(ctx context.Context, e *domain.IntegrationLogEntry) error
	// DeleteOlderThan удаляет записи старше cutoff; возвращает число удалённых строк.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type StrategyCreateInput struct {
	Name           string
	Type           string
	Params         []byte
	Constraints    []byte
	FallbackPolicy string
	Priority       int
	Enabled        bool
}

type StrategyUpdateInput struct {
	Name           *string
	Type           *string
	Params         []byte // nil = не менять
	Constraints    []byte // nil = не менять
	FallbackPolicy *string
	Priority       *int
	Enabled        *bool
}

type StrategiesRepository interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.Strategy, error)
	GetByIDForUser(ctx context.Context, userID, strategyID uuid.UUID) (*domain.Strategy, error)
	Create(ctx context.Context, userID uuid.UUID, input StrategyCreateInput) (*domain.Strategy, error)
	Update(ctx context.Context, userID, strategyID uuid.UUID, input StrategyUpdateInput) (*domain.Strategy, error)
	Delete(ctx context.Context, userID, strategyID uuid.UUID) error
	CountAssignments(ctx context.Context, strategyID uuid.UUID) (int, error)
}

type StrategyAssignmentsRepository interface {
	AssignToProducts(ctx context.Context, userID, strategyID uuid.UUID, productIDs []uuid.UUID) error
	UnassignFromProducts(ctx context.Context, userID, strategyID uuid.UUID, productIDs []uuid.UUID) error
	ListProductIDsByStrategy(ctx context.Context, userID, strategyID uuid.UUID) ([]uuid.UUID, error)
}

type PriceChangesRepository interface {
	// ListForUser возвращает страницу записей журнала и общее число записей,
	// удовлетворяющих фильтру (для пагинатора).
	ListForUser(ctx context.Context, userID uuid.UUID, f PriceChangeFilter) (items []*domain.PriceChange, total int, err error)
	// ExportForUser применяет тот же фильтр, что и ListForUser, но без LIMIT/OFFSET
	// (защитный потолок 10 000 строк зашит в реализации).
	ExportForUser(ctx context.Context, userID uuid.UUID, f PriceChangeFilter) ([]*domain.PriceChange, error)
	SummaryForUser(ctx context.Context, userID uuid.UUID, f PriceChangeFilter) (*domain.PriceChangeSummary, error)
	// Create — пишет одну запись в price_change_log при отправке/skip/fail (Этап 6).
	Create(ctx context.Context, change PriceChangeCreate) error
	// DeleteOlderThan — retention 180 дней. Возвращает кол-во удалённых строк.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// PriceChangeCreate — параметры записи в price_change_log.
// status принимает значения plan_item_status: 'applied' / 'failed' / 'skipped'.
type PriceChangeCreate struct {
	ShopID        uuid.UUID
	ProductID     uuid.UUID
	StrategyID    *uuid.UUID
	OldPrice      float64
	NewPrice      float64
	TargetPrice   float64
	Reason        string
	ConstraintHit string
	Status        string
	CorrelationID uuid.UUID
}

// PriceChangeFilter — параметры фильтрации/пагинации для запросов к price_change_log.
// From/Until должен заполнять сервис (репозиторий обязательных дефолтов не подставляет).
// Status принимает публичные значения 'success'/'failed'/'skipped'; реализация
// сама маппит в plan_item_status. SortDir: 'asc'/'desc' (default 'desc').
// Page/PerPage используются только в ListForUser; для ExportForUser/SummaryForUser игнорируются.
type PriceChangeFilter struct {
	ShopID      *uuid.UUID
	ProductID   *uuid.UUID
	ExternalSKU string
	Status      string
	From        time.Time
	Until       time.Time
	Page        int
	PerPage     int
	SortDir     string
}

// PricePlansRepository — план изменений цен (Этап 5).
// Owner-проверки: GetByIDForUser/ListByUser делают JOIN с shops по user_id.
// Create/UpdateStatus/InsertItems вызываются только из доверенных кодовых путей
// (worker, service.Recalculate), которые сами уже проверили ownership shop.
type PricePlansRepository interface {
	Create(ctx context.Context, shopID uuid.UUID) (*domain.PricePlan, error)
	GetByIDForUser(ctx context.Context, userID, planID uuid.UUID) (*domain.PricePlan, []*domain.PricePlanItem, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.PricePlan, int, error)
	UpdateStatus(ctx context.Context, planID uuid.UUID, status string) error
	InsertItems(ctx context.Context, planID uuid.UUID, items []*domain.PricePlanItem) error
	// LatestItemCreatedAt возвращает время самого свежего price_plan_item для товара
	// со статусом pending или applied (НЕ skipped/failed).
	// Используется для проверки constraint min_interval_minutes.
	// Возвращает (nil, nil) если истории нет.
	LatestItemCreatedAt(ctx context.Context, productID uuid.UUID) (*time.Time, error)

	// ListItemsForDispatch возвращает items со status='pending' и связанным external_sku
	// продукта — для отправки в МП. Используется dispatcher worker.
	ListItemsForDispatch(ctx context.Context, planID uuid.UUID) ([]*PricePlanItemForDispatch, error)
	// UpdateItemAfterDispatch — обновляет статус и текст ошибки item-а после реальной отправки.
	UpdateItemAfterDispatch(ctx context.Context, itemID uuid.UUID, status, errorText string) error
	// CountItemsByStatus — для финализации статуса плана (applied/partial/failed).
	CountItemsByStatus(ctx context.Context, planID uuid.UUID) (map[string]int, error)
	// ResolveOwnerAndShop возвращает (userID, shopID) для plan по его id.
	ResolveOwnerAndShop(ctx context.Context, planID uuid.UUID) (userID, shopID uuid.UUID, err error)
	// TransitionStatus атомарно меняет статус с fromStatus на toStatus.
	// Возвращает (true,nil) если переход применился, (false,nil) если статус был не fromStatus
	// (например, план уже отменён или уже dispatching). При ErrNoRows → (false, ErrNotFound).
	TransitionStatus(ctx context.Context, planID uuid.UUID, fromStatus, toStatus string) (bool, error)
}

// PricePlanItemForDispatch — view-структура с полями, нужными для UpdatePrices.
type PricePlanItemForDispatch struct {
	ItemID        uuid.UUID
	ProductID     uuid.UUID
	ExternalSKU   string
	StrategyID    *uuid.UUID
	CurrentPrice  float64
	FinalPrice    float64
	TargetPrice   float64
	ConstraintHit string
	CorrelationID uuid.UUID
}

type CompetitorCreateInput struct {
	ProductID               uuid.UUID
	Marketplace             string
	Source                  string
	CompetitorURL           string
	NormalizedCompetitorURL string
	OzonPublicProductID     *string
	OzonSKU                 *string
}

type CompetitorUpdateInput struct {
	CompetitorURL           string
	NormalizedCompetitorURL string
	OzonPublicProductID     *string
	OzonSKU                 *string
}

type CompetitorCheckResult struct {
	Price        *float64
	Availability string
	Status       string
	ErrorCode    string
	RawSource    string
	CheckedAt    time.Time
}

// StaleCompetitor — view-структура для scheduler-tick (Этап 7).
type StaleCompetitor struct {
	CompetitorID uuid.UUID
	UserID       uuid.UUID
}

type CompetitorSignalContext struct {
	ProductID    uuid.UUID
	UserID       uuid.UUID
	ExternalSKU  string
	CurrentPrice float64
}

type CompetitorPriceStats struct {
	Count  int
	Min    *float64
	Median *float64
}

type ProductCompetitorsRepository interface {
	Create(ctx context.Context, userID uuid.UUID, input CompetitorCreateInput) (*domain.ProductCompetitor, error)
	ListByProduct(ctx context.Context, userID, productID uuid.UUID) ([]*domain.ProductCompetitor, error)
	GetByIDForUser(ctx context.Context, userID, competitorID uuid.UUID) (*domain.ProductCompetitor, error)
	Update(ctx context.Context, userID, competitorID uuid.UUID, input CompetitorUpdateInput) (*domain.ProductCompetitor, error)
	Delete(ctx context.Context, userID, competitorID uuid.UUID) error
	SaveCheckResult(ctx context.Context, competitorID uuid.UUID, result CompetitorCheckResult) (*domain.ProductCompetitor, error)
	LatestFreshPrice(ctx context.Context, userID, productID uuid.UUID, maxAge time.Duration) (*float64, error)
	SignalContext(ctx context.Context, userID, productID uuid.UUID) (CompetitorSignalContext, error)
	StatsBefore(ctx context.Context, productID uuid.UUID, before time.Time) (CompetitorPriceStats, error)
	CurrentStats(ctx context.Context, productID uuid.UUID) (CompetitorPriceStats, error)
	// ListStaleForRefresh — для scheduler competitorRefreshTick (Этап 7).
	// Возвращает competitor_id + user_id (через JOIN products + shops) для всех
	// конкурентов с last_checked_at < since (или NULL) и активным статусом.
	ListStaleForRefresh(ctx context.Context, since time.Time, limit int) ([]StaleCompetitor, error)
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

// BackgroundJobEnqueue — параметры enqueue нового джоба.
type BackgroundJobEnqueue struct {
	JobType     string
	Queue       string // "default" если пусто
	Priority    int
	Payload     []byte
	MaxAttempts int       // 3 если 0
	RunAt       time.Time // now если zero
}

type BackgroundJobsRepository interface {
	ClaimNext(ctx context.Context, queue, workerID string, lockTTL time.Duration) (*domain.BackgroundJob, error)
	Succeed(ctx context.Context, id uuid.UUID, result []byte) error
	Retry(ctx context.Context, id uuid.UUID, runAt time.Time, lastError string) error
	Fail(ctx context.Context, id uuid.UUID, lastError string, result []byte) error
	// Enqueue — общий метод постановки задачи. Используется для price_recalculation
	// и других generic-джобов. EnqueueProductImport остаётся специализированным
	// (под import_log + дедуп cooldown).
	Enqueue(ctx context.Context, in BackgroundJobEnqueue) (*domain.BackgroundJob, error)
}

// NotificationCreate — параметры создания нового события для пользователя.
// CorrelationID опционально; если задан — поможет дедупу и трассировке.
type NotificationCreate struct {
	UserID        uuid.UUID
	EventType     string
	Severity      string
	Title         string
	Body          string
	Data          []byte // JSON
	ShopID        *uuid.UUID
	PlanID        *uuid.UUID
	CorrelationID *uuid.UUID
}

// NotificationListFilter — постраничный фильтр для GET /api/notifications.
type NotificationListFilter struct {
	EventType  string
	Severity   string
	UnreadOnly bool
	From       time.Time
	Until      time.Time
	ShopID     *uuid.UUID
	Page       int
	PerPage    int
}

type NotificationsRepository interface {
	Create(ctx context.Context, tx pgx.Tx, in NotificationCreate) (*domain.Notification, error)
	GetByIDForUser(ctx context.Context, userID, id uuid.UUID) (*domain.Notification, error)
	ListForUser(ctx context.Context, userID uuid.UUID, f NotificationListFilter) (items []*domain.Notification, total int, err error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) (int64, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	// ExistsRecentByDedupe — для дедупликации (например, integration_error по
	// (user_id, event_type, shop_id) за окно). Реализация JOIN'ит data->>'shop_id'.
	ExistsRecentByDedupe(ctx context.Context, userID uuid.UUID, eventType string, shopID *uuid.UUID, since time.Time) (bool, error)
	ExistsRecentByCorrelation(ctx context.Context, userID uuid.UUID, eventType string, correlationID uuid.UUID, since time.Time) (bool, error)
	// DeleteOlderThan — retention уведомлений.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type NotificationPreferencesRepository interface {
	List(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error)
	IsEnabled(ctx context.Context, userID uuid.UUID, eventType, channel string, defaultEnabled bool) (bool, error)
	Upsert(ctx context.Context, prefs []domain.NotificationPreference) error
}

type NotificationDeliveryCreate struct {
	NotificationID uuid.UUID
	Channel        string
	Status         string
}

// PendingDigestRow — строка для DigestFlushTick.
type PendingDigestRow struct {
	UserID  uuid.UUID
	Channel string
}

type NotificationDeliveriesRepository interface {
	Create(ctx context.Context, tx pgx.Tx, in NotificationDeliveryCreate) (*domain.NotificationDelivery, error)
	ListByNotification(ctx context.Context, notificationID uuid.UUID) ([]*domain.NotificationDelivery, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status, lastError string, sentAt *time.Time) error
	AttachJob(ctx context.Context, id, jobID uuid.UUID) error
	IncrementAttempts(ctx context.Context, id uuid.UUID) error
	// ListPendingDigests возвращает уникальные пары (user_id, channel) с накопленными
	// pending_digest строками. Используется DigestFlushTick.
	ListPendingDigestPairs(ctx context.Context, channel string) ([]PendingDigestRow, error)
	// LockPendingDigestForUser атомарно переводит pending_digest → queued_digest
	// для пары (user, channel) и возвращает ID-шники переведённых строк.
	// Использует FOR UPDATE SKIP LOCKED для multi-replica safety.
	LockPendingDigestForUser(ctx context.Context, userID uuid.UUID, channel string) ([]uuid.UUID, error)
	// LoadDigestNotifications возвращает notifications + deliveries по списку delivery-id
	// для рендера дайджеста.
	LoadByIDs(ctx context.Context, deliveryIDs []uuid.UUID) ([]*domain.NotificationDelivery, []*domain.Notification, error)
}

// UserChannelSettingsCreate — параметры upsert для GetOrCreate.
type UserChannelSettingsUpdate struct {
	DigestWindowMinutes *int
	DigestMinSeverity   *string
	QuietHoursStart     *int
	QuietHoursEnd       *int
	ClearQuietHours     bool
}

type UserChannelSettingsRepository interface {
	List(ctx context.Context, userID uuid.UUID) ([]*domain.UserChannelSettings, error)
	Get(ctx context.Context, userID uuid.UUID, channel string) (*domain.UserChannelSettings, error)
	Upsert(ctx context.Context, userID uuid.UUID, channel string, in UserChannelSettingsUpdate) (*domain.UserChannelSettings, error)
	MarkDigestSent(ctx context.Context, userID uuid.UUID, channel string, at time.Time) error
}

type TelegramLinksRepository interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TelegramLink, error)
	GetByToken(ctx context.Context, token string) (*domain.TelegramLink, error)
	GetByChatID(ctx context.Context, chatID int64) (*domain.TelegramLink, error)
	IssueToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error
	Confirm(ctx context.Context, token string, chatID int64, username string) (*domain.TelegramLink, error)
	Unlink(ctx context.Context, userID uuid.UUID) error
	UnlinkByChatID(ctx context.Context, chatID int64) error
}

type WebhookCreate struct {
	URL         string
	Secret      string
	Description string
}

type WebhooksRepository interface {
	List(ctx context.Context, userID uuid.UUID) ([]*domain.Webhook, error)
	GetByIDForUser(ctx context.Context, userID, id uuid.UUID) (*domain.Webhook, error)
	Create(ctx context.Context, userID uuid.UUID, in WebhookCreate) (*domain.Webhook, error)
	SetEnabled(ctx context.Context, userID, id uuid.UUID, enabled bool) error
	Delete(ctx context.Context, userID, id uuid.UUID) error
	ListEnabledForUser(ctx context.Context, userID uuid.UUID) ([]*domain.Webhook, error)
}

// OAuthIdentitiesRepository — операции с таблицей oauth_identities.
type OAuthIdentitiesRepository interface {
	Create(ctx context.Context, identity *domain.OAuthIdentity) error
	// GetByProviderAndExternalID ищет привязку (provider, external_id).
	GetByProviderAndExternalID(ctx context.Context, provider domain.OAuthProvider, externalID string) (*domain.OAuthIdentity, error)
	TouchLastLogin(ctx context.Context, id uuid.UUID) error
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.OAuthIdentity, error)
}
