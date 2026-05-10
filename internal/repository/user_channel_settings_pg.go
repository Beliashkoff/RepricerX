package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userChannelSettingsPg struct{ db *pgxpool.Pool }

func NewUserChannelSettingsRepository(db *pgxpool.Pool) UserChannelSettingsRepository {
	return &userChannelSettingsPg{db: db}
}

func (r *userChannelSettingsPg) List(ctx context.Context, userID uuid.UUID) ([]*domain.UserChannelSettings, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, channel, digest_window_minutes, digest_min_severity,
		       quiet_hours_start, quiet_hours_end, digest_sent_at, updated_at
		FROM user_channel_settings
		WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.UserChannelSettings
	for rows.Next() {
		s, err := scanChannelSettings(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *userChannelSettingsPg) Get(ctx context.Context, userID uuid.UUID, channel string) (*domain.UserChannelSettings, error) {
	row := r.db.QueryRow(ctx, `
		SELECT user_id, channel, digest_window_minutes, digest_min_severity,
		       quiet_hours_start, quiet_hours_end, digest_sent_at, updated_at
		FROM user_channel_settings
		WHERE user_id = $1 AND channel = $2`, userID, channel)
	return scanChannelSettings(row)
}

func (r *userChannelSettingsPg) Upsert(ctx context.Context, userID uuid.UUID, channel string, in UserChannelSettingsUpdate) (*domain.UserChannelSettings, error) {
	// Сначала пытаемся обновить существующую запись.
	sets := []string{}
	args := []any{userID, channel}
	idx := 3
	if in.DigestWindowMinutes != nil {
		sets = append(sets, fmt.Sprintf("digest_window_minutes = $%d", idx))
		args = append(args, *in.DigestWindowMinutes)
		idx++
	}
	if in.DigestMinSeverity != nil {
		sets = append(sets, fmt.Sprintf("digest_min_severity = $%d", idx))
		args = append(args, *in.DigestMinSeverity)
		idx++
	}
	if in.ClearQuietHours {
		sets = append(sets, "quiet_hours_start = NULL", "quiet_hours_end = NULL")
	} else {
		if in.QuietHoursStart != nil {
			sets = append(sets, fmt.Sprintf("quiet_hours_start = $%d", idx))
			args = append(args, *in.QuietHoursStart)
			idx++
		}
		if in.QuietHoursEnd != nil {
			sets = append(sets, fmt.Sprintf("quiet_hours_end = $%d", idx))
			args = append(args, *in.QuietHoursEnd)
		}
	}
	sets = append(sets, "updated_at = NOW()")

	updateQuery := fmt.Sprintf(`
		UPDATE user_channel_settings
		SET %s
		WHERE user_id = $1 AND channel = $2
		RETURNING user_id, channel, digest_window_minutes, digest_min_severity,
		          quiet_hours_start, quiet_hours_end, digest_sent_at, updated_at`,
		strings.Join(sets, ", "))
	row := r.db.QueryRow(ctx, updateQuery, args...)
	got, err := scanChannelSettings(row)
	if err == nil {
		return got, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Записи не было — вставляем с дефолтами и переданными значениями.
	insertCols := []string{"user_id", "channel"}
	insertArgs := []any{userID, channel}
	insertVals := []string{"$1", "$2"}
	idx = 3
	if in.DigestWindowMinutes != nil {
		insertCols = append(insertCols, "digest_window_minutes")
		insertArgs = append(insertArgs, *in.DigestWindowMinutes)
		insertVals = append(insertVals, fmt.Sprintf("$%d", idx))
		idx++
	}
	if in.DigestMinSeverity != nil {
		insertCols = append(insertCols, "digest_min_severity")
		insertArgs = append(insertArgs, *in.DigestMinSeverity)
		insertVals = append(insertVals, fmt.Sprintf("$%d", idx))
		idx++
	}
	if !in.ClearQuietHours && in.QuietHoursStart != nil {
		insertCols = append(insertCols, "quiet_hours_start")
		insertArgs = append(insertArgs, *in.QuietHoursStart)
		insertVals = append(insertVals, fmt.Sprintf("$%d", idx))
		idx++
	}
	if !in.ClearQuietHours && in.QuietHoursEnd != nil {
		insertCols = append(insertCols, "quiet_hours_end")
		insertArgs = append(insertArgs, *in.QuietHoursEnd)
		insertVals = append(insertVals, fmt.Sprintf("$%d", idx))
	}
	insertQuery := fmt.Sprintf(`
		INSERT INTO user_channel_settings (%s)
		VALUES (%s)
		RETURNING user_id, channel, digest_window_minutes, digest_min_severity,
		          quiet_hours_start, quiet_hours_end, digest_sent_at, updated_at`,
		strings.Join(insertCols, ", "), strings.Join(insertVals, ", "))
	row = r.db.QueryRow(ctx, insertQuery, insertArgs...)
	return scanChannelSettings(row)
}

func (r *userChannelSettingsPg) MarkDigestSent(ctx context.Context, userID uuid.UUID, channel string, at time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_channel_settings (user_id, channel, digest_sent_at, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, channel) DO UPDATE SET digest_sent_at = $3, updated_at = NOW()`,
		userID, channel, at)
	return err
}

func scanChannelSettings(row scannable) (*domain.UserChannelSettings, error) {
	var s domain.UserChannelSettings
	err := row.Scan(
		&s.UserID, &s.Channel, &s.DigestWindowMinutes, &s.DigestMinSeverity,
		&s.QuietHoursStart, &s.QuietHoursEnd, &s.DigestSentAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}
