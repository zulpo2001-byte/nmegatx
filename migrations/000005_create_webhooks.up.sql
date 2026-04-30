-- Migration 005: webhook_endpoints 完整建表（含全部字段）
-- 删除旧表重建，确保结构完整
DROP TABLE IF EXISTS webhook_endpoints CASCADE;

CREATE TABLE webhook_endpoints (
    id             BIGSERIAL    PRIMARY KEY,
    user_id        BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- 端点类型：a = A站回调端点，b = B站下单端点
    type           VARCHAR(1)   NOT NULL CHECK (type IN ('a','b')),

    label          VARCHAR(100) NOT NULL DEFAULT '',
    url            TEXT         NOT NULL DEFAULT '',
    payment_method VARCHAR(20)  NOT NULL DEFAULT 'all', -- all|stripe|paypal
    enabled        BOOLEAN      NOT NULL DEFAULT true,

    -- A 站密钥（A站下单回调鉴权）
    shared_secret  VARCHAR(255) NOT NULL DEFAULT '',  -- HMAC 签名密钥（对 A站）
    a_api_key      VARCHAR(255) NOT NULL DEFAULT '',  -- X-Api-Key 标识符

    -- B 站密钥（B站主动调 NME 下单/回调）
    b_api_key      VARCHAR(255) NOT NULL DEFAULT '',  -- B站 X-Api-Key
    b_shared_secret VARCHAR(255) NOT NULL DEFAULT '', -- B站 HMAC 签名密钥

    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- 复合索引：按用户+类型+状态快速筛选
CREATE INDEX idx_webhook_user_type_enabled ON webhook_endpoints(user_id, type, enabled);

-- 唯一索引：gateway 用 a_api_key/b_api_key 反查，必须唯一且快
CREATE UNIQUE INDEX idx_webhook_a_api_key ON webhook_endpoints(a_api_key) WHERE a_api_key <> '';
CREATE UNIQUE INDEX idx_webhook_b_api_key ON webhook_endpoints(b_api_key) WHERE b_api_key <> '';

COMMENT ON TABLE  webhook_endpoints              IS 'A站/B站 Webhook 端点配置，每条记录代表一个对接点';
COMMENT ON COLUMN webhook_endpoints.type         IS 'a=A站回调端点, b=B站主动调NME端点';
COMMENT ON COLUMN webhook_endpoints.shared_secret IS 'A站 HMAC 签名验证密钥（不对外展示）';
COMMENT ON COLUMN webhook_endpoints.b_shared_secret IS 'B站 HMAC 签名验证密钥（不对外展示）';
