package port

import (
	"context"

	"github.com/olyamironova/exchange-engine/internal/domain"
)

type Cache interface {
	SetOrderbook(ctx context.Context, symbol string, ob *domain.OrderbookSnapshot) error
	GetOrderbook(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error)
}
