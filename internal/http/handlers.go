package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/singleflight"

	"github.com/kudanilll/favget/internal/cache"
	"github.com/kudanilll/favget/internal/cloud"
	"github.com/kudanilll/favget/internal/resolver"
	"github.com/kudanilll/favget/internal/store"
)

// Server aggregates all dependencies required by HTTP handlers.
// Keep it small and explicit so it's easy to test and reason about.
type Server struct {
	DB                  *store.DB
	Cache               *cache.Cache
	CLD                 *cloud.Cloud
	Resolver            *resolver.Resolver
	APIKeys             []string // API keys enforced by middleware; empty means "no auth"
	AllowedOrigins      []string // allowed CORS origins
	NegativeCacheTTLSec int      // TTL in seconds for negative cache entries
	RateLimitRPS        int      // rate limit requests per second per IP; 0 = no limit

	singleflight singleflight.Group
}

// CORS middleware for handling cross-origin requests
func (s *Server) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if s.isOriginAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-API-Key, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) isOriginAllowed(origin string) bool {
	if len(s.AllowedOrigins) == 0 {
		return false
	}
	for _, allowed := range s.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

// Routes builds the chi router. Public and secured routes are grouped explicitly
// to make the security posture obvious to readers and reviewers.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	// Root route: human & machine-friendly service index
	r.Get("/", s.handleRoot)

	// --- Public endpoints (no API key required) ---
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		s.setSecurityHeaders(w)
		w.WriteHeader(http.StatusOK)
	})

	// Apply CORS middleware to all routes
	r.With(s.CORS).Group(func(cr chi.Router) {

		// --- Secured endpoints (API key required if configured) ---
		cr.Group(func(sr chi.Router) {
			// Apply API-key middleware. If no keys were configured, this is a no-op.
			sr.Use(APIKeyAuth(s.APIKeys))

			// Apply rate limiting if Redis is configured and RPS > 0.
			if s.RateLimitRPS > 0 {
				sr.Use(RateLimitMiddleware(s.Cache, s.RateLimitRPS))
			}

			// Main icon endpoint
			sr.Get("/v1/icon", s.handleIcon)
		})
	})

	return r
}

// handleRoot serves a compact index of the service: author/contact,
// available routes, and auth requirements. JSON is the default response.
// Pass `?format=html` (or Accept: text/html) for a tiny HTML page.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	s.setSecurityHeaders(w)
	authorName := "Achmad Daniel Syahputra"
	authorURL := "https://www.kudaniel.my.id"
	repoURL := "https://github.com/kudanilll/favget"
	env := "production"

	type route struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		Auth        string `json:"auth"`
		Description string `json:"description"`
		Example     string `json:"example,omitempty"`
	}
	payload := struct {
		Service string  `json:"service"`
		Env     string  `json:"env"`
		Author  string  `json:"author"`
		Contact string  `json:"contact"`
		Repo    string  `json:"repo"`
		Routes  []route `json:"routes"`
	}{
		Service: "Favget",
		Env:     env,
		Author:  authorName,
		Contact: authorURL,
		Repo:    repoURL,
		Routes: []route{
			{
				Method:      "GET",
				Path:        "/healthz",
				Auth:        "none",
				Description: "Health probe",
			},
			{
				Method:      "GET",
				Path:        "/v1/icon",
				Auth:        "required (API key)",
				Description: "Resolve best icon for a domain and redirect to optimized Cloudinary URL",
				Example:     `curl -i "https://<host>/v1/icon?domain=github.com" -H "Authorization: Bearer <API_KEY>"`,
			},
		},
	}

	// Switch to a minimal HTML view if requested by Accept or query param.
	if strings.Contains(r.Header.Get("Accept"), "text/html") || r.URL.Query().Get("format") == "html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.setSecurityHeaders(w)
		const tpl = `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Favget · Service Index</title>
<style>
body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Ubuntu,"Helvetica Neue",Arial,sans-serif;margin:2rem;line-height:1.45}
code{background:#f3f4f6;padding:.15rem .35rem;border-radius:.35rem}
h1{margin:.2rem 0 1rem}
small{color:#6b7280}
.section{margin:1.2rem 0}
.route{margin:.5rem 0;padding:.6rem .8rem;border:1px solid #e5e7eb;border-radius:.5rem}
.k{color:#374151}
.v{color:#111827;font-weight:600}
</style>
</head><body>
<h1>Favget <small>· {{.Env}}</small></h1>
<div class="section">
  <div class="k">Author:</div>
  <div class="v"><a href="{{.Contact}}" target="_blank" rel="noreferrer">{{.Author}}</a></div>
  <div class="k" style="margin-top:.4rem">Repository:</div>
  <div class="v"><a href="{{.Repo}}" target="_blank" rel="noreferrer">{{.Repo}}</a></div>
</div>

<div class="section">
  <div class="k">Routes:</div>
  {{range .Routes}}
    <div class="route">
      <div><b>{{.Method}}</b> <code>{{.Path}}</code></div>
      <div><small>Auth: {{.Auth}}</small></div>
      {{if .Description}}<div style="margin-top:.25rem">{{.Description}}</div>{{end}}
      {{if .Example}}<div style="margin-top:.4rem"><code>{{.Example}}</code></div>{{end}}
    </div>
  {{end}}
</div>
</body></html>`
		t := template.Must(template.New("index").Parse(tpl))
		_ = t.Execute(w, payload)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
}

// handleIcon resolves the best icon for the given domain, uploads (remote fetch)
// to Cloudinary, persists metadata in Postgres, caches the result in Redis,
// and finally issues a 302 redirect to the Cloudinary URL.
//
// Cache strategy:
//   - Redis GET first (hot path).
//   - DB lookup second (warm path) with backfill into Redis.
//   - Resolve + Upload + Upsert + Cache on miss (cold path).
//   - Negative cache for misses to avoid repeated upstream lookups.
func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	s.setSecurityHeaders(w)
	domain := r.URL.Query().Get("domain")
	var err error
	if domain, err = resolver.NormalizeDomain(domain); err != nil {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1) Redis (hot path) — positive cache
	if u, err := s.Cache.Get(ctx, "icon:"+domain); err == nil && u != "" {
		w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		http.Redirect(w, r, u, http.StatusFound)
		return
	}

	// 1b) Redis — negative cache (icon was previously not found)
	if u, err := s.Cache.Get(ctx, "icon-miss:"+domain); err == nil && u == "1" {
		http.Error(w, "icon not found", http.StatusNotFound)
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
	// Use singleflight to prevent duplicate concurrent resolves for the same domain.
	iconURL, err := s.resolveAndUpload(domain, ctx)
	if err != nil {
		// Cache the miss to avoid repeated upstream lookups.
		if s.Cache != nil {
			negTTL := time.Duration(s.NegativeCacheTTLSec) * time.Second
			if negTTL <= 0 {
				negTTL = 5 * time.Minute
			}
			_ = s.Cache.SetWithTTL(ctx, "icon-miss:"+domain, "1", negTTL)
		}
		http.Error(w, "icon not found", http.StatusNotFound)
		return
	}

	// Final response
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	http.Redirect(w, r, iconURL, http.StatusFound)
}

// resolveAndUpload deduplicates concurrent requests for the same domain using singleflight,
// then resolves the icon, uploads to Cloudinary, persists metadata, and caches the result.
func (s *Server) resolveAndUpload(domain string, ctx context.Context) (string, error) {
	v, err, _ := s.singleflight.Do("icon:"+domain, func() (interface{}, error) {
		src, meta, err := s.Resolver.ResolveBestIcon(ctx, domain)
		if err != nil {
			return nil, err
		}

		cldURL, err := s.CLD.UploadRemote(ctx, domain, src)
		if err != nil {
			log.Printf("upload failed for %s: %v", domain, err)
			return nil, errors.New("upload failed")
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

		return cldURL, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}
