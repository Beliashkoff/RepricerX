package notifier

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/mailer"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// EmailChannel — отправляет уведомления по электронной почте.
//
// Использует mailer.Mailer (smtp/log реализации). Поддерживает дайджест.
type EmailChannel struct {
	mailer    mailer.Mailer
	users     repository.UsersRepository
	frontend  string // origin сайта для deeplink-ов и unsubscribe-link
}

// NewEmailChannel создаёт канал. frontendURL — origin вида
// «https://app.example.com» (без завершающего слэша).
func NewEmailChannel(m mailer.Mailer, users repository.UsersRepository, frontendURL string) *EmailChannel {
	return &EmailChannel{mailer: m, users: users, frontend: frontendURL}
}

func (c *EmailChannel) Name() string { return domain.NotificationChannelEmail }

func (c *EmailChannel) Deliver(ctx context.Context, n *domain.Notification, _ *domain.NotificationDelivery) error {
	to, err := c.userEmail(ctx, n.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSkip
		}
		return err
	}
	if to == "" {
		return ErrSkip
	}

	label, color := severityPresentation(n.Severity)
	html, text, err := mailer.RenderNotification(mailer.NotificationData{
		Title:          n.Title,
		Body:           n.Body,
		SeverityLabel:  label,
		SeverityColor:  color,
		OpenURL:        c.deeplink(n),
		PreferencesURL: c.preferencesURL(),
	})
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	subject := fmt.Sprintf("[%s] %s", label, truncate(n.Title, 120))
	return c.mailer.Send(ctx, to, subject, html, text)
}

func (c *EmailChannel) DigestDeliver(ctx context.Context, userID uuid.UUID, items []*domain.Notification) error {
	to, err := c.userEmail(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSkip
		}
		return err
	}
	if to == "" || len(items) == 0 {
		return ErrSkip
	}

	digestItems := make([]mailer.NotificationDigestItem, 0, len(items))
	periodStart := items[0].CreatedAt
	periodEnd := items[0].CreatedAt
	for _, it := range items {
		if it.CreatedAt.Before(periodStart) {
			periodStart = it.CreatedAt
		}
		if it.CreatedAt.After(periodEnd) {
			periodEnd = it.CreatedAt
		}
		label, color := severityPresentation(it.Severity)
		digestItems = append(digestItems, mailer.NotificationDigestItem{
			Title:         it.Title,
			Body:          it.Body,
			SeverityLabel: label,
			SeverityColor: color,
			CreatedAt:     it.CreatedAt.UTC().Format("02.01.2006 15:04 MST"),
		})
	}

	html, text, err := mailer.RenderNotificationDigest(mailer.NotificationDigestData{
		PeriodStart:    periodStart.UTC().Format("02.01.2006 15:04 MST"),
		PeriodEnd:      periodEnd.UTC().Format("02.01.2006 15:04 MST"),
		Count:          len(items),
		Items:          digestItems,
		OpenURL:        c.notificationsListURL(),
		PreferencesURL: c.preferencesURL(),
	})
	if err != nil {
		return fmt.Errorf("render digest: %w", err)
	}

	subject := fmt.Sprintf("Сводка RepricerX: %d событий", len(items))
	return c.mailer.Send(ctx, to, subject, html, text)
}

func (c *EmailChannel) userEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	u, err := c.users.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if u.Status != domain.UserStatusActive {
		return "", nil
	}
	return u.Email, nil
}

func (c *EmailChannel) deeplink(n *domain.Notification) string {
	if c.frontend == "" {
		return ""
	}
	return fmt.Sprintf("%s/notifications/%s", c.frontend, n.ID)
}

func (c *EmailChannel) notificationsListURL() string {
	if c.frontend == "" {
		return ""
	}
	return c.frontend + "/notifications"
}

func (c *EmailChannel) preferencesURL() string {
	if c.frontend == "" {
		return ""
	}
	return c.frontend + "/settings"
}

// severityPresentation возвращает русскую подпись и цвет бейджа.
func severityPresentation(severity string) (label, color string) {
	switch severity {
	case domain.NotificationSeverityWarning:
		return "Внимание", "#d97706"
	case domain.NotificationSeverityError:
		return "Ошибка", "#dc2626"
	default:
		return "Информация", "#2563eb"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// безопасный обрез по байту — для заголовков письма допустимо
	return s[:n] + "…"
}

// silence "time" import not used if compiler reorders; keep explicit.
var _ = time.Time{}
