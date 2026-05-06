-- Recreate empty legacy table stubs so that migrate down can complete without errors.
-- These tables will not contain data; they exist only to satisfy the schema expectations
-- of any remaining references during a rollback sequence.

CREATE TABLE IF NOT EXISTS shops_wb (
    id          UUID PRIMARY KEY,
    user_id     UUID NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    api_token_wb TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS shops_ozon (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    api_client_id   TEXT NOT NULL DEFAULT '',
    api_key_ozon    TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS products_wb (
    id           UUID PRIMARY KEY,
    shop_id      UUID NOT NULL,
    sku          TEXT NOT NULL DEFAULT '',
    name         TEXT NOT NULL DEFAULT '',
    price        NUMERIC(12,2) NOT NULL DEFAULT 0,
    min_price    NUMERIC(12,2),
    max_price    NUMERIC(12,2),
    count        INT NOT NULL DEFAULT 0,
    rating       NUMERIC(3,2),
    reviews_count INT NOT NULL DEFAULT 0,
    updated_at   TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS products_ozon (
    id             UUID PRIMARY KEY,
    shop_id        UUID NOT NULL,
    sku            TEXT NOT NULL DEFAULT '',
    name           TEXT NOT NULL DEFAULT '',
    price          NUMERIC(12,2) NOT NULL DEFAULT 0,
    min_price      NUMERIC(12,2),
    max_price      NUMERIC(12,2),
    count          INT NOT NULL DEFAULT 0,
    is_out_of_stock BOOLEAN NOT NULL DEFAULT FALSE,
    rating         NUMERIC(3,2),
    updated_at     TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS competitors_wb (
    id              UUID PRIMARY KEY,
    product_id      UUID NOT NULL,
    sku             TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL DEFAULT '',
    price           NUMERIC(12,2) NOT NULL DEFAULT 0,
    rating          NUMERIC(3,2),
    reviews_count   INT NOT NULL DEFAULT 0,
    is_out_of_stock BOOLEAN NOT NULL DEFAULT FALSE,
    last_check_at   TIMESTAMP NOT NULL DEFAULT NOW(),
    price_history   JSONB NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS competitors_ozon (
    id              UUID PRIMARY KEY,
    product_id      UUID NOT NULL,
    sku             TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL DEFAULT '',
    price           NUMERIC(12,2) NOT NULL DEFAULT 0,
    rating          NUMERIC(3,2),
    is_out_of_stock BOOLEAN NOT NULL DEFAULT FALSE,
    last_check_at   TIMESTAMP NOT NULL DEFAULT NOW(),
    price_history   JSONB NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS payments (
    id         UUID PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
