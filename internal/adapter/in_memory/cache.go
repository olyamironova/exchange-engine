package in_memory

import (
	"context"
	"sync"

	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
)

type Cache struct {
	mu    sync.RWMutex
	store map[string]*domain.OrderbookSnapshot
}

var _ port.Cache = (*Cache)(nil)

func NewCache() *Cache {
	return &Cache{store: make(map[string]*domain.OrderbookSnapshot)}
}

func (c *Cache) SetOrderbook(ctx context.Context, symbol string, ob *domain.OrderbookSnapshot) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := deepCopySnapshot(ob)
	c.store[symbol] = &copy
	return nil
}

func (c *Cache) GetOrderbook(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ob, ok := c.store[symbol]
	if !ok {
		return nil, nil
	}
	copy := deepCopySnapshot(ob)
	return &copy, nil
}

func deepCopySnapshot(in *domain.OrderbookSnapshot) domain.OrderbookSnapshot {
	out := *in
	if len(in.Bids) > 0 {
		out.Bids = append([]domain.Order(nil), in.Bids...)
	}
	if len(in.Asks) > 0 {
		out.Asks = append([]domain.Order(nil), in.Asks...)
	}
	if len(in.Trades) > 0 {
		out.Trades = append([]*domain.Trade(nil), in.Trades...)
	}
	return out
}
