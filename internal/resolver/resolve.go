package resolver

import (
	"context"
	"crypto/tls"
	"errors"
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
	allowLoopback   bool
)

func init() {
	for _, cidr := range []string{
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

func SetAllowLoopback(allow bool) {
	allowLoopback = allow
}

func isPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	if allowLoopback && (ip == "127.0.0.1" || ip == "::1") {
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

func validateURL(u *url.URL) error {
	if u == nil {
		return errors.New("invalid URL")
	}
	if !isValidScheme(u.Scheme) {
		return errors.New("unsupported scheme")
	}
	if u.Host == "" {
		return errors.New("missing host")
	}
	host := u.Host
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		host = host[:colonIdx]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if isPrivateIP(ip.String()) {
			return errors.New("reserved/private IP not allowed")
		}
	}
	return nil
}

var safeHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DisableKeepAlives:  true,
		DisableCompression: false,
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return errors.New("redirects disabled")
	},
}

func ResolveBestIcon(target string) (src string, meta Meta, err error) {
	destURL := "https://" + target
	parsed, err := url.Parse(destURL)
	if err != nil {
		return "", meta, err
	}
	if err := validateURL(parsed); err != nil {
		return "", meta, err
	}
	resp, err := safeHTTPClient.Get(destURL)
	if err != nil {
		return "", meta, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
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
		if err := validateURL(absParsed); err != nil {
			continue
		}

		req, err := http.NewRequest("HEAD", abs, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Favget/1.0")
		h, err := safeHTTPClient.Do(req)
		if err != nil || h.StatusCode >= 400 {
			continue
		}
		defer h.Body.Close()

		ct := h.Header.Get("Content-Type")
		etag := h.Header.Get("ETag")
		meta = Meta{SourceURL: abs}
		if ct != "" {
			meta.ContentType = &ct
		}
		if etag != "" {
			meta.ETag = &etag
		}
		return abs, meta, nil
	}
	return "", meta, errors.New("no icon found")
}
