package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	_ "time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
)

var _ port.Repository = (*PgRepo)(nil)
var _ port.Tx = (*pgTx)(nil)

var validSides = map[string]domain.Side{
	"BUY":  domain.Buy,
	"SELL": domain.Sell,
}

var validTypes = map[string]domain.OrderType{
	"LIMIT":  domain.Limit,
	"MARKET": domain.Market,
}

var validStatuses = map[string]domain.OrderStatus{
	"OPEN":      domain.Open,
	"FILLED":    domain.Filled,
	"CANCELLED": domain.Cancelled,
}

type PgRepo struct {
	pool *pgxpool.Pool
}

type pgTx struct {
	tx pgx.Tx
}

func (p *PgRepo) BeginTx(ctx context.Context) (port.Tx, error) {
	if p.pool == nil {
		return nil, fmt.Errorf("pg pool nil")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &pgTx{tx: tx}, nil
}

// call Close when you finish work with database.
func NewPgRepo(ctx context.Context, dsn string) (*PgRepo, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pg: create pool: %w", err)
	}
	return &PgRepo{pool: pool}, nil
}

func (p *PgRepo) Close() {
	if p.pool != nil {
		p.pool.Close()
	}
}

func (p *PgRepo) SaveOrder(ctx context.Context, o *domain.Order) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		err := tx.Rollback(ctx)
		if err != nil {

		}
	}(tx, ctx)

	if o == nil {
		return errors.New("nil order")
	}
	_, err = tx.Exec(ctx, `
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
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (t *pgTx) SaveOrder(ctx context.Context, o *domain.Order) error {
	_, err := t.tx.Exec(ctx, `
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
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		err := tx.Rollback(ctx)
		if err != nil {

		}
	}(tx, ctx)

	if t == nil {
		return errors.New("nil trade")
	}
	_, err = p.pool.Exec(ctx, `
INSERT INTO trades(id, buy_order, sell_order, price, quantity, timestamp)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT (id) DO NOTHING
`, t.ID, t.BuyOrder, t.SellOrder, t.Price, t.Quantity, t.Timestamp)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (t *pgTx) SaveTrade(ctx context.Context, tr *domain.Trade) error {
	_, err := t.tx.Exec(ctx, `INSERT INTO trades(id, symbol, buy_order, sell_order, price, quantity, timestamp)
		VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
		tr.ID, tr.Symbol, tr.BuyOrder, tr.SellOrder, tr.Price, tr.Quantity, tr.Timestamp)
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
		var side, orderType, status string
		if err := rows.Scan(&o.ID, &o.ClientID, &o.ClientOrderID, &o.Symbol,
			&side, &orderType, &o.Price, &o.Quantity, &o.Remaining,
			&status, &o.CreatedAt); err != nil {
			return nil, err
		}
		// validate
		s, ok := validSides[side]
		if !ok {
			return nil, fmt.Errorf("pg: invalid side value %q", side)
		}
		t, ok := validTypes[orderType]
		if !ok {
			return nil, fmt.Errorf("pg: invalid type value %q", orderType)
		}
		st, ok := validStatuses[status]
		if !ok {
			return nil, fmt.Errorf("pg: invalid status value %q", status)
		}
		o.Side, o.Type, o.Status = s, t, st
		res = append(res, &o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error in load: %w", err)
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

func (t *pgTx) CancelOrder(ctx context.Context, orderID, clientID string) error {
	_, err := t.tx.Exec(ctx, `UPDATE orders SET remaining=0, status='CANCELLED'
		WHERE id=$1 AND client_id=$2 AND COALESCE(remaining,0) > 0 AND status='OPEN'`, orderID, clientID)
	return err
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

func (t *pgTx) ModifyOrder(ctx context.Context, orderID, clientID string, price, qty float64) error {
	_, err := t.tx.Exec(ctx, `UPDATE orders SET price=$1, quantity=$2, remaining=$2
		WHERE id=$3 AND client_id=$4 AND COALESCE(remaining,0) > 0 AND status='OPEN'`,
		price, qty, orderID, clientID)
	return err
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

func (t *pgTx) SaveSnapshot(ctx context.Context, id, symbol string, ob *domain.OrderbookSnapshot) error {
	// same as SaveSnapshot in PgRepo but using t.tx
	b, err := json.Marshal(ob)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(ctx, `INSERT INTO orderbook_snapshots(id, symbol, snapshot_json, created_at)
		VALUES($1,$2,$3,NOW()) ON CONFLICT (id) DO UPDATE SET snapshot_json = EXCLUDED.snapshot_json, created_at = NOW()`,
		id, symbol, string(b))
	return err
}

func (t *pgTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}
func (t *pgTx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}
func (p *PgRepo) SaveOrderAndTradeTx(ctx context.Context, o *domain.Order, t *domain.Trade) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pg: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := p.saveOrderTx(ctx, tx, o); err != nil {
		return err
	}
	if err := p.saveTradeTx(ctx, tx, t); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg: commit tx: %w", err)
	}
	return nil
}

func (p *PgRepo) saveOrderTx(ctx context.Context, tx pgx.Tx, o *domain.Order) error {
	_, err := tx.Exec(ctx, `
INSERT INTO orders(
  id, client_id, client_order_id, symbol, side, type, price, quantity, remaining, status, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
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
	if err != nil {
		return fmt.Errorf("pg: saveOrderTx: %w", err)
	}
	return nil
}

func (p *PgRepo) saveTradeTx(ctx context.Context, tx pgx.Tx, t *domain.Trade) error {
	_, err := tx.Exec(ctx, `
INSERT INTO trades(id, buy_order, sell_order, price, quantity, timestamp)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (id) DO NOTHING
`, t.ID, t.BuyOrder, t.SellOrder, t.Price, t.Quantity, t.Timestamp)
	if err != nil {
		return fmt.Errorf("pg: saveTradeTx: %w", err)
	}
	return nil
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
