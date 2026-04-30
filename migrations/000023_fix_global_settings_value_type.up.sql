-- Migration 023: global_settings.value 列从 jsonb 改为 text
-- 原来用 jsonb 导致 seed 写入普通字符串时报错
ALTER TABLE global_settings ALTER COLUMN value TYPE TEXT USING value::text;
