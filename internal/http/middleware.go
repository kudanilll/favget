package httpx

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kudanilll/favget/internal/cache"
)

type rateLimitStore struct {
	rdb   *redis.Client
	ttl   time.Duration
	rps   int
	cache *cache.Cache
}

func (s *rateLimitStore) isRateLimited(ctx context.Context, key string) (bool, error) {
	if s.rdb == nil {
		return false, nil
	}
	count, err := s.rdb.Incr(ctx, "rl:"+key).Result()
	if err != nil {
		log.Printf("Rate limit Redis error: %v", err)
		return false, err
	}
	if count == 1 {
		s.rdb.Expire(ctx, "rl:"+key, s.ttl)
	}
	if count > int64(s.rps) {
		return true, nil
	}
	return false, nil
}

func (s *rateLimitStore) getRetryAfter(ctx context.Context, key string) int {
	if s.rdb == nil {
		return 0
	}
	ttl, err := s.rdb.TTL(ctx, "rl:"+key).Result()
	if err != nil {
		return 0
	}
	return int(ttl.Seconds()) + 1
}

func APIKeyAuth(keys []string) func(next http.Handler) http.Handler {
	var bkeys [][]byte
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k != "" {
			bkeys = append(bkeys, []byte(k))
		}
	}

	if len(bkeys) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var provided string
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				provided = strings.TrimSpace(auth[len("Bearer "):])
			}

			if provided == "" {
				provided = strings.TrimSpace(r.Header.Get("X-API-Key"))
			}

			if provided == "" {
				provided = strings.TrimSpace(r.URL.Query().Get("api_key"))
				if provided == "" {
					provided = strings.TrimSpace(r.URL.Query().Get("apikey"))
				}
			}

			if provided == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="favget"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ok := false
			pb := []byte(provided)
			for _, kb := range bkeys {
				if subtle.ConstantTimeCompare(pb, kb) == 1 {
					ok = true
					break
				}
			}
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RateLimitMiddleware(c *cache.Cache, rps int) func(next http.Handler) http.Handler {
	rdb := getRedisClient(c)
	ttl := time.Minute

	if rps <= 0 {
		rps = 10
	}

	store := &rateLimitStore{
		rdb:   rdb,
		ttl:   ttl,
		rps:   rps,
		cache: c,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			clientIP := getClientIP(r)
			apiKey := getAPIKeyFromRequest(r)

			var rateLimitKey string
			if apiKey != "" {
				rateLimitKey = clientIP + ":" + apiKey
			} else {
				rateLimitKey = clientIP
			}

			isLimited, err := store.isRateLimited(ctx, rateLimitKey)
			if err != nil {
				log.Printf("Rate limit check error: %v", err)
			}

			if isLimited {
				retryAfter := store.getRetryAfter(ctx, rateLimitKey)
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getRedisClient(c *cache.Cache) *redis.Client {
	if c == nil {
		return nil
	}
	return c.GetRedisClient()
}

func getClientIP(r *http.Request) string {
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP"} {
		val := r.Header.Get(h)
		if val != "" {
			parts := strings.Split(val, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
	}
	if addr := r.RemoteAddr; addr != "" {
		return addr
	}
	return "127.0.0.1"
}

func getAPIKeyFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):])
	}

	key := r.Header.Get("X-API-Key")
	if key != "" {
		return strings.TrimSpace(key)
	}

	key = r.URL.Query().Get("api_key")
	if key == "" {
		key = r.URL.Query().Get("apikey")
	}
	return strings.TrimSpace(key)
}
