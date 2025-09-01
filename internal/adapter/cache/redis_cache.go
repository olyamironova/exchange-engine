package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(addr string, password string, db int, ttl time.Duration) *RedisCache {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &RedisCache{
		client: rdb,
		ttl:    ttl,
	}
}

func key(symbol string) string { return "ob:" + symbol }
func (c *RedisCache) SetOrderbook(ctx context.Context, symbol string, ob *domain.OrderbookSnapshot) error {
	b, err := json.Marshal(ob)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key(symbol), b, c.ttl).Err()
}

func (c *RedisCache) GetOrderbook(ctx context.Context, symbol string) (*domain.OrderbookSnapshot, error) {
	b, err := c.client.Get(ctx, key(symbol)).Bytes()
	if err != nil {
		return nil, err
	}
	var ob domain.OrderbookSnapshot
	if err := json.Unmarshal(b, &ob); err != nil {
		return nil, err
	}
	return &ob, nil
}

func (c *RedisCache) Invalidate(ctx context.Context, symbol string) error {
	return c.client.Del(ctx, key(symbol)).Err()
}

func (r *RedisCache) SetSnapshot(ctx context.Context, snapshotID string, data []byte, ttl time.Duration) error {
	return r.client.Set(ctx, "snapshot:"+snapshotID, data, ttl).Err()
}

func (r *RedisCache) GetSnapshot(ctx context.Context, snapshotID string) ([]byte, error) {
	res, err := r.client.Get(ctx, "snapshot:"+snapshotID).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return res, err
}
