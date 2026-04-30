-- Migration 012: 补全支付通道所需字段
-- webhook_endpoints 的字段已在 000005 完整建表，此处不再重复 ADD COLUMN

-- products 表
ALTER TABLE products
  ADD COLUMN IF NOT EXISTS label          VARCHAR(100) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS payment_method VARCHAR(20)  NOT NULL DEFAULT 'stripe',
  ADD COLUMN IF NOT EXISTS sandbox        BOOLEAN      NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS total_used     BIGINT       NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_used_at   TIMESTAMPTZ  NULL;

COMMENT ON COLUMN products.label          IS '产品备注名，仅后台展示';
COMMENT ON COLUMN products.payment_method IS '支付方式: stripe|paypal';
COMMENT ON COLUMN products.sandbox        IS '是否沙盒模式';
COMMENT ON COLUMN products.total_used     IS '累计被选中次数';
COMMENT ON COLUMN products.last_used_at   IS 'round_robin 策略使用，最近被选时间';

-- orders 表
ALTER TABLE orders
  ADD COLUMN IF NOT EXISTS product_id       BIGINT       NULL REFERENCES products(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS b_transaction_id VARCHAR(128) NULL,
  ADD COLUMN IF NOT EXISTS email            VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS ip               VARCHAR(45)  NULL,
  ADD COLUMN IF NOT EXISTS risk_score       INT          NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS payment_method   VARCHAR(20)  NOT NULL DEFAULT 'stripe',
  ADD COLUMN IF NOT EXISTS currency         VARCHAR(10)  NOT NULL DEFAULT 'USD';

CREATE INDEX IF NOT EXISTS idx_orders_product        ON orders(product_id);
CREATE INDEX IF NOT EXISTS idx_orders_payment_method ON orders(payment_method);

-- users 表：每用户产品选择策略
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS product_strategy VARCHAR(20) NOT NULL DEFAULT 'round_robin';

COMMENT ON COLUMN users.product_strategy IS 'round_robin|random|fixed';
