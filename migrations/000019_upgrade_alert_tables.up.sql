-- Migration 019: alert 表升级
-- 补全 V7 AlertService 所需字段：多渠道推送、状态流转、告警类型

-- alert_records 补字段
ALTER TABLE alert_records
    ADD COLUMN IF NOT EXISTS type             VARCHAR(40)  NULL,
    ADD COLUMN IF NOT EXISTS title            VARCHAR(255) NULL,
    ADD COLUMN IF NOT EXISTS context          JSONB        NULL,
    ADD COLUMN IF NOT EXISTS acknowledged_by  VARCHAR(100) NULL,
    ADD COLUMN IF NOT EXISTS acknowledged_at  TIMESTAMPTZ  NULL,
    ADD COLUMN IF NOT EXISTS resolved_at      TIMESTAMPTZ  NULL;

-- 将原有 status 列的默认值改为 'open'（原为 'new'）
ALTER TABLE alert_records ALTER COLUMN status SET DEFAULT 'open';
UPDATE alert_records SET status = 'open' WHERE status = 'new';

-- alert_channels 补字段
ALTER TABLE alert_channels
    ADD COLUMN IF NOT EXISTS name   VARCHAR(100) NULL,
    ADD COLUMN IF NOT EXISTS config JSONB        NULL,
    ADD COLUMN IF NOT EXISTS levels JSONB        NULL;   -- ["warning","critical"] 或 ["all"]

-- 重命名 channel_type → type（保持与 V7 一致）
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'alert_channels' AND column_name = 'channel_type'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'alert_channels' AND column_name = 'type'
    ) THEN
        ALTER TABLE alert_channels RENAME COLUMN channel_type TO type;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_alert_records_status ON alert_records(status);
CREATE INDEX IF NOT EXISTS idx_alert_records_type   ON alert_records(type);
CREATE INDEX IF NOT EXISTS idx_alert_records_level  ON alert_records(level);

COMMENT ON COLUMN alert_records.type            IS 'no_channel|channel_isolated|chargeback|high_risk|anomaly';
COMMENT ON COLUMN alert_records.status          IS 'open|acknowledged|resolved';
COMMENT ON COLUMN alert_channels.type           IS 'telegram|email|webhook';
COMMENT ON COLUMN alert_channels.config         IS '渠道配置，telegram: {chat_id, bot_token}，webhook: {url, headers}';
COMMENT ON COLUMN alert_channels.levels         IS '接收的告警级别，如 ["warning","critical"] 或 ["all"]';
