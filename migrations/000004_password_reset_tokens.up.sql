-- Password reset: one-shot tokens with hashed storage only.
-- token_hash = sha256(plaintext) в hex; plaintext отправляется только в письмо.
CREATE TABLE password_reset_tokens (
    id          UUID      PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT      NOT NULL UNIQUE,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMP NOT NULL,
    used_at     TIMESTAMP NULL
);

CREATE INDEX idx_password_reset_tokens_user_id    ON password_reset_tokens(user_id);
CREATE INDEX idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at);
CREATE INDEX idx_password_reset_tokens_pending_user
    ON password_reset_tokens(user_id)
    WHERE used_at IS NULL;
