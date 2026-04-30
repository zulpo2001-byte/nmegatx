ALTER TABLE roles
  DROP COLUMN IF EXISTS display_name,
  DROP COLUMN IF EXISTS description;

DROP INDEX IF EXISTS idx_audit_action;
DROP INDEX IF EXISTS idx_audit_created_at;
DROP INDEX IF EXISTS idx_alert_channels_enabled;
DROP INDEX IF EXISTS idx_orders_a_order_id;
DROP INDEX IF EXISTS idx_orders_b_order_id;
