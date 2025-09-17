package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/you/favget/internal/cache"
	"github.com/you/favget/internal/cloud"
	"github.com/you/favget/internal/config"
	httpx "github.com/you/favget/internal/http"
	"github.com/you/favget/internal/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil { log.Fatal(err) }

	cch := cache.New(cfg.RedisURL, cfg.CacheTTLSec)

	cld, err := cloud.New(cfg.CloudinaryURL)
	if err != nil { log.Fatal(err) }

	s := &httpx.Server{DB: db, Cache: cch, CLD: cld}
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on :%s ...", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}
