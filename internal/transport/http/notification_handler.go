package transport

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	notifiersvc "github.com/Beliashkoff/RepricerX/internal/service/notifier"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type notificationHandler struct {
	svc         *notifiersvc.Service
	telegramURL string // префикс https://t.me/<botname>?start=
	httpClient  *http.Client
}

// NewNotificationHandler — конструктор. telegramBotURL — пустая строка, если
// бот не подключён; в таком случае линк-эндпоинт вернёт 503.
func NewNotificationHandler(svc *notifiersvc.Service, telegramBotURL string) *notificationHandler {
	return &notificationHandler{
		svc:         svc,
		telegramURL: telegramBotURL,
		httpClient:  &http.Client{Timeout: 8 * time.Second},
	}
}

// List godoc
//
//	@Summary	Список уведомлений пользователя
//	@Tags		notifications
//	@Produce	json
//	@Success	200	{object}	notificationListResponse
//	@Security	SessionCookie
//	@Router		/api/notifications [get]
func (h *notificationHandler) List(c *gin.Context) {
	user := mustUser(c)
	f, ok := parseNotificationFilter(c)
	if !ok {
		return
	}
	items, total, err := h.svc.ListNotifications(c.Request.Context(), user.ID, f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	unread, err := h.svc.UnreadCount(c.Request.Context(), user.ID)
	if err != nil {
		unread = 0
	}
	resp := notificationListResponse{
		Items:      make([]notificationResponse, 0, len(items)),
		Pagination: paginationInfo{Page: maxInt(f.Page, 1), PerPage: defaultPerPage(f.PerPage), Total: total},
		Unread:     unread,
	}
	for _, n := range items {
		resp.Items = append(resp.Items, toNotificationResponse(n))
	}
	c.JSON(http.StatusOK, resp)
}

// UnreadCount godoc
//
//	@Summary	Счётчик непрочитанных уведомлений
//	@Tags		notifications
//	@Produce	json
//	@Success	200	{object}	unreadCountResponse
//	@Security	SessionCookie
//	@Router		/api/notifications/unread-count [get]
func (h *notificationHandler) UnreadCount(c *gin.Context) {
	user := mustUser(c)
	count, err := h.svc.UnreadCount(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.JSON(http.StatusOK, unreadCountResponse{Count: count})
}

// Get — детали одного уведомления + статусы доставки.
func (h *notificationHandler) Get(c *gin.Context) {
	user := mustUser(c)
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	n, deliveries, err := h.svc.GetNotification(c.Request.Context(), user.ID, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			errResp(c, http.StatusNotFound, "not_found", "Уведомление не найдено")
			return
		}
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	resp := notificationDetailResponse{
		Notification: toNotificationResponse(n),
		Deliveries:   make([]notificationDeliveryResponse, 0, len(deliveries)),
	}
	for _, d := range deliveries {
		resp.Deliveries = append(resp.Deliveries, notificationDeliveryResponse{
			ID:        d.ID.String(),
			Channel:   d.Channel,
			Status:    d.Status,
			Attempts:  d.Attempts,
			LastError: d.LastError,
			SentAt:    d.SentAt,
			UpdatedAt: d.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (h *notificationHandler) MarkRead(c *gin.Context) {
	user := mustUser(c)
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.MarkNotificationRead(c.Request.Context(), user.ID, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			errResp(c, http.StatusNotFound, "not_found", "Уведомление не найдено")
			return
		}
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *notificationHandler) MarkAllRead(c *gin.Context) {
	user := mustUser(c)
	updated, err := h.svc.MarkAllRead(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": updated})
}

func (h *notificationHandler) Delete(c *gin.Context) {
	user := mustUser(c)
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteNotification(c.Request.Context(), user.ID, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			errResp(c, http.StatusNotFound, "not_found", "Уведомление не найдено")
			return
		}
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Preferences

func (h *notificationHandler) GetPreferences(c *gin.Context) {
	user := mustUser(c)
	prefs, err := h.svc.ListPreferences(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	resp := preferencesResponse{Items: make([]preferenceItem, 0, len(prefs))}
	for _, p := range prefs {
		resp.Items = append(resp.Items, preferenceItem{
			EventType: p.EventType, Channel: p.Channel, Enabled: p.Enabled,
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (h *notificationHandler) UpdatePreferences(c *gin.Context) {
	user := mustUser(c)
	var req updatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	prefs := make([]domain.NotificationPreference, 0, len(req.Items))
	for _, it := range req.Items {
		prefs = append(prefs, domain.NotificationPreference{
			EventType: it.EventType, Channel: it.Channel, Enabled: it.Enabled,
		})
	}
	if err := h.svc.UpsertPreferences(c.Request.Context(), user.ID, prefs); err != nil {
		if errors.Is(err, notifiersvc.ErrInvalidInput) {
			errResp(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Channel settings

func (h *notificationHandler) ListChannelSettings(c *gin.Context) {
	user := mustUser(c)
	list, err := h.svc.ListChannelSettings(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	resp := channelSettingsListResponse{Items: make([]channelSettingsResponse, 0, len(list))}
	for _, s := range list {
		resp.Items = append(resp.Items, toChannelSettingsResponse(s))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *notificationHandler) UpdateChannelSettings(c *gin.Context) {
	user := mustUser(c)
	channel := c.Param("channel")
	var req updateChannelSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	updated, err := h.svc.UpsertChannelSettings(c.Request.Context(), user.ID, channel, repository.UserChannelSettingsUpdate{
		DigestWindowMinutes: req.DigestWindowMinutes,
		DigestMinSeverity:   req.DigestMinSeverity,
		QuietHoursStart:     req.QuietHoursStart,
		QuietHoursEnd:       req.QuietHoursEnd,
		ClearQuietHours:     req.ClearQuietHours,
	})
	if err != nil {
		if errors.Is(err, notifiersvc.ErrInvalidInput) {
			errResp(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.JSON(http.StatusOK, toChannelSettingsResponse(updated))
}

// ---------------------------------------------------------------------------
// Telegram link

func (h *notificationHandler) IssueTelegramToken(c *gin.Context) {
	if h.telegramURL == "" {
		errResp(c, http.StatusServiceUnavailable, "telegram_disabled", "Telegram-канал не настроен на сервере")
		return
	}
	user := mustUser(c)
	tok, expires, err := h.svc.IssueTelegramLinkToken(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.JSON(http.StatusOK, telegramLinkTokenResponse{
		Token:     tok,
		ExpiresAt: expires,
		StartURL:  h.telegramURL + tok,
	})
}

func (h *notificationHandler) TelegramStatus(c *gin.Context) {
	user := mustUser(c)
	st, err := h.svc.TelegramStatus(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.JSON(http.StatusOK, telegramStatusResponse{
		Linked: st.Linked, Username: st.Username, ChatID: st.ChatID, LinkedAt: st.LinkedAt,
	})
}

func (h *notificationHandler) UnlinkTelegram(c *gin.Context) {
	user := mustUser(c)
	if err := h.svc.UnlinkTelegram(c.Request.Context(), user.ID); err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Webhooks

func (h *notificationHandler) ListWebhooks(c *gin.Context) {
	user := mustUser(c)
	list, err := h.svc.ListWebhooks(c.Request.Context(), user.ID)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	resp := webhookListResponse{Items: make([]webhookResponse, 0, len(list))}
	for _, w := range list {
		resp.Items = append(resp.Items, webhookResponse{
			ID:          w.ID.String(),
			URL:         w.URL,
			Enabled:     w.Enabled,
			Description: w.Description,
			CreatedAt:   w.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (h *notificationHandler) CreateWebhook(c *gin.Context) {
	user := mustUser(c)
	var req createWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
		return
	}
	w, err := h.svc.CreateWebhook(c.Request.Context(), user.ID, req.URL, req.Description)
	if err != nil {
		if errors.Is(err, notifiersvc.ErrInvalidInput) {
			errResp(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	// Секрет показываем при создании один раз.
	c.JSON(http.StatusCreated, webhookResponse{
		ID:          w.ID.String(),
		URL:         w.URL,
		Secret:      w.Secret,
		Enabled:     w.Enabled,
		Description: w.Description,
		CreatedAt:   w.CreatedAt,
	})
}

func (h *notificationHandler) DeleteWebhook(c *gin.Context) {
	user := mustUser(c)
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteWebhook(c.Request.Context(), user.ID, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			errResp(c, http.StatusNotFound, "not_found", "Webhook не найден")
			return
		}
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	c.Status(http.StatusNoContent)
}

// TestWebhook отправляет тестовый payload на URL вебхука и возвращает ответ.
// Не использует worker — вызов синхронный, чтобы UI сразу показал результат.
func (h *notificationHandler) TestWebhook(c *gin.Context) {
	user := mustUser(c)
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	w, err := h.svc.GetWebhook(c.Request.Context(), user.ID, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			errResp(c, http.StatusNotFound, "not_found", "Webhook не найден")
			return
		}
		errResp(c, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
		return
	}
	body, _ := json.Marshal(map[string]any{
		"event_type": "webhook_test",
		"severity":   "info",
		"title":      "Тестовое событие RepricerX",
		"body":       "Если вы видите это сообщение — webhook настроен правильно.",
		"data": map[string]any{
			"webhook_id": w.ID,
			"sent_at":    time.Now().UTC().Format(time.RFC3339),
		},
	})
	mac := hmac.New(sha256.New, []byte(w.Secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, strings.NewReader(string(body)))
	if err != nil {
		errResp(c, http.StatusBadRequest, "bad_request", "Неверный URL")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-RepricerX-Signature", sig)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, webhookTestResponse{Error: err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	c.JSON(http.StatusOK, webhookTestResponse{
		HTTPStatus: resp.StatusCode,
		Body:       string(respBody),
	})
}

// ---------------------------------------------------------------------------
// helpers

func parseNotificationFilter(c *gin.Context) (repository.NotificationListFilter, bool) {
	var f repository.NotificationListFilter
	f.EventType = c.Query("event_type")
	f.Severity = c.Query("severity")
	if v := c.Query("unread_only"); v == "true" || v == "1" {
		f.UnreadOnly = true
	}
	if raw := c.Query("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат from (RFC3339)")
			return f, false
		}
		f.From = t
	}
	if raw := c.Query("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат to (RFC3339)")
			return f, false
		}
		f.Until = t
	}
	if raw := firstQuery(c, "shop_id", "shopId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат shop_id")
			return f, false
		}
		f.ShopID = &id
	}
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := firstQuery(c, "per_page", "perPage"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.PerPage = n
		}
	}
	return f, true
}

func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	raw := c.Param(name)
	id, err := uuid.Parse(raw)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid_id", fmt.Sprintf("Неверный формат %s", name))
		return uuid.Nil, false
	}
	return id, true
}

func toNotificationResponse(n *domain.Notification) notificationResponse {
	resp := notificationResponse{
		ID:        n.ID.String(),
		EventType: n.EventType,
		Severity:  n.Severity,
		Title:     n.Title,
		Body:      n.Body,
		Data:      n.Data,
		ReadAt:    n.ReadAt,
		CreatedAt: n.CreatedAt,
	}
	if n.ShopID != nil {
		s := n.ShopID.String()
		resp.ShopID = &s
	}
	if n.PlanID != nil {
		s := n.PlanID.String()
		resp.PlanID = &s
	}
	if n.CorrelationID != nil {
		s := n.CorrelationID.String()
		resp.CorrelationID = &s
	}
	return resp
}

func toChannelSettingsResponse(s *domain.UserChannelSettings) channelSettingsResponse {
	return channelSettingsResponse{
		Channel:             s.Channel,
		DigestWindowMinutes: s.DigestWindowMinutes,
		DigestMinSeverity:   s.DigestMinSeverity,
		QuietHoursStart:     s.QuietHoursStart,
		QuietHoursEnd:       s.QuietHoursEnd,
		DigestSentAt:        s.DigestSentAt,
	}
}

func defaultPerPage(v int) int {
	if v <= 0 {
		return 20
	}
	if v > 100 {
		return 100
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
