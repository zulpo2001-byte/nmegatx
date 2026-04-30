ALTER TABLE IF EXISTS balance_ledgers
  ALTER COLUMN amount_usd TYPE DOUBLE PRECISION USING amount_usd::double precision,
  ALTER COLUMN balance_usd TYPE DOUBLE PRECISION USING balance_usd::double precision;

ALTER TABLE IF EXISTS orders
  ALTER COLUMN channel_fee_rate TYPE DOUBLE PRECISION USING channel_fee_rate::double precision,
  ALTER COLUMN channel_fee TYPE DOUBLE PRECISION USING channel_fee::double precision;

ALTER TABLE IF EXISTS users
  ALTER COLUMN balance_usd TYPE DOUBLE PRECISION USING balance_usd::double precision,
  ALTER COLUMN paypal_fee_rate TYPE DOUBLE PRECISION USING paypal_fee_rate::double precision,
  ALTER COLUMN stripe_fee_rate TYPE DOUBLE PRECISION USING stripe_fee_rate::double precision;
