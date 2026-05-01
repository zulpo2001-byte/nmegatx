-- No full rollback for dropped legacy tables in cleanup migration.
-- Keep compatibility by recreating only minimal stubs if needed.
CREATE TABLE IF NOT EXISTS api_keys (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  api_key VARCHAR(255) NOT NULL,
  secret VARCHAR(255) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS products (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  label VARCHAR(100) NOT NULL DEFAULT '',
  b_product_id VARCHAR(128) NOT NULL,
  weight INT NOT NULL DEFAULT 1,
  poll_order INT NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
