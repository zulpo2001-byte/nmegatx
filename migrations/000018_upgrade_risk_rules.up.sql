-- Migration 018: risk_rules 表升级
-- V9 原有表只有 name/enabled/config，现补全 V7 风控引擎字段

ALTER TABLE risk_rules
    ADD COLUMN IF NOT EXISTS type        VARCHAR(40)  NOT NULL DEFAULT 'amount_range',
    ADD COLUMN IF NOT EXISTS conditions  JSONB        NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS action      VARCHAR(20)  NOT NULL DEFAULT 'block',
    ADD COLUMN IF NOT EXISTS risk_score  INT          NOT NULL DEFAULT 20,
    ADD COLUMN IF NOT EXISTS hit_count   BIGINT       NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS description TEXT         NULL;

-- 补 UNIQUE 约束（幂等：若 009 建表时已加则跳过，ON CONFLICT (name) 依赖此约束）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'risk_rules'::regclass
          AND contype   = 'u'
          AND conname   = 'risk_rules_name_key'
    ) THEN
        ALTER TABLE risk_rules ADD CONSTRAINT risk_rules_name_key UNIQUE (name);
    END IF;
END;
$$;


CREATE INDEX IF NOT EXISTS idx_risk_rules_type    ON risk_rules(type);
CREATE INDEX IF NOT EXISTS idx_risk_rules_enabled ON risk_rules(enabled);

-- 预置 4 条默认规则（覆盖 V7 seed 数据）
-- 先删除旧的简单规则，再插入完整格式
DELETE FROM risk_rules WHERE name IN ('high_amount_block', 'ip_rate_limit');

INSERT INTO risk_rules (name, type, enabled, action, risk_score, conditions, config, description) VALUES
(
    'large_amount_block',
    'amount_range',
    true,
    'block',
    50,
    '{"min": 1000, "max": 999999}',
    '{}',
    '单笔金额超过 $1000 触发高风险拦截'
),
(
    'ip_region_blacklist',
    'ip_region',
    true,
    'block',
    50,
    '{"blocked_prefixes": []}',
    '{}',
    'IP 前缀黑名单，在 conditions.blocked_prefixes 中添加屏蔽的 IP 段'
),
(
    'ip_frequency_limit',
    'ip_frequency',
    true,
    'warn',
    30,
    '{"max_per_minute": 5}',
    '{}',
    '同一 IP 每分钟请求超过阈值时叠加风险分'
),
(
    'user_daily_frequency',
    'user_frequency',
    true,
    'warn',
    25,
    '{"max_per_day": 50}',
    '{}',
    '同一用户当天下单超过阈值时叠加风险分'
)
ON CONFLICT (name) DO NOTHING;

COMMENT ON COLUMN risk_rules.type        IS 'amount_range|ip_region|ip_frequency|user_frequency|device_fingerprint';
COMMENT ON COLUMN risk_rules.conditions  IS '规则条件参数，JSON格式，不同type有不同schema';
COMMENT ON COLUMN risk_rules.action      IS 'block=直接拒单，warn=叠加风险分但放行';
COMMENT ON COLUMN risk_rules.risk_score  IS '命中此规则时叠加的风险分（1-100）';
COMMENT ON COLUMN risk_rules.hit_count   IS '历史命中次数，由风控引擎每次命中时递增';
