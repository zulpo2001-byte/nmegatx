-- Cleanup legacy structures no longer used by runtime
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS products;

-- remove legacy permission key from JSON permissions to reduce confusion
UPDATE users
SET permissions = (permissions::jsonb - 'products_manage')::text
WHERE permissions IS NOT NULL AND permissions::jsonb ? 'products_manage';
