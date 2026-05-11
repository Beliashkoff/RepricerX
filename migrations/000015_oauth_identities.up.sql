-- OAuth-аутентификация через VK ID и Яндекс ID (Этап X).
--
-- Ключевые решения:
--   * password_hash становится nullable: OAuth-only пользователи могут не
--     иметь пароля. Существующие записи остаются с непустым password_hash.
--   * oauth_identities хранит привязки внешний_провайдер → user. Один user
--     может иметь несколько identity (VK + Яндекс + email/пароль).
--   * UNIQUE(provider, external_id) гарантирует, что одну VK-учётку нельзя
--     привязать к двум аккаунтам в системе одновременно.

ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

CREATE TABLE IF NOT EXISTS oauth_identities (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      VARCHAR(20)  NOT NULL
        CONSTRAINT oauth_identities_provider_check CHECK (provider IN ('vk', 'yandex')),
    external_id   VARCHAR(64)  NOT NULL,
    email         VARCHAR(255) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (provider, external_id)
);

CREATE INDEX IF NOT EXISTS idx_oauth_identities_user_id
    ON oauth_identities(user_id);
