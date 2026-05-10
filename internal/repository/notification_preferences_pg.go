package repository

import (
	"context"
	"errors"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type notificationPreferencesPg struct{ db *pgxpool.Pool }

func NewNotificationPreferencesRepository(db *pgxpool.Pool) NotificationPreferencesRepository {
	return &notificationPreferencesPg{db: db}
}

func (r *notificationPreferencesPg) List(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, event_type, channel, enabled, updated_at
		FROM notification_preferences
		WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.NotificationPreference
	for rows.Next() {
		var p domain.NotificationPreference
		if err := rows.Scan(&p.UserID, &p.EventType, &p.Channel, &p.Enabled, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *notificationPreferencesPg) IsEnabled(ctx context.Context, userID uuid.UUID, eventType, channel string, defaultEnabled bool) (bool, error) {
	var enabled bool
	err := r.db.QueryRow(ctx, `
		SELECT enabled FROM notification_preferences
		WHERE user_id = $1 AND event_type = $2 AND channel = $3`,
		userID, eventType, channel).Scan(&enabled)
	if err == nil {
		return enabled, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultEnabled, nil
	}
	return defaultEnabled, err
}

func (r *notificationPreferencesPg) Upsert(ctx context.Context, prefs []domain.NotificationPreference) error {
	if len(prefs) == 0 {
		return nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, p := range prefs {
		_, err := tx.Exec(ctx, `
			INSERT INTO notification_preferences (user_id, event_type, channel, enabled, updated_at)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (user_id, event_type, channel)
			DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = NOW()`,
			p.UserID, p.EventType, p.Channel, p.Enabled)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

