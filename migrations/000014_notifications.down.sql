DROP TABLE IF EXISTS user_webhooks;
DROP TABLE IF EXISTS user_telegram_links;
DROP TABLE IF EXISTS user_channel_settings;
DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS notifications;

DROP INDEX IF EXISTS idx_users_is_admin;

ALTER TABLE users
    DROP COLUMN IF EXISTS tg_muted_until,
    DROP COLUMN IF EXISTS is_admin;
