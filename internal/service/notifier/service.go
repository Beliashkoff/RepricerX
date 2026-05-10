// Package notifier — сервис уведомлений: персистит событие и фан-аутит
// доставку по включённым каналам (in-app/email/telegram/webhook). См.
// /home/belia/.claude/plans/radiant-percolating-pillow.md (Этап A).
package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Channel — канал доставки. Реализации могут блокировать вызов: воркер
// исполняет их в отдельной goroutine с тайм-аутом.
type Channel interface {
	Name() string
	// Deliver выполняет одну попытку доставки. Возвращает:
	//   nil — отправлено успешно
	//   errors.Is(err, ErrSkip) — событие осознанно пропущено (mute, нет адреса)
	//   любой другой error — будет ретрай через background_jobs
	Deliver(ctx context.Context, n *domain.Notification, d *domain.NotificationDelivery) error
	// DigestDeliver — отправка пакетом (для email-дайджеста). Каналы, не
	// поддерживающие digest (telegram/webhook/in_app), могут возвращать
	// ErrDigestNotSupported.
	DigestDeliver(ctx context.Context, userID uuid.UUID, items []*domain.Notification) error
}

// ErrSkip — мягкий пропуск без ретрая (например, у пользователя нет TG-привязки).
var ErrSkip = errors.New("notifier: skip delivery")

// ErrDigestNotSupported — канал не поддерживает дайджест.
var ErrDigestNotSupported = errors.New("notifier: digest not supported")

// Event — фабрика, рендерящая запись в notifications.Create.
type Event struct {
	Type          string
	Severity      string
	Title         string
	Body          string
	Data          map[string]any
	ShopID        *uuid.UUID
	PlanID        *uuid.UUID
	CorrelationID *uuid.UUID
	// DedupeWindow > 0 → перед Create проверим, не было ли уже такого события
	// для (user_id, event_type [, shop_id]) за окно.
	DedupeWindow time.Duration
}

// Deps — зависимости сервиса.
type Deps struct {
	Pool          *pgxpool.Pool
	Notifications repository.NotificationsRepository
	Preferences   repository.NotificationPreferencesRepository
	Deliveries    repository.NotificationDeliveriesRepository
	ChannelSet    repository.UserChannelSettingsRepository
	TelegramRepo  repository.TelegramLinksRepository
	WebhooksRepo  repository.WebhooksRepository
	Jobs          repository.BackgroundJobsRepository
	Users         repository.UsersRepository
	Log           *slog.Logger
	// Каналы регистрируются после создания сервиса через Register.
}

// Accessor-обёртки используются API-методами в api.go: писать
// `s.deps.Telegram().…` короче и выразительнее, чем `s.deps.TelegramRepo.…`.
func (d Deps) Telegram() repository.TelegramLinksRepository { return d.TelegramRepo }
func (d Deps) Webhooks() repository.WebhooksRepository      { return d.WebhooksRepo }

// Service — единая точка эмиссии.
type Service struct {
	deps     Deps
	mu       sync.RWMutex
	channels map[string]Channel
	now      func() time.Time
}

// New создаёт сервис без каналов. Каналы регистрируются через Register
// после конструирования (чтобы избежать циклической зависимости в wiring-е).
func New(deps Deps) *Service {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Service{
		deps:     deps,
		channels: map[string]Channel{},
		now:      time.Now,
	}
}

// Register добавляет канал. Можно вызывать несколько раз; повторная
// регистрация перезаписывает.
func (s *Service) Register(c Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[c.Name()] = c
}

// HasChannel — для проверки активности канала из вызывающего кода.
func (s *Service) HasChannel(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.channels[name]
	return ok
}

// channel — внутренний резолвер.
func (s *Service) channel(name string) (Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.channels[name]
	return c, ok
}

// Emit публикует событие пользователю. Гарантия:
//   - в БД появится notifications + notification_deliveries для всех
//     включённых каналов (одной транзакцией);
//   - для не-instant каналов в digest-режиме delivery остаётся в статусе
//     'pending_digest';
//   - для остальных каналов enqueue-ится background_job
//     'notification_deliver' (одна на канал).
//
// Дедуп: если e.DedupeWindow > 0 и аналогичное событие уже было — Emit
// возвращает nil без записи.
func (s *Service) Emit(ctx context.Context, userID uuid.UUID, e Event) error {
	if userID == uuid.Nil {
		return errors.New("notifier: empty user id")
	}
	if e.Type == "" || e.Severity == "" || e.Title == "" {
		return fmt.Errorf("notifier: empty required fields in event")
	}

	if e.DedupeWindow > 0 {
		since := s.now().Add(-e.DedupeWindow)
		var exists bool
		var err error
		if e.CorrelationID != nil {
			exists, err = s.deps.Notifications.ExistsRecentByCorrelation(ctx, userID, e.Type, *e.CorrelationID, since)
		} else {
			exists, err = s.deps.Notifications.ExistsRecentByDedupe(ctx, userID, e.Type, e.ShopID, since)
		}
		if err != nil {
			s.deps.Log.Warn("notifier: dedupe check failed", "err", err, "event_type", e.Type)
		} else if exists {
			return nil
		}
	}

	// Решаем, какие каналы используем для этого юзера.
	channels, err := s.resolveChannels(ctx, userID, e.Type)
	if err != nil {
		return fmt.Errorf("resolve channels: %w", err)
	}
	if len(channels) == 0 {
		// Минимум один канал должен быть — in_app форсим всегда.
		channels = []string{domain.NotificationChannelInApp}
	}

	dataBytes, err := encodeData(e.Data)
	if err != nil {
		return fmt.Errorf("encode data: %w", err)
	}

	tx, err := s.deps.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	n, err := s.deps.Notifications.Create(ctx, tx, repository.NotificationCreate{
		UserID:        userID,
		EventType:     e.Type,
		Severity:      e.Severity,
		Title:         e.Title,
		Body:          e.Body,
		Data:          dataBytes,
		ShopID:        e.ShopID,
		PlanID:        e.PlanID,
		CorrelationID: e.CorrelationID,
	})
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	type plannedDelivery struct {
		delivery *domain.NotificationDelivery
		channel  string
		instant  bool
	}
	var planned []plannedDelivery

	settingsByChannel, err := s.loadChannelSettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("load channel settings: %w", err)
	}

	for _, ch := range channels {
		status := domain.NotificationDeliveryStatusPending
		instant := true
		if ch != domain.NotificationChannelInApp {
			settings := settingsByChannel[ch]
			if settings != nil && settings.DigestWindowMinutes > 0 && !severityMeetsMin(e.Severity, settings.DigestMinSeverity) {
				status = domain.NotificationDeliveryStatusSkipped
				instant = false
			} else if shouldDigest(settings, e.Severity) {
				status = domain.NotificationDeliveryStatusPendingDigest
				instant = false
			}
		}
		d, err := s.deps.Deliveries.Create(ctx, tx, repository.NotificationDeliveryCreate{
			NotificationID: n.ID,
			Channel:        ch,
			Status:         status,
		})
		if err != nil {
			return fmt.Errorf("create delivery (%s): %w", ch, err)
		}
		planned = append(planned, plannedDelivery{delivery: d, channel: ch, instant: instant})
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Enqueue фоновые задачи для instant-каналов кроме in_app.
	for _, p := range planned {
		if p.channel == domain.NotificationChannelInApp {
			// In-app — это просто запись в БД; ничего отправлять не надо.
			if err := s.deps.Deliveries.UpdateStatus(ctx, p.delivery.ID,
				domain.NotificationDeliveryStatusSent, "", ptrTime(s.now())); err != nil {
				s.deps.Log.Warn("notifier: mark in_app sent", "err", err)
			}
			continue
		}
		if p.delivery.Status == domain.NotificationDeliveryStatusSkipped {
			if err := s.deps.Deliveries.UpdateStatus(ctx, p.delivery.ID,
				domain.NotificationDeliveryStatusSkipped, "below digest_min_severity", ptrTime(s.now())); err != nil {
				s.deps.Log.Warn("notifier: mark skipped", "err", err)
			}
			continue
		}
		if !p.instant {
			continue
		}
		payload, err := json.Marshal(domain.NotificationDeliveryJobPayload{
			NotificationID: n.ID, Channel: p.channel, DeliveryID: p.delivery.ID,
		})
		if err != nil {
			s.deps.Log.Warn("notifier: marshal payload", "err", err)
			continue
		}
		job, err := s.deps.Jobs.Enqueue(ctx, repository.BackgroundJobEnqueue{
			JobType:     domain.BackgroundJobTypeNotificationDeliver,
			Payload:     payload,
			MaxAttempts: 5,
		})
		if err != nil {
			s.deps.Log.Warn("notifier: enqueue delivery", "err", err, "channel", p.channel)
			continue
		}
		if err := s.deps.Deliveries.AttachJob(ctx, p.delivery.ID, job.ID); err != nil {
			s.deps.Log.Warn("notifier: attach job", "err", err)
		}
	}

	return nil
}

// EmitToAdmins — для системных событий, у которых нет конкретного владельца.
// Резолвится в пользователей с is_admin=TRUE; если админов нет — событие
// записывается в лог и теряется (не критично).
func (s *Service) EmitToAdmins(ctx context.Context, e Event) {
	ids, err := s.deps.Users.ListAdminIDs(ctx)
	if err != nil {
		s.deps.Log.Warn("notifier: list admins", "err", err)
		return
	}
	if len(ids) == 0 {
		s.deps.Log.Warn("notifier: no admins to notify", "event_type", e.Type, "title", e.Title)
		return
	}
	for _, id := range ids {
		if err := s.Emit(ctx, id, e); err != nil {
			s.deps.Log.Warn("notifier: emit to admin", "err", err, "user_id", id)
		}
	}
}

// resolveChannels возвращает список каналов, по которым нужно слать
// событие EventType пользователю. In-app — всегда; остальные — по prefs.
func (s *Service) resolveChannels(ctx context.Context, userID uuid.UUID, eventType string) ([]string, error) {
	out := []string{domain.NotificationChannelInApp}
	for _, ch := range []string{domain.NotificationChannelEmail, domain.NotificationChannelTelegram, domain.NotificationChannelWebhook} {
		if _, ok := s.channel(ch); !ok {
			continue
		}
		dflt := defaultPreference(eventType, ch)
		enabled, err := s.deps.Preferences.IsEnabled(ctx, userID, eventType, ch, dflt)
		if err != nil {
			return nil, err
		}
		if enabled {
			out = append(out, ch)
		}
	}
	return out, nil
}

func (s *Service) loadChannelSettings(ctx context.Context, userID uuid.UUID) (map[string]*domain.UserChannelSettings, error) {
	list, err := s.deps.ChannelSet.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*domain.UserChannelSettings, len(list))
	for _, s := range list {
		out[s.Channel] = s
	}
	return out, nil
}

// shouldDigest решает, копить ли событие в дайджест или слать сразу.
// Если settings == nil — берём дефолт (instant).
// Severity 'error' пропускает дайджест, если фильтр настроен иначе.
func shouldDigest(settings *domain.UserChannelSettings, severity string) bool {
	if settings == nil || settings.DigestWindowMinutes == 0 {
		return false
	}
	if severity == domain.NotificationSeverityError && settings.DigestMinSeverity != domain.NotificationSeverityError {
		return false
	}
	if !severityMeetsMin(severity, settings.DigestMinSeverity) {
		// Если severity ниже фильтра — событие в дайджест не попадает.
		// Соответственно для канала это будет 'sent' instant'ом? Нет:
		// фильтр касается дайджеста; если событие не проходит фильтр,
		// то по этому каналу его не отправим вовсе. Возвращаем false,
		// canalу останется отметить delivery как skipped.
		return false
	}
	return true
}

func severityMeetsMin(actual, min string) bool {
	rank := func(s string) int {
		switch s {
		case domain.NotificationSeverityError:
			return 3
		case domain.NotificationSeverityWarning:
			return 2
		default:
			return 1
		}
	}
	return rank(actual) >= rank(min)
}

// defaultPreference задаёт дефолтное значение, если пользователь ещё не
// настраивал свои предпочтения.
func defaultPreference(eventType, channel string) bool {
	switch channel {
	case domain.NotificationChannelInApp:
		return true
	case domain.NotificationChannelEmail:
		// Email по умолчанию для итогов и ошибок интеграции/cron, не для каждого
		// мелкого сигнала.
		switch eventType {
		case domain.NotificationEventDispatchCompleted,
			domain.NotificationEventImportCompleted,
			domain.NotificationEventIntegrationError,
			domain.NotificationEventScheduledJobFailed,
			domain.NotificationEventBusinessWarningNoCost,
			domain.NotificationEventBusinessWarningNoCompetitors,
			domain.NotificationEventBusinessWarningPriceDrift:
			return true
		default:
			return false
		}
	case domain.NotificationChannelTelegram, domain.NotificationChannelWebhook:
		// Эти каналы — opt-in; пока пользователь явно не подключит и не
		// включит конкретные события, ничего не шлём.
		return false
	}
	return false
}

func encodeData(m map[string]any) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func ptrTime(t time.Time) *time.Time { return &t }
