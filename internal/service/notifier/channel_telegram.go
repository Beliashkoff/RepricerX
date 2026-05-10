package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

type TelegramChannel struct {
	token    string
	links    repository.TelegramLinksRepository
	users    repository.UsersRepository
	frontend string
	client   *http.Client
}

func NewTelegramChannel(token string, links repository.TelegramLinksRepository, users repository.UsersRepository, frontendURL string) *TelegramChannel {
	return &TelegramChannel{
		token:    strings.TrimSpace(token),
		links:    links,
		users:    users,
		frontend: strings.TrimRight(frontendURL, "/"),
		client:   &http.Client{Timeout: 8 * time.Second},
	}
}

func (c *TelegramChannel) Name() string { return domain.NotificationChannelTelegram }

func (c *TelegramChannel) Deliver(ctx context.Context, n *domain.Notification, _ *domain.NotificationDelivery) error {
	if c.token == "" {
		return ErrSkip
	}
	link, err := c.links.GetByUserID(ctx, n.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSkip
		}
		return err
	}
	if link.ChatID == nil {
		return ErrSkip
	}
	u, err := c.users.GetByID(ctx, n.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSkip
		}
		return err
	}
	if u.TelegramMutedUntil != nil && u.TelegramMutedUntil.After(time.Now().UTC()) {
		return ErrSkip
	}
	return c.sendMessage(ctx, *link.ChatID, telegramText(n, c.frontend))
}

func (c *TelegramChannel) DigestDeliver(context.Context, uuid.UUID, []*domain.Notification) error {
	return ErrDigestNotSupported
}

func (c *TelegramChannel) sendMessage(ctx context.Context, chatID int64, text string) error {
	form := url.Values{}
	form.Set("chat_id", fmt.Sprintf("%d", chatID))
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+c.token+"/sendMessage",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return fmt.Errorf("telegram status %d: %s", resp.StatusCode, body.Description)
	}
	return nil
}

func telegramText(n *domain.Notification, frontend string) string {
	var b strings.Builder
	b.WriteString(n.Title)
	if n.Body != "" {
		b.WriteString("\n\n")
		b.WriteString(n.Body)
	}
	if frontend != "" {
		b.WriteString("\n\n")
		b.WriteString(frontend)
		b.WriteString("/notifications/")
		b.WriteString(n.ID.String())
	}
	return b.String()
}
