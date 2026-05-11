package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

// ExecuteDeliveryJob — обработчик BackgroundJobTypeNotificationDeliver.
//
// Контракт ошибок такой же, как у dispatcher.ExecuteDispatchJob:
//
//	nil           → worker.Succeed
//	ErrSkip       → доставка пропущена (delivery статусом 'skipped'); job — Succeed
//	любой другой  → worker.Retry/Fail (через background_jobs)
func (s *Service) ExecuteDeliveryJob(ctx context.Context, job *domain.BackgroundJob) error {
	var p domain.NotificationDeliveryJobPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("notifier: parse payload: %w", err)
	}
	log := s.deps.Log.With(
		"module", "notifier",
		"job_id", job.ID,
		"channel", p.Channel,
		"notification_id", p.NotificationID,
		"delivery_id", p.DeliveryID,
	)

	channel, ok := s.channel(p.Channel)
	if !ok {
		log.Warn("notifier: unknown channel; marking skipped")
		return s.deps.Deliveries.UpdateStatus(ctx, p.DeliveryID,
			domain.NotificationDeliveryStatusSkipped, "unknown channel", ptrTime(s.now()))
	}

	deliveries, notifications, err := s.deps.Deliveries.LoadByIDs(ctx, []uuid.UUID{p.DeliveryID})
	if err != nil {
		return fmt.Errorf("notifier: load delivery: %w", err)
	}
	if len(deliveries) == 0 || len(notifications) == 0 {
		log.Warn("notifier: delivery or notification gone; succeeding")
		return nil
	}
	d := deliveries[0]
	n := notifications[0]

	if err := s.deps.Deliveries.IncrementAttempts(ctx, p.DeliveryID); err != nil {
		log.Warn("notifier: increment attempts", "err", err)
	}

	err = channel.Deliver(ctx, n, d)
	if err == nil {
		return s.deps.Deliveries.UpdateStatus(ctx, p.DeliveryID,
			domain.NotificationDeliveryStatusSent, "", ptrTime(s.now()))
	}
	if errors.Is(err, ErrSkip) {
		log.Info("notifier: delivery skipped", "reason", err.Error())
		return s.deps.Deliveries.UpdateStatus(ctx, p.DeliveryID,
			domain.NotificationDeliveryStatusSkipped, err.Error(), ptrTime(s.now()))
	}
	// Реальная ошибка — пишем last_error, но статус возвращаем 'pending',
	// чтобы при retry worker заново enqueue-ил эту же запись.
	if upd := s.deps.Deliveries.UpdateStatus(ctx, p.DeliveryID,
		domain.NotificationDeliveryStatusPending, err.Error(), nil); upd != nil {
		log.Warn("notifier: persist last_error", "err", upd)
	}
	return err
}

// ExecuteDigestJob — обработчик BackgroundJobTypeNotificationDigestDeliver.
func (s *Service) ExecuteDigestJob(ctx context.Context, job *domain.BackgroundJob) error {
	var p domain.NotificationDigestJobPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("notifier: parse digest payload: %w", err)
	}
	log := s.deps.Log.With(
		"module", "notifier_digest",
		"job_id", job.ID,
		"channel", p.Channel,
		"user_id", p.UserID,
		"count", len(p.DeliveryIDs),
	)

	channel, ok := s.channel(p.Channel)
	if !ok {
		log.Warn("notifier: unknown channel for digest")
		return s.markBatchStatus(ctx, p.DeliveryIDs, domain.NotificationDeliveryStatusSkipped, "unknown channel")
	}

	deliveries, notifications, err := s.deps.Deliveries.LoadByIDs(ctx, p.DeliveryIDs)
	if err != nil {
		return fmt.Errorf("notifier: load digest items: %w", err)
	}
	if len(notifications) == 0 {
		log.Info("notifier: digest empty")
		return nil
	}
	pendingIDs, pendingNotifications := digestPendingItems(deliveries, notifications)
	if len(pendingNotifications) == 0 {
		log.Info("notifier: digest already finalized")
		return nil
	}

	if err := channel.DigestDeliver(ctx, p.UserID, pendingNotifications); err != nil {
		if errors.Is(err, ErrSkip) || errors.Is(err, ErrDigestNotSupported) {
			log.Info("notifier: digest skipped", "reason", err.Error())
			if err := s.markBatchStatus(ctx, pendingIDs, domain.NotificationDeliveryStatusSkipped, err.Error()); err != nil {
				log.Warn("notifier: mark digest skipped", "err", err)
			}
			return nil
		}
		// Возвращаем ошибку — worker устроит retry; статус оставляем
		// queued_digest до повторной попытки.
		return err
	}

	// Письмо уже ушло. Ошибки финализации статусов нельзя возвращать как
	// retryable, иначе worker повторно отправит тот же digest.
	if err := s.markBatchStatus(ctx, pendingIDs, domain.NotificationDeliveryStatusSent, ""); err != nil {
		log.Warn("notifier: mark digest sent", "err", err)
	}
	if err := s.deps.ChannelSet.MarkDigestSent(ctx, p.UserID, p.Channel, s.now()); err != nil {
		log.Warn("notifier: mark digest sent_at", "err", err)
	}
	return nil
}

func digestPendingItems(deliveries []*domain.NotificationDelivery, notifications []*domain.Notification) ([]uuid.UUID, []*domain.Notification) {
	if len(deliveries) != len(notifications) {
		return nil, notifications
	}
	ids := make([]uuid.UUID, 0, len(deliveries))
	items := make([]*domain.Notification, 0, len(notifications))
	for i, d := range deliveries {
		switch d.Status {
		case domain.NotificationDeliveryStatusSent, domain.NotificationDeliveryStatusSkipped, domain.NotificationDeliveryStatusFailed:
			continue
		default:
			ids = append(ids, d.ID)
			items = append(items, notifications[i])
		}
	}
	return ids, items
}

func (s *Service) markBatchStatus(ctx context.Context, ids []uuid.UUID, status, reason string) error {
	now := s.now()
	for _, id := range ids {
		var sent *time.Time
		if status == domain.NotificationDeliveryStatusSent || status == domain.NotificationDeliveryStatusSkipped {
			sent = &now
		}
		if err := s.deps.Deliveries.UpdateStatus(ctx, id, status, reason, sent); err != nil {
			return err
		}
	}
	return nil
}
