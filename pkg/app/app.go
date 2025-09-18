package app

import (
	"context"
	"net/http"
	"os"

	"github.com/kudanilll/favget/internal/cache"
	"github.com/kudanilll/favget/internal/cloud"
	"github.com/kudanilll/favget/internal/config"
	httpx "github.com/kudanilll/favget/internal/http"
	"github.com/kudanilll/favget/internal/store"
)

func NewHandler() (http.Handler, error) {
	cfg := config.Load()
	_ = os.Setenv("PORT", cfg.Port) // if anyone reads PORT downstream

	ctx := context.Background()
	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	cch := cache.New(cfg.RedisURL, cfg.CacheTTLSec)

	cld, err := cloud.New(cfg.CloudinaryURL) // exp: CLOUDINARY_URL=cloudinary://KEY:SECRET@cloud
	if err != nil {
		return nil, err
	}

	s := &httpx.Server{DB: db, Cache: cch, CLD: cld}
	return s.Routes(), nil
}
