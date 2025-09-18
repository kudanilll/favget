package main

import (
	"log"
	"net/http"
	"time"

	"github.com/joho/godotenv"
	"github.com/kudanilll/favget/internal/config"
	"github.com/kudanilll/favget/pkg/app"
)

func main() {
	_ = godotenv.Load(".env")

	h, err := app.NewHandler()
	if err != nil { log.Fatal(err) }

	cfg := config.Load()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on :%s ...", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}
