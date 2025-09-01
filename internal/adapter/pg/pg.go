package pg

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
	"github.com/shopspring/decimal"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) BeginTx(ctx context.Context) (port.Tx, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx}, nil
}

func (r *Repository) SaveOrder(ctx context.Context, o *domain.Order) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	t := &Tx{tx: tx}
	if err := t.SaveOrder(ctx, o); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) SaveTrade(ctx context.Context, t *domain.Trade) error {
	_, err := r.db.Exec(ctx, `
		insert into trades (id, symbol, buy_order, sell_order, price, quantity, executed_at)
		values ($1,$2,$3,$4,$5,$6,$7)
	`, t.ID, t.Symbol, t.BuyOrder, t.SellOrder, t.Price, t.Quantity, t.Timestamp)
	return err
}

func (r *Repository) LoadOpenOrders(ctx context.Context, symbol string) ([]*domain.Order, error) {
	rows, err := r.db.Query(ctx, `
		select id, client_id, symbol, side, type, price, quantity, remaining, status, created_at, updated_at
		from orders
		where symbol=$1 and status='OPEN'
		order by created_at asc
	`, symbol)
	if err != nil {
		return nil, err
	}
	return collectOrders(rows)
}

func (r *Repository) CancelOrder(ctx context.Context, orderID, clientID string) error {
	cmd, err := r.db.Exec(ctx, `
		update orders set status='CANCELLED', remaining=0
		where id=$1 and client_id=$2 and status='OPEN'
	`, orderID, clientID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("order not found or not OPEN")
	}
	return nil
}

func (r *Repository) ModifyOrder(ctx context.Context, orderID, clientID string, price, qty decimal.Decimal) error {
	cmd, err := r.db.Exec(ctx, `
		update orders set price=$3, quantity=$4, remaining=$4, status='OPEN'
		where id=$1 and client_id=$2 and status='OPEN'
	`, orderID, clientID, price, qty)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("order not found or not OPEN")
	}
	return nil
}

func (r *Repository) LoadSnapshot(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error) {
	orders, err := r.LoadOpenOrders(ctx, symbol)
	if err != nil {
		return nil, err
	}
	var bids, asks []domain.Order
	for _, o := range orders {
		if o.Side == domain.Buy {
			bids = append(bids, *o)
		} else {
			asks = append(asks, *o)
		}
	}
	return &domain.OrderbookSnapshot{
		Symbol: symbol,
		Bids:   bids,
		Asks:   asks,
	}, nil
}

func (r *Repository) LoadOrderByIDForClient(ctx context.Context, orderID, clientID string) (*domain.Order, error) {
	row := r.db.QueryRow(ctx, `
		select id, client_id, symbol, side, type, price, quantity, remaining, status, created_at, updated_at
		from orders
		where id=$1 and client_id=$2
	`, orderID, clientID)
	return scanOrder(row)
}

// returns best bid/ask
func (r *Repository) LoadTopOfBook(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error) {
	rowBid := r.db.QueryRow(ctx, `
		select id, client_id, symbol, side, type, price, quantity, remaining, status, created_at, updated_at
		from orders
		where symbol=$1 and side='BUY' and status='OPEN'
		order by price desc, created_at asc
		limit 1
	`, symbol)
	rowAsk := r.db.QueryRow(ctx, `
		select id, client_id, symbol, side, type, price, quantity, remaining, status, created_at, updated_at
		from orders
		where symbol=$1 and side='SELL' and status='OPEN'
		order by price asc, created_at asc
		limit 1
	`, symbol)

	bid, _ := scanOrder(rowBid)
	ask, _ := scanOrder(rowAsk)

	var bids, asks []domain.Order
	if bid != nil {
		bids = append(bids, *bid)
	}
	if ask != nil {
		asks = append(asks, *ask)
	}

	return &domain.OrderbookSnapshot{
		Symbol: symbol,
		Bids:   bids,
		Asks:   asks,
	}, nil
}

type Tx struct{ tx pgx.Tx }

func (t *Tx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *Tx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

func scanOrder(row pgx.Row) (*domain.Order, error) {
	var o domain.Order
	err := row.Scan(&o.ID, &o.ClientID, &o.Symbol, &o.Side, &o.Type, &o.Price, &o.Quantity, &o.Remaining, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (t *Tx) LoadOrderByIDForClient(ctx context.Context, orderID, clientID string) (*domain.Order, error) {
	row := t.tx.QueryRow(ctx, `
    select id, client_id, symbol, side, type, price, quantity, remaining, status, created_at, updated_at
    from orders where id=$1 and client_id=$2 for update`, orderID, clientID)
	return scanOrder(row)
}

// candidates for glass-lock matching
func (t *Tx) LoadCandidatesForMatch(ctx context.Context, symbol string, side domain.Side, limitPrice *decimal.Decimal, limit int) ([]*domain.Order, error) {
	// buyer matches the ASK (sales) in ascending order of price
	if side == domain.Buy {
		if limitPrice != nil {
			rows, err := t.tx.Query(ctx, `
        select id, client_id, symbol, side, type, price, quantity, remaining, status, created_at, updated_at
        from orders
        where symbol=$1 and side='SELL' and status='OPEN' and price <= $2
        order by price asc, created_at asc
        for update skip locked
        limit $3
      `, symbol, limitPrice, limit)
			if err != nil {
				return nil, err
			}
			return collectOrders(rows)
		}
		rows, err := t.tx.Query(ctx, `
      select ... from orders
      where symbol=$1 and side='SELL' and status='OPEN'
      order by price asc, created_at asc
      for update skip locked
      limit $2
    `, symbol, limit)
		if err != nil {
			return nil, err
		}
		return collectOrders(rows)
	}
	// for the seller, we select the BID in descending order of price
	if limitPrice != nil {
		rows, err := t.tx.Query(ctx, `
      select ... from orders
      where symbol=$1 and side='BUY' and status='OPEN' and price >= $2
      order by price desc, created_at asc
      for update skip locked
      limit $3
    `, symbol, limitPrice, limit)
		if err != nil {
			return nil, err
		}
		return collectOrders(rows)
	}
	rows, err := t.tx.Query(ctx, `
    select ... from orders
    where symbol=$1 and side='BUY' and status='OPEN'
    order by price desc, created_at asc
    for update skip locked
    limit $2
  `, symbol, limit)
	if err != nil {
		return nil, err
	}
	return collectOrders(rows)
}

func collectOrders(rows pgx.Rows) ([]*domain.Order, error) {
	defer rows.Close()
	out := make([]*domain.Order, 0, 64)
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.ID, &o.ClientID, &o.Symbol, &o.Side, &o.Type, &o.Price, &o.Quantity, &o.Remaining, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &o)
	}
	return out, rows.Err()
}

func (t *Tx) SaveOrder(ctx context.Context, o *domain.Order) error {
	_, err := t.tx.Exec(ctx, `
    insert into orders (id, client_id, symbol, side, type, price, quantity, remaining, status, created_at, updated_at)
    values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
    on conflict (id) do update set
      price=excluded.price, quantity=excluded.quantity, remaining=excluded.remaining, status=excluded.status, updated_at=excluded.updated_at
  `, o.ID, o.ClientID, o.Symbol, o.Side, o.Type, o.Price, o.Quantity, o.Remaining, o.Status, o.CreatedAt)
	return err
}

func (t *Tx) SaveTrade(ctx context.Context, tr *domain.Trade) error {
	_, err := t.tx.Exec(ctx, `
    insert into trades (id, symbol, buy_order, sell_order, price, quantity, executed_at)
    values ($1,$2,$3,$4,$5,$6,$7)
  `, tr.ID, tr.Symbol, tr.BuyOrder, tr.SellOrder, tr.Price, tr.Quantity, tr.Timestamp)
	return err
}

func (t *Tx) ModifyOrder(ctx context.Context, orderID, clientID string, price, qty *decimal.Decimal) error {
	if price == nil || qty == nil {
		return errors.New("price and qty must not be nil")
	}
	cmd, err := t.tx.Exec(ctx, `
    update orders set price=$3, quantity=$4, remaining=$4, status='OPEN'
    where id=$1 and client_id=$2 and status='OPEN'
  `, orderID, clientID, price, qty)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("order not found or not OPEN")
	}
	return nil
}

func (t *Tx) CancelOrder(ctx context.Context, orderID, clientID string) error {
	cmd, err := t.tx.Exec(ctx, `
    update orders set status='CANCELLED', remaining=0
    where id=$1 and client_id=$2 and status='OPEN'
  `, orderID, clientID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("order not found or not OPEN")
	}
	return nil
}

func (r *Repository) LoadTradesForOrder(ctx context.Context, orderID string) ([]*domain.Trade, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, symbol, buy_order, sell_order, price, quantity, executed_at
		FROM trades
		WHERE buy_order = $1 OR sell_order = $1
		ORDER BY executed_at ASC
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []*domain.Trade
	for rows.Next() {
		var t domain.Trade
		if err := rows.Scan(&t.ID, &t.Symbol, &t.BuyOrder, &t.SellOrder, &t.Price, &t.Quantity, &t.Timestamp); err != nil {
			return nil, err
		}
		trades = append(trades, &t)
	}
	return trades, rows.Err()
}
