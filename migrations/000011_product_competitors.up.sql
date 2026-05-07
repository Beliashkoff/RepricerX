CREATE TABLE IF NOT EXISTS product_competitors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    marketplace marketplace_type NOT NULL,
    source VARCHAR(80) NOT NULL DEFAULT '',
    competitor_url TEXT NOT NULL DEFAULT '',
    normalized_competitor_url TEXT NOT NULL DEFAULT '',
    ozon_public_product_id VARCHAR(100) NULL,
    ozon_sku VARCHAR(100) NULL,
    last_price NUMERIC(12, 2) NULL,
    last_availability VARCHAR(40) NOT NULL DEFAULT 'unknown',
    last_checked_at TIMESTAMPTZ NULL,
    last_error_code VARCHAR(80) NOT NULL DEFAULT '',
    last_status VARCHAR(40) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT product_competitors_last_price_check CHECK (last_price IS NULL OR last_price >= 0),
    CONSTRAINT product_competitors_availability_check CHECK (
        last_availability IN ('unknown', 'available', 'out_of_stock', 'not_found')
    ),
    CONSTRAINT product_competitors_status_check CHECK (
        last_status IN ('pending', 'ok', 'failed', 'rate_limited', 'blocked', 'disabled')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_product_competitors_product_url
    ON product_competitors(product_id, marketplace, normalized_competitor_url)
    WHERE normalized_competitor_url <> '';

CREATE UNIQUE INDEX IF NOT EXISTS ux_product_competitors_product_ozon_ids
    ON product_competitors(product_id, marketplace, ozon_public_product_id, ozon_sku)
    WHERE marketplace = 'ozon'
      AND (ozon_public_product_id IS NOT NULL OR ozon_sku IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_product_competitors_product_id
    ON product_competitors(product_id);

CREATE INDEX IF NOT EXISTS idx_product_competitors_marketplace_source
    ON product_competitors(marketplace, source);

CREATE INDEX IF NOT EXISTS idx_product_competitors_last_checked
    ON product_competitors(last_checked_at NULLS FIRST, id);

CREATE TABLE IF NOT EXISTS competitor_price_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    competitor_id UUID NOT NULL REFERENCES product_competitors(id) ON DELETE CASCADE,
    price NUMERIC(12, 2) NULL,
    availability VARCHAR(40) NOT NULL DEFAULT 'unknown',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status VARCHAR(40) NOT NULL,
    error_code VARCHAR(80) NOT NULL DEFAULT '',
    raw_source VARCHAR(80) NOT NULL DEFAULT '',
    CONSTRAINT competitor_price_snapshots_price_check CHECK (price IS NULL OR price >= 0),
    CONSTRAINT competitor_price_snapshots_availability_check CHECK (
        availability IN ('unknown', 'available', 'out_of_stock', 'not_found')
    ),
    CONSTRAINT competitor_price_snapshots_status_check CHECK (
        status IN ('ok', 'failed', 'rate_limited', 'blocked')
    )
);

CREATE INDEX IF NOT EXISTS idx_competitor_price_snapshots_competitor_checked
    ON competitor_price_snapshots(competitor_id, checked_at DESC);
