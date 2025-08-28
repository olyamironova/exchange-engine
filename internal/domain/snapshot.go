package domain

import (
	"sync"
	"time"
)

type OrderbookSnapshot struct {
	mu        sync.RWMutex
	Bids      []Order
	Asks      []Order
	Trades    []*Trade
	Timestamp time.Time
	Symbol    string
}

func (o *OrderbookSnapshot) DeepCopy() *OrderbookSnapshot {
	if o == nil {
		return nil
	}
	copyBids := make([]Order, len(o.Bids))
	copy(copyBids, o.Bids)
	copyAsks := make([]Order, len(o.Asks))
	copy(copyAsks, o.Asks)
	copyTrades := make([]*Trade, len(o.Trades))
	for i, t := range o.Trades {
		if t == nil {
			copyTrades[i] = nil
			continue
		}
		tc := *t
		copyTrades[i] = &tc
	}
	return &OrderbookSnapshot{
		Bids:      copyBids,
		Asks:      copyAsks,
		Timestamp: o.Timestamp,
		Symbol:    o.Symbol,
		Trades:    copyTrades,
	}
}

func (s *OrderbookSnapshot) AddTrade(t *Trade) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Trades = append(s.Trades, t)
}
