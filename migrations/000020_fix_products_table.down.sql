-- 回滚：恢复被删除的字段（数据不可恢复）
ALTER TABLE products ADD COLUMN IF NOT EXISTS payment_mode VARCHAR(32) NOT NULL DEFAULT 'default';
