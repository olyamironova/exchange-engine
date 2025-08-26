package engine

import (
	"context"
	"errors"
	_ "sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Engine struct {
	mu        sync.Mutex
	orders    map[string]*Order
	trades    map[string][]*Trade
	orderbook map[string]*OrderbookSnapshot
	snapshots map[string]*OrderbookSnapshot
}

func NewEngine() *Engine {
	return &Engine{
		orders:    make(map[string]*Order),
		trades:    make(map[string][]*Trade),
		orderbook: make(map[string]*OrderbookSnapshot),
		snapshots: make(map[string]*OrderbookSnapshot),
	}
}

func (e *Engine) SubmitOrder(ctx context.Context, o *Order) ([]*Trade, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	o.Remaining = o.Quantity
	o.Status = Open
	o.CreatedAt = time.Now()
	e.orders[o.ID] = o

	// Добавим в стакан
	ob := e.getOrCreateOrderbook(o.Symbol)
	if o.Side == Buy {
		ob.Bids = append(ob.Bids, *o)
	} else {
		ob.Asks = append(ob.Asks, *o)
	}

	// Простая имитация сделок (matching)
	var executedTrades []*Trade
	for _, other := range e.orders {
		if other.Symbol != o.Symbol || other.Status != Open || other.ID == o.ID {
			continue
		}
		if o.Side == Buy && other.Side == Sell && o.Price >= other.Price {
			qty := min(o.Remaining, other.Remaining)
			t := &Trade{
				ID:        uuid.New().String(),
				BuyOrder:  o.ID,
				SellOrder: other.ID,
				Price:     other.Price,
				Quantity:  qty,
				Timestamp: time.Now(),
			}
			executedTrades = append(executedTrades, t)
			o.Remaining -= qty
			other.Remaining -= qty
			if other.Remaining <= 0 {
				other.Status = Filled
			}
			if o.Remaining <= 0 {
				o.Status = Filled
				break
			}
			e.trades[other.ID] = append(e.trades[other.ID], t)
		}
	}

	e.trades[o.ID] = executedTrades
	return executedTrades, nil
}

func (e *Engine) ModifyOrder(ctx context.Context, orderID, clientID string, newPrice, newQty float64) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	o, ok := e.orders[orderID]
	if !ok || o.ClientID != clientID {
		return false, errors.New("order not found")
	}
	if o.Status != Open {
		return false, errors.New("cannot modify executed/canceled order")
	}
	o.Price = newPrice
	o.Quantity = newQty
	o.Remaining = newQty
	return true, nil
}

func (e *Engine) CancelOrder(ctx context.Context, orderID, clientID string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	o, ok := e.orders[orderID]
	if !ok || o.ClientID != clientID {
		return false, errors.New("order not found")
	}
	if o.Status != Open {
		return false, errors.New("cannot cancel executed/canceled order")
	}
	o.Status = Canceled
	o.Remaining = 0
	return true, nil
}

func (e *Engine) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	o, ok := e.orders[orderID]
	if !ok {
		return nil, errors.New("order not found")
	}
	return o, nil
}

func (e *Engine) GetTradesForOrder(orderID string) []*Trade {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.trades[orderID]
}

func (e *Engine) GetOrderbook(symbol string) (*OrderbookSnapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ob, ok := e.orderbook[symbol]
	if !ok {
		return nil, errors.New("symbol not found")
	}
	return ob, nil
}

func (e *Engine) SnapshotOrderbook(symbol string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ob, ok := e.orderbook[symbol]
	if !ok {
		return "", errors.New("symbol not found")
	}
	id := uuid.New().String()
	copySnap := *ob
	e.snapshots[id] = &copySnap
	return id, nil
}

func (e *Engine) RestoreOrderbook(snapshotID string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	snap, ok := e.snapshots[snapshotID]
	if !ok {
		return false, errors.New("snapshot not found")
	}
	e.orderbook["restored"] = snap
	return true, nil
}

func (e *Engine) getOrCreateOrderbook(symbol string) *OrderbookSnapshot {
	ob, ok := e.orderbook[symbol]
	if !ok {
		ob = &OrderbookSnapshot{}
		e.orderbook[symbol] = ob
	}
	return ob
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
