CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
    CREATE TYPE marketplace_type AS ENUM ('wb', 'ozon', 'ym');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    CREATE TYPE shop_status AS ENUM ('draft', 'active', 'error', 'disabled');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    CREATE TYPE product_status AS ENUM ('active', 'archived', 'out_of_stock');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    CREATE TYPE strategy_type AS ENUM ('below_median_pct', 'min_competitor_plus_step', 'min_margin_pct', 'fixed');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    CREATE TYPE fallback_policy AS ENUM ('keep_current', 'set_fixed', 'set_min');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    CREATE TYPE plan_status AS ENUM ('pending', 'processing', 'applied', 'failed', 'cancelled');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    CREATE TYPE plan_item_status AS ENUM ('pending', 'applied', 'skipped', 'failed');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS shops (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    marketplace marketplace_type NOT NULL,
    name VARCHAR(120) NOT NULL,
    credentials_encrypted BYTEA NOT NULL,
    status shop_status NOT NULL DEFAULT 'draft',
    auto_update_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    schedule_cron VARCHAR(100) NOT NULL DEFAULT '0 3 * * *',
    last_checked_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shops_user_id ON shops(user_id);
CREATE INDEX IF NOT EXISTS idx_shops_marketplace ON shops(marketplace);

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    external_sku VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL DEFAULT '',
    current_price NUMERIC(12, 2) NOT NULL DEFAULT 0,
    currency CHAR(3) NOT NULL DEFAULT 'RUB',
    status product_status NOT NULL DEFAULT 'active',
    min_price NUMERIC(12, 2) NULL,
    max_price NUMERIC(12, 2) NULL,
    cost_price NUMERIC(12, 2) NULL,
    stock_count INT NOT NULL DEFAULT 0,
    rating NUMERIC(3, 2) NULL,
    reviews_count INT NOT NULL DEFAULT 0,
    last_synced_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT products_price_bounds_check CHECK (
        min_price IS NULL OR max_price IS NULL OR min_price <= max_price
    ),
    UNIQUE(shop_id, external_sku)
);

CREATE INDEX IF NOT EXISTS idx_products_shop_id ON products(shop_id);
CREATE INDEX IF NOT EXISTS idx_products_external_sku ON products(external_sku);
CREATE INDEX IF NOT EXISTS idx_products_status ON products(status);

CREATE TABLE IF NOT EXISTS competitors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    external_sku VARCHAR(100) NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    price NUMERIC(12, 2) NOT NULL,
    source VARCHAR(80) NOT NULL DEFAULT '',
    rating NUMERIC(3, 2) NULL,
    reviews_count INT NOT NULL DEFAULT 0,
    is_out_of_stock BOOLEAN NOT NULL DEFAULT FALSE,
    fetched_at TIMESTAMP NOT NULL DEFAULT NOW(),
    history JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_competitors_product_fetched ON competitors(product_id, fetched_at DESC);

CREATE TABLE IF NOT EXISTS strategies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    type strategy_type NOT NULL,
    params JSONB NOT NULL DEFAULT '{}'::jsonb,
    constraints JSONB NOT NULL DEFAULT '{}'::jsonb,
    fallback_policy fallback_policy NOT NULL DEFAULT 'keep_current',
    priority INT NOT NULL DEFAULT 100,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_strategies_user_id ON strategies(user_id);
CREATE INDEX IF NOT EXISTS idx_strategies_enabled_priority ON strategies(enabled, priority);

CREATE TABLE IF NOT EXISTS strategy_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    strategy_id UUID NOT NULL REFERENCES strategies(id) ON DELETE CASCADE,
    product_id UUID NULL REFERENCES products(id) ON DELETE CASCADE,
    filter JSONB NULL,
    priority INT NOT NULL DEFAULT 100,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT strategy_assignments_target_check CHECK (
        (product_id IS NOT NULL AND filter IS NULL)
        OR (product_id IS NULL AND filter IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_strategy_assignments_product
    ON strategy_assignments(product_id)
    WHERE product_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_strategy_assignments_strategy_id ON strategy_assignments(strategy_id);

CREATE TABLE IF NOT EXISTS price_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    status plan_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_price_plans_shop_created ON price_plans(shop_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_price_plans_status ON price_plans(status);

CREATE TABLE IF NOT EXISTS price_plan_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES price_plans(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    strategy_id UUID NULL REFERENCES strategies(id) ON DELETE SET NULL,
    current_price NUMERIC(12, 2) NOT NULL,
    target_price NUMERIC(12, 2) NOT NULL,
    final_price NUMERIC(12, 2) NOT NULL,
    constraint_hit VARCHAR(120) NOT NULL DEFAULT '',
    status plan_item_status NOT NULL DEFAULT 'pending',
    error TEXT NOT NULL DEFAULT '',
    correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_price_plan_items_plan_id ON price_plan_items(plan_id);
CREATE INDEX IF NOT EXISTS idx_price_plan_items_product_id ON price_plan_items(product_id);
CREATE INDEX IF NOT EXISTS idx_price_plan_items_status ON price_plan_items(status);
CREATE UNIQUE INDEX IF NOT EXISTS ux_price_plan_items_dedup
    ON price_plan_items(product_id, final_price, status)
    WHERE status IN ('pending', 'applied');

CREATE TABLE IF NOT EXISTS price_change_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    strategy_id UUID NULL REFERENCES strategies(id) ON DELETE SET NULL,
    old_price NUMERIC(12, 2) NOT NULL,
    new_price NUMERIC(12, 2) NOT NULL,
    target_price NUMERIC(12, 2) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    constraint_hit VARCHAR(120) NOT NULL DEFAULT '',
    status plan_item_status NOT NULL,
    correlation_id UUID NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_price_change_log_shop_created ON price_change_log(shop_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_price_change_log_product_created ON price_change_log(product_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_price_change_log_correlation_id ON price_change_log(correlation_id);

CREATE TABLE IF NOT EXISTS integration_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NULL REFERENCES shops(id) ON DELETE SET NULL,
    operation VARCHAR(120) NOT NULL,
    http_status INT NULL,
    error_text TEXT NOT NULL DEFAULT '',
    correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_integration_log_shop_created ON integration_log(shop_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_integration_log_correlation_id ON integration_log(correlation_id);

CREATE TABLE IF NOT EXISTS import_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMP NULL,
    added INT NOT NULL DEFAULT 0,
    updated INT NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_import_log_shop_started ON import_log(shop_id, started_at DESC);

INSERT INTO shops (id, user_id, marketplace, name, credentials_encrypted, status, created_at, updated_at)
SELECT
    id,
    user_id,
    'wb',
    COALESCE(NULLIF(name, ''), 'Wildberries shop'),
    -- NOTE: хранится plaintext-JSON; до использования магазина обязательно запустить cmd/credbackfill.
    convert_to(jsonb_build_object('api_key', COALESCE(api_token_wb, ''))::text, 'UTF8'),
    'draft',
    COALESCE(created_at, NOW()),
    NOW()
FROM shops_wb
ON CONFLICT (id) DO NOTHING;

INSERT INTO shops (id, user_id, marketplace, name, credentials_encrypted, status, created_at, updated_at)
SELECT
    id,
    user_id,
    'ozon',
    COALESCE(NULLIF(name, ''), 'Ozon shop'),
    convert_to(jsonb_build_object(
        'client_id', COALESCE(api_client_id, ''),
        'api_key', COALESCE(api_key_ozon, '')
    )::text, 'UTF8'),
    'draft',
    COALESCE(created_at, NOW()),
    NOW()
FROM shops_ozon
ON CONFLICT (id) DO NOTHING;

INSERT INTO products (
    id, shop_id, external_sku, name, current_price, status, min_price, max_price,
    stock_count, rating, reviews_count, created_at, updated_at
)
SELECT
    id,
    shop_id,
    sku,
    COALESCE(name, ''),
    COALESCE(price, 0),
    'active',
    min_price,
    max_price,
    COALESCE(count, 0),
    rating,
    COALESCE(reviews_count, 0),
    COALESCE(updated_at, NOW()),
    COALESCE(updated_at, NOW())
FROM products_wb
ON CONFLICT (shop_id, external_sku) DO NOTHING;

INSERT INTO products (
    id, shop_id, external_sku, name, current_price, status, min_price, max_price,
    stock_count, rating, reviews_count, created_at, updated_at
)
SELECT
    id,
    shop_id,
    sku,
    COALESCE(name, ''),
    COALESCE(price, 0),
    'active',
    min_price,
    max_price,
    COALESCE(count, 0),
    rating,
    COALESCE(reviews_count, 0),
    NOW(),
    NOW()
FROM products_ozon
ON CONFLICT (shop_id, external_sku) DO NOTHING;

INSERT INTO competitors (
    id, product_id, external_sku, url, price, source, rating, reviews_count,
    is_out_of_stock, fetched_at, history
)
SELECT
    id,
    product_id,
    COALESCE(sku, ''),
    COALESCE(url, ''),
    COALESCE(price, 0),
    'wb',
    rating,
    COALESCE(reviews_count, 0),
    COALESCE(is_out_of_stock, FALSE),
    COALESCE(last_check_at, NOW()),
    COALESCE(price_history, '[]'::jsonb)
FROM competitors_wb
ON CONFLICT (id) DO NOTHING;

INSERT INTO competitors (
    id, product_id, external_sku, url, price, source, rating, reviews_count,
    is_out_of_stock, fetched_at, history
)
SELECT
    id,
    product_id,
    COALESCE(sku, ''),
    COALESCE(url, ''),
    COALESCE(price, 0),
    'ozon',
    rating,
    0,
    COALESCE(is_out_of_stock, FALSE),
    COALESCE(last_check_at, NOW()),
    COALESCE(price_history, '[]'::jsonb)
FROM competitors_ozon
ON CONFLICT (id) DO NOTHING;

-- Legacy tables intentionally NOT dropped here.
-- Run cmd/credbackfill to re-encrypt plaintext credentials, then apply migration 000008.
