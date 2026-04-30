-- Migration 015: stripe_configs 表
-- 每用户独立的 Stripe 账号配置，支持日限额、状态机、动态权重

CREATE TABLE IF NOT EXISTS stripe_configs (
    id                 BIGSERIAL PRIMARY KEY,
    user_id            BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- 基础信息
    label              VARCHAR(100)  NOT NULL DEFAULT '',
    secret_key         TEXT          NOT NULL DEFAULT '',
    publishable_key    TEXT          NOT NULL DEFAULT '',
    webhook_secret     TEXT          NOT NULL DEFAULT '',
    sandbox            BOOLEAN       NOT NULL DEFAULT false,

    -- 状态机
    account_state      VARCHAR(20)   NOT NULL DEFAULT 'active',   -- active | paused | abandoned
    enabled            BOOLEAN       NOT NULL DEFAULT true,

    -- 轮询策略
    poll_order         INT           NOT NULL DEFAULT 0,
    poll_mode          VARCHAR(20)   NOT NULL DEFAULT 'random',   -- random | sequence

    -- 权重
    weight             INT           NOT NULL DEFAULT 10,
    smart_weight       NUMERIC(6,2)  NOT NULL DEFAULT 50.0,

    -- 累计统计
    total_orders       BIGINT        NOT NULL DEFAULT 0,
    fail_count         INT           NOT NULL DEFAULT 0,

    -- 金额门槛
    min_amount         NUMERIC(10,2) NOT NULL DEFAULT 0,
    max_amount         NUMERIC(10,2) NOT NULL DEFAULT 99999,
    max_orders         INT           NOT NULL DEFAULT 0,
    max_amount_total   NUMERIC(12,2) NOT NULL DEFAULT 0,

    -- 日限额
    daily_orders       INT           NOT NULL DEFAULT 0,
    daily_amount       NUMERIC(12,2) NOT NULL DEFAULT 0,
    daily_reset_hour   INT           NOT NULL DEFAULT 0,
    last_reset_date    DATE          NULL,

    created_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_stripe_configs_user_state ON stripe_configs(user_id, account_state, enabled);
CREATE INDEX IF NOT EXISTS idx_stripe_configs_user_order ON stripe_configs(user_id, poll_order);

COMMENT ON COLUMN stripe_configs.account_state  IS 'active/paused/abandoned 三态状态机';
COMMENT ON COLUMN stripe_configs.smart_weight   IS '智能路由动态权重，由 smart_routing 服务每小时写入';
COMMENT ON COLUMN stripe_configs.max_orders     IS '日最大单数限制，0表示不限';
COMMENT ON COLUMN stripe_configs.max_amount_total IS '日最大金额限制，0表示不限';
