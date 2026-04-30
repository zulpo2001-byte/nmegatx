-- Migration 020: 清理 products 表遗留字段（针对已存在的生产库）
-- 新库从 000003 建表时已不含这些字段，此迁移仅对老库生效

ALTER TABLE products DROP COLUMN IF EXISTS payment_mode;
ALTER TABLE products DROP COLUMN IF EXISTS payment_method;
ALTER TABLE products DROP COLUMN IF EXISTS sandbox;

-- 补充新字段（如果从旧 003 建表的库没有这些列）
ALTER TABLE products ADD COLUMN IF NOT EXISTS label        VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS total_used   BIGINT       NOT NULL DEFAULT 0;
ALTER TABLE products ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ  NULL;

-- 修正 poll_order 默认值（旧表默认为 1，统一为 0）
ALTER TABLE products ALTER COLUMN poll_order SET DEFAULT 0;

COMMENT ON TABLE  products              IS 'B站产品，仅存 WooCommerce 产品ID和轮询策略参数';
COMMENT ON COLUMN products.b_product_id IS 'B站 WooCommerce 产品ID';
