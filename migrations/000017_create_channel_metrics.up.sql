-- Migration 017: channel_metrics 表
-- 每小时一条，记录 PayPal/Stripe 账号的成功率、响应时间、风控通过率
-- 供 SmartRouting 服务计算动态权重

CREATE TABLE IF NOT EXISTS channel_metrics (
    id                BIGSERIAL    PRIMARY KEY,
    channel_type      VARCHAR(20)  NOT NULL,              -- paypal | stripe
    channel_id        BIGINT       NOT NULL,
    channel_label     VARCHAR(100) NOT NULL DEFAULT '',
    hour_slot         TIMESTAMPTZ  NOT NULL,              -- 精确到小时，如 2024-01-01 14:00:00+00

    -- 请求计数
    total_requests    INT          NOT NULL DEFAULT 0,
    success_count     INT          NOT NULL DEFAULT 0,
    fail_count        INT          NOT NULL DEFAULT 0,
    risk_reject_count INT          NOT NULL DEFAULT 0,

    -- 性能
    avg_response_ms   INT          NOT NULL DEFAULT 0,

    -- 派生指标（由 smart_routing 服务写入）
    success_rate      NUMERIC(5,2) NOT NULL DEFAULT 0,
    risk_pass_rate    NUMERIC(5,2) NOT NULL DEFAULT 100,
    dynamic_weight    NUMERIC(6,2) NOT NULL DEFAULT 50,

    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),

    UNIQUE(channel_type, channel_id, hour_slot)
);

CREATE INDEX IF NOT EXISTS idx_channel_metrics_lookup  ON channel_metrics(channel_type, channel_id, hour_slot DESC);
CREATE INDEX IF NOT EXISTS idx_channel_metrics_slot    ON channel_metrics(hour_slot DESC);

COMMENT ON TABLE  channel_metrics                 IS 'SmartRouting 每小时性能指标，用于计算动态权重';
COMMENT ON COLUMN channel_metrics.channel_type    IS 'paypal 或 stripe';
COMMENT ON COLUMN channel_metrics.hour_slot       IS '小时时间槽，格式 YYYY-MM-DD HH:00:00+00';
COMMENT ON COLUMN channel_metrics.dynamic_weight  IS '最终计算得出的动态权重，写回账号 smart_weight 字段';
