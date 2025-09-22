package httpx

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kudanilll/favget/internal/cache"
	"github.com/kudanilll/favget/internal/cloud"
	"github.com/kudanilll/favget/internal/resolver"
	"github.com/kudanilll/favget/internal/store"
)

// Server aggregates all dependencies required by HTTP handlers.
// Keep it small and explicit so it's easy to test and reason about.
type Server struct {
	DB      *store.DB
	Cache   *cache.Cache
	CLD     *cloud.Cloud
	APIKeys []string // API keys enforced by middleware; empty means "no auth"
}

// Routes builds the chi router. Public and secured routes are grouped explicitly
// to make the security posture obvious to readers and reviewers.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	// --- Public endpoints (no API key required) ---
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// --- Secured endpoints (API key required if configured) ---
	r.Group(func(sr chi.Router) {
		// Apply API-key middleware. If no keys were configured, this is a no-op.
		sr.Use(APIKeyAuth(s.APIKeys))

		// Main icon endpoint
		sr.Get("/v1/icon", s.handleIcon)

		// Add other protected endpoints here, e.g.:
		// sr.Post("/v1/refresh", s.handleRefresh)
	})

	return r
}

// handleIcon resolves the best icon for the given domain, uploads (remote fetch)
// to Cloudinary, persists metadata in Postgres, caches the result in Redis,
// and finally issues a 302 redirect to the Cloudinary URL.
//
// Cache strategy:
//   - Redis GET first (fast path).
//   - DB lookup second (warm path) with backfill into Redis.
//   - Resolve + Upload + Upsert + Cache on miss (cold path).
func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	var err error
	if domain, err = resolver.NormalizeDomain(domain); err != nil {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1) Redis (hot path)
	if u, err := s.Cache.Get(ctx, "icon:"+domain); err == nil && u != "" {
		w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		http.Redirect(w, r, u, http.StatusFound)
		return
	}

	// 2) DB (warm path)
	if rec, err := s.DB.FindByDomain(ctx, domain); err == nil && rec.IconURL != "" {
		_ = s.Cache.Set(ctx, "icon:"+domain, rec.IconURL)
		w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		http.Redirect(w, r, rec.IconURL, http.StatusFound)
		return
	}

	// 3) Resolve → Upload → Upsert → Cache (cold path)
	src, meta, err := resolver.ResolveBestIcon(domain)
	if err != nil {
		http.Error(w, "icon not found", http.StatusNotFound)
		return
	}

	cldURL, err := s.CLD.UploadRemote(ctx, domain, src)
	if err != nil {
		http.Error(w, "upload failed", http.StatusBadGateway)
		return
	}

	// Persist metadata (best-effort; the redirect should not depend on these writes)
	_ = s.DB.Upsert(ctx, store.IconRecord{
		Domain:      domain,
		IconURL:     cldURL,
		SourceURL:   meta.SourceURL,
		ETag:        meta.ETag,
		ContentType: meta.ContentType,
	})

	// Backfill cache
	_ = s.Cache.Set(ctx, "icon:"+domain, cldURL)

	// Final response
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	http.Redirect(w, r, cldURL, http.StatusFound)
}
