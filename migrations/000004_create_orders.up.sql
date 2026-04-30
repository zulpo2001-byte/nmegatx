CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    a_order_id VARCHAR(128) NOT NULL,
    b_order_id VARCHAR(128),
    pay_token VARCHAR(128) NOT NULL UNIQUE,
    amount NUMERIC(12,2) NOT NULL,
    payment_url TEXT,
    return_url TEXT,
    checkout_url TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    paid_at TIMESTAMPTZ NULL,
    abandoned_at TIMESTAMPTZ NULL,
    callback_state VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, a_order_id)
);

CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_user_status ON orders(user_id, status);
