<p align="center">
  <img src="logo.svg" width="120" height="120" alt="Favget Logo" />
</p>

<h1 align="center">Favget</h1>

<p align="center">
  <b>High-performance backend service for fetching, caching, and serving website favicons with CDN delivery.</b>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License MIT"></a>
  <img src="https://img.shields.io/badge/go-1.25-00ADD8.svg" alt="Go Version">
  <a href="API.md"><img src="https://img.shields.io/badge/API-OpenAPI%203.0-green.svg" alt="OpenAPI 3.0"></a>
  <img src="https://img.shields.io/badge/docker-ready-2496ED.svg" alt="Docker Ready">
</p>

---

## Highlights

- **Smart Resolver**: Automatically discovers `<link rel="icon">`, `apple-touch-icon`, `mask-icon`, with automatic fallback to `/favicon.ico`.
- **Anti-Bot Resistance**: Full desktop browser header emulation (`User-Agent`, `Sec-Ch-Ua`, `Sec-Fetch-*`) and dual-target (`apex` vs `www.`) fallback routing to avoid WAF/CAPTCHA blocks.
- **Singleflight Concurrency**: Request deduplication ensures high-traffic cache misses never overwhelm upstream servers or database pools.
- **Multi-Tier Caching**:
  - **Hot Path**: Upstash / Redis positive caching (`icon:<domain>`).
  - **Warm Path**: Neon / PostgreSQL persistent metadata storage.
  - **Cold Path**: Autonomous background resolution, Cloudinary remote fetch upload, and backfill.
  - **Negative Caching**: Short-lived negative caching (`icon-miss:<domain>`) prevents repeated lookups for invalid/iconless sites.
- **CDN Optimization**: Automatically transforms and delivers icons via Cloudinary (`f_auto,q_auto`).
- **Security-First**: Strict SSRF protection blocking loopback, RFC1918 private subnets, carrier-grade NAT, and documentation IP blocks. Constant-time API key verification.

---

## Architecture & Request Flow

```text
Client (GET /v1/icon?domain=github.com)
  │
  ├── 1. [Auth & Rate Limiting]
  │        • API Key verification (Bearer / X-API-Key)
  │        • Redis sliding-window rate limiting
  │
  ├── 2. [Hot Path - Redis]
  │        • Hit: icon:<domain> ───────────────► 302 Redirect (Cloudinary URL)
  │        • Hit: icon-miss:<domain> ──────────► 404 Not Found
  │
  ├── 3. [Warm Path - PostgreSQL]
  │        • Hit: icons table ────────────────► Backfill Redis ──► 302 Redirect
  │
  └── 4. [Cold Path - Singleflight Worker]
           • Dual-target discovery (apex & www)
           • HTML link extraction + HEAD/GET probe
           • Cloudinary CDN remote fetch upload
           • Postgres upsert + Redis backfill
           • Respond 302 Redirect
```

---

## API Reference

Comprehensive endpoint documentation, error handling, curl examples, and OpenAPI 3.0 / Swagger specifications are maintained in **[API.md](API.md)**.

### Quick Example

```bash
# Resolve and redirect to Cloudinary CDN URL
curl -i "http://localhost:8080/v1/icon?domain=github.com" \
  -H "Authorization: Bearer <API_KEY>"
```

---

## Quickstart

### 1. Clone & Configure

```bash
git clone https://github.com/kudanilll/favget.git
cd favget
cp .env.example .env
```

Edit `.env` with your credentials:

```ini
PORT=8080
APP_ENV=production
API_KEY=your-strong-random-api-key
DATABASE_URL=postgres://user:pass@host:port/db?sslmode=require
CLOUDINARY_URL=cloudinary://API_KEY:API_SECRET@CLOUD_NAME

# Optional: omit to disable Redis caching & rate limiting
REDIS_URL=rediss://default:password@host:port
```

### 2. Run Locally

```bash
go run ./cmd/server
```

Test your local endpoint:

```bash
curl -i "http://localhost:8080/v1/icon?domain=github.com" -H "Authorization: Bearer your-strong-random-api-key"
```

### 3. Run with Docker

```bash
# Build Docker image
docker build -t favget:latest .

# Run container
docker run --rm -p 8080:8080 \
  -e PORT=8080 \
  -e API_KEY="your-strong-random-api-key" \
  -e CLOUDINARY_URL="cloudinary://KEY:SECRET@cloud" \
  -e DATABASE_URL="postgres://user:pass@host:port/db?sslmode=require" \
  favget:latest
```

---

## Environment Variables

| Variable                     | Required       | Default            | Description                                             |
| ---------------------------- | -------------- | ------------------ | ------------------------------------------------------- |
| `DATABASE_URL`               | **Yes**        | —                  | PostgreSQL connection string (Neon, Supabase, etc.)     |
| `CLOUDINARY_URL`             | **Yes**        | —                  | Cloudinary API URL (`cloudinary://KEY:SECRET@cloud`)    |
| `API_KEY`                    | **Yes (Prod)** | —                  | API key(s), comma-separated for zero-downtime rotation  |
| `PORT`                       | No             | `8080`             | HTTP listen port                                        |
| `APP_ENV`                    | No             | `production`       | `production` or `dev` (`production` enforces `API_KEY`) |
| `REDIS_URL`                  | No             | —                  | Redis connection URL; omit to disable caching           |
| `CACHE_TTL_SECONDS`          | No             | `86400` (24h)      | Positive cache TTL for resolved icons                   |
| `NEGATIVE_CACHE_TTL_SECONDS` | No             | `300` (5m)         | Negative cache TTL for iconless domains                 |
| `RATE_LIMIT_RPS`             | No             | `10`               | Per-IP request limit per second (requires Redis)        |
| `USER_AGENT`                 | No             | Chrome 131 Desktop | Outbound User-Agent string used by the resolver         |
| `ALLOW_INSECURE_TLS`         | No             | `false`            | Set `true` to skip upstream SSL certificate checks      |
| `MAX_HTML_BYTES`             | No             | `1048576` (1MB)    | Max HTML response size to inspect                       |
| `CORS_ALLOWED_ORIGINS`       | No             | —                  | Comma-separated list of allowed CORS origins            |

---

## Database Schema

Favget uses a single metadata table in PostgreSQL:

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

---

## Development

```bash
# Run unit & integration tests
go test -v ./...

# Vet codebase
go vet ./...

# Format source files
gofmt -w .
```

---

## License

Distributed under the [MIT License](LICENSE). Copyright &copy; 2025 Achmad Daniel Syahputra.

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
