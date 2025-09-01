package port

import (
	"context"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/shopspring/decimal"
)

type Repository interface {
	SaveOrder(ctx context.Context, o *domain.Order) error
	SaveTrade(ctx context.Context, t *domain.Trade) error
	LoadOpenOrders(ctx context.Context, symbol string) ([]*domain.Order, error)
	CancelOrder(ctx context.Context, orderID, clientID string) error
	ModifyOrder(ctx context.Context, orderID, clientID string, price, qty decimal.Decimal) error
	LoadSnapshot(ctx context.Context, id string) (*domain.OrderbookSnapshot, error)
	BeginTx(ctx context.Context) (Tx, error)
	LoadOrderByIDForClient(ctx context.Context, orderID, clientID string) (*domain.Order, error)
	LoadTopOfBook(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error)
	LoadTradesForOrder(ctx context.Context, orderID string) ([]*domain.Trade, error)
}

type Tx interface {
	SaveOrder(ctx context.Context, o *domain.Order) error
	SaveTrade(ctx context.Context, t *domain.Trade) error
	CancelOrder(ctx context.Context, orderID, clientID string) error
	ModifyOrder(ctx context.Context, orderID, clientID string, price, qty *decimal.Decimal) error
	LoadOrderByIDForClient(ctx context.Context, orderID, clientID string) (*domain.Order, error)
	LoadCandidatesForMatch(ctx context.Context, symbol string, side domain.Side, limitPrice *decimal.Decimal, limit int) ([]*domain.Order, error)

	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}
