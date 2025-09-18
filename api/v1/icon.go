package handler

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

func normalizeDomain(d string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(d))
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "www.")
	if s == "" || strings.ContainsAny(s, "/?#") {
		return "", errors.New("invalid domain")
	}
	return s, nil
}

var linkRe = regexp.MustCompile(`(?i)<link[^>]+rel=["'][^"']*icon[^"']*["'][^>]*>`)
var hrefRe  = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)

func resolveIcon(domain string) (string, error) {
	base := "https://" + domain

	// coba baca sebagian kecil HTML (hemat)
	if resp, err := http.Get(base); err == nil && resp.StatusCode < 400 {
		buf := make([]byte, 128*1024)
		n, _ := resp.Body.Read(buf)
		_ = resp.Body.Close()
		html := string(buf[:n])

		tags := linkRe.FindAllString(html, -1)
		for _, tag := range tags {
			if m := hrefRe.FindStringSubmatch(tag); len(m) > 1 {
				abs := m[1]
				if u, err := url.Parse(abs); err == nil {
					abs = (&url.URL{Scheme: "https", Host: domain}).ResolveReference(u).String()
				} else {
					abs = base + "/favicon.ico"
				}
				if h, err := http.Head(abs); err == nil && h.StatusCode < 400 {
					return abs, nil
				}
			}
		}
	}
	// fallback
	f := base + "/favicon.ico"
	if h, err := http.Head(f); err == nil && h.StatusCode < 400 {
		return f, nil
	}
	return "", errors.New("icon not found")
}

// Handler is required by Vercel's Go runtime
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	d := r.URL.Query().Get("domain")
	dom, err := normalizeDomain(d)
	if err != nil {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}

	src, err := resolveIcon(dom)
	if err != nil {
		http.Error(w, "icon not found", http.StatusNotFound)
		return
	}

	cloud := os.Getenv("CLOUDINARY_CLOUD_NAME")
	if cloud == "" {
		http.Error(w, "missing CLOUDINARY_CLOUD_NAME", http.StatusInternalServerError)
		return
	}

	cld := "https://res.cloudinary.com/" + cloud +
		"/image/fetch/f_auto,q_auto/" + url.QueryEscape(src)

	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Redirect(w, r, cld, http.StatusFound)
}
