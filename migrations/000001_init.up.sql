-- 1. Таблица Пользователей (Users)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Личные данные
    fname VARCHAR(100), -- First Name
    sname VARCHAR(100), -- Second Name
    tel VARCHAR(20),
    email VARCHAR(255) UNIQUE NOT NULL, -- Для входа
    
    -- Маркетинг и Безопасность
    role_type VARCHAR(20) DEFAULT 'client', -- admin/client
    utm_source VARCHAR(100),
    last_login_at TIMESTAMP,
    
    created_at TIMESTAMP DEFAULT NOW()
);

-- 2. Платежи (Payments)
CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    amount DECIMAL(10, 2) NOT NULL,
    date TIMESTAMP DEFAULT NOW(),
    status VARCHAR(20) DEFAULT 'pending' -- pending/success/failed
);


-- БЛОК WILDBERRIES (WB)

-- 3. Магазины WB (Shops_WB)
CREATE TABLE shops_wb (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    name VARCHAR(100),     -- Название магазина
    api_token_wb TEXT,     -- Токен 
    
    created_at TIMESTAMP DEFAULT NOW()
);

-- 4. Товары WB (Products)
CREATE TABLE products_wb (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops_wb(id) ON DELETE CASCADE,
    
    -- Данные товара
    sku VARCHAR(50) NOT NULL,
    name VARCHAR(255),
    price DECIMAL(10, 2),        -- Текущая цена
    count INT,                   -- Остаток на складе
    
    -- Настройки репрайсера
    min_price DECIMAL(10, 2),    -- Ниже нельзя
    max_price DECIMAL(10, 2),    -- Выше нет смысла
    step DECIMAL(10, 2) DEFAULT 10,
    
    -- Метрики (Analytics)
    velocity INT DEFAULT 0,      -- Скорость продаж
    rating DECIMAL(3, 2),        -- Рейтинг (0.00 - 5.00)
    reviews_count INT DEFAULT 0, -- Кол-во отзывов
    
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(shop_id, sku) -- Защита от дублей товаров в одном магазине
);

-- 5. Конкуренты WB (Competitors)
CREATE TABLE competitors_wb (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products_wb(id) ON DELETE CASCADE,
    
    sku VARCHAR(50),             -- Артикул конкурента
    url TEXT,                    -- Ссылка на товар
    
    -- Данные парсинга
    price DECIMAL(10, 2),        -- Последняя цена
    rating DECIMAL(3, 2),
    reviews_count INT,
    
    is_out_of_stock BOOLEAN DEFAULT FALSE, -- Нет в наличии 
    
    last_check_at TIMESTAMP,     -- Когда проверяли последний раз
    price_history JSONB          -- Историю храним в JSON
);


-- БЛОК OZON 

-- 6. Магазины Ozon (Shops_Ozon)
CREATE TABLE shops_ozon (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    name VARCHAR(100),
    api_client_id VARCHAR(100), -- У Озона два ключа
    api_key_ozon TEXT,
    
    created_at TIMESTAMP DEFAULT NOW()
);

-- 7. Товары Ozon
CREATE TABLE products_ozon (
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

-- 8. Конкуренты Ozon
CREATE TABLE competitors_ozon (
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
