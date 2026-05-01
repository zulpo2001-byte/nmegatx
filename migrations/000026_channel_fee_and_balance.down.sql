ALTER TABLE IF EXISTS orders
  DROP COLUMN IF EXISTS channel_fee,
  DROP COLUMN IF EXISTS channel_fee_rate;

ALTER TABLE IF EXISTS users
  DROP COLUMN IF EXISTS stripe_fee_rate,
  DROP COLUMN IF EXISTS paypal_fee_rate,
  DROP COLUMN IF EXISTS balance_usd;
