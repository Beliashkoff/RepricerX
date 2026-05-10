package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Каналы доставки уведомлений. БД хранит как TEXT — список валидируется в коде.
const (
	NotificationChannelInApp    = "in_app"
	NotificationChannelEmail    = "email"
	NotificationChannelTelegram = "telegram"
	NotificationChannelWebhook  = "webhook"
)

func AllNotificationChannels() []string {
	return []string{
		NotificationChannelInApp,
		NotificationChannelEmail,
		NotificationChannelTelegram,
		NotificationChannelWebhook,
	}
}

// Уровни критичности.
const (
	NotificationSeverityInfo    = "info"
	NotificationSeverityWarning = "warning"
	NotificationSeverityError   = "error"
)

// Статусы доставки в notification_deliveries.
const (
	NotificationDeliveryStatusPending       = "pending"
	NotificationDeliveryStatusPendingDigest = "pending_digest"
	NotificationDeliveryStatusQueuedDigest  = "queued_digest"
	NotificationDeliveryStatusSent          = "sent"
	NotificationDeliveryStatusFailed        = "failed"
	NotificationDeliveryStatusSkipped       = "skipped"
)

// Типы событий. Перечислены централизованно, чтобы не было опечаток
// в местах эмиссии и в шаблонах.
const (
	NotificationEventDispatchCompleted             = "dispatch_completed"
	NotificationEventRecalcCompleted               = "recalc_completed"
	NotificationEventImportCompleted               = "import_completed"
	NotificationEventIntegrationError              = "integration_error"
	NotificationEventScheduledJobFailed            = "scheduled_job_failed"
	NotificationEventConstraintHit                 = "constraint_hit"
	NotificationEventBusinessWarningNoCost         = "business_warning_no_cost"
	NotificationEventBusinessWarningNoCompetitors  = "business_warning_no_competitors"
	NotificationEventBusinessWarningPriceDrift     = "business_warning_price_drift"
	NotificationEventCompetitorPriceDropped        = "competitor_price_dropped"
	NotificationEventCompetitorAppeared            = "competitor_appeared"
	NotificationEventMedianShifted                 = "median_shifted"
)

// AllNotificationEvents — стабильный список для UI/preferences и валидации.
func AllNotificationEvents() []string {
	return []string{
		NotificationEventDispatchCompleted,
		NotificationEventRecalcCompleted,
		NotificationEventImportCompleted,
		NotificationEventIntegrationError,
		NotificationEventScheduledJobFailed,
		NotificationEventConstraintHit,
		NotificationEventBusinessWarningNoCost,
		NotificationEventBusinessWarningNoCompetitors,
		NotificationEventBusinessWarningPriceDrift,
		NotificationEventCompetitorPriceDropped,
		NotificationEventCompetitorAppeared,
		NotificationEventMedianShifted,
	}
}

// Допустимые окна дайджеста (в минутах). 0 — instant.
var DigestWindowMinutesAllowed = []int{0, 15, 60, 240, 1440}

// Типы фоновых джоб для notifier-а.
const (
	BackgroundJobTypeNotificationDeliver       = "notification_deliver"
	BackgroundJobTypeNotificationDigestDeliver = "notification_digest_deliver"
)

// NotificationDeliveryJobPayload — payload для одиночной доставки.
type NotificationDeliveryJobPayload struct {
	NotificationID uuid.UUID `json:"notification_id"`
	Channel        string    `json:"channel"`
	DeliveryID     uuid.UUID `json:"delivery_id"`
}

// NotificationDigestJobPayload — payload для дайджеста.
type NotificationDigestJobPayload struct {
	UserID      uuid.UUID   `json:"user_id"`
	Channel     string      `json:"channel"`
	DeliveryIDs []uuid.UUID `json:"delivery_ids"`
}

// Notification — одно событие, адресованное пользователю.
// data хранит структурированные параметры (counts, ids, etc.).
type Notification struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	EventType     string
	Severity      string
	Title         string
	Body          string
	Data          json.RawMessage
	ShopID        *uuid.UUID
	PlanID        *uuid.UUID
	CorrelationID *uuid.UUID
	ReadAt        *time.Time
	CreatedAt     time.Time
}

// NotificationPreference — переключатель «слать ли событие EventType
// по каналу Channel пользователю UserID». Отсутствие строки = дефолт
// (резолвится в сервисе).
type NotificationPreference struct {
	UserID    uuid.UUID
	EventType string
	Channel   string
	Enabled   bool
	UpdatedAt time.Time
}

// NotificationDelivery — состояние доставки конкретного notification по
// конкретному каналу. Уникален по (notification_id, channel).
type NotificationDelivery struct {
	ID             uuid.UUID
	NotificationID uuid.UUID
	Channel        string
	Status         string
	Attempts       int
	LastError      string
	JobID          *uuid.UUID
	SentAt         *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UserChannelSettings — расписание/фильтр конкретного канала для пользователя.
// digest_window_minutes=0 значит instant (немедленная отправка).
type UserChannelSettings struct {
	UserID              uuid.UUID
	Channel             string
	DigestWindowMinutes int
	DigestMinSeverity   string
	QuietHoursStart     *int
	QuietHoursEnd       *int
	DigestSentAt        *time.Time
	UpdatedAt           time.Time
}

// TelegramLink — связка пользователя с его chat_id в Telegram.
// link_token заполнен только в окно «выдан токен → пользователь нажал /start».
type TelegramLink struct {
	UserID              uuid.UUID
	ChatID              *int64
	Username            string
	LinkToken           *string
	LinkTokenExpiresAt  *time.Time
	LinkedAt            *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Webhook — outbound URL пользователя для доставки событий.
type Webhook struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	URL         string
	Secret      string
	Enabled     bool
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
