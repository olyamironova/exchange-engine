package in_memory

import (
	"context"
	"errors"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"sync"
)

type MemoryRepo struct {
	mu        sync.Mutex
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
			res = append(res, o)
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
