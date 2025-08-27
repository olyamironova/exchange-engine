package core

import (
	"github.com/google/uuid"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"sort"
	"time"
)

type OrderBook struct {
	Symbol string
	Buy    []*domain.Order
	Sell   []*domain.Order
	Trades []*domain.Trade
}

func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{Symbol: symbol}
}

func (ob *OrderBook) AddOrder(o *domain.Order) []*domain.Trade {
	if o.Type == domain.Market {
		return ob.matchMarketOrder(o)
	}
	return ob.matchLimitOrder(o)
}

func (ob *OrderBook) matchMarketOrder(o *domain.Order) []*domain.Trade {
	var trades []*domain.Trade
	var bookSide *[]*domain.Order

	// determine the side of the order book (the opposite side for execution)
	if o.Side == domain.Buy {
		bookSide = &ob.Sell
	} else {
		bookSide = &ob.Buy
	}

	// go through the list of orders on the opposite side
	for len(*bookSide) > 0 && o.Remaining > 0 {
		best := (*bookSide)[0]

		// price match is not required for a market order
		tradeQty := min(o.Remaining, best.Remaining)
		tradePrice := best.Price // take the price from the limit order

		// creating a trade
		trade := &domain.Trade{
			ID:        uuid.NewString(),
			BuyOrder:  chooseOrderID(o, best, domain.Buy),
			SellOrder: chooseOrderID(o, best, domain.Sell),
			Symbol:    o.Symbol,
			Quantity:  tradeQty,
			Price:     tradePrice,
			Timestamp: time.Now(),
		}
		trades = append(trades, trade)

		// update remains
		o.Remaining -= tradeQty
		best.Remaining -= tradeQty

		// if the counter order is fully executed, delete it
		if best.Remaining == 0 {
			*bookSide = (*bookSide)[1:]
		}
	}

	// do not add market orders to the book — they are executed immediately
	return trades
}

func (ob *OrderBook) matchLimitOrder(o *domain.Order) []*domain.Trade {
	var trades []*domain.Trade
	if o.Side == domain.Buy {
		trades = ob.executeBuy(o)
		if o.Remaining > 0 {
			ob.Buy = append(ob.Buy, o)
			ob.sortBuy()
		}
	} else {
		trades = ob.executeSell(o)
		if o.Remaining > 0 {
			ob.Sell = append(ob.Sell, o)
			ob.sortSell()
		}
	}
	return trades
}

func (ob *OrderBook) executeBuy(o *domain.Order) []*domain.Trade {
	var trades []*domain.Trade
	i := 0
	for i < len(ob.Sell) && o.Remaining > 0 && ob.Sell[i].Price <= o.Price {
		match := ob.Sell[i]
		qty := min(o.Remaining, match.Remaining)
		price := match.Price

		trades = append(trades, &domain.Trade{
			ID:        generateTradeID(),
			BuyOrder:  o.ID,
			SellOrder: match.ID,
			Price:     price,
			Quantity:  qty,
			Timestamp: time.Now(),
		})

		o.Remaining -= qty
		match.Remaining -= qty
		if match.Remaining == 0 {
			ob.Sell = append(ob.Sell[:i], ob.Sell[i+1:]...)
		} else {
			i++
		}
	}
	return trades
}

func (ob *OrderBook) executeSell(o *domain.Order) []*domain.Trade {
	var trades []*domain.Trade
	i := 0
	for i < len(ob.Buy) && o.Remaining > 0 && ob.Buy[i].Price >= o.Price {
		match := ob.Buy[i]
		qty := min(o.Remaining, match.Remaining)
		price := match.Price

		trades = append(trades, &domain.Trade{
			ID:        generateTradeID(),
			BuyOrder:  match.ID,
			SellOrder: o.ID,
			Price:     price,
			Quantity:  qty,
			Timestamp: time.Now(),
		})

		o.Remaining -= qty
		match.Remaining -= qty
		if match.Remaining == 0 {
			ob.Buy = append(ob.Buy[:i], ob.Buy[i+1:]...)
		} else {
			i++
		}
	}
	return trades
}

func (ob *OrderBook) sortBuy() {
	sort.Slice(ob.Buy, func(i, j int) bool {
		if ob.Buy[i].Price == ob.Buy[j].Price {
			return ob.Buy[i].CreatedAt.Before(ob.Buy[j].CreatedAt)
		}
		return ob.Buy[i].Price > ob.Buy[j].Price
	})
}

func (ob *OrderBook) sortSell() {
	sort.Slice(ob.Sell, func(i, j int) bool {
		if ob.Sell[i].Price == ob.Sell[j].Price {
			return ob.Sell[i].CreatedAt.Before(ob.Sell[j].CreatedAt)
		}
		return ob.Sell[i].Price < ob.Sell[j].Price
	})
}

func generateTradeID() string {
	return time.Now().Format("20060102150405.000000")
}

func chooseOrderID(o1, o2 *domain.Order, side domain.Side) string {
	if o1.Side == side {
		return o1.ID
	}
	return o2.ID
}
