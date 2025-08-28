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

	defer e.mu.Unlock()

	var executed []*domain.Trade

	var tx port.Tx
	if e.repo != nil {
		txx, err := e.repo.BeginTx(ctx)
		if err == nil && txx != nil {
			tx = txx
			defer func() {
				if tx != nil {
					if err != nil {
						_ = tx.Rollback(ctx)
					} else {
						if cerr := tx.Commit(ctx); cerr != nil {
							err = cerr
						}
					}
				}
			}()
		}
	}

	if o.Side == domain.Buy {
		asks := ob.Asks // slice of values, but we use map pointers by id
		for i := 0; i < len(asks) && o.Remaining > 0; i++ {
			otherPtr, ok := e.orders[asks[i].ID]
			if !ok || otherPtr.Status != domain.Open {
				continue
			}
			// price check: market orders match any; limit orders must satisfy price
			if !(o.Type == domain.Market || o.Price >= otherPtr.Price) {
				// remaining asks are too expensive
				break
			}
			q := min(o.Remaining, otherPtr.Remaining)
			if q <= 0 {
				continue
			}
			tr := &domain.Trade{
				ID:        uuid.New().String(),
				Symbol:    o.Symbol,
				BuyOrder:  o.ID,
				SellOrder: otherPtr.ID,
				Price:     otherPtr.Price,
				Quantity:  q,
				Timestamp: time.Now(),
			}
			executed = append(executed, tr)
			o.Remaining -= q
			otherPtr.Remaining -= q
			if otherPtr.Remaining <= 0 {
				otherPtr.Status = domain.Filled
			}
			if o.Remaining <= 0 {
				o.Status = domain.Filled
			}
			e.trades[otherPtr.ID] = append(e.trades[otherPtr.ID], tr)
			// persist trade via tx/repo
			if tx != nil {
				if err := tx.SaveTrade(ctx, tr); err != nil {
					_ = tx.Rollback(ctx)
					return nil, err
				}
			} else if e.repo != nil {
				if err := e.repo.SaveTrade(ctx, tr); err != nil {
					return nil, err
				}
			}
		}
	} else {
		bids := ob.Bids
		for i := 0; i < len(bids) && o.Remaining > 0; i++ {
			otherPtr, ok := e.orders[bids[i].ID]
			if !ok || otherPtr.Status != domain.Open {
				continue
			}
			if !(o.Type == domain.Market || otherPtr.Price >= o.Price) {
				break
			}
			q := min(o.Remaining, otherPtr.Remaining)
			if q <= 0 {
				continue
			}
			tr := &domain.Trade{
				ID:        uuid.New().String(),
				Symbol:    o.Symbol,
				BuyOrder:  otherPtr.ID,
				SellOrder: o.ID,
				Price:     otherPtr.Price,
				Quantity:  q,
				Timestamp: time.Now(),
			}
			executed = append(executed, tr)
			o.Remaining -= q
			otherPtr.Remaining -= q
			if otherPtr.Remaining <= 0 {
				otherPtr.Status = domain.Filled
			}
			if o.Remaining <= 0 {
				o.Status = domain.Filled
			}
			e.trades[otherPtr.ID] = append(e.trades[otherPtr.ID], tr)
			if tx != nil {
				if err := tx.SaveTrade(ctx, tr); err != nil {
					_ = tx.Rollback(ctx)
					return nil, err
				}
			} else if e.repo != nil {
				if err := e.repo.SaveTrade(ctx, tr); err != nil {
					return nil, err
				}
			}
		}
	}

	// persist final state of the incoming order
	if tx != nil {
		if err := tx.SaveOrder(ctx, o); err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
	} else if e.repo != nil {
		if err := e.repo.SaveOrder(ctx, o); err != nil {
			return nil, err
		}
	}

	// update cache
	ob = e.getOrCreateOrderbook(o.Symbol)
	// rebuild orderbook slices from current map to ensure they reflect pointer updates
	e.rebuildOrderbookFromMap(o.Symbol)

	if e.cache != nil {
		_ = e.cache.SetOrderbook(ctx, o.Symbol, ob.DeepCopy())
	}

	// append executed trades to own trades map
	e.trades[o.ID] = append(e.trades[o.ID], executed...)

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

func (e *Engine) rebuildOrderbookFromMap(symbol string) {
	ob := e.getOrCreateOrderbook(symbol)
	ob.Bids = ob.Bids[:0]
	ob.Asks = ob.Asks[:0]
	for _, o := range e.orders {
		if o.Symbol != symbol {
			continue
		}
		if o.Side == domain.Buy && o.Remaining > 0 && o.Status == domain.Open {
			ob.Bids = append(ob.Bids, *o)
		}
		if o.Side == domain.Sell && o.Remaining > 0 && o.Status == domain.Open {
			ob.Asks = append(ob.Asks, *o)
		}
	}
	sortOrders(ob)
}
