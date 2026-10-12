package resolver

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type Meta struct {
	SourceURL   string
	Width       *int32
	Height      *int32
	ContentType *string
	ETag        *string
}

var (
	privateIPBlocks []*net.IPNet
	reservedBlocks  []*net.IPNet
)

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	} {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
	for _, cidr := range []string{
		"0.0.0.0/8",
		"100.64.0.0/10",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"240.0.0.0/4",
		"::/128",
		"2001:db8::/32",
		"fec0::/10",
	} {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			reservedBlocks = append(reservedBlocks, block)
		}
	}
}

// DefaultUserAgent mimics a modern desktop browser to prevent anti-bot blocks (WAF/Cloudflare/Akamai).
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Resolver resolves favicons for domains with configurable HTTP client settings.
type Resolver struct {
	Client        *http.Client
	AllowLoopback bool
	MaxHTMLBytes  int64
	UserAgent     string
	LookupIP      func(ctx context.Context, host string) ([]net.IPAddr, error)
}

// SetClient overrides the HTTP client (useful for testing).
func (r *Resolver) SetClient(c *http.Client) {
	r.Client = c
}

// New creates a Resolver.
//   - insecureSkipVerify: if false (recommended), TLS certificates are verified.
//   - maxHTMLBytes: max bytes to read when fetching a page's HTML; 0 defaults to 1 MiB.
//   - allowLoopback: if true, 127.0.0.1 / ::1 are allowed (for testing only).
//   - userAgent: optional custom user agent string (defaults to DefaultUserAgent).
func New(insecureSkipVerify bool, maxHTMLBytes int64, allowLoopback bool, userAgent ...string) *Resolver {
	if maxHTMLBytes <= 0 {
		maxHTMLBytes = 1 << 20 // 1 MiB
	}

	ua := DefaultUserAgent
	if len(userAgent) > 0 && userAgent[0] != "" {
		ua = userAgent[0]
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // user-configured via ALLOW_INSECURE_TLS
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives:  true,
			DisableCompression: false,
			TLSClientConfig:    tlsCfg,
		},
		// We allow redirects (up to the Go default of 10) because many sites
		// redirect from apex to www (e.g. google.com -> www.google.com)
		// or HTTP to HTTPS before serving the HTML.
	}

	return &Resolver{
		Client:        client,
		AllowLoopback: allowLoopback,
		MaxHTMLBytes:  maxHTMLBytes,
		UserAgent:     ua,
	}
}

func (r *Resolver) applyDocumentHeaders(req *http.Request) {
	req.Header.Set("User-Agent", r.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

func (r *Resolver) applyAssetHeaders(req *http.Request) {
	req.Header.Set("User-Agent", r.UserAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
}

func (r *Resolver) isPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	if r.AllowLoopback && parsedIP.IsLoopback() {
		return false
	}
	for _, block := range privateIPBlocks {
		if block.Contains(parsedIP) {
			return true
		}
	}
	for _, block := range reservedBlocks {
		if block.Contains(parsedIP) {
			return true
		}
	}
	return false
}

func NormalizeDomain(d string) (string, error) {
	d = strings.TrimSpace(strings.ToLower(d))
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "www.")
	if d == "" || strings.ContainsAny(d, "/?#") {
		return "", errors.New("invalid domain")
	}
	if len(d) > 253 {
		return "", errors.New("domain too long")
	}
	return d, nil
}

func isValidScheme(scheme string) bool {
	return scheme == "http" || scheme == "https"
}

// validateURL checks scheme, host, and resolves the host to ensure no private/reserved IPs are targeted.
func (r *Resolver) validateURL(ctx context.Context, u *url.URL) error {
	if u == nil {
		return errors.New("invalid URL")
	}
	if !isValidScheme(u.Scheme) {
		return errors.New("unsupported scheme")
	}
	if u.Host == "" {
		return errors.New("missing host")
	}
	host := u.Hostname()

	var ips []net.IPAddr
	var err error
	if r.LookupIP != nil {
		ips, err = r.LookupIP(ctx, host)
	} else {
		ips, err = net.DefaultResolver.LookupIPAddr(ctx, host)
	}
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if r.isPrivateIP(ip.String()) {
			return errors.New("reserved/private IP not allowed")
		}
	}
	return nil
}

// isAllowedContentType checks if the Content-Type header represents a safe image type.
func isAllowedContentType(ct string) bool {
	ct = strings.TrimSpace(ct)
	// Strip any parameters (e.g., charset)
	if i := strings.IndexByte(ct, ';'); i != -1 {
		ct = strings.TrimSpace(ct[:i])
	}
	ct = strings.ToLower(ct)
	switch ct {
	case "image/x-icon",
		"image/vnd.microsoft.icon",
		"image/png",
		"image/jpeg",
		"image/svg+xml",
		"image/webp",
		"image/gif",
		"image/avif",
		"image/apng",
		"image/bmp",
		"image/tiff":
		return true
	}
	return false
}

// ResolveBestIcon fetches the target domain's favicon using a dual-target fallback (apex vs www)
// and probing hierarchy (HTML parsing candidates with direct /favicon.ico fallback).
func (r *Resolver) ResolveBestIcon(ctx context.Context, target string) (src string, meta Meta, err error) {
	// 1. Try primary target first
	src, meta, err = r.resolveForHost(ctx, target)
	if err == nil {
		return src, meta, nil
	}

	// 2. Dual-target fallback: if apex failed, try www (or vice versa)
	var altTarget string
	if strings.HasPrefix(target, "www.") {
		altTarget = strings.TrimPrefix(target, "www.")
	} else {
		altTarget = "www." + target
	}

	if altTarget != "" && altTarget != target {
		if altSrc, altMeta, altErr := r.resolveForHost(ctx, altTarget); altErr == nil {
			return altSrc, altMeta, nil
		}
	}

	return "", meta, errors.New("no icon found")
}

// resolveForHost resolves an icon for a specific host by checking HTML candidates first,
// then falling back to a direct /favicon.ico probe if HTML parsing fails or yields no candidates.
func (r *Resolver) resolveForHost(ctx context.Context, host string) (string, Meta, error) {
	baseURL := "https://" + host
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", Meta{}, err
	}
	if err := r.validateURL(ctx, parsed); err != nil {
		return "", Meta{}, err
	}

	// 1. Try resolving candidates discovered from HTML
	if src, meta, err := r.resolveHTMLCandidates(ctx, baseURL); err == nil {
		return src, meta, nil
	}

	// 2. Direct /favicon.ico fallback probe (works when HTML is blocked by WAF/CAPTCHA or is a SPA)
	favURL := baseURL + "/favicon.ico"
	favParsed, err := url.Parse(favURL)
	if err == nil && r.validateURL(ctx, favParsed) == nil {
		if iconURL, m, ok := r.probeIcon(ctx, favURL); ok {
			return iconURL, m, nil
		}
		if iconURL, m, ok := r.probeIconGet(ctx, favURL); ok {
			return iconURL, m, nil
		}
	}

	return "", Meta{}, errors.New("no icon found for host")
}

// resolveHTMLCandidates fetches the page HTML, parses icon link tags, and probes each candidate.
func (r *Resolver) resolveHTMLCandidates(ctx context.Context, baseURL string) (string, Meta, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL, nil)
	if err != nil {
		return "", Meta{}, err
	}
	r.applyDocumentHeaders(req)

	resp, err := r.Client.Do(req)
	if err != nil {
		return "", Meta{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", Meta{}, errors.New("non-2xx response")
	}

	limitedBody := io.LimitReader(resp.Body, r.MaxHTMLBytes)
	doc, err := goquery.NewDocumentFromReader(limitedBody)
	if err != nil {
		return "", Meta{}, err
	}

	var candidates []string
	doc.Find(`link[rel~="icon"], link[rel="apple-touch-icon"], link[rel="mask-icon"]`).Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists && href != "" {
			candidates = append(candidates, href)
		}
	})

	if len(candidates) == 0 {
		return "", Meta{}, errors.New("no icon link tags found in HTML")
	}

	base, _ := url.Parse(baseURL)
	if resp.Request != nil && resp.Request.URL != nil {
		base = resp.Request.URL
	}

	for _, c := range candidates {
		u, err := url.Parse(c)
		if err != nil {
			continue
		}
		abs := base.ResolveReference(u).String()

		absParsed, err := url.Parse(abs)
		if err != nil || r.validateURL(ctx, absParsed) != nil {
			continue
		}

		// Try HEAD first; fall back to GET if HEAD is unsupported (405/401/403).
		iconURL, m, ok := r.probeIcon(ctx, abs)
		if !ok {
			iconURL, m, ok = r.probeIconGet(ctx, abs)
		}
		if ok {
			return iconURL, m, nil
		}
	}
	return "", Meta{}, errors.New("no valid icon candidate among HTML links")
}

// probeIcon sends a HEAD request to candidateURL. Returns (url, meta, true) on success.
func (r *Resolver) probeIcon(ctx context.Context, candidateURL string) (string, Meta, bool) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", candidateURL, nil)
	if err != nil {
		return "", Meta{}, false
	}
	r.applyAssetHeaders(req)

	h, err := r.Client.Do(req)
	if err != nil {
		return "", Meta{}, false
	}
	h.Body.Close()

	if h.StatusCode < 200 || h.StatusCode >= 400 {
		return "", Meta{}, false
	}

	ct := h.Header.Get("Content-Type")
	if ct != "" && !isAllowedContentType(ct) {
		return "", Meta{}, false
	}

	etag := h.Header.Get("ETag")
	meta := Meta{SourceURL: candidateURL}
	if ct != "" {
		meta.ContentType = &ct
	}
	if etag != "" {
		meta.ETag = &etag
	}
	return candidateURL, meta, true
}

// probeIconGet sends a GET request with a small body read to verify the icon exists
// and validate content type. Used when HEAD is not supported.
func (r *Resolver) probeIconGet(ctx context.Context, candidateURL string) (string, Meta, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", candidateURL, nil)
	if err != nil {
		return "", Meta{}, false
	}
	r.applyAssetHeaders(req)

	h, err := r.Client.Do(req)
	if err != nil {
		return "", Meta{}, false
	}
	defer h.Body.Close()

	if h.StatusCode < 200 || h.StatusCode >= 400 {
		return "", Meta{}, false
	}

	ct := h.Header.Get("Content-Type")
	if ct != "" && !isAllowedContentType(ct) {
		return "", Meta{}, false
	}

	// Read a small amount to verify content and detect content sniffing issues.
	buf := make([]byte, 512)
	n, _ := io.ReadFull(h.Body, buf)
	_ = buf[:n] // content read but not used further; we trust the Content-Type header

	etag := h.Header.Get("ETag")
	meta := Meta{SourceURL: candidateURL}
	if ct != "" {
		meta.ContentType = &ct
	}
	if etag != "" {
		meta.ETag = &etag
	}
	return candidateURL, meta, true
}
