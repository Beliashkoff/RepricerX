package notifier

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// ErrInvalidInput — публичная ошибка для handler-а (400).
var ErrInvalidInput = errors.New("notifier: invalid input")

// ListNotifications — постраничный список уведомлений пользователя.
func (s *Service) ListNotifications(ctx context.Context, userID uuid.UUID, f repository.NotificationListFilter) ([]*domain.Notification, int, error) {
	return s.deps.Notifications.ListForUser(ctx, userID, f)
}

func (s *Service) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.deps.Notifications.CountUnread(ctx, userID)
}

func (s *Service) MarkNotificationRead(ctx context.Context, userID, id uuid.UUID) error {
	return s.deps.Notifications.MarkRead(ctx, userID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.deps.Notifications.MarkAllRead(ctx, userID)
}

func (s *Service) DeleteNotification(ctx context.Context, userID, id uuid.UUID) error {
	return s.deps.Notifications.Delete(ctx, userID, id)
}

func (s *Service) GetNotification(ctx context.Context, userID, id uuid.UUID) (*domain.Notification, []*domain.NotificationDelivery, error) {
	n, err := s.deps.Notifications.GetByIDForUser(ctx, userID, id)
	if err != nil {
		return nil, nil, err
	}
	deliveries, err := s.deps.Deliveries.ListByNotification(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return n, deliveries, nil
}

// ---------------------------------------------------------------------------
// Preferences

func (s *Service) ListPreferences(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error) {
	return s.deps.Preferences.List(ctx, userID)
}

func (s *Service) UpsertPreferences(ctx context.Context, userID uuid.UUID, prefs []domain.NotificationPreference) error {
	if len(prefs) == 0 {
		return nil
	}
	for i := range prefs {
		prefs[i].UserID = userID
		if !contains(domain.AllNotificationEvents(), prefs[i].EventType) {
			return fmt.Errorf("%w: unknown event_type %q", ErrInvalidInput, prefs[i].EventType)
		}
		if !contains(domain.AllNotificationChannels(), prefs[i].Channel) {
			return fmt.Errorf("%w: unknown channel %q", ErrInvalidInput, prefs[i].Channel)
		}
		if prefs[i].Channel == domain.NotificationChannelInApp && !prefs[i].Enabled {
			// In-app нельзя выключить — это база.
			return fmt.Errorf("%w: in_app channel cannot be disabled", ErrInvalidInput)
		}
	}
	return s.deps.Preferences.Upsert(ctx, prefs)
}

// ---------------------------------------------------------------------------
// User channel settings

func (s *Service) ListChannelSettings(ctx context.Context, userID uuid.UUID) ([]*domain.UserChannelSettings, error) {
	return s.deps.ChannelSet.List(ctx, userID)
}

// UpsertChannelSettings проверяет валидность входных данных и сохраняет.
// Передавай nil-указатель в полях, которые менять не надо.
func (s *Service) UpsertChannelSettings(ctx context.Context, userID uuid.UUID, channel string, in repository.UserChannelSettingsUpdate) (*domain.UserChannelSettings, error) {
	if !contains(domain.AllNotificationChannels(), channel) {
		return nil, fmt.Errorf("%w: unknown channel %q", ErrInvalidInput, channel)
	}
	if channel == domain.NotificationChannelInApp {
		// In-app не имеет дайджеста и quiet hours.
		return nil, fmt.Errorf("%w: in_app channel has no schedule", ErrInvalidInput)
	}
	if in.DigestWindowMinutes != nil {
		if !contains(domain.DigestWindowMinutesAllowed, *in.DigestWindowMinutes) {
			return nil, fmt.Errorf("%w: digest_window_minutes %d not allowed", ErrInvalidInput, *in.DigestWindowMinutes)
		}
	}
	if in.DigestMinSeverity != nil {
		switch *in.DigestMinSeverity {
		case domain.NotificationSeverityInfo, domain.NotificationSeverityWarning, domain.NotificationSeverityError:
		default:
			return nil, fmt.Errorf("%w: bad digest_min_severity %q", ErrInvalidInput, *in.DigestMinSeverity)
		}
	}
	if !in.ClearQuietHours && (in.QuietHoursStart != nil) != (in.QuietHoursEnd != nil) {
		return nil, fmt.Errorf("%w: quiet_hours_start and quiet_hours_end must be set together", ErrInvalidInput)
	}
	for _, h := range []*int{in.QuietHoursStart, in.QuietHoursEnd} {
		if h == nil {
			continue
		}
		if *h < 0 || *h > 23 {
			return nil, fmt.Errorf("%w: hour out of [0,23]", ErrInvalidInput)
		}
	}
	return s.deps.ChannelSet.Upsert(ctx, userID, channel, in)
}

// ---------------------------------------------------------------------------
// Telegram link

const telegramLinkTokenTTL = 15 * time.Minute

// IssueTelegramLinkToken генерирует одноразовый токен для привязки пользователя.
// Возвращает уже base64-encoded строку, которую нужно положить в start-параметр
// Telegram-ссылки.
func (s *Service) IssueTelegramLinkToken(ctx context.Context, userID uuid.UUID) (string, time.Time, error) {
	tok, err := generateLinkToken(24)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().Add(telegramLinkTokenTTL)
	if err := s.deps.Telegram().IssueToken(ctx, userID, tok, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return tok, expiresAt, nil
}

// TelegramStatus — связан ли пользователь с Telegram.
type TelegramStatus struct {
	Linked   bool
	Username string
	ChatID   *int64
	LinkedAt *time.Time
}

func (s *Service) TelegramStatus(ctx context.Context, userID uuid.UUID) (TelegramStatus, error) {
	link, err := s.deps.Telegram().GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return TelegramStatus{}, nil
		}
		return TelegramStatus{}, err
	}
	return TelegramStatus{
		Linked:   link.ChatID != nil && link.LinkedAt != nil,
		Username: link.Username,
		ChatID:   link.ChatID,
		LinkedAt: link.LinkedAt,
	}, nil
}

func (s *Service) UnlinkTelegram(ctx context.Context, userID uuid.UUID) error {
	err := s.deps.Telegram().Unlink(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// Webhooks

func (s *Service) ListWebhooks(ctx context.Context, userID uuid.UUID) ([]*domain.Webhook, error) {
	return s.deps.Webhooks().List(ctx, userID)
}

func (s *Service) CreateWebhook(ctx context.Context, userID uuid.UUID, urlStr, description string) (*domain.Webhook, error) {
	urlStr = strings.TrimSpace(urlStr)
	parsed, err := url.Parse(urlStr)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("%w: webhook url must be http(s)", ErrInvalidInput)
	}
	if len(description) > 200 {
		description = description[:200]
	}
	secret, err := generateLinkToken(32)
	if err != nil {
		return nil, err
	}
	return s.deps.Webhooks().Create(ctx, userID, repository.WebhookCreate{
		URL:         urlStr,
		Secret:      secret,
		Description: description,
	})
}

func (s *Service) SetWebhookEnabled(ctx context.Context, userID, id uuid.UUID, enabled bool) error {
	return s.deps.Webhooks().SetEnabled(ctx, userID, id, enabled)
}

func (s *Service) DeleteWebhook(ctx context.Context, userID, id uuid.UUID) error {
	return s.deps.Webhooks().Delete(ctx, userID, id)
}

// GetWebhook — для эндпоинта «тест webhook».
func (s *Service) GetWebhook(ctx context.Context, userID, id uuid.UUID) (*domain.Webhook, error) {
	return s.deps.Webhooks().GetByIDForUser(ctx, userID, id)
}

// ---------------------------------------------------------------------------
// helpers

func contains[T comparable](xs []T, v T) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func generateLinkToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
