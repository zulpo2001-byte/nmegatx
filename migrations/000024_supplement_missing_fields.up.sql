-- Migration 024: 补全缺失字段和索引

-- ── roles 表：补 display_name（SaaS 有，写操作需要）────────────────────
ALTER TABLE roles
  ADD COLUMN IF NOT EXISTS display_name VARCHAR(100) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS description  TEXT         NOT NULL DEFAULT '';

COMMENT ON COLUMN roles.display_name IS '角色显示名，如「超级管理员」「运营」';
COMMENT ON COLUMN roles.description  IS '角色说明';

-- ── audit_logs：补过滤用索引（action/actor/path/created_at）─────────────
CREATE INDEX IF NOT EXISTS idx_audit_action     ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs(created_at DESC);
-- actor_id 索引 000011 已建，跳过

-- ── alert_channels：enabled 字段已有，补索引加速 toggle 后的查询 ─────────
CREATE INDEX IF NOT EXISTS idx_alert_channels_enabled ON alert_channels(enabled);

-- ── orders：补单条查询所需索引 ────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_orders_a_order_id ON orders(a_order_id);
CREATE INDEX IF NOT EXISTS idx_orders_b_order_id ON orders(b_order_id) WHERE b_order_id <> '';
