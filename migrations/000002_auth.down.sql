DROP TABLE IF EXISTS email_verifications;
DROP TABLE IF EXISTS sessions;

ALTER TABLE users
    DROP COLUMN IF EXISTS lockout_until,
    DROP COLUMN IF EXISTS failed_login_count,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS display_name,
    DROP COLUMN IF EXISTS password_hash;
