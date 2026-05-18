DROP INDEX IF EXISTS idx_products_vendor_code;
ALTER TABLE products DROP COLUMN IF EXISTS vendor_code;
