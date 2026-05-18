-- WB-адаптер теперь сохраняет nmID (числовой идентификатор) как external_sku;
-- vendorCode (артикул продавца) — отдельная колонка, отображается в UI.
-- Существующие WB-строки невалидны (external_sku хранит vendorCode), их удаляем.
ALTER TABLE products ADD COLUMN IF NOT EXISTS vendor_code TEXT;

CREATE INDEX IF NOT EXISTS idx_products_vendor_code
    ON products (shop_id, vendor_code)
    WHERE vendor_code IS NOT NULL;

DELETE FROM products WHERE shop_id IN (SELECT id FROM shops WHERE marketplace = 'wb');
