ALTER TABLE global_settings ALTER COLUMN value TYPE JSONB USING value::jsonb;
