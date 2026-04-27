CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount DECIMAL(10, 2) NOT NULL,
    date TIMESTAMP DEFAULT NOW(),
    status VARCHAR(20) DEFAULT 'pending'
);

CREATE TABLE IF NOT EXISTS shops_wb (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100),
    api_token_wb TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS products_wb (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops_wb(id) ON DELETE CASCADE,
    sku VARCHAR(50) NOT NULL,
    name VARCHAR(255),
    price DECIMAL(10, 2),
    count INT,
    min_price DECIMAL(10, 2),
    max_price DECIMAL(10, 2),
    step DECIMAL(10, 2) DEFAULT 10,
    velocity INT DEFAULT 0,
    rating DECIMAL(3, 2),
    reviews_count INT DEFAULT 0,
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(shop_id, sku)
);

CREATE TABLE IF NOT EXISTS competitors_wb (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products_wb(id) ON DELETE CASCADE,
    sku VARCHAR(50),
    url TEXT,
    price DECIMAL(10, 2),
    rating DECIMAL(3, 2),
    reviews_count INT,
    is_out_of_stock BOOLEAN DEFAULT FALSE,
    last_check_at TIMESTAMP,
    price_history JSONB
);

CREATE TABLE IF NOT EXISTS shops_ozon (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100),
    api_client_id VARCHAR(100),
    api_key_ozon TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS products_ozon (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops_ozon(id) ON DELETE CASCADE,
    sku VARCHAR(50) NOT NULL,
    name VARCHAR(255),
    price DECIMAL(10, 2),
    count INT,
    min_price DECIMAL(10, 2),
    max_price DECIMAL(10, 2),
    velocity INT DEFAULT 0,
    rating DECIMAL(3, 2),
    reviews_count INT DEFAULT 0,
    UNIQUE(shop_id, sku)
);

CREATE TABLE IF NOT EXISTS competitors_ozon (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products_ozon(id) ON DELETE CASCADE,
    sku VARCHAR(50),
    url TEXT,
    price DECIMAL(10, 2),
    rating DECIMAL(3, 2),
    is_out_of_stock BOOLEAN DEFAULT FALSE,
    last_check_at TIMESTAMP,
    price_history JSONB
);

DROP TABLE IF EXISTS import_log;
DROP TABLE IF EXISTS integration_log;
DROP TABLE IF EXISTS price_change_log;
DROP TABLE IF EXISTS price_plan_items;
DROP TABLE IF EXISTS price_plans;
DROP TABLE IF EXISTS strategy_assignments;
DROP TABLE IF EXISTS strategies;
DROP TABLE IF EXISTS competitors;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS shops;

DROP TYPE IF EXISTS plan_item_status;
DROP TYPE IF EXISTS plan_status;
DROP TYPE IF EXISTS fallback_policy;
DROP TYPE IF EXISTS strategy_type;
DROP TYPE IF EXISTS product_status;
DROP TYPE IF EXISTS shop_status;
DROP TYPE IF EXISTS marketplace_type;
