-- restore minimal products feature schema
CREATE TABLE IF NOT EXISTS products (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  label VARCHAR(100) DEFAULT '',
  b_product_id VARCHAR(128) NOT NULL,
  weight INT DEFAULT 1,
  poll_order INT DEFAULT 0,
  enabled BOOLEAN DEFAULT TRUE,
  total_used BIGINT DEFAULT 0,
  last_used_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_user_enabled ON products(user_id, enabled, poll_order);
ALTER TABLE IF EXISTS orders ADD COLUMN IF NOT EXISTS product_id BIGINT NULL;
CREATE INDEX IF NOT EXISTS idx_orders_product_id ON orders(product_id);
