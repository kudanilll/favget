# Favget

Favget is a high-performance backend service for fetching and delivering favicons (and other website icons) with global caching and CDN support.
It is designed to be **fast, reliable, and scalable** — ideal for projects that need to resolve, cache, and serve icons for multiple domains.

## ✨ Features

- 🔍 **Smart resolver** – Parses HTML `<link rel="icon">`, `apple-touch-icon`, `mask-icon`, and falls back to `/favicon.ico`.
- 🚀 **Fast delivery** – Cached in Redis (Upstash) and served instantly on subsequent requests.
- ☁️ **Cloud storage** – Icons are uploaded and optimized via Cloudinary (`f_auto`, `q_auto`).
- 🗄️ **Persistent storage** – Metadata stored in Neon (Postgres) for consistency and revalidation.
- 🌍 **Scalable hosting** – Deployable on Fly.io or Google Cloud Run with minimal setup.
- 🔒 **Rate limiting** – Prevents abuse with per-IP and per-domain control (via Redis).
- 📦 **API-first** – Simple endpoints for fetching icons or metadata.

## 🛠️ Tech Stack

- **Language:** [Go](https://go.dev/)
- **Framework:** [chi](https://github.com/go-chi/chi) (lightweight HTTP router)
- **Database:** [Neon Postgres](https://neon.tech/)
- **Cache:** [Upstash Redis](https://upstash.com/)
- **Storage/CDN:** [Cloudinary](https://cloudinary.com/)
- **Hosting:** [Fly.io](https://fly.io/) or [Cloud Run](https://cloud.google.com/run)

## 📐 Architecture

```text
Client → Favget API
  ↳ Redis (Upstash) – fast cache lookup
  ↳ Postgres (Neon) – metadata persistence
  ↳ Cloudinary – optimized icon storage & CDN delivery
```

## 🚦 API Endpoints

- `GET /v1/icon?domain=example.com`
  → Redirects (302) to Cloudinary URL for <img> usage.

- `POST /v1/refresh (protected)`
  → Forces re-crawl and refresh of icon.

## ⚡ Quickstart

### 1. Clone the repository

```bash
git clone https://github.com/kudanilll/favget.git
cd favget
```

### 2. Set environment variables

1. Copy the sample and fill your credentials:

```bash
cp .env.example .env
```

2. Edit `.env` with your credentials:

```bash
PORT=8080
DATABASE_URL=postgres://user:pass@host:port/db?sslmode=require
REDIS_URL=rediss://default:password@host:port
CLOUDINARY_URL=cloudinary://API_KEY:API_SECRET@CLOUD_NAME
APP_ENV=dev
CACHE_TTL_SECONDS=86400
RATE_LIMIT_RPS=10
```

3. Make sure `.env` is ignored by git:

```bash
echo ".env" >> .gitignore
```

### 3. Run locally

```bash
go run ./cmd/server
```

### 4. Test endpoint

```bash
curl -i "http://localhost:8080/v1/icon?domain=github.com"
```

### 5. Deploy

- Fly.io

```bash
flyctl launch
flyctl secrets set DATABASE_URL=... REDIS_URL=... CLOUDINARY_URL=...
flyctl deploy
```

- Cloud Run

```bash
gcloud builds submit --tag gcr.io/PROJECT/favget
gcloud run deploy favget --image gcr.io/PROJECT/favget --platform managed --region asia-southeast2
```

## 📊 Database Schema

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

## 🔮 Roadmap

- Add GraphQL endpoint
- Support multiple sizes & formats (ICO, SVG, PNG)
- Rate limiting middleware
- Background refresh jobs

## 📄 License

```text
MIT License

Copyright (c) 2025 kudanilll.

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

Built with ❤️ using Go, Fly.io, Neon, Upstash, and Cloudinary.
