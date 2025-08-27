CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS orders (
                                      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                      client_id BIGINT NOT NULL,
                                      client_order_id BIGSERIAL,
                                      symbol TEXT NOT NULL,
                                      side TEXT NOT NULL,
                                      type TEXT NOT NULL,
                                      price DOUBLE PRECISION,
                                      quantity DOUBLE PRECISION NOT NULL,
                                      remaining DOUBLE PRECISION NOT NULL,
                                      status TEXT NOT NULL,
                                      created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_orders_symbol_status ON orders(symbol, status);

CREATE UNIQUE INDEX IF NOT EXISTS ux_orders_client_clientorderid ON orders (client_id, client_order_id);

CREATE TABLE IF NOT EXISTS trades (
                                      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                      buy_order UUID,
                                      sell_order UUID,
                                      price DOUBLE PRECISION NOT NULL,
                                      quantity DOUBLE PRECISION NOT NULL,
                                      timestamp TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Allow foreign keys if desired (optional):
-- ALTER TABLE trades ADD CONSTRAINT fk_buy_order FOREIGN KEY (buy_order) REFERENCES orders(id) ON DELETE SET NULL;
-- ALTER TABLE trades ADD CONSTRAINT fk_sell_order FOREIGN KEY (sell_order) REFERENCES orders(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS orderbook_snapshots (
                                                   id UUID PRIMARY KEY,
                                                   symbol TEXT NOT NULL,
                                                   snapshot_json JSONB NOT NULL,
                                                   created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
    );

CREATE INDEX IF NOT EXISTS idx_snapshots_symbol ON orderbook_snapshots(symbol);
