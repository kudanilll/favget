package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(redisURL string, ttlSec int) *Cache {
	opt, err := redis.ParseURL(redisURL)
	if err != nil { panic(err) }
	return &Cache{
		rdb: redis.NewClient(opt),
		ttl: time.Duration(ttlSec) * time.Second,
	}
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

func (c *Cache) Set(ctx context.Context, key, val string) error {
	return c.rdb.Set(ctx, key, val, c.ttl).Err()
}
