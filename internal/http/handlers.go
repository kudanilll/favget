package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/you/favget/internal/cache"
	"github.com/you/favget/internal/cloud"
	"github.com/you/favget/internal/resolver"
	"github.com/you/favget/internal/store"
)

type Server struct {
	DB    *store.DB
	Cache *cache.Cache
	CLD   *cloud.Cloud
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/v1/icon", s.handleIcon)

	return r
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	var err error
	if domain, err = resolver.NormalizeDomain(domain); err != nil {
		http.Error(w, "invalid domain", http.StatusBadRequest); return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1) Redis
	if u, err := s.Cache.Get(ctx, "icon:"+domain); err == nil && u != "" {
		http.Redirect(w, r, u, http.StatusFound)
		return
	}

	// 2) DB
	if rec, err := s.DB.FindByDomain(ctx, domain); err == nil && rec.IconURL != "" {
		_ = s.Cache.Set(ctx, "icon:"+domain, rec.IconURL)
		http.Redirect(w, r, rec.IconURL, http.StatusFound)
		return
	}

	// 3) Resolve → Upload → Upsert
	src, meta, err := resolver.ResolveBestIcon(domain)
	if err != nil {
		http.Error(w, "icon not found", http.StatusNotFound); return
	}
	cldURL, err := s.CLD.UploadRemote(ctx, domain, src)
	if err != nil {
		http.Error(w, "upload failed", http.StatusBadGateway); return
	}
	_ = s.DB.Upsert(ctx, store.IconRecord{
		Domain:      domain,
		IconURL:     cldURL,
		SourceURL:   meta.SourceURL,
		ETag:        meta.ETag,
		ContentType: meta.ContentType,
	})
	_ = s.Cache.Set(ctx, "icon:"+domain, cldURL)

	// **Kamu bisa juga return JSON; di sini aku pilih redirect cepat untuk <img>**
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	http.Redirect(w, r, cldURL, http.StatusFound)
}

type JSON map[string]any

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
