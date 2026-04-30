-- remove B-station product management feature
ALTER TABLE IF EXISTS orders DROP COLUMN IF EXISTS product_id;
DROP TABLE IF EXISTS products;
