package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	Port             string
	DatabaseURL      string
	RedisURL         string
	CloudinaryURL    string
	Env              string
	RateLimitRPS     int
	CacheTTLSec      int
}

func mustGet(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func Load() Config {
	rl := 10
	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { rl = n }
	}
	ttl := 86400
	if v := os.Getenv("CACHE_TTL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { ttl = n }
	}

	return Config{
		Port:          getDefault("PORT", "8080"),
		DatabaseURL:   mustGet("DATABASE_URL"),
		RedisURL:      mustGet("REDIS_URL"),
		CloudinaryURL: mustGet("CLOUDINARY_URL"),
		Env:           getDefault("APP_ENV", "prod"),
		RateLimitRPS:  rl,
		CacheTTLSec:   ttl,
	}
}

func getDefault(k, d string) string {
	if v := os.Getenv(k); v != "" { return v }
	return d
}
