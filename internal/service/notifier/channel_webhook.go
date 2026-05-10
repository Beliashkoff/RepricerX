package notifier

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

type WebhookChannel struct {
	repo       repository.WebhooksRepository
	httpClient *http.Client
}

func NewWebhookChannel(repo repository.WebhooksRepository) *WebhookChannel {
	return &WebhookChannel{
		repo:       repo,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *WebhookChannel) Name() string { return domain.NotificationChannelWebhook }

func (c *WebhookChannel) Deliver(ctx context.Context, n *domain.Notification, _ *domain.NotificationDelivery) error {
	webhooks, err := c.repo.ListEnabledForUser(ctx, n.UserID)
	if err != nil {
		return err
	}
	if len(webhooks) == 0 {
		return ErrSkip
	}

	var lastErr error
	sent := 0
	for _, w := range webhooks {
		if err := c.deliverOne(ctx, w, n); err != nil {
			lastErr = err
			continue
		}
		sent++
	}
	if sent == 0 && lastErr != nil {
		return lastErr
	}
	return nil
}

func (c *WebhookChannel) DigestDeliver(context.Context, uuid.UUID, []*domain.Notification) error {
	return ErrDigestNotSupported
}

func (c *WebhookChannel) deliverOne(ctx context.Context, w *domain.Webhook, n *domain.Notification) error {
	payload := map[string]any{
		"event_type": n.EventType,
		"severity":   n.Severity,
		"title":      n.Title,
		"body":       n.Body,
		"data":       json.RawMessage(n.Data),
		"created_at": n.CreatedAt,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "RepricerX-Notifier/1.0")
	req.Header.Set("X-RepricerX-Signature", webhookSignature(w.Secret, body))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func webhookSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
