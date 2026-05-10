package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type telegramLinksPg struct{ db *pgxpool.Pool }

func NewTelegramLinksRepository(db *pgxpool.Pool) TelegramLinksRepository {
	return &telegramLinksPg{db: db}
}

func (r *telegramLinksPg) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TelegramLink, error) {
	row := r.db.QueryRow(ctx, baseTelegramSelect+" WHERE user_id = $1", userID)
	return scanTelegramLink(row)
}

func (r *telegramLinksPg) GetByToken(ctx context.Context, token string) (*domain.TelegramLink, error) {
	row := r.db.QueryRow(ctx, baseTelegramSelect+" WHERE link_token = $1", token)
	return scanTelegramLink(row)
}

func (r *telegramLinksPg) GetByChatID(ctx context.Context, chatID int64) (*domain.TelegramLink, error) {
	row := r.db.QueryRow(ctx, baseTelegramSelect+" WHERE chat_id = $1", chatID)
	return scanTelegramLink(row)
}

func (r *telegramLinksPg) IssueToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_telegram_links (user_id, link_token, link_token_expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET link_token = EXCLUDED.link_token,
		    link_token_expires_at = EXCLUDED.link_token_expires_at,
		    updated_at = NOW()`,
		userID, token, expiresAt)
	return err
}

func (r *telegramLinksPg) Confirm(ctx context.Context, token string, chatID int64, username string) (*domain.TelegramLink, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE user_telegram_links
		SET chat_id = $2,
		    username = $3,
		    link_token = NULL,
		    link_token_expires_at = NULL,
		    linked_at = NOW(),
		    updated_at = NOW()
		WHERE link_token = $1 AND link_token_expires_at > NOW()
		RETURNING user_id, chat_id, username, link_token, link_token_expires_at, linked_at, created_at, updated_at`,
		token, chatID, username)
	return scanTelegramLink(row)
}

func (r *telegramLinksPg) Unlink(ctx context.Context, userID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM user_telegram_links WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *telegramLinksPg) UnlinkByChatID(ctx context.Context, chatID int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM user_telegram_links WHERE chat_id = $1`, chatID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const baseTelegramSelect = `
	SELECT user_id, chat_id, username, link_token, link_token_expires_at,
	       linked_at, created_at, updated_at
	FROM user_telegram_links`

func scanTelegramLink(row scannable) (*domain.TelegramLink, error) {
	var l domain.TelegramLink
	err := row.Scan(
		&l.UserID, &l.ChatID, &l.Username, &l.LinkToken, &l.LinkTokenExpiresAt,
		&l.LinkedAt, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}
