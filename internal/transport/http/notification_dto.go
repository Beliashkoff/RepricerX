package transport

import (
	"encoding/json"
	"time"
)

type notificationResponse struct {
	ID            string          `json:"id"`
	EventType     string          `json:"event_type"`
	Severity      string          `json:"severity"`
	Title         string          `json:"title"`
	Body          string          `json:"body"`
	Data          json.RawMessage `json:"data"`
	ShopID        *string         `json:"shop_id,omitempty"`
	PlanID        *string         `json:"plan_id,omitempty"`
	CorrelationID *string         `json:"correlation_id,omitempty"`
	ReadAt        *time.Time      `json:"read_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type notificationListResponse struct {
	Items      []notificationResponse `json:"items"`
	Pagination paginationInfo         `json:"pagination"`
	Unread     int                    `json:"unread"`
}

type unreadCountResponse struct {
	Count int `json:"count"`
}

type notificationDeliveryResponse struct {
	ID        string     `json:"id"`
	Channel   string     `json:"channel"`
	Status    string     `json:"status"`
	Attempts  int        `json:"attempts"`
	LastError string     `json:"last_error,omitempty"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type notificationDetailResponse struct {
	Notification notificationResponse           `json:"notification"`
	Deliveries   []notificationDeliveryResponse `json:"deliveries"`
}

type preferenceItem struct {
	EventType string `json:"event_type"`
	Channel   string `json:"channel"`
	Enabled   bool   `json:"enabled"`
}

type preferencesResponse struct {
	Items []preferenceItem `json:"items"`
}

type updatePreferencesRequest struct {
	Items []preferenceItem `json:"items"`
}

type channelSettingsResponse struct {
	Channel             string     `json:"channel"`
	DigestWindowMinutes int        `json:"digest_window_minutes"`
	DigestMinSeverity   string     `json:"digest_min_severity"`
	QuietHoursStart     *int       `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd       *int       `json:"quiet_hours_end,omitempty"`
	DigestSentAt        *time.Time `json:"digest_sent_at,omitempty"`
}

type channelSettingsListResponse struct {
	Items []channelSettingsResponse `json:"items"`
}

type updateChannelSettingsRequest struct {
	DigestWindowMinutes *int    `json:"digest_window_minutes,omitempty"`
	DigestMinSeverity   *string `json:"digest_min_severity,omitempty"`
	QuietHoursStart     *int    `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd       *int    `json:"quiet_hours_end,omitempty"`
	ClearQuietHours     bool    `json:"clear_quiet_hours,omitempty"`
}

type telegramLinkTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	StartURL  string    `json:"start_url"`
}

type telegramStatusResponse struct {
	Linked   bool       `json:"linked"`
	Username string     `json:"username,omitempty"`
	ChatID   *int64     `json:"chat_id,omitempty"`
	LinkedAt *time.Time `json:"linked_at,omitempty"`
}

type webhookResponse struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Secret      string    `json:"secret,omitempty"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type webhookListResponse struct {
	Items []webhookResponse `json:"items"`
}

type createWebhookRequest struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type webhookTestResponse struct {
	HTTPStatus int    `json:"http_status"`
	Body       string `json:"body"`
	Error      string `json:"error,omitempty"`
}
