DROP INDEX IF EXISTS idx_import_log_status_started;
DROP INDEX IF EXISTS idx_strategy_assignments_product_strategy;
DROP INDEX IF EXISTS idx_products_updated_id;
DROP INDEX IF EXISTS idx_products_shop_status_updated_id;

ALTER TABLE products DROP CONSTRAINT IF EXISTS products_currency_check;
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_non_negative_values_check;

ALTER TABLE import_log DROP CONSTRAINT IF EXISTS import_log_counts_check;
ALTER TABLE import_log DROP CONSTRAINT IF EXISTS import_log_status_check;

ALTER TABLE import_log
    DROP COLUMN IF EXISTS failed,
    DROP COLUMN IF EXISTS skipped,
    DROP COLUMN IF EXISTS total,
    DROP COLUMN IF EXISTS status;
