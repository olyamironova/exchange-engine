package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/port"
	"github.com/shopspring/decimal"
)

// Engine implements business logic (matching, submit, cancel, modify, snapshot)
type Engine struct {
	repo  port.Repository
	cache port.Cache
}

func NewEngine(repo port.Repository, cache port.Cache) *Engine {
	return &Engine{
		repo:  repo,
		cache: cache,
	}
}

func validateOrder(o *domain.Order) error {
	if o.Type == domain.Limit && o.Price.LessThanOrEqual(decimal.Zero) {
		return errors.New("limit price must be > 0")
	}
	if o.Quantity.LessThanOrEqual(decimal.Zero) {
		return errors.New("quantity must be > 0")
	}
	return nil
}

func updateOrderStatus(o *domain.Order) {
	switch {
	case o.Remaining.IsZero():
		o.Status = domain.Filled
	case o.Remaining.LessThan(o.Quantity):
		o.Status = domain.PartiallyFilled
	default:
		o.Status = domain.Open
	}
}
func (e *Engine) SubmitOrder(ctx context.Context, o *domain.Order) ([]*domain.Trade, error) {
	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	o.CreatedAt = time.Now().UTC()
	o.Status = domain.Open
	o.Remaining = o.Quantity

	if err := validateOrder(o); err != nil {
		return nil, err
	}

	var executed []*domain.Trade
	err := withTx(ctx, e.repo, func(tx port.Tx) error {
		if err := tx.SaveOrder(ctx, o); err != nil {
			return err
		}
		var err error
		executed, err = e.matchOrder(ctx, tx, o)
		return err
	})
	if err != nil {
		return nil, err
	}

	updateOrderStatus(o)
	err = withTx(ctx, e.repo, func(tx port.Tx) error {
		return tx.SaveOrder(ctx, o)
	})
	if err != nil {
		return nil, err
	}

	updateCache(ctx, e.repo, e.cache, o.Symbol)
	return executed, nil
}

func (e *Engine) matchOrder(ctx context.Context, tx port.Tx, o *domain.Order) ([]*domain.Trade, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	executed := []*domain.Trade{}
	const batchSize = 200
	now := time.Now().UTC()

	for o.Remaining.GreaterThan(decimal.Zero) {
		select {
		case <-ctx.Done():
			return executed, ctx.Err()
		default:
		}

		var lp *decimal.Decimal
		if o.Type == domain.Limit {
			lp = &o.Price
		}

		cands, err := tx.LoadCandidatesForMatch(ctx, o.Symbol, o.Side, lp, batchSize)
		if err != nil {
			return executed, err
		}
		if len(cands) == 0 {
			break
		}

		progressed := false
		for _, other := range cands {
			if o.Remaining.LessThanOrEqual(decimal.Zero) {
				break
			}
			if !priceMatch(o, other) {
				continue
			}

			q := decimal.Min(o.Remaining, other.Remaining)
			if q.LessThanOrEqual(decimal.Zero) {
				continue
			}

			tr := &domain.Trade{
				ID:        uuid.New().String(),
				Symbol:    o.Symbol,
				BuyOrder:  chooseOrderID(o, other, domain.Buy),
				SellOrder: chooseOrderID(o, other, domain.Sell),
				Price:     other.Price,
				Quantity:  q,
				Timestamp: now,
			}

			if err := tx.SaveTrade(ctx, tr); err != nil {
				return executed, err
			}
			executed = append(executed, tr)

			o.Remaining = o.Remaining.Sub(q)
			other.Remaining = other.Remaining.Sub(q)

			updateOrderStatus(other)
			if err := tx.SaveOrder(ctx, other); err != nil {
				return executed, err
			}

			progressed = true
		}

		if !progressed {
			break
		}
	}

	return executed, nil
}

func priceMatch(o, other *domain.Order) bool {
	if o.Type != domain.Limit {
		return true
	}
	if o.Side == domain.Buy && o.Price.LessThan(other.Price) {
		return false
	}
	if o.Side == domain.Sell && o.Price.GreaterThan(other.Price) {
		return false
	}
	return true
}

func chooseOrderID(o1, o2 *domain.Order, side domain.Side) string {
	if o1.Side == side {
		return o1.ID
	}
	return o2.ID
}

func (e *Engine) loadSnapshot(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error) {
	if e.cache != nil {
		if ob, err := e.cache.GetOrderbook(ctx, symbol); err == nil && ob != nil {
			return ob, nil
		}
	}
	if e.repo != nil {
		ob, err := e.repo.LoadSnapshot(ctx, symbol)
		if err == nil {
			if e.cache != nil {
				_ = e.cache.SetOrderbook(ctx, symbol, ob.DeepCopy())
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

func (e *Engine) ModifyOrder(ctx context.Context, orderID, clientID string, newPrice, newQty decimal.Decimal) error {
	var symbol string
	err := withTx(ctx, e.repo, func(tx port.Tx) error {
		o, err := tx.LoadOrderByIDForClient(ctx, orderID, clientID)
		if err != nil {
			return err
		}
		if o.Status != domain.Open {
			return errors.New("cannot modify non-open order")
		}
		o.Price = newPrice
		o.Quantity = newQty
		o.Remaining = newQty
		symbol = o.Symbol
		return tx.SaveOrder(ctx, o)
	})
	if err != nil {
		return err
	}

	updateCache(ctx, e.repo, e.cache, symbol)
	return nil
}

func (e *Engine) CancelOrder(ctx context.Context, orderID, clientID string) (bool, error) {
	var symbol string
	err := withTx(ctx, e.repo, func(tx port.Tx) error {
		o, err := tx.LoadOrderByIDForClient(ctx, orderID, clientID)
		if err != nil {
			return err
		}
		if o.Status != domain.Open {
			return errors.New("cannot cancel non-open order")
		}
		symbol = o.Symbol
		return tx.CancelOrder(ctx, orderID, clientID)
	})
	if err != nil {
		return false, err
	}

	updateCache(ctx, e.repo, e.cache, symbol)
	return true, nil
}

func (e *Engine) GetOrderbook(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error) {
	return getOrLoadSnapshot(ctx, e.repo, e.cache, symbol)
}

func (e *Engine) SnapshotOrderbook(ctx context.Context, symbol string) (string, error) {
	if e.cache == nil {
		return "", errors.New("cache not configured")
	}

	ob, err := e.GetOrderbook(ctx, symbol)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(ob)
	if err != nil {
		return "", err
	}

	snapshotID := uuid.NewString()
	ttl := 24 * time.Hour
	if err := e.cache.SetSnapshot(ctx, snapshotID, data, ttl); err != nil {
		return "", err
	}
	return snapshotID, nil
}

func (e *Engine) RestoreOrderbook(ctx context.Context, snapshotID string) (bool, error) {
	if e.cache == nil {
		return false, errors.New("cache not configured")
	}

	data, err := e.cache.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return false, err
	}
	if data == nil {
		return false, errors.New("snapshot not found")
	}

	var ob domain.OrderbookSnapshot
	if err := json.Unmarshal(data, &ob); err != nil {
		return false, err
	}

	if err := e.cache.SetOrderbook(ctx, ob.Symbol, ob.DeepCopy()); err != nil {
		return false, err
	}

	return true, nil
}

func (e *Engine) GetOrder(ctx context.Context, orderID string) (*domain.Order, error) {
	order, err := e.repo.LoadOrderByIDForClient(ctx, orderID, "")
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (e *Engine) GetTradesForOrder(ctx context.Context, orderID string) ([]*domain.Trade, error) {
	trades, err := e.repo.LoadTradesForOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	return trades, nil
}
