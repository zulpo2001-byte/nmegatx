CREATE TABLE IF NOT EXISTS products (
    id           BIGSERIAL    PRIMARY KEY,
    user_id      BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    b_product_id VARCHAR(128) NOT NULL,
    label        VARCHAR(100) NOT NULL DEFAULT '',
    weight       INT          NOT NULL DEFAULT 1,
    poll_order   INT          NOT NULL DEFAULT 0,
    enabled      BOOLEAN      NOT NULL DEFAULT true,
    total_used   BIGINT       NOT NULL DEFAULT 0,
    last_used_at TIMESTAMPTZ  NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_products_user_enabled_order ON products(user_id, enabled, poll_order);

COMMENT ON TABLE  products             IS 'B站产品列表，只存 WooCommerce 产品ID和轮询参数';
COMMENT ON COLUMN products.b_product_id IS 'B站 WooCommerce 产品ID';
COMMENT ON COLUMN products.weight       IS '加权随机权重（random策略），1-100';
COMMENT ON COLUMN products.poll_order   IS '顺序轮询编号，越小越优先';
