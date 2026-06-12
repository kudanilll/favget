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

// Resolver resolves favicons for domains with configurable HTTP client settings.
type Resolver struct {
	Client        *http.Client
	AllowLoopback bool
	MaxHTMLBytes  int64
}

// SetClient overrides the HTTP client (useful for testing).
func (r *Resolver) SetClient(c *http.Client) {
	r.Client = c
}

// New creates a Resolver.
//   - insecureSkipVerify: if false (recommended), TLS certificates are verified.
//   - maxHTMLBytes: max bytes to read when fetching a page's HTML; 0 defaults to 1 MiB.
//   - allowLoopback: if true, 127.0.0.1 / ::1 are allowed (for testing only).
func New(insecureSkipVerify bool, maxHTMLBytes int64, allowLoopback bool) *Resolver {
	if maxHTMLBytes <= 0 {
		maxHTMLBytes = 1 << 20 // 1 MiB
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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("redirects disabled")
		},
	}

	return &Resolver{
		Client:        client,
		AllowLoopback: allowLoopback,
		MaxHTMLBytes:  maxHTMLBytes,
	}
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
func (r *Resolver) validateURL(u *url.URL) error {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
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

// ResolveBestIcon fetches the target domain's HTML, parses <link> icon candidates,
// probes each candidate via HEAD (with GET fallback), and returns the first valid icon URL.
func (r *Resolver) ResolveBestIcon(target string) (src string, meta Meta, err error) {
	destURL := "https://" + target
	parsed, err := url.Parse(destURL)
	if err != nil {
		return "", meta, err
	}
	if err := r.validateURL(parsed); err != nil {
		return "", meta, err
	}
	resp, err := r.Client.Get(destURL)
	if err != nil {
		return "", meta, err
	}
	defer resp.Body.Close()

	// Limit HTML body reading to prevent abuse from huge responses.
	limitedBody := io.LimitReader(resp.Body, r.MaxHTMLBytes)
	doc, err := goquery.NewDocumentFromReader(limitedBody)
	if err != nil {
		return "", meta, err
	}

	var candidates []string
	doc.Find(`link[rel~="icon"], link[rel="apple-touch-icon"], link[rel="mask-icon"]`).Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists && href != "" {
			candidates = append(candidates, href)
		}
	})

	candidates = append(candidates, "/favicon.ico")

	base, _ := url.Parse("https://" + target)
	for _, c := range candidates {
		u, err := url.Parse(c)
		if err != nil {
			continue
		}
		abs := base.ResolveReference(u).String()

		absParsed, err := url.Parse(abs)
		if err != nil {
			continue
		}
		if err := r.validateURL(absParsed); err != nil {
			continue
		}

		// Try HEAD first; fall back to GET if HEAD is unsupported (405/401/403).
		iconURL, m, ok := r.probeIcon(abs)
		if !ok {
			iconURL, m, ok = r.probeIconGet(abs)
		}
		if ok {
			return iconURL, m, nil
		}
	}
	return "", meta, errors.New("no icon found")
}

// probeIcon sends a HEAD request to candidateURL. Returns (url, meta, true) on success.
func (r *Resolver) probeIcon(candidateURL string) (string, Meta, bool) {
	req, err := http.NewRequest("HEAD", candidateURL, nil)
	if err != nil {
		return "", Meta{}, false
	}
	req.Header.Set("User-Agent", "Favget/1.0")

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
func (r *Resolver) probeIconGet(candidateURL string) (string, Meta, bool) {
	req, err := http.NewRequest("GET", candidateURL, nil)
	if err != nil {
		return "", Meta{}, false
	}
	req.Header.Set("User-Agent", "Favget/1.0")

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
