package in_memory

import (
	"context"
	"errors"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
	"sync"
)

type noTx struct {
	r *MemoryRepo
}

type MemoryRepo struct {
	mu        sync.RWMutex
	orders    map[string]*domain.Order
	trades    map[string][]*domain.Trade
	snapshots map[string]*domain.OrderbookSnapshot
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		orders:    make(map[string]*domain.Order),
		trades:    make(map[string][]*domain.Trade),
		snapshots: make(map[string]*domain.OrderbookSnapshot),
	}
}

func (r *MemoryRepo) BeginTx(ctx context.Context) (port.Tx, error) {
	return &noTx{r: r}, nil
}

func (t *noTx) SaveOrder(ctx context.Context, o *domain.Order) error { return t.r.SaveOrder(ctx, o) }

func (t *noTx) SaveTrade(ctx context.Context, tr *domain.Trade) error { return t.r.SaveTrade(ctx, tr) }

func (t *noTx) CancelOrder(ctx context.Context, orderID, clientID string) error {
	return t.r.CancelOrder(ctx, orderID, clientID)
}

func (t *noTx) ModifyOrder(ctx context.Context, orderID, clientID string, price, qty float64) error {
	return t.r.ModifyOrder(ctx, orderID, clientID, price, qty)
}

func (t *noTx) SaveSnapshot(ctx context.Context, id, symbol string, ob *domain.OrderbookSnapshot) error {
	return t.r.SaveSnapshot(ctx, id, symbol, ob)
}

func (t *noTx) Commit(ctx context.Context) error { return nil }

func (t *noTx) Rollback(ctx context.Context) error { return nil }

func (r *MemoryRepo) SaveOrder(ctx context.Context, o *domain.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[o.ID] = o
	return nil
}

func (r *MemoryRepo) SaveTrade(ctx context.Context, t *domain.Trade) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trades[t.BuyOrder] = append(r.trades[t.BuyOrder], t)
	r.trades[t.SellOrder] = append(r.trades[t.SellOrder], t)
	return nil
}

func (r *MemoryRepo) LoadOpenOrders(ctx context.Context, symbol string) ([]*domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var res []*domain.Order
	for _, o := range r.orders {
		if o.Symbol == symbol && o.Status == domain.Open && o.Remaining > 0 {
			cp := *o
			res = append(res, &cp)
		}
	}
	return res, nil
}

func (r *MemoryRepo) CancelOrder(ctx context.Context, orderID, clientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[orderID]
	if !ok || o.ClientID != clientID {
		return errors.New("order not found")
	}
	o.Status = domain.Cancelled
	o.Remaining = 0
	return nil
}

func (r *MemoryRepo) ModifyOrder(ctx context.Context, orderID, clientID string, newPrice, newQty float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[orderID]
	if !ok || o.ClientID != clientID {
		return errors.New("order not found")
	}
	o.Price = newPrice
	o.Quantity = newQty
	o.Remaining = newQty
	return nil
}

func (r *MemoryRepo) SaveSnapshot(ctx context.Context, snapshotID, symbol string, ob *domain.OrderbookSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copySnap := *ob
	r.snapshots[snapshotID] = &copySnap
	return nil
}

func (r *MemoryRepo) LoadSnapshot(ctx context.Context, snapshotID string) (*domain.OrderbookSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ob, ok := r.snapshots[snapshotID]
	if !ok {
		return nil, errors.New("snapshot not found")
	}
	return ob, nil
}

func (r *MemoryRepo) Close(ctx context.Context) {}
