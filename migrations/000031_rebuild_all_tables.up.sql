-- WARNING:
-- This migration is a destructive full rebuild for environments that want a clean schema.
-- It drops and recreates the public schema, then creates all active tables/fields.

DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;

CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  email VARCHAR(255) UNIQUE NOT NULL,
  password VARCHAR(255) NOT NULL,
  role VARCHAR(32) NOT NULL DEFAULT 'user',
  permissions JSONB,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  expires_at TIMESTAMPTZ,
  balance_usd NUMERIC(14,4) NOT NULL DEFAULT 0,
  paypal_fee_rate NUMERIC(8,4) NOT NULL DEFAULT 0,
  stripe_fee_rate NUMERIC(8,4) NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_status ON users(status);

CREATE TABLE refresh_tokens (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token VARCHAR(255) UNIQUE NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE webhook_endpoints (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type VARCHAR(10) NOT NULL,
  label VARCHAR(100) NOT NULL,
  url TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  a_api_key VARCHAR(128),
  shared_secret VARCHAR(255),
  b_api_key VARCHAR(128),
  b_shared_secret VARCHAR(255),
  payment_method VARCHAR(20) NOT NULL DEFAULT 'all',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(type, a_api_key),
  UNIQUE(type, b_api_key)
);
CREATE INDEX idx_webhook_endpoints_user_type ON webhook_endpoints(user_id, type);

CREATE TABLE paypal_accounts (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label VARCHAR(100) NOT NULL,
  mode VARCHAR(10) NOT NULL DEFAULT 'email',
  email VARCHAR(255),
  paypal_me_username VARCHAR(128),
  client_id TEXT,
  client_secret TEXT,
  sandbox BOOLEAN NOT NULL DEFAULT false,
  sandbox_mode BOOLEAN NOT NULL DEFAULT false,
  sandbox_email VARCHAR(255),
  sandbox_paypal_me_username VARCHAR(128),
  sandbox_client_id TEXT,
  sandbox_client_secret TEXT,
  account_state VARCHAR(20) NOT NULL DEFAULT 'active',
  enabled BOOLEAN NOT NULL DEFAULT true,
  poll_order INTEGER NOT NULL DEFAULT 0,
  poll_mode VARCHAR(20) NOT NULL DEFAULT 'random',
  weight INTEGER NOT NULL DEFAULT 10,
  smart_weight NUMERIC(6,2) NOT NULL DEFAULT 50,
  total_orders BIGINT NOT NULL DEFAULT 0,
  total_success BIGINT NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  min_amount NUMERIC(10,2) NOT NULL DEFAULT 0,
  max_amount NUMERIC(10,2) NOT NULL DEFAULT 99999,
  max_orders INTEGER NOT NULL DEFAULT 0,
  max_amount_total NUMERIC(12,2) NOT NULL DEFAULT 0,
  daily_orders INTEGER NOT NULL DEFAULT 0,
  daily_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
  daily_reset_hour INTEGER NOT NULL DEFAULT 0,
  last_reset_date DATE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_paypal_user_state ON paypal_accounts(user_id, account_state);

CREATE TABLE stripe_configs (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label VARCHAR(100) NOT NULL,
  secret_key TEXT,
  publishable_key TEXT,
  webhook_secret TEXT,
  account_state VARCHAR(20) NOT NULL DEFAULT 'active',
  enabled BOOLEAN NOT NULL DEFAULT true,
  sandbox BOOLEAN NOT NULL DEFAULT false,
  poll_order INTEGER NOT NULL DEFAULT 0,
  poll_mode VARCHAR(20) NOT NULL DEFAULT 'random',
  weight INTEGER NOT NULL DEFAULT 10,
  smart_weight NUMERIC(6,2) NOT NULL DEFAULT 50,
  total_orders BIGINT NOT NULL DEFAULT 0,
  total_success BIGINT NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  min_amount NUMERIC(10,2) NOT NULL DEFAULT 0,
  max_amount NUMERIC(10,2) NOT NULL DEFAULT 99999,
  max_orders INTEGER NOT NULL DEFAULT 0,
  max_amount_total NUMERIC(12,2) NOT NULL DEFAULT 0,
  daily_orders INTEGER NOT NULL DEFAULT 0,
  daily_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
  daily_reset_hour INTEGER NOT NULL DEFAULT 0,
  last_reset_date DATE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_stripe_user_state ON stripe_configs(user_id, account_state);

CREATE TABLE orders (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  a_order_id VARCHAR(128),
  b_order_id VARCHAR(128),
  b_transaction_id VARCHAR(128),
  pay_token VARCHAR(128) UNIQUE NOT NULL,
  amount NUMERIC(12,2) NOT NULL,
  payment_url VARCHAR(1024),
  return_url VARCHAR(1024),
  checkout_url VARCHAR(1024),
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  payment_method VARCHAR(20) NOT NULL DEFAULT 'stripe',
  currency VARCHAR(10) NOT NULL DEFAULT 'USD',
  email VARCHAR(255),
  ip VARCHAR(45),
  risk_score INTEGER NOT NULL DEFAULT 0,
  paypal_account_id BIGINT REFERENCES paypal_accounts(id) ON DELETE SET NULL,
  stripe_config_id BIGINT REFERENCES stripe_configs(id) ON DELETE SET NULL,
  gateway_label VARCHAR(100),
  validity_days INTEGER,
  expires_at TIMESTAMPTZ,
  paid_at TIMESTAMPTZ,
  abandoned_at TIMESTAMPTZ,
  callback_state VARCHAR(32),
  channel_fee_rate NUMERIC(8,4) NOT NULL DEFAULT 0,
  channel_fee NUMERIC(12,4) NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_order_user_status ON orders(user_id, status);
CREATE INDEX idx_a_order_user ON orders(a_order_id, user_id);

CREATE TABLE balance_ledgers (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  order_id BIGINT REFERENCES orders(id) ON DELETE SET NULL,
  type VARCHAR(20) NOT NULL,
  amount_usd NUMERIC(14,4) NOT NULL,
  balance_usd NUMERIC(14,4) NOT NULL,
  note VARCHAR(255),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_balance_ledgers_user ON balance_ledgers(user_id);
CREATE INDEX idx_balance_ledgers_order ON balance_ledgers(order_id);

CREATE TABLE roles (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(64) UNIQUE NOT NULL,
  display_name VARCHAR(100),
  description TEXT,
  permissions JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE risk_rules (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) UNIQUE NOT NULL,
  type VARCHAR(32) NOT NULL,
  action VARCHAR(20) NOT NULL,
  risk_score INTEGER NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT true,
  conditions TEXT,
  config TEXT,
  description TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_records (
  id BIGSERIAL PRIMARY KEY,
  level VARCHAR(16) NOT NULL,
  category VARCHAR(32),
  title VARCHAR(255),
  content TEXT,
  source VARCHAR(64),
  source_id BIGINT,
  status VARCHAR(16) NOT NULL DEFAULT 'open',
  assigned_to BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ
);
CREATE INDEX idx_alert_records_status ON alert_records(status);

CREATE TABLE alert_channels (
  id BIGSERIAL PRIMARY KEY,
  type VARCHAR(32) NOT NULL,
  name VARCHAR(100) NOT NULL,
  webhook_url TEXT,
  bot_token TEXT,
  chat_id TEXT,
  config JSONB,
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE global_settings (
  id BIGSERIAL PRIMARY KEY,
  key VARCHAR(128) UNIQUE NOT NULL,
  value TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
  id BIGSERIAL PRIMARY KEY,
  actor_user_id BIGINT,
  actor_email VARCHAR(255),
  action VARCHAR(128),
  resource VARCHAR(128),
  resource_id VARCHAR(128),
  detail TEXT,
  ip VARCHAR(45),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_user_id);

CREATE TABLE channel_metrics (
  id BIGSERIAL PRIMARY KEY,
  channel_type VARCHAR(20),
  channel_id BIGINT,
  channel_label VARCHAR(100),
  hour_slot TIMESTAMPTZ,
  total_requests INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  risk_reject_count INTEGER NOT NULL DEFAULT 0,
  avg_response_ms INTEGER NOT NULL DEFAULT 0,
  success_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
  risk_pass_rate NUMERIC(5,2) NOT NULL DEFAULT 100,
  dynamic_weight NUMERIC(6,2) NOT NULL DEFAULT 50,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_channel_metrics_type_id_hour ON channel_metrics(channel_type, channel_id, hour_slot);
