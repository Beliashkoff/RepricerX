DROP TABLE IF EXISTS oauth_identities;

-- Возвращаем NOT NULL: для записей с password_hash=NULL миграция вниз упадёт,
-- что корректно — данные OAuth-only юзеров не должны теряться при откате.
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
