ALTER TABLE import_log
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'running',
    ADD COLUMN IF NOT EXISTS total INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS skipped INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS failed INT NOT NULL DEFAULT 0;

UPDATE import_log
SET status = CASE
    WHEN finished_at IS NULL THEN 'running'
    WHEN jsonb_array_length(errors) > 0 THEN 'partial'
    ELSE 'completed'
END
WHERE status = 'running';

DO $$
BEGIN
    ALTER TABLE import_log
        ADD CONSTRAINT import_log_status_check
        CHECK (status IN ('running', 'completed', 'failed', 'partial'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE import_log
        ADD CONSTRAINT import_log_counts_check
        CHECK (
            total >= 0 AND added >= 0 AND updated >= 0
            AND skipped >= 0 AND failed >= 0
        );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE products
        ADD CONSTRAINT products_non_negative_values_check
        CHECK (
            current_price >= 0
            AND (min_price IS NULL OR min_price >= 0)
            AND (max_price IS NULL OR max_price >= 0)
            AND (cost_price IS NULL OR cost_price >= 0)
            AND stock_count >= 0
            AND reviews_count >= 0
            AND (rating IS NULL OR (rating >= 0 AND rating <= 5))
        );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE products
        ADD CONSTRAINT products_currency_check
        CHECK (currency ~ '^[A-Z]{3}$');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_products_shop_status_updated_id
    ON products(shop_id, status, updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_products_updated_id
    ON products(updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_strategy_assignments_product_strategy
    ON strategy_assignments(product_id, strategy_id)
    WHERE product_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_import_log_status_started
    ON import_log(status, started_at DESC);
