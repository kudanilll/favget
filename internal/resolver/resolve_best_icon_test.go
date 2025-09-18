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
// and hijacks http.DefaultTransport so that any "https://<domain>/..."
// requests made by the resolver are routed to this server.
//
// The returned cleanup MUST be deferred by the caller.
// Rationale:
//   - Each scenario gets a brand-new mux, so we never re-register "/"
//     on the same ServeMux (which would panic).
//   - We isolate state between scenarios (handlers, HTML, etc.).
func startTLSSite(t *testing.T, homeHTML string, extra map[string]http.HandlerFunc) (testDomain string, cleanup func()) {
	t.Helper()

	mux := http.NewServeMux()

	// Home page ("/") — serves minimal HTML/html-like content
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(homeHTML))
	})

	// Extra endpoints (e.g., /fav.png, /favicon.ico, /abs.png) with simple behavior
	for p, h := range extra {
		mux.HandleFunc(p, h)
	}

	ts := httptest.NewTLSServer(mux)

	// Prepare hijack of http.DefaultTransport so that resolver hitting
	// "https://<domain>/..." is routed to this TLS test server.
	orig := http.DefaultTransport
	hostPort := strings.TrimPrefix(ts.URL, "https://") // <host:port>

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // test cert only
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, hostPort)
		},
		ForceAttemptHTTP2: true,
	}
	http.DefaultTransport = tr

	// Expose "domain" as the host:port so the resolver builds https://<domain>/...
	u, _ := url.Parse(ts.URL)

	cleanup = func() {
		http.DefaultTransport = orig
		ts.Close()
	}
	return u.Host, cleanup
}

// TestResolveBestIcon validates three distinct scenarios against the resolver,
// using a fresh test server for each case.
//
// NOTE: This test intentionally does NOT use t.Parallel(), because we hijack
// http.DefaultTransport and do not want concurrent mutation across subtests.
func TestResolveBestIcon(t *testing.T) {
	// --- Case 1: page declares a relative link icon; prefer it over /favicon.ico
	{
		home := `<!doctype html><head><link rel="icon" href="/fav.png"></head>`
		extra := map[string]http.HandlerFunc{
			"/fav.png": func(w http.ResponseWriter, r *http.Request) {
				// Return 200 for HEAD/GET; body only for GET.
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte("PNG"))
				}
			},
			"/favicon.ico": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		}
		domain, cleanup := startTLSSite(t, home, extra)
		defer cleanup()

		src, _, err := resolver.ResolveBestIcon(domain)
		if err != nil {
			t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
		}
		if !strings.HasSuffix(src, "/fav.png") {
			t.Fatalf("got %q, want path to end with /fav.png", src)
		}
	}

	// --- Case 2: page declares an absolute icon URL; carry it through
	{
		// We need the test server URL to craft the absolute href.
		// Spin a temporary server to learn its URL, then rebuild with the absolute href.
		temp := httptest.NewTLSServer(http.NewServeMux())
		temp.Close()
		absBase := temp.URL // e.g., https://127.0.0.1:12345

		home := `<!doctype html><head><link rel="icon" href="` + absBase + `/abs.png"></head>`
		extra := map[string]http.HandlerFunc{
			"/abs.png": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		}
		domain, cleanup := startTLSSite(t, home, extra)
		defer cleanup()

		src, _, err := resolver.ResolveBestIcon(domain)
		if err != nil {
			t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
		}
		if !strings.HasSuffix(src, "/abs.png") {
			t.Fatalf("got %q, want absolute /abs.png", src)
		}
	}

	// --- Case 3: no link icons; fall back to /favicon.ico if it exists
	{
		home := `<!doctype html><head></head>`
		extra := map[string]http.HandlerFunc{
			"/favicon.ico": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		}
		domain, cleanup := startTLSSite(t, home, extra)
		defer cleanup()

		src, _, err := resolver.ResolveBestIcon(domain)
		if err != nil {
			t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
		}
		if !strings.HasSuffix(src, "/favicon.ico") {
			t.Fatalf("got %q, want fallback /favicon.ico", src)
		}
	}
}
