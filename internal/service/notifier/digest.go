package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// FlushDigests сканирует пары (user_id, channel) с накопленными pending_digest
// доставками; для каждой пары проверяет:
//   - окно (digest_window_minutes) уже закрыто;
//   - текущий момент не в quiet_hours;
//
// и enqueue-ит digest-job для отправки сводки. LockPendingDigestForUser
// атомарно переводит pending_digest → queued_digest под FOR UPDATE SKIP
// LOCKED, чтобы две replica scheduler'а не продублировали сводку.
//
// Возвращает количество поставленных в очередь digest-job-ов.
func (s *Service) FlushDigests(ctx context.Context, channel string, now time.Time) (int, error) {
	if channel == "" {
		return 0, fmt.Errorf("notifier: empty channel for digest flush")
	}
	pairs, err := s.deps.Deliveries.ListPendingDigestPairs(ctx, channel)
	if err != nil {
		return 0, fmt.Errorf("list pending: %w", err)
	}

	enqueued := 0
	for _, p := range pairs {
		settings, err := s.deps.ChannelSet.Get(ctx, p.UserID, p.Channel)
		if err != nil {
			s.deps.Log.Warn("digest: get settings", "err", err, "user_id", p.UserID, "channel", p.Channel)
			continue
		}
		// Если окно 0 — пользователь, видимо, переключился в instant. Тогда
		// нужно перевести 'pending_digest' → 'pending' (instant отправку),
		// что выполнит этот же flush, поскольку digest-job не enqueue'ит,
		// а пользователь увидит письмо при следующем Emit. Сейчас же —
		// просто пропускаем такие записи; они обработаются после нового
		// Emit (с новой prefs). Это даёт детерминизм без дополнительных
		// миграций статусов.
		if settings.DigestWindowMinutes <= 0 {
			continue
		}
		// Окно ещё не закрыто?
		if settings.DigestSentAt != nil {
			elapsed := now.Sub(*settings.DigestSentAt)
			if elapsed < time.Duration(settings.DigestWindowMinutes)*time.Minute {
				continue
			}
		}
		// Quiet hours?
		if IsInQuietHours(now, settings) {
			continue
		}

		ids, err := s.deps.Deliveries.LockPendingDigestForUser(ctx, p.UserID, p.Channel)
		if err != nil {
			s.deps.Log.Warn("digest: lock", "err", err, "user_id", p.UserID, "channel", p.Channel)
			continue
		}
		if len(ids) == 0 {
			continue
		}
		// Severity-фильтр: отбрасываем те, что ниже digest_min_severity.
		filtered, dropped, err := s.filterByMinSeverity(ctx, ids, settings.DigestMinSeverity)
		if err != nil {
			s.deps.Log.Warn("digest: filter severity", "err", err)
			// Возвращаем все ids в pending_digest — попробуем в следующий
			// тик. Не критично, в худшем случае письмо чуть задержится.
			_ = s.unlockBack(ctx, ids)
			continue
		}
		// Те, что отфильтрованы — помечаем skipped без отправки.
		if len(dropped) > 0 {
			for _, id := range dropped {
				_ = s.deps.Deliveries.UpdateStatus(ctx, id,
					domain.NotificationDeliveryStatusSkipped, "below digest_min_severity",
					ptrTime(s.now()))
			}
		}
		if len(filtered) == 0 {
			// Всё отфильтровалось — фиксируем как «пуск был», чтобы окно
			// сдвинулось.
			_ = s.deps.ChannelSet.MarkDigestSent(ctx, p.UserID, p.Channel, now)
			continue
		}

		payload, _ := json.Marshal(domain.NotificationDigestJobPayload{
			UserID:      p.UserID,
			Channel:     p.Channel,
			DeliveryIDs: filtered,
		})
		job, err := s.deps.Jobs.Enqueue(ctx, repository.BackgroundJobEnqueue{
			JobType:     domain.BackgroundJobTypeNotificationDigestDeliver,
			Payload:     payload,
			MaxAttempts: 5,
		})
		if err != nil {
			s.deps.Log.Warn("digest: enqueue", "err", err, "user_id", p.UserID)
			_ = s.unlockBack(ctx, filtered)
			continue
		}
		// AttachJob — для каждого delivery (best-effort).
		for _, id := range filtered {
			_ = s.deps.Deliveries.AttachJob(ctx, id, job.ID)
		}
		enqueued++
	}
	return enqueued, nil
}

// filterByMinSeverity делит ids на (passes, drops) согласно minSeverity.
// Если все проходят — drops пустой. Загружает notifications через LoadByIDs.
func (s *Service) filterByMinSeverity(ctx context.Context, ids []uuid.UUID, minSeverity string) ([]uuid.UUID, []uuid.UUID, error) {
	if minSeverity == "" || minSeverity == domain.NotificationSeverityInfo {
		return ids, nil, nil
	}
	deliveries, notifications, err := s.deps.Deliveries.LoadByIDs(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	if len(deliveries) != len(notifications) {
		return ids, nil, nil
	}
	passes := make([]uuid.UUID, 0, len(ids))
	drops := make([]uuid.UUID, 0)
	for i, d := range deliveries {
		if severityMeetsMin(notifications[i].Severity, minSeverity) {
			passes = append(passes, d.ID)
		} else {
			drops = append(drops, d.ID)
		}
	}
	return passes, drops, nil
}

// unlockBack возвращает доставки из 'queued_digest' обратно в 'pending_digest',
// чтобы следующий тик их забрал. Используется при ошибках enqueue-job.
func (s *Service) unlockBack(ctx context.Context, ids []uuid.UUID) error {
	for _, id := range ids {
		if err := s.deps.Deliveries.UpdateStatus(ctx, id,
			domain.NotificationDeliveryStatusPendingDigest, "", nil); err != nil {
			return err
		}
	}
	return nil
}
