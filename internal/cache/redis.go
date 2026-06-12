package cache

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	RDB *redis.Client
	ttl time.Duration
}

func New(redisURL string, ttlSec int) *Cache {
	if redisURL == "" {
		return &Cache{ttl: time.Duration(ttlSec) * time.Second}
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("Warning: Redis connection failed: %v", err)
		return &Cache{ttl: time.Duration(ttlSec) * time.Second}
	}
	return &Cache{
		RDB: redis.NewClient(opt),
		ttl: time.Duration(ttlSec) * time.Second,
	}
}

func (c *Cache) GetRedisClient() *redis.Client {
	return c.RDB
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	if c.RDB == nil {
		return "", redis.Nil
	}
	return c.RDB.Get(ctx, key).Result()
}

func (c *Cache) Set(ctx context.Context, key, val string) error {
	if c.RDB == nil {
		return nil
	}
	return c.RDB.Set(ctx, key, val, c.ttl).Err()
}

// SetWithTTL stores val with an explicit TTL. Useful for negative caching.
func (c *Cache) SetWithTTL(ctx context.Context, key, val string, ttl time.Duration) error {
	if c.RDB == nil {
		return nil
	}
	return c.RDB.Set(ctx, key, val, ttl).Err()
}

// Close closes the underlying Redis client if configured.
func (c *Cache) Close() error {
	if c.RDB == nil {
		return nil
	}
	return c.RDB.Close()
}
