-- Этап notifier: персистированные уведомления + per-channel расписание.
-- См. /home/belia/.claude/plans/radiant-percolating-pillow.md
--
-- Ключевые решения:
--   * notifications хранят событие в БД, а доставка по каналам — в
--     notification_deliveries, чтобы статус каждого канала отслеживался
--     независимо.
--   * digest-режим: deliveries в статусе 'pending_digest' накапливаются,
--     scheduler-ом (DigestFlushTick) перевозятся в 'queued_digest' и
--     отправляются одним письмом.
--   * is_admin/tg_muted_until добавляются в users этой же миграцией —
--     чтобы system-scoped события (упавший cron) могли быть адресованы
--     множеству админов без отдельного fanout-шага.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_admin       BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS tg_muted_until TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_users_is_admin ON users(is_admin) WHERE is_admin = TRUE;

CREATE TABLE IF NOT EXISTS notifications (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type      TEXT        NOT NULL,
    severity        TEXT        NOT NULL DEFAULT 'info'
        CONSTRAINT notifications_severity_check CHECK (severity IN ('info', 'warning', 'error')),
    title           TEXT        NOT NULL,
    body            TEXT        NOT NULL DEFAULT '',
    data            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    shop_id         UUID        NULL REFERENCES shops(id) ON DELETE SET NULL,
    plan_id         UUID        NULL,
    correlation_id  UUID        NULL,
    read_at         TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_created
    ON notifications(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON notifications(user_id)
    WHERE read_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_notifications_correlation
    ON notifications(correlation_id)
    WHERE correlation_id IS NOT NULL;

-- Дедуп: для events типа 'integration_error' хотим быстро искать
-- "было ли уже за последние N минут событие с таким shop_id".
CREATE INDEX IF NOT EXISTS idx_notifications_dedupe
    ON notifications(user_id, event_type, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id     UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type  TEXT    NOT NULL,
    channel     TEXT    NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, event_type, channel)
);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID        NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel         TEXT        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'pending'
        CONSTRAINT notification_deliveries_status_check CHECK (
            status IN ('pending', 'pending_digest', 'queued_digest', 'sent', 'failed', 'skipped')
        ),
    attempts        INT         NOT NULL DEFAULT 0,
    last_error      TEXT        NOT NULL DEFAULT '',
    job_id          UUID        NULL REFERENCES background_jobs(id) ON DELETE SET NULL,
    sent_at         TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (notification_id, channel)
);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_pending_digest
    ON notification_deliveries(channel, status)
    WHERE status = 'pending_digest';

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_status
    ON notification_deliveries(notification_id, status);

CREATE TABLE IF NOT EXISTS user_channel_settings (
    user_id                 UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel                 TEXT    NOT NULL,
    digest_window_minutes   INT     NOT NULL DEFAULT 0
        CONSTRAINT user_channel_settings_window_check CHECK (
            digest_window_minutes IN (0, 15, 60, 240, 1440)
        ),
    digest_min_severity     TEXT    NOT NULL DEFAULT 'info'
        CONSTRAINT user_channel_settings_severity_check CHECK (
            digest_min_severity IN ('info', 'warning', 'error')
        ),
    quiet_hours_start       INT     NULL CHECK (quiet_hours_start IS NULL OR (quiet_hours_start BETWEEN 0 AND 23)),
    quiet_hours_end         INT     NULL CHECK (quiet_hours_end   IS NULL OR (quiet_hours_end   BETWEEN 0 AND 23)),
    digest_sent_at          TIMESTAMPTZ NULL,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, channel)
);

CREATE TABLE IF NOT EXISTS user_telegram_links (
    user_id                 UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    chat_id                 BIGINT      NULL,
    username                TEXT        NOT NULL DEFAULT '',
    link_token              TEXT        NULL,
    link_token_expires_at   TIMESTAMPTZ NULL,
    linked_at               TIMESTAMPTZ NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_user_telegram_links_token
    ON user_telegram_links(link_token)
    WHERE link_token IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_user_telegram_links_chat_id
    ON user_telegram_links(chat_id)
    WHERE chat_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS user_webhooks (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    url         TEXT        NOT NULL,
    secret      TEXT        NOT NULL,
    enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_webhooks_user
    ON user_webhooks(user_id)
    WHERE enabled = TRUE;
