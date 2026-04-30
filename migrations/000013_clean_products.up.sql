-- Migration 013: products 表瘦身
-- B站产品只负责 WooCommerce 产品 ID + 轮询参数，支付方式由账号层决定

ALTER TABLE products DROP COLUMN IF EXISTS payment_method;
ALTER TABLE products DROP COLUMN IF EXISTS sandbox;

COMMENT ON TABLE products IS 'B站产品列表，仅存储 b_product_id 和轮询策略参数';
COMMENT ON COLUMN products.b_product_id IS 'B站 WooCommerce 产品ID';
COMMENT ON COLUMN products.weight       IS '加权随机权重（random策略用），1-100';
COMMENT ON COLUMN products.poll_order   IS '顺序轮询编号，越小越优先（sequence策略用）';
