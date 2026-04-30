-- Migration 014: paypal_accounts 表
-- 完整复刻 V7 paypal_accounts，支持 email 和 rest 两种模式、日限额、自动熔断

CREATE TABLE IF NOT EXISTS paypal_accounts (
    id                    BIGSERIAL PRIMARY KEY,
    user_id               BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- 基础信息
    label                 VARCHAR(100) NOT NULL DEFAULT '',
    mode                  VARCHAR(10)  NOT NULL DEFAULT 'email',   -- email | rest
    email                 VARCHAR(255) NOT NULL DEFAULT '',
    client_id             TEXT         NOT NULL DEFAULT '',
    client_secret         TEXT         NOT NULL DEFAULT '',

    -- 沙盒凭据
    sandbox               BOOLEAN      NOT NULL DEFAULT false,
    sandbox_mode          BOOLEAN      NOT NULL DEFAULT false,
    sandbox_email         VARCHAR(255) NOT NULL DEFAULT '',
    sandbox_client_id     TEXT         NOT NULL DEFAULT '',
    sandbox_client_secret TEXT         NOT NULL DEFAULT '',

    -- 状态机
    account_state         VARCHAR(20)  NOT NULL DEFAULT 'active',  -- active | paused | abandoned
    enabled               BOOLEAN      NOT NULL DEFAULT true,

    -- 轮询策略
    poll_order            INT          NOT NULL DEFAULT 0,
    poll_mode             VARCHAR(20)  NOT NULL DEFAULT 'random',  -- random | sequence

    -- 权重（静态 + 动态）
    weight                INT          NOT NULL DEFAULT 10,
    smart_weight          NUMERIC(6,2) NOT NULL DEFAULT 50.0,

    -- 累计统计
    total_orders          BIGINT       NOT NULL DEFAULT 0,
    total_success         BIGINT       NOT NULL DEFAULT 0,
    fail_count            INT          NOT NULL DEFAULT 0,

    -- 金额门槛
    min_amount            NUMERIC(10,2) NOT NULL DEFAULT 0,
    max_amount            NUMERIC(10,2) NOT NULL DEFAULT 99999,
    max_orders            INT           NOT NULL DEFAULT 0,         -- 0=不限
    max_amount_total      NUMERIC(12,2) NOT NULL DEFAULT 0,         -- 0=不限

    -- 日限额
    daily_orders          INT          NOT NULL DEFAULT 0,
    daily_amount          NUMERIC(12,2) NOT NULL DEFAULT 0,
    daily_reset_hour      INT          NOT NULL DEFAULT 0,          -- 0-23，每天几点重置
    last_reset_date       DATE         NULL,

    created_at            TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_paypal_accounts_user_state   ON paypal_accounts(user_id, account_state, enabled);
CREATE INDEX IF NOT EXISTS idx_paypal_accounts_user_order   ON paypal_accounts(user_id, poll_order);

COMMENT ON COLUMN paypal_accounts.mode            IS 'email=直接构造PayPal.me链接，rest=走OAuth2 API';
COMMENT ON COLUMN paypal_accounts.account_state   IS 'active/paused/abandoned 三态状态机';
COMMENT ON COLUMN paypal_accounts.smart_weight    IS '智能路由动态权重，由 smart_routing 服务每小时写入';
COMMENT ON COLUMN paypal_accounts.daily_reset_hour IS '每日限额重置时刻（0-23时），可自定义，非固定0点';
COMMENT ON COLUMN paypal_accounts.max_orders      IS '日最大单数限制，0表示不限';
COMMENT ON COLUMN paypal_accounts.max_amount_total IS '日最大金额限制，0表示不限';
