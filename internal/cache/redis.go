package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache wraps a Redis client with a configurable TTL.
// If rdb is nil, all operations are no-ops so Redis is fully optional.
type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

// New creates a Cache backed by Redis. Returns a no-op Cache (rdb=nil)
// if redisURL is empty, so callers do not need to check for nil.
func New(redisURL string, ttlSec int) *Cache {
	if redisURL == "" {
		return &Cache{ttl: time.Duration(ttlSec) * time.Second}
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		panic(err)
	}
	return &Cache{
		rdb: redis.NewClient(opt),
		ttl: time.Duration(ttlSec) * time.Second,
	}
}

// Get retrieves a value by key. Returns ("", redis.Nil) when the cache is
// disabled or the key is not found.
func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	if c.rdb == nil {
		return "", redis.Nil
	}
	return c.rdb.Get(ctx, key).Result()
}

// Set stores a value by key with the configured TTL. It is a no-op when
// the cache is disabled.
func (c *Cache) Set(ctx context.Context, key, val string) error {
	if c.rdb == nil {
		return nil
	}
	return c.rdb.Set(ctx, key, val, c.ttl).Err()
}
