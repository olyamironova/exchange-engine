package core

import (
	"context"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
	"sort"
)

func updateCache(ctx context.Context, repo port.Repository, cache port.Cache, symbol string) {
	if cache == nil {
		return
	}
	snap, err := repo.LoadSnapshot(ctx, symbol)
	if err == nil {
		_ = cache.SetOrderbook(ctx, symbol, snap.DeepCopy())
	} else {
		_ = cache.Invalidate(ctx, symbol)
	}
}

func getOrLoadSnapshot(ctx context.Context, repo port.Repository, cache port.Cache, symbol string) (*domain.OrderbookSnapshot, error) {
	if cache != nil {
		if ob, err := cache.GetOrderbook(ctx, symbol); err == nil && ob != nil {
			return ob, nil
		}
	}
	if repo != nil {
		ob, err := repo.LoadSnapshot(ctx, symbol)
		if err == nil {
			if cache != nil {
				_ = cache.SetOrderbook(ctx, symbol, ob.DeepCopy())
			}
			return ob, nil
		}
	}
	return &domain.OrderbookSnapshot{
		Symbol: symbol,
		Bids:   []domain.Order{},
		Asks:   []domain.Order{},
	}, nil
}

func sortOrders(snapshot *domain.OrderbookSnapshot) {
	sort.Slice(snapshot.Bids, func(i, j int) bool {
		return snapshot.Bids[i].Price.GreaterThan(snapshot.Bids[j].Price)
	})
	sort.Slice(snapshot.Asks, func(i, j int) bool {
		return snapshot.Asks[i].Price.LessThan(snapshot.Asks[j].Price)
	})
}
