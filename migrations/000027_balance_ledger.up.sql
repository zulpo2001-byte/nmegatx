CREATE TABLE IF NOT EXISTS balance_ledgers (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  order_id BIGINT NULL REFERENCES orders(id) ON DELETE SET NULL,
  type VARCHAR(20) NOT NULL,
  amount_usd DOUBLE PRECISION NOT NULL,
  balance_usd DOUBLE PRECISION NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_balance_ledgers_user_id ON balance_ledgers(user_id);
CREATE INDEX IF NOT EXISTS idx_balance_ledgers_order_id ON balance_ledgers(order_id);
CREATE INDEX IF NOT EXISTS idx_balance_ledgers_type ON balance_ledgers(type);
