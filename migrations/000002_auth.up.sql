-- Добавляем поля аутентификации и статуса к существующей таблице users.
-- DEFAULT '' нужен для миграции на непустой таблице; после применения удаляем.
ALTER TABLE users
    ADD COLUMN password_hash      TEXT        NOT NULL DEFAULT '',
    ADD COLUMN display_name       VARCHAR(100) NOT NULL DEFAULT '',
    ADD COLUMN status             VARCHAR(30)  NOT NULL DEFAULT 'pending_verification'
        CONSTRAINT users_status_check CHECK (status IN ('pending_verification', 'active', 'blocked')),
    ADD COLUMN failed_login_count INT         NOT NULL DEFAULT 0,
    ADD COLUMN lockout_until      TIMESTAMP   NULL;

ALTER TABLE users
    ALTER COLUMN password_hash DROP DEFAULT,
    ALTER COLUMN display_name  DROP DEFAULT;

-- Сессии: два независимых TTL по OWASP (idle + absolute).
-- token_hash = sha256(cookie_token) в hex; plaintext в БД не попадает.
CREATE TABLE sessions (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash           TEXT        NOT NULL UNIQUE,
    created_at           TIMESTAMP   NOT NULL DEFAULT NOW(),
    last_seen_at         TIMESTAMP   NOT NULL DEFAULT NOW(),
    idle_expires_at      TIMESTAMP   NOT NULL,
    absolute_expires_at  TIMESTAMP   NOT NULL,
    user_agent           VARCHAR(255) NOT NULL DEFAULT '',
    ip_prefix            VARCHAR(64)  NOT NULL DEFAULT ''
);

-- Поиск по хешу — основная операция валидации (O(1) через UNIQUE-индекс).
-- Индексы на TTL-колонках нужны для cleanup job.
CREATE INDEX idx_sessions_user_id             ON sessions(user_id);
CREATE INDEX idx_sessions_idle_expires_at     ON sessions(idle_expires_at);
CREATE INDEX idx_sessions_absolute_expires_at ON sessions(absolute_expires_at);

-- Email-верификация: one-shot токены (used_at = NULL → доступен).
-- token_hash = sha256(plaintext) в hex; plaintext уходит только в письмо.
CREATE TABLE email_verifications (
    id          UUID      PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT      NOT NULL UNIQUE,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMP NOT NULL,
    used_at     TIMESTAMP NULL
);

CREATE INDEX idx_email_verifications_user_id    ON email_verifications(user_id);
CREATE INDEX idx_email_verifications_expires_at ON email_verifications(expires_at);
