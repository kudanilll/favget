package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/kudanilll/favget/internal/cache"
	"github.com/kudanilll/favget/internal/cloud"
	"github.com/kudanilll/favget/internal/config"
	httpx "github.com/kudanilll/favget/internal/http"
	"github.com/kudanilll/favget/internal/store"
)

func main() {
	_ = godotenv.Load(".env")

	cfg := config.Load()
	_ = os.Setenv("PORT", cfg.Port)

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