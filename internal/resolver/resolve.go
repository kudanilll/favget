package resolver

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type Meta struct {
	SourceURL   string
	Width       *int32
	Height      *int32
	ContentType *string
	ETag        *string
}

func NormalizeDomain(d string) (string, error) {
	d = strings.TrimSpace(strings.ToLower(d))
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "www.")
	if d == "" || strings.ContainsAny(d, "/?#") {
		return "", errors.New("invalid domain")
	}
	return d, nil
}

func ResolveBestIcon(target string) (src string, meta Meta, err error) {
	// 1) GET homepage
	resp, err := http.Get("https://" + target)
	if err != nil { return "", meta, err }
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil { return "", meta, err }

	var candidates []string
	doc.Find(`link[rel~="icon"], link[rel="apple-touch-icon"], link[rel="mask-icon"]`).Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists && href != "" {
			candidates = append(candidates, href)
		}
	})

	// fallback
	candidates = append(candidates, "/favicon.ico")

	base, _ := url.Parse("https://" + target)
	for _, c := range candidates {
		u, err := url.Parse(c)
		if err != nil { continue }
		abs := base.ResolveReference(u).String()

		// HEAD untuk cek ketersediaan
		h, err := http.Head(abs)
		if err != nil || h.StatusCode >= 400 { continue }

		ct := h.Header.Get("Content-Type")
		etag := h.Header.Get("ETag")
		meta = Meta{SourceURL: abs}
		if ct != "" { meta.ContentType = &ct }
		if etag != "" { meta.ETag = &etag }
		return abs, meta, nil
	}
	return "", meta, errors.New("no icon found")
}
