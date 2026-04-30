-- Migration 021: 订单时效字段
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS validity_days INT         NULL,
    ADD COLUMN IF NOT EXISTS expires_at    TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_orders_expires_at ON orders(expires_at)
    WHERE expires_at IS NOT NULL;

COMMENT ON COLUMN orders.validity_days IS '订单有效期（天）: 1|7|30|90|180，NULL=仅210s放弃检测';
COMMENT ON COLUMN orders.expires_at    IS '到期时间，超过此时间pending订单变为expired';
