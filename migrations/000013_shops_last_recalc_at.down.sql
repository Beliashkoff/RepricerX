DROP INDEX IF EXISTS idx_shops_last_recalc_at;
ALTER TABLE shops DROP COLUMN IF EXISTS last_recalc_at;
