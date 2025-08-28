package port

import (
	"context"
	"github.com/olyamironova/exchange-engine/internal/domain"
)

type Repository interface {
	SaveOrder(ctx context.Context, o *domain.Order) error
	SaveTrade(ctx context.Context, t *domain.Trade) error
	LoadOpenOrders(ctx context.Context, symbol string) ([]*domain.Order, error)
	CancelOrder(ctx context.Context, orderID, clientID string) error
	ModifyOrder(ctx context.Context, orderID, clientID string, price, qty float64) error
	SaveSnapshot(ctx context.Context, id, symbol string, ob *domain.OrderbookSnapshot) error
	LoadSnapshot(ctx context.Context, id string) (*domain.OrderbookSnapshot, error)
	ListSymbols(ctx context.Context) ([]string, error)
	BeginTx(ctx context.Context) (Tx, error)
}

type Tx interface {
	SaveOrder(ctx context.Context, o *domain.Order) error
	SaveTrade(ctx context.Context, t *domain.Trade) error
	CancelOrder(ctx context.Context, orderID, clientID string) error
	ModifyOrder(ctx context.Context, orderID, clientID string, price, qty float64) error
	SaveSnapshot(ctx context.Context, id, symbol string, ob *domain.OrderbookSnapshot) error

	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}
