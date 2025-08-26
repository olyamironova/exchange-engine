package engine

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Engine struct {
	mtx    sync.Mutex
	orders map[string][]*Order
	trades []*Trade
}

func NewEngine() *Engine {
	return &Engine{
		orders: make(map[string][]*Order),
		trades: []*Trade{},
	}
}

// SubmitOrder добавляет новый ордер и выполняет matching
func (e *Engine) SubmitOrder(ctx context.Context, o *Order) ([]*Trade, error) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if o.Remaining == 0 {
		o.Remaining = o.Quantity
	}
	o.ID = uuid.New().String()
	o.CreatedAt = time.Now()

	ob, ok := e.orders[o.Symbol]
	if !ok {
		ob = []*Order{}
	}

	var trades []*Trade
	if o.Side == Buy {
		trades = e.match(o, ob, Sell)
	} else {
		trades = e.match(o, ob, Buy)
	}

	if o.Remaining > 0 {
		ob = append(ob, o)
	}
	e.orders[o.Symbol] = ob
	e.trades = append(e.trades, trades...)
	return trades, nil
}

// match ищет контраордеры для выполнения сделки
func (e *Engine) match(o *Order, book []*Order, opposite Side) []*Trade {
	var trades []*Trade
	// фильтруем по стороне
	var candidates []*Order
	for _, ord := range book {
		if ord.Side == opposite && ord.Remaining > 0 {
			candidates = append(candidates, ord)
		}
	}

	// сортировка по цене, затем FIFO
	sort.SliceStable(candidates, func(i, j int) bool {
		if o.Side == Buy {
			if candidates[i].Price == candidates[j].Price {
				return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
			}
			return candidates[i].Price < candidates[j].Price
		}
		if candidates[i].Price == candidates[j].Price {
			return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
		}
		return candidates[i].Price > candidates[j].Price
	})

	for _, c := range candidates {
		if o.Remaining == 0 {
			break
		}
		price := c.Price
		if o.Type == Market {
			price = c.Price
		} else if o.Side == Buy && o.Price < c.Price {
			continue
		} else if o.Side == Sell && o.Price > c.Price {
			continue
		}

		qty := min(o.Remaining, c.Remaining)
		o.Remaining -= qty
		c.Remaining -= qty

		trades = append(trades, &Trade{
			ID:        uuid.New().String(),
			BuyOrder:  chooseID(o, c, Buy),
			SellOrder: chooseID(o, c, Sell),
			Price:     price,
			Quantity:  qty,
			Timestamp: time.Now(),
		})
	}
	return trades
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func chooseID(a, b *Order, side Side) string {
	if side == Buy {
		if a.Side == Buy {
			return a.ID
		}
		return b.ID
	} else {
		if a.Side == Sell {
			return a.ID
		}
		return b.ID
	}
}

func (e *Engine) GetOrderbook(symbol string) (*OrderbookSnapshot, error) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	book, ok := e.orders[symbol]
	if !ok {
		return nil, errors.New("symbol not found")
	}

	var bids, asks []Order
	for _, o := range book {
		if o.Remaining > 0 {
			if o.Side == Buy {
				bids = append(bids, *o)
			} else {
				asks = append(asks, *o)
			}
		}
	}
	// сортировка для внешнего API
	sort.Slice(bids, func(i, j int) bool { return bids[i].Price > bids[j].Price })
	sort.Slice(asks, func(i, j int) bool { return asks[i].Price < asks[j].Price })

	return &OrderbookSnapshot{Bids: bids, Asks: asks}, nil
}
