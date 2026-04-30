-- Migration 016: orders 表补字段
-- 记录每笔订单实际使用的支付账号

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS paypal_account_id BIGINT       NULL REFERENCES paypal_accounts(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS stripe_config_id  BIGINT       NULL REFERENCES stripe_configs(id)  ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS gateway_label     VARCHAR(100) NULL;

CREATE INDEX IF NOT EXISTS idx_orders_paypal_account ON orders(paypal_account_id);
CREATE INDEX IF NOT EXISTS idx_orders_stripe_config  ON orders(stripe_config_id);

COMMENT ON COLUMN orders.paypal_account_id IS '实际使用的 PayPal 账号ID，payment_method=paypal时填写';
COMMENT ON COLUMN orders.stripe_config_id  IS '实际使用的 Stripe 配置ID，payment_method=stripe时填写';
COMMENT ON COLUMN orders.gateway_label     IS '账号备注名快照，账号删除后仍可追溯';
