package config

import (
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                string
	DatabaseURL         string
	RedisURL            string // optional; empty = caching disabled
	CloudinaryURL       string
	Env                 string
	RateLimitRPS        int
	CacheTTLSec         int
	NegativeCacheTTLSec int
	APIKeys             []string // one or more API keys (comma-separated in env)
	AllowedOrigins      string   // comma-separated list of allowed CORS origins
	AllowInsecureTLS    bool     // default false; allow InsecureSkipVerify for broken sites
	MaxHTMLBytes        int64    // max bytes to read when fetching a page's HTML for icon parsing
}

func mustGet(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func getDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func parseAllowedOrigins(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if origin := strings.TrimSpace(p); origin != "" {
			out = append(out, origin)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseAPIKeys splits API_KEY on commas and trims whitespace.
// Empty fragments are ignored. Returns nil if no keys are configured.
//
// Example:
//
//	API_KEY="k1,k2,  k3 " → []string{"k1","k2","k3"}
func parseAPIKeys(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if k := strings.TrimSpace(p); k != "" {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeEnv maps common shorthands while preserving custom values.
// "prod" → "production", "dev" → "development". Everything else is kept as-is.
func normalizeEnv(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "prod":
		return "production"
	case "dev":
		return "development"
	default:
		return v
	}
}

// Load reads environment variables and builds a Config.
// Required variables use mustGet; optional ones use getDefault.
func Load() Config {
	// Parse numeric knobs with sensible defaults.
	rl := 10
	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl = n
		}
	}
	ttl := 86400
	if v := os.Getenv("CACHE_TTL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			ttl = n
		}
	}
	negTTL := 300 // 5 minutes default for negative cache
	if v := os.Getenv("NEGATIVE_CACHE_TTL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			negTTL = n
		}
	}
	maxHTML := int64(1 << 20) // 1 MiB default
	if v := os.Getenv("MAX_HTML_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxHTML = n
		}
	}

	apiKeys := parseAPIKeys(os.Getenv("API_KEY"))
	env := normalizeEnv(getDefault("APP_ENV", "production"))
	allowedOrigins := parseAllowedOrigins(getDefault("CORS_ALLOWED_ORIGINS", ""))

	allowInsecure := false
	if v := os.Getenv("ALLOW_INSECURE_TLS"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes":
			allowInsecure = true
		}
	}

	// Production safety: require API_KEY when APP_ENV=production.
	if env == "production" && len(apiKeys) == 0 {
		log.Fatal("API_KEY is required when APP_ENV=production; set a strong random key")
	}

	return Config{
		Port:                getDefault("PORT", "8080"),
		DatabaseURL:         mustGet("DATABASE_URL"),
		RedisURL:            getDefault("REDIS_URL", ""), // optional – omit to disable caching
		CloudinaryURL:       mustGet("CLOUDINARY_URL"),
		Env:                 env,
		RateLimitRPS:        rl,
		CacheTTLSec:         ttl,
		NegativeCacheTTLSec: negTTL,
		APIKeys:             apiKeys,
		AllowedOrigins:      strings.Join(allowedOrigins, ","),
		AllowInsecureTLS:    allowInsecure,
		MaxHTMLBytes:        maxHTML,
	}
}
