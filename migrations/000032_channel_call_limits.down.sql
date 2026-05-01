ALTER TABLE paypal_accounts
  DROP COLUMN IF EXISTS max_calls_total,
  DROP COLUMN IF EXISTS max_calls_daily,
  DROP COLUMN IF EXISTS call_count_total,
  DROP COLUMN IF EXISTS call_count_daily;

ALTER TABLE stripe_configs
  DROP COLUMN IF EXISTS max_calls_total,
  DROP COLUMN IF EXISTS max_calls_daily,
  DROP COLUMN IF EXISTS call_count_total,
  DROP COLUMN IF EXISTS call_count_daily;
