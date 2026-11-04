package resolver_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kudanilll/favget/internal/resolver"
)

// icoBytes starts with the ICO signature, so http.DetectContentType reports image/x-icon.
var icoBytes = []byte("\x00\x00\x01\x00\x01\x00\x10\x10\x00\x00\x01\x00\x20\x00")

func forbidden(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }

// serve answers with contentType and body (body omitted for HEAD).
func serve(status int, contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	}
}

func newResolver(client *http.Client) *resolver.Resolver {
	r := resolver.New(true, 1<<20, true)
	r.SetClient(client)
	return r
}

func TestUpstreamFallback(t *testing.T) {
	tests := []struct {
		name     string
		upstream http.HandlerFunc
		enabled  bool
		wantErr  bool
	}{
		{"used when the site blocks every probe", serve(http.StatusOK, "image/png", []byte("PNG")), true, false},
		{"404 means the service has no icon", serve(http.StatusNotFound, "image/png", []byte("PNG")), true, true},
		{"non-image answers are rejected", serve(http.StatusOK, "text/html", []byte("<html>")), true, true},
		{"empty images are rejected", serve(http.StatusOK, "image/png", nil), true, true},
		{"never called when disabled", serve(http.StatusOK, "image/png", []byte("PNG")), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			extra := map[string]http.HandlerFunc{
				"/":            forbidden,
				"/favicon.ico": forbidden,
				"/s2": func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					tt.upstream(w, r)
				},
			}
			domain, client, cleanup := startTLSSite(t, "", extra)
			defer cleanup()

			r := newResolver(client)
			upstream := "https://" + domain + "/s2?sz=128&domain="
			if tt.enabled {
				r.UpstreamURL = upstream
			}

			src, _, err := r.ResolveBestIcon(context.Background(), domain)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("got icon %q, want no icon", src)
				}
			} else {
				if err != nil {
					t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
				}
				if want := upstream + url.QueryEscape(domain); src != want {
					t.Fatalf("got %q, want %q", src, want)
				}
			}
			if !tt.enabled && calls.Load() != 0 {
				t.Fatalf("upstream called %d times while disabled", calls.Load())
			}
		})
	}
}

func TestOctetStreamIcons(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    []byte
		wantErr bool
	}{
		{"ico bytes behind octet-stream are accepted", "/site.ico", icoBytes, false},
		{"html behind octet-stream is rejected", "/site.ico", []byte("<html>login</html>"), true},
		{"octet-stream without an icon extension is rejected", "/icon", icoBytes, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := `<!doctype html><head><link rel="icon" href="` + tt.path + `"></head>`
			extra := map[string]http.HandlerFunc{
				tt.path:        serve(http.StatusOK, "application/octet-stream", tt.body),
				"/favicon.ico": forbidden,
			}
			domain, client, cleanup := startTLSSite(t, home, extra)
			defer cleanup()

			src, meta, err := newResolver(client).ResolveBestIcon(context.Background(), domain)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("got icon %q, want no icon", src)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
			}
			if !strings.HasSuffix(src, tt.path) {
				t.Fatalf("got %q, want path %s", src, tt.path)
			}
			if meta.ContentType == nil || *meta.ContentType != "image/x-icon" {
				t.Fatalf("got content type %v, want the sniffed image/x-icon", meta.ContentType)
			}
		})
	}
}

func TestRequestHeaders(t *testing.T) {
	// Channels, not shared variables, so the race detector sees the handoff.
	docHeaders, iconHeaders := make(chan http.Header, 4), make(chan http.Header, 4)
	home := `<!doctype html><head><link rel="icon" href="/fav.png"></head>`
	extra := map[string]http.HandlerFunc{
		"/": func(w http.ResponseWriter, r *http.Request) {
			docHeaders <- r.Header.Clone()
			_, _ = w.Write([]byte(home))
		},
		"/fav.png": func(w http.ResponseWriter, r *http.Request) {
			iconHeaders <- r.Header.Clone()
			serve(http.StatusOK, "image/png", []byte("PNG"))(w, r)
		},
	}
	domain, client, cleanup := startTLSSite(t, home, extra)
	defer cleanup()

	r := newResolver(client)
	r.UserAgent = "FavgetTest/1.0"
	if _, _, err := r.ResolveBestIcon(context.Background(), domain); err != nil {
		t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
	}

	doc, icon := <-docHeaders, <-iconHeaders
	for _, h := range []http.Header{doc, icon} {
		if got := h.Get("User-Agent"); got != "FavgetTest/1.0" {
			t.Errorf("User-Agent = %q, want the configured one", got)
		}
	}
	if got := doc.Get("Sec-Fetch-Dest"); got != "document" {
		t.Errorf("page Sec-Fetch-Dest = %q, want document", got)
	}
	if got := icon.Get("Sec-Fetch-Dest"); got != "image" {
		t.Errorf("icon Sec-Fetch-Dest = %q, want image", got)
	}
	if got := icon.Get("Accept"); !strings.HasPrefix(got, "image/") {
		t.Errorf("icon Accept = %q, want image types first", got)
	}
}

func TestIconCandidateOrder(t *testing.T) {
	png := serve(http.StatusOK, "image/png", []byte("PNG"))
	tests := []struct {
		name string
		head string
		want string
	}{
		{
			"mask icons come after regular icons",
			`<link rel="mask-icon" href="/mask.svg"><link rel="apple-touch-icon-precomposed" href="/touch.png">`,
			"/touch.png",
		},
		{
			"shortcut icon is found",
			`<link rel="shortcut icon" href="/shortcut.png">`,
			"/shortcut.png",
		},
		{
			"tile image before the social image",
			`<meta property="og:image" content="/og.png"><meta name="msapplication-TileImage" content="/tile.png">`,
			"/tile.png",
		},
		{
			"social image as the last resort",
			`<meta property="og:image" content="/og.png">`,
			"/og.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extra := map[string]http.HandlerFunc{
				"/mask.svg":     serve(http.StatusOK, "image/svg+xml", []byte("<svg/>")),
				"/touch.png":    png,
				"/shortcut.png": png,
				"/tile.png":     png,
				"/og.png":       png,
				"/favicon.ico":  forbidden,
			}
			domain, client, cleanup := startTLSSite(t, "<!doctype html><head>"+tt.head+"</head>", extra)
			defer cleanup()

			src, _, err := newResolver(client).ResolveBestIcon(context.Background(), domain)
			if err != nil {
				t.Fatalf("ResolveBestIcon(%q) error: %v", domain, err)
			}
			if !strings.HasSuffix(src, tt.want) {
				t.Fatalf("got %q, want path %s", src, tt.want)
			}
		})
	}
}
