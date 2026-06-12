# Favget

Favget is a high-performance backend service for fetching and delivering favicons (and other website icons) with CDN support.
It is designed to be **fast, reliable, and scalable** — ideal for projects that need to resolve, cache, and serve icons for multiple domains.

## Features

- **Smart resolver** — Parses HTML `<link rel="icon">`, `apple-touch-icon`, `mask-icon`, and falls back to `/favicon.ico`.
- **Fast delivery** — Optionally cache results in Redis for instant subsequent fetches.
- **Cloud delivery** — Icons are delivered and optimized via Cloudinary (`f_auto`, `q_auto`) using remote fetch.
- **Persistent storage (optional)** — Store metadata in Neon (Postgres) for consistency and revalidation.
- **Simple hosting** — Deployable via Docker or any Go-compatible server.
- **Rate limiting (optional)** — Per-IP/per-domain control via Redis.
- **API-first** — Simple endpoints for fetching icons or metadata.
- **API key protection (required)** — All non-health endpoints require a valid API key in production.
- **SSRF protection** — Blocks requests to private/internal/reserved IP ranges.
- **Negative caching** — Avoids repeated upstream lookups for domains without icons.
- **Request deduplication** — Singleflight prevents duplicate concurrent resolves for the same domain.

## Architecture

- **Request Path**
  - **Client → `cmd/server`** → **`pkg/app.NewHandler()`** → **`internal/http.Routes()`** (chi router) → endpoints (e.g. `/v1/icon`, `/healthz`).

- **One-Time Initialization**
  - `internal/config`: reads env (`CLOUDINARY_URL`, `DATABASE_URL`, `REDIS_URL`, `APP_ENV`, `CACHE_TTL_SECONDS`, etc.).
  - `internal/store`: creates a pooled **Neon Postgres** connection.
  - `internal/cache`: sets up **Upstash Redis** if `REDIS_URL` is set; otherwise acts as a no-op.
  - `internal/cloud`: configures **Cloudinary** from `CLOUDINARY_URL`.
  - `internal/resolver`: builds an HTTP client with configurable TLS verification and body size limits.
  - `pkg/app`: composes the above and returns an `http.Handler` from `internal/http`.
  - `internal/http`: applies **API key middleware** and **rate limiting** to protected routes (see **Authentication**).

- **Request Flow: `GET /v1/icon?domain=...`**
  1. **Normalize domain** via `internal/resolver`.
  2. **Check Redis cache** (if enabled): key `icon:<domain>`.
     - **HIT** → respond **302 Redirect** to the cached Cloudinary URL.
     - **MISS or disabled** → continue.
  3. **Check negative cache**: key `icon-miss:<domain>`.
     - **HIT** → respond **404 Not Found** immediately.
  4. **Resolve icon** via `internal/resolver`:
     - Parse HTML `<link rel="icon">`, `apple-touch-icon`, `mask-icon`; fallback to `/favicon.ico`.
     - Probe each candidate via HEAD (with GET fallback) with content type validation.
  5. **Cloud delivery** via `internal/cloud`:
     - Build a **Cloudinary Remote Fetch** URL: `https://res.cloudinary.com/<cloud>/image/fetch/f_auto,q_auto/<source_url>`.
  6. **Persist and cache**:
     - Upsert metadata in **Postgres** (`icons` table).
     - Store Cloudinary URL in **Redis** with TTL (`CACHE_TTL_SECONDS`), if Redis is configured.
     - On miss, cache negative result with shorter TTL (`NEGATIVE_CACHE_TTL_SECONDS`).
  7. **Respond**: **302 Redirect** to Cloudinary with `Cache-Control` and permissive CORS.

- **Data Model**
  - **Postgres `icons`**: `domain` (PK), `icon_url` (Cloudinary), `source_url`, `etag`, `width`, `height`, `content_type`, `updated_at`.
  - **Redis** (optional): `icon:<domain>` → `icon_url` (TTL = `CACHE_TTL_SECONDS`); `icon-miss:<domain>` → `1` (TTL = `NEGATIVE_CACHE_TTL_SECONDS`).

## Authentication (API Key)

All non-health endpoints require a valid API key.
**In production (`APP_ENV=production`), startup fails if no API key is configured.**

- **Header (recommended):**
  - `Authorization: Bearer <API_KEY>`
  - or `X-API-Key: <API_KEY>`
- **Query param (discouraged; for quick tests only):**
  - `?api_key=<API_KEY>` or `?apikey=<API_KEY>`

> **Warning:** Query param authentication is discouraged in production because values may appear in logs and browser history. Prefer header-based authentication.

### Error codes

- Missing/invalid key → `401 Unauthorized` (with `WWW-Authenticate` header).

### Key management

- Configure via environment variable `API_KEY`.
- Rotate keys by comma-separating them: `API_KEY="old_key,new_key"` (both accepted until you remove the old one).

> **Security tips**
>
> - Prefer the `Authorization: Bearer` header over query params (query values may end up in logs and browser history).
> - Rotate keys by comma-separating values in `API_KEY` during the rollout window.
> - Health checks can stay public (`/healthz`); move them behind the middleware if you require full lockdown.

## Security

### SSRF Protection

Favget resolves domains and fetches remote resources, making SSRF a critical concern.
The resolver blocks requests to:

- **Loopback:** `127.0.0.0/8`, `::1/128`
- **Private networks:** `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
- **Link-local:** `169.254.0.0/16`
- **Carrier-grade NAT:** `100.64.0.0/10`
- **IPv6 private:** `fc00::/7`, `fe80::/10`
- **Reserved/documentation:** `0.0.0.0/8`, `192.0.0.0/24`, `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`, `240.0.0.0/4`, `2001:db8::/32`, `fec0::/10`

All resolved IPs are validated before fetching.

### TLS Verification

TLS certificate verification is **enabled by default**.
Set `ALLOW_INSECURE_TLS=true` only if you need to fetch from sites with broken TLS.
Enabling this disables certificate verification for all outbound requests — use with caution.

### Response Body Limits

HTML pages are read up to `MAX_HTML_BYTES` (default 1 MiB) to prevent abuse from oversized responses.

### Content Type Validation

Icon candidates are validated against a whitelist of safe image content types before acceptance.

### Logging

API keys, secrets, tokens, and URLs containing credentials are never logged.

## API Endpoints

> All endpoints below **require a valid API key** unless explicitly noted.

- `GET /v1/icon?domain=example.com`
  → Redirects (302) to a Cloudinary URL (suitable for `<img>`).
  **Auth:** required
  **Example:**

  ```bash
  curl -i "http://localhost:8080/v1/icon?domain=github.com" \
    -H "Authorization: Bearer <API_KEY>"
  ```

- `GET /healthz`
  → Health probe.
  **Auth:** not required

## Environment Variables

### Required

| Variable         | Description                                                           | Default |
| ---------------- | --------------------------------------------------------------------- | ------- |
| `DATABASE_URL`   | Postgres connection string                                            | —       |
| `CLOUDINARY_URL` | Cloudinary URL (`cloudinary://KEY:SECRET@cloud`)                      | —       |
| `API_KEY`        | API key(s), comma-separated for rotation. **Required in production.** | —       |

### Optional

| Variable                     | Description                                                        | Default           |
| ---------------------------- | ------------------------------------------------------------------ | ----------------- |
| `PORT`                       | HTTP listen port                                                   | `8080`            |
| `APP_ENV`                    | `dev` (development) or `production`                                | `production`      |
| `REDIS_URL`                  | Redis connection string; omit to disable caching and rate limiting | —                 |
| `CACHE_TTL_SECONDS`          | Positive cache TTL for resolved icons                              | `86400` (24h)     |
| `NEGATIVE_CACHE_TTL_SECONDS` | TTL for "icon not found" cache entries                             | `300` (5min)      |
| `RATE_LIMIT_RPS`             | Per-IP requests per second (requires Redis)                        | `10`              |
| `ALLOW_INSECURE_TLS`         | `true` to disable TLS certificate verification                     | `false`           |
| `MAX_HTML_BYTES`             | Max bytes to read when fetching HTML for icon parsing              | `1048576` (1 MiB) |
| `CORS_ALLOWED_ORIGINS`       | Comma-separated list of allowed CORS origins                       | —                 |

## Quickstart

### 1. Clone the repository

```bash
git clone https://github.com/kudanilll/favget.git
cd favget
```

### 2. Set environment variables

Copy the sample and fill in your credentials:

```bash
cp .env.example .env
```

Edit `.env`:

```bash
PORT=8080

# Required
API_KEY=your-long-random-key   # or multiple: key1,key2,key3
CLOUDINARY_URL=cloudinary://API_KEY:API_SECRET@CLOUD_NAME
DATABASE_URL=postgres://user:pass@host:port/db

# Optional — leave empty to disable Redis caching
REDIS_URL=rediss://default:password@host:port

# Tuning
APP_ENV=production
CACHE_TTL_SECONDS=86400
RATE_LIMIT_RPS=10
```

Make sure `.env` is ignored by git:

```bash
echo ".env" >> .gitignore
```

### 3. Run locally

```bash
go run ./cmd/server
curl -i "http://localhost:8080/v1/icon?domain=github.com"
```

### 4. Run with Docker

```bash
# Build the image
docker build -t favget:latest .

# Run the container
docker run --rm -p 8080:8080 \
  -e PORT=8080 \
  -e API_KEY="your-api-key" \
  -e CLOUDINARY_URL="cloudinary://KEY:SECRET@cloud" \
  -e DATABASE_URL="postgres://user:pass@host:port/db?sslmode=require" \
  favget:latest
```

Your local endpoint:

```text
http://localhost:8080/v1/icon?domain=github.com
```

## Database Schema

```sql
CREATE TABLE IF NOT EXISTS icons (
  domain TEXT PRIMARY KEY,
  icon_url TEXT NOT NULL,
  source_url TEXT,
  etag TEXT,
  width INT,
  height INT,
  content_type VARCHAR(64),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## Redis (Optional)

Redis is **not required**. Favget works fully without it — caching and rate limiting are simply disabled.

Redis is recommended when:

- Traffic grows and you want to avoid repeated upstream fetches.
- You need per-IP rate limiting.
- You want negative caching to avoid repeated lookups for domains without icons.

When `REDIS_URL` is set:

- Positive cache: `icon:<domain>` with TTL = `CACHE_TTL_SECONDS`
- Negative cache: `icon-miss:<domain>` with TTL = `NEGATIVE_CACHE_TTL_SECONDS`
- Rate limit counters: `rl:<ip>` with 1-minute window

## Development / Testing

```bash
# Run tests
go test ./...

# Format code
gofmt -w .

# Vet code
go vet ./...

# Run locally
go run ./cmd/server
```

## Production Deployment

- Set `APP_ENV=production` — this requires `API_KEY` to be set.
- Use `Authorization: Bearer <key>` or `X-API-Key: <key>` headers.
- Do **not** set `ALLOW_INSECURE_TLS=true` unless absolutely necessary.
- Use a reverse proxy (nginx, Caddy, Cloudflare) in front for TLS termination.
- Monitor the `/healthz` endpoint for uptime checks.
- PostgreSQL is required. Redis is optional but recommended for production traffic.

## Roadmap

- Support multiple sizes and formats (ICO, SVG, PNG)
- Background refresh jobs

## Support

If you appreciate my work, you can [**buy me a coffee**](https://www.buymeacoffee.com/kudanil) and share your feedback!

## License

```text
MIT License

Copyright (c) 2025 Achmad Daniel Syahputra

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
