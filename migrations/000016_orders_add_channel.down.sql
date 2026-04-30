ALTER TABLE orders DROP COLUMN IF EXISTS paypal_account_id;
ALTER TABLE orders DROP COLUMN IF EXISTS stripe_config_id;
ALTER TABLE orders DROP COLUMN IF EXISTS gateway_label;
