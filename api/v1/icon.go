package handler

import (
	"errors"
	"fmt"
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

	// take part of the homepage HTML (save)
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

func cloudNameFromCloudinaryURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid CLOUDINARY_URL: %w", err)
	}
	if u.Scheme != "cloudinary" || u.Host == "" {
		return "", fmt.Errorf("invalid CLOUDINARY_URL: want scheme cloudinary and host as cloud name")
	}
	return u.Host, nil // host = cloud name
}

// Vercel Go runtime: exported Handler(http.ResponseWriter, *http.Request)
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

	raw := os.Getenv("CLOUDINARY_URL")
	if raw == "" {
		http.Error(w, "missing CLOUDINARY_URL", http.StatusInternalServerError)
		return
	}
	cloud, err := cloudNameFromCloudinaryURL(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cloudinary remote fetch delivery
	cld := "https://res.cloudinary.com/" + cloud +
		"/image/fetch/f_auto,q_auto/" + url.QueryEscape(src)

	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Redirect(w, r, cld, http.StatusFound)
}
