ALTER TABLE IF EXISTS paypal_accounts
  DROP COLUMN IF EXISTS sandbox_paypal_me_username,
  DROP COLUMN IF EXISTS paypal_me_username;
