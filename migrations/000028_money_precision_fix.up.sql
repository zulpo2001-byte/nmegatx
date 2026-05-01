ALTER TABLE IF EXISTS users
  ALTER COLUMN balance_usd TYPE NUMERIC(14,4) USING balance_usd::numeric,
  ALTER COLUMN paypal_fee_rate TYPE NUMERIC(7,4) USING paypal_fee_rate::numeric,
  ALTER COLUMN stripe_fee_rate TYPE NUMERIC(7,4) USING stripe_fee_rate::numeric;

ALTER TABLE IF EXISTS orders
  ALTER COLUMN channel_fee_rate TYPE NUMERIC(7,4) USING channel_fee_rate::numeric,
  ALTER COLUMN channel_fee TYPE NUMERIC(14,4) USING channel_fee::numeric;

ALTER TABLE IF EXISTS balance_ledgers
  ALTER COLUMN amount_usd TYPE NUMERIC(14,4) USING amount_usd::numeric,
  ALTER COLUMN balance_usd TYPE NUMERIC(14,4) USING balance_usd::numeric;
