package in_memory

import (
	"context"
	"sync"

	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
)

type Cache struct {
	mu    sync.Mutex
	store map[string]*domain.OrderbookSnapshot
}

var _ port.Cache = (*Cache)(nil)

func NewCache() *Cache {
	return &Cache{store: make(map[string]*domain.OrderbookSnapshot)}
}

func (c *Cache) SetOrderbook(ctx context.Context, symbol string, ob *domain.OrderbookSnapshot) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := *ob
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
	copy := *ob
	return &copy, nil
}
