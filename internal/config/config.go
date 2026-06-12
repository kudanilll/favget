package config

import (
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port          string
	DatabaseURL   string
	RedisURL      string // optional; empty = caching disabled
	CloudinaryURL string
	Env           string
	RateLimitRPS  int
	CacheTTLSec   int
	APIKeys       []string // one or more API keys (comma-separated in env)
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

	apiKeys := parseAPIKeys(os.Getenv("API_KEY"))
	env := normalizeEnv(getDefault("APP_ENV", "production"))

	return Config{
		Port:          getDefault("PORT", "8080"),
		DatabaseURL:   mustGet("DATABASE_URL"),
		RedisURL:      getDefault("REDIS_URL", ""), // optional – omit to disable caching
		CloudinaryURL: mustGet("CLOUDINARY_URL"),
		Env:           env,
		RateLimitRPS:  rl,
		CacheTTLSec:   ttl,
		APIKeys:       apiKeys,
	}
}
