package port

import (
	"context"
	"time"

	"github.com/olyamironova/exchange-engine/internal/domain"
)

type Cache interface {
	SetOrderbook(ctx context.Context, symbol string, ob *domain.OrderbookSnapshot) error
	GetOrderbook(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error)
	Invalidate(ctx context.Context, symbol string) error
	SetSnapshot(ctx context.Context, snapshotID string, data []byte, ttl time.Duration) error
	GetSnapshot(ctx context.Context, snapshotID string) ([]byte, error)
}
