package core

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
)

// Engine implements business logic (matching, submit, cancel, modify, snapshot)
type Engine struct {
	repo  port.Repository
	cache port.Cache

	mu        sync.Mutex
	orders    map[string]*domain.Order
	trades    map[string][]*domain.Trade
	orderbook map[string]*domain.OrderbookSnapshot
	snapshots map[string]*domain.OrderbookSnapshot
}

func NewEngine(repo port.Repository, cache port.Cache) *Engine {
	return &Engine{
		repo:      repo,
		cache:     cache,
		orders:    make(map[string]*domain.Order),
		trades:    make(map[string][]*domain.Trade),
		orderbook: make(map[string]*domain.OrderbookSnapshot),
		snapshots: make(map[string]*domain.OrderbookSnapshot),
	}
}

// LoadOpenOrdersFromRepo loads open orders into memory (used on startup)
func (e *Engine) LoadOpenOrdersFromRepo(ctx context.Context, symbols []string) error {
	if e.repo == nil {
		return nil
	}
	for _, s := range symbols {
		orders, err := e.repo.LoadOpenOrders(ctx, s)
		if err != nil {
			return err
		}
		ob := e.getOrCreateOrderbook(s)
		for _, o := range orders {
			e.orders[o.ID] = o
			if o.Side == domain.Buy {
				ob.Bids = append(ob.Bids, *o)
			} else {
				ob.Asks = append(ob.Asks, *o)
			}
		}
	}
	// ensure orderbook sorted
	for _, ob := range e.orderbook {
		sortOrders(ob)
	}
	return nil
}

// SubmitOrder processes order, runs matching, persists and publishes events
func (e *Engine) SubmitOrder(ctx context.Context, o *domain.Order) ([]*domain.Trade, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	o.Remaining = o.Quantity
	o.Status = domain.Open
	o.CreatedAt = time.Now()
	e.orders[o.ID] = o

	ob := e.getOrCreateOrderbook(o.Symbol)
	if o.Side == domain.Buy {
		ob.Bids = append(ob.Bids, *o)
	} else {
		ob.Asks = append(ob.Asks, *o)
	}
	sortOrders(ob)

	var executed []*domain.Trade

	for _, other := range e.orders {
		if other.Symbol != o.Symbol || other.Status != domain.Open || other.ID == o.ID {
			continue
		}
		if o.Side == domain.Buy && other.Side == domain.Sell && (o.Type == domain.Market || o.Price >= other.Price) {
			q := min(o.Remaining, other.Remaining)
			if q <= 0 {
				continue
			}
			tr := &domain.Trade{
				ID:        uuid.New().String(),
				BuyOrder:  o.ID,
				SellOrder: other.ID,
				Price:     other.Price,
				Quantity:  q,
				Timestamp: time.Now(),
			}
			executed = append(executed, tr)
			o.Remaining -= q
			other.Remaining -= q
			if other.Remaining <= 0 {
				other.Status = domain.Filled
			}
			if o.Remaining <= 0 {
				o.Status = domain.Filled
				break
			}
			e.trades[other.ID] = append(e.trades[other.ID], tr)
			if e.repo != nil {
				_ = e.repo.SaveTrade(ctx, tr)
			}
		}
		if o.Side == domain.Sell && other.Side == domain.Buy && (o.Type == domain.Market || other.Price >= o.Price) {
			q := min(o.Remaining, other.Remaining)
			if q <= 0 {
				continue
			}
			tr := &domain.Trade{
				ID:        uuid.New().String(),
				BuyOrder:  other.ID,
				SellOrder: o.ID,
				Price:     other.Price,
				Quantity:  q,
				Timestamp: time.Now(),
			}
			executed = append(executed, tr)
			o.Remaining -= q
			other.Remaining -= q
			if other.Remaining <= 0 {
				other.Status = domain.Filled
			}
			if o.Remaining <= 0 {
				o.Status = domain.Filled
				break
			}
			e.trades[other.ID] = append(e.trades[other.ID], tr)
			if e.repo != nil {
				_ = e.repo.SaveTrade(ctx, tr)
			}
		}
	}

	e.trades[o.ID] = append(e.trades[o.ID], executed...)

	if e.repo != nil {
		_ = e.repo.SaveOrder(ctx, o)
	}

	// update cache & publish
	ob = e.getOrCreateOrderbook(o.Symbol)
	if e.cache != nil {
		_ = e.cache.SetOrderbook(ctx, o.Symbol, ob)
	}

	return executed, nil
}

func (e *Engine) ModifyOrder(ctx context.Context, orderID, clientID string, newPrice, newQty float64) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	o, ok := e.orders[orderID]
	if !ok || o.ClientID != clientID {
		return false, errors.New("order not found")
	}
	if o.Status != domain.Open {
		return false, errors.New("cannot modify non-open order")
	}
	ob := e.getOrCreateOrderbook(o.Symbol)
	if o.Side == domain.Buy {
		ob.Bids = removeOrder(ob.Bids, orderID)
	} else {
		ob.Asks = removeOrder(ob.Asks, orderID)
	}
	o.Price = newPrice
	o.Quantity = newQty
	o.Remaining = newQty

	if o.Side == domain.Buy {
		ob.Bids = append(ob.Bids, *o)
	} else {
		ob.Asks = append(ob.Asks, *o)
	}
	sortOrders(ob)

	if e.repo != nil {
		_ = e.repo.ModifyOrder(ctx, orderID, clientID, newPrice, newQty)
	}
	if e.cache != nil {
		_ = e.cache.SetOrderbook(ctx, o.Symbol, ob)
	}
	return true, nil
}

func (e *Engine) CancelOrder(ctx context.Context, orderID, clientID string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	o, ok := e.orders[orderID]
	if !ok || o.ClientID != clientID {
		return false, errors.New("order not found")
	}
	if o.Status != domain.Open {
		return false, errors.New("cannot cancel non-open order")
	}
	o.Status = domain.Cancelled
	o.Remaining = 0
	ob := e.getOrCreateOrderbook(o.Symbol)
	if o.Side == domain.Buy {
		ob.Bids = removeOrder(ob.Bids, orderID)
	} else {
		ob.Asks = removeOrder(ob.Asks, orderID)
	}
	if e.repo != nil {
		_ = e.repo.CancelOrder(ctx, orderID, clientID)
	}
	if e.cache != nil {
		_ = e.cache.SetOrderbook(ctx, o.Symbol, ob)
	}
	return true, nil
}

func (e *Engine) GetOrder(ctx context.Context, orderID string) (*domain.Order, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	o, ok := e.orders[orderID]
	if !ok {
		return nil, errors.New("order not found")
	}
	return o, nil
}

func (e *Engine) GetTradesForOrder(ctx context.Context, orderID string) ([]*domain.Trade, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	trades := e.trades[orderID]
	return trades, nil
}

func (e *Engine) GetOrderbook(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// try cache first
	if e.cache != nil {
		if ob, err := e.cache.GetOrderbook(ctx, symbol); err == nil && ob != nil {
			return ob, nil
		}
	}
	ob, ok := e.orderbook[symbol]
	if !ok {
		return nil, errors.New("symbol not found")
	}
	return ob, nil
}

func (e *Engine) SnapshotOrderbook(ctx context.Context, symbol string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ob, ok := e.orderbook[symbol]
	if !ok {
		return "", errors.New("symbol not found")
	}
	id := uuid.New().String()
	copy := *ob
	e.snapshots[id] = &copy
	if e.repo != nil {
		_ = e.repo.SaveSnapshot(ctx, id, symbol, &copy)
	}
	return id, nil
}

func (e *Engine) RestoreOrderbook(ctx context.Context, snapshotID string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	snap, ok := e.snapshots[snapshotID]
	if !ok && e.repo != nil {
		s, err := e.repo.LoadSnapshot(ctx, snapshotID)
		if err != nil {
			return false, err
		}
		snap = s
		e.snapshots[snapshotID] = snap
	} else if !ok {
		return false, errors.New("snapshot not found")
	}
	e.orderbook[snap.Symbol] = snap

	return true, nil
}

func (e *Engine) getOrCreateOrderbook(symbol string) *domain.OrderbookSnapshot {
	ob, ok := e.orderbook[symbol]
	if !ok {
		ob = &domain.OrderbookSnapshot{}
		e.orderbook[symbol] = ob
	}
	return ob
}

func sortOrders(ob *domain.OrderbookSnapshot) {
	// bids: price desc, FIFO on CreatedAt
	sort.SliceStable(ob.Bids, func(i, j int) bool {
		if ob.Bids[i].Price != ob.Bids[j].Price {
			return ob.Bids[i].Price > ob.Bids[j].Price
		}
		return ob.Bids[i].CreatedAt.Before(ob.Bids[j].CreatedAt)
	})
	// asks: price asc, FIFO on CreatedAt
	sort.SliceStable(ob.Asks, func(i, j int) bool {
		if ob.Asks[i].Price != ob.Asks[j].Price {
			return ob.Asks[i].Price < ob.Asks[j].Price
		}
		return ob.Asks[i].CreatedAt.Before(ob.Asks[j].CreatedAt)
	})
}

func removeOrder(orders []domain.Order, orderID string) []domain.Order {
	for i, o := range orders {
		if o.ID == orderID {
			return append(orders[:i], orders[i+1:]...)
		}
	}
	return orders
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
