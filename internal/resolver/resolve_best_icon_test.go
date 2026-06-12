package resolver_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kudanilll/favget/internal/resolver"
)

// startTLSSite spins up a fresh HTTPS test server for each scenario
// and returns a custom HTTP client that routes all requests to this server.
//
// The returned cleanup MUST be deferred by the caller.
func startTLSSite(t *testing.T, homeHTML string, extra map[string]http.HandlerFunc) (testDomain string, client *http.Client, cleanup func()) {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(homeHTML))
	})

	for p, h := range extra {
		mux.HandleFunc(p, h)
	}

	ts := httptest.NewTLSServer(mux)

	hostPort := strings.TrimPrefix(ts.URL, "https://")

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test cert only
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, hostPort)
		},
		ForceAttemptHTTP2: true,
	}

	client = &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	u, _ := url.Parse(ts.URL)

	cleanup = func() {
		ts.Close()
	}
	return u.Host, client, cleanup
}

// TestResolveBestIcon validates three distinct scenarios against the resolver,
// using a fresh test server for each case.
//
// NOTE: This test intentionally does NOT use t.Parallel(), because each subtest
// creates a resolver with a client that dials a specific test server.
func TestResolveBestIcon(t *testing.T) {
	// --- Case 1: page declares a relative link icon; prefer it over /favicon.ico
	{
		home := `<!doctype html><head><link rel="icon" href="/fav.png"></head>`
		extra := map[string]http.HandlerFunc{
			"/fav.png": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte("PNG"))
				}
			},
			"/favicon.ico": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		}
		domain, client, cleanup := startTLSSite(t, home, extra)
		defer cleanup()

		r := resolver.New(true, 1<<20, true)
		r.SetClient(client)

		src, _, err := r.ResolveBestIcon(domain)
		if err != nil {
			t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
		}
		if !strings.HasSuffix(src, "/fav.png") {
			t.Fatalf("got %q, want path to end with /fav.png", src)
		}
	}

	// --- Case 2: no link icons; fall back to /favicon.ico if it exists
	{
		home := `<!doctype html><head></head>`
		extra := map[string]http.HandlerFunc{
			"/favicon.ico": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte("ICO"))
				}
			},
		}
		domain, client, cleanup := startTLSSite(t, home, extra)
		defer cleanup()

		r := resolver.New(true, 1<<20, true)
		r.SetClient(client)

		src, _, err := r.ResolveBestIcon(domain)
		if err != nil {
			t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
		}
		if !strings.HasSuffix(src, "/favicon.ico") {
			t.Fatalf("got %q, want fallback /favicon.ico", src)
		}
	}

	// --- Case 3: HEAD fails with 405, but GET works (fallback) ---
	{
		home := `<!doctype html><head><link rel="icon" href="/headless.ico"></head>`
		extra := map[string]http.HandlerFunc{
			"/headless.ico": func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "image/x-icon")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ICO"))
			},
		}
		domain, client, cleanup := startTLSSite(t, home, extra)
		defer cleanup()

		r := resolver.New(true, 1<<20, true)
		r.SetClient(client)

		src, _, err := r.ResolveBestIcon(domain)
		if err != nil {
			t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
		}
		if !strings.HasSuffix(src, "/headless.ico") {
			t.Fatalf("got %q, want /headless.ico via GET fallback", src)
		}
	}

	// --- Case 4: icon with wrong content type is rejected ---
	{
		home := `<!doctype html><head><link rel="icon" href="/bad.ico"></head>`
		extra := map[string]http.HandlerFunc{
			"/bad.ico": func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
			},
			"/favicon.ico": func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/x-icon")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ICO"))
			},
		}
		domain, client, cleanup := startTLSSite(t, home, extra)
		defer cleanup()

		r := resolver.New(true, 1<<20, true)
		r.SetClient(client)

		src, _, err := r.ResolveBestIcon(domain)
		if err != nil {
			t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
		}
		if !strings.HasSuffix(src, "/favicon.ico") {
			t.Fatalf("got %q, want fallback /favicon.ico after content type rejection", src)
		}
	}
}
