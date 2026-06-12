package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/kudanilll/favget/internal/cache"
	"github.com/kudanilll/favget/internal/cloud"
	"github.com/kudanilll/favget/internal/config"
	httpx "github.com/kudanilll/favget/internal/http"
	"github.com/kudanilll/favget/internal/resolver"
	"github.com/kudanilll/favget/internal/store"
)

// NewHandler builds the full HTTP handler tree and returns a cleanup function
// that should be deferred by the caller to close DB/Redis connections.
func NewHandler() (http.Handler, func(), error) {
	cfg := config.Load()
	_ = os.Setenv("PORT", cfg.Port) // if anyone reads PORT downstream

	ctx := context.Background()
	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, func() {}, err
	}

	cch := cache.New(cfg.RedisURL, cfg.CacheTTLSec)

	cld, err := cloud.New(cfg.CloudinaryURL)
	if err != nil {
		db.Close()
		return nil, func() {}, err
	}

	res := resolver.New(cfg.AllowInsecureTLS, cfg.MaxHTMLBytes, false)

	s := &httpx.Server{
		DB:                  db,
		Cache:               cch,
		CLD:                 cld,
		Resolver:            res,
		APIKeys:             cfg.APIKeys,
		AllowedOrigins:      parseAllowedOrigins(cfg.AllowedOrigins),
		NegativeCacheTTLSec: cfg.NegativeCacheTTLSec,
		RateLimitRPS:        cfg.RateLimitRPS,
	}

	cleanup := func() {
		db.Close()
		if err := cch.Close(); err != nil {
			log.Printf("warning: Redis close: %v", err)
		}
	}

	return s.Routes(), cleanup, nil
}

func parseAllowedOrigins(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if origin := strings.TrimSpace(p); origin != "" {
			out = append(out, origin)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
