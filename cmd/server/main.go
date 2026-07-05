package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/kudanilll/favget/internal/config"
	"github.com/kudanilll/favget/pkg/app"
)

func main() {
	// Load .env for local/dev runs. It is safe if the file does not exist.
	_ = godotenv.Load(".env")

	// Build the full HTTP handler tree (DB, Redis, Cloudinary, router).
	h, cleanup, err := app.NewHandler()
	if err != nil {
		log.Fatalf("init error: %v", err)
	}
	// We handle cleanup explicitly during graceful shutdown.
	// But in case of an early fatal error, this ensures it's run.

	// Load config after handler (so app.NewHandler() can validate env too).
	cfg := config.Load()

	// Configure a production-friendly HTTP server.
	// Timeouts help protect against slowloris and misbehaving clients.
	srv := &http.Server{
		Addr:              ":" + cfg.Port, // example: ":8080"
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,  // time to read request headers
		ReadTimeout:       15 * time.Second, // total time to read request
		WriteTimeout:      15 * time.Second, // total time to write response
		IdleTimeout:       60 * time.Second, // keep-alive connections
	}

	// Run server in background so we can handle OS signals.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on :%s ...", cfg.Port)
		// http.ErrServerClosed is returned on graceful shutdown; treat it as normal.
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Create a cancellable context bound to SIGINT (Ctrl+C) and SIGTERM (Docker/k8s).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		// Received interrupt/term: attempt graceful shutdown with timeout.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		log.Println("shutting down gracefully...")
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// If graceful shutdown fails, force close.
			log.Printf("graceful shutdown failed: %v; forcing close", err)
			_ = srv.Close()
		}
		
		// Wait for singleflight routines to complete if possible, but cleanup resources at the end
		log.Println("cleaning up resources...")
		cleanup()
		log.Println("server stopped")

	case err := <-errCh:
		// Fatal error from ListenAndServe (port in use, etc.).
		log.Fatalf("server error: %v", err)
	}
}
