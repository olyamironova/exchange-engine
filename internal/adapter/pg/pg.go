package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_ "time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
)

var _ port.Repository = (*PgRepo)(nil)

type PgRepo struct {
	pool *pgxpool.Pool
}

// call Close when finish to work with database.
func NewPgRepo(ctx context.Context, dsn string) (*PgRepo, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pg: create pool: %w", err)
	}
	return &PgRepo{pool: pool}, nil
}

func (p *PgRepo) Close(ctx context.Context) {
	if p.pool != nil {
		p.pool.Close()
	}
}

func (p *PgRepo) SaveOrder(ctx context.Context, o *domain.Order) error {
	if o == nil {
		return errors.New("nil order")
	}
	_, err := p.pool.Exec(ctx, `
INSERT INTO orders(id, client_id, client_order_id, symbol, side, type, price, quantity, remaining, status, created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (id) DO UPDATE SET
  client_id = EXCLUDED.client_id,
  client_order_id = EXCLUDED.client_order_id,
  symbol = EXCLUDED.symbol,
  side = EXCLUDED.side,
  type = EXCLUDED.type,
  price = EXCLUDED.price,
  quantity = EXCLUDED.quantity,
  remaining = EXCLUDED.remaining,
  status = EXCLUDED.status,
  created_at = EXCLUDED.created_at
`, o.ID, o.ClientID, o.ClientOrderID, o.Symbol, string(o.Side), string(o.Type),
		o.Price, o.Quantity, o.Remaining, string(o.Status), o.CreatedAt)
	return err
}

func (p *PgRepo) SaveTrade(ctx context.Context, t *domain.Trade) error {
	if t == nil {
		return errors.New("nil trade")
	}
	_, err := p.pool.Exec(ctx, `
INSERT INTO trades(id, buy_order, sell_order, price, quantity, timestamp)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT (id) DO NOTHING
`, t.ID, t.BuyOrder, t.SellOrder, t.Price, t.Quantity, t.Timestamp)
	return err
}

// LoadOpenOrders returns open orders for a symbol ordered by created_at ASC (FIFO)
func (p *PgRepo) LoadOpenOrders(ctx context.Context, symbol string) ([]*domain.Order, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id, client_id, client_order_id, symbol, side, type, price, quantity, remaining, status, created_at
FROM orders
WHERE symbol = $1 AND COALESCE(remaining,0) > 0 AND status = 'OPEN'
ORDER BY created_at ASC
`, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*domain.Order
	for rows.Next() {
		var o domain.Order
		var side, typ, status string
		if err := rows.Scan(&o.ID, &o.ClientID, &o.ClientOrderID, &o.Symbol, &side, &typ, &o.Price, &o.Quantity, &o.Remaining, &status, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.Side = domain.Side(side)
		o.Type = domain.OrderType(typ)
		o.Status = domain.OrderStatus(status)
		res = append(res, &o)
	}
	return res, nil
}

// CancelOrder marks an order as cancelled if it's still open
func (p *PgRepo) CancelOrder(ctx context.Context, orderID, clientID string) error {
	res, err := p.pool.Exec(ctx, `
UPDATE orders
SET remaining = 0, status = 'CANCELLED'
WHERE id = $1 AND client_id = $2 AND COALESCE(remaining,0) > 0 AND status = 'OPEN'
`, orderID, clientID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("order not found or already closed")
	}
	return nil
}

// ModifyOrder updates price/quantity/remaining for an open order
func (p *PgRepo) ModifyOrder(ctx context.Context, orderID, clientID string, price, qty float64) error {
	res, err := p.pool.Exec(ctx, `
UPDATE orders
SET price = $1, quantity = $2, remaining = $2
WHERE id = $3 AND client_id = $4 AND COALESCE(remaining,0) > 0 AND status = 'OPEN'
`, price, qty, orderID, clientID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("order not found or already closed")
	}
	return nil
}

// SaveSnapshot persists orderbook snapshot as JSONB
func (p *PgRepo) SaveSnapshot(ctx context.Context, snapshotID, symbol string, ob *domain.OrderbookSnapshot) error {
	if ob == nil {
		return errors.New("nil snapshot")
	}
	b, err := json.Marshal(ob)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `
INSERT INTO orderbook_snapshots(id, symbol, snapshot_json, created_at)
VALUES($1,$2,$3,NOW())
ON CONFLICT (id) DO UPDATE SET snapshot_json = EXCLUDED.snapshot_json, created_at = NOW()
`, snapshotID, symbol, string(b))
	return err
}

// LoadSnapshot loads snapshot JSONB and unmarshals
func (p *PgRepo) LoadSnapshot(ctx context.Context, snapshotID string) (*domain.OrderbookSnapshot, error) {
	var data string
	if err := p.pool.QueryRow(ctx, `SELECT snapshot_json FROM orderbook_snapshots WHERE id = $1`, snapshotID).Scan(&data); err != nil {
		return nil, err
	}
	var ob domain.OrderbookSnapshot
	if err := json.Unmarshal([]byte(data), &ob); err != nil {
		return nil, err
	}
	return &ob, nil
}

// ListSymbols returns distinct symbols present in orders table
func (p *PgRepo) ListSymbols(ctx context.Context) ([]string, error) {
	rows, err := p.pool.Query(ctx, `SELECT DISTINCT symbol FROM orders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		res = append(res, s)
	}
	return res, nil
}
