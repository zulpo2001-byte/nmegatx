-- Migration 022: 全局默认订单时效配置
INSERT INTO global_settings (key, value)
VALUES ('default_validity_days', '1')
ON CONFLICT (key) DO NOTHING;

COMMENT ON COLUMN global_settings.value IS 'default_validity_days 可选值: 1|7|30|90|180';
