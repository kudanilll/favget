# Favget

Favget is a high-performance backend service for fetching and delivering favicons (and other website icons) with global caching and CDN support.
It is designed to be **fast, reliable, and scalable** — ideal for projects that need to resolve, cache, and serve icons for multiple domains.

## ✨ Features

- 🔍 **Smart resolver** – Parses HTML `<link rel="icon">`, `apple-touch-icon`, `mask-icon`, and falls back to `/favicon.ico`.
- 🚀 **Fast delivery** – Optionally cache hot results in Redis (Upstash) for instant subsequent fetches.
- ☁️ **Cloud delivery** – Icons are delivered & optimized via Cloudinary (`f_auto`, `q_auto`) using remote fetch.
- 🗄️ **Persistent storage (optional)** – Store metadata in Neon (Postgres) for consistency and revalidation.
- 🌍 **Simple hosting** – Deployable on **Vercel** using Go Serverless Functions.
- 🔒 **Rate limiting (optional)** – Per-IP/per-domain control via Redis.
- 📦 **API-first** – Simple endpoints for fetching icons or metadata.

## 🛠️ Tech Stack

- **Language:** [Go](https://go.dev/)
- **Runtime:** Vercel Go Serverless Functions
- **Framework:** [chi](https://github.com/go-chi/chi) (lightweight HTTP router)
- **Database (optional):** [Neon Postgres](https://neon.tech/)
- **Cache (optional):** [Upstash Redis](https://upstash.com/)
- **Storage/CDN:** [Cloudinary](https://cloudinary.com/)
- **Hosting:** [Vercel](https://vercel.com/)

## 📐 Architecture

- **Request Path**

  - **Client → Vercel Function** (`/api/index.go`) → **`pkg/app.NewHandler()`** → **`internal/http.Routes()`** (chi router) → endpoints (e.g. `/v1/icon`, `/healthz`).
  - The Vercel bridge strips the `/api` prefix before passing the request to the internal router, so internal routes remain `/v1/...`.

- **One-Time Initialization (lazy via `sync.Once`)**

  - `internal/config`: reads env (`CLOUDINARY_URL`, `DATABASE_URL`, `REDIS_URL`, `APP_ENV=production`, `CACHE_TTL_SECONDS`, etc.).
  - `internal/store`: creates a pooled **Neon Postgres** connection.
  - `internal/cache`: sets up **Upstash Redis**.
  - `internal/cloud`: configures **Cloudinary** from `CLOUDINARY_URL`.
  - `pkg/app`: composes the above and returns an `http.Handler` from `internal/http`.

- **Request Flow: `GET /v1/icon?domain=...`**

  1. **Normalize domain** via `internal/resolver`.
  2. **Check Redis cache**: key `icon:<domain>`.

     - **HIT** → respond **302 Redirect** to the cached Cloudinary URL.
     - **MISS** → continue.

  3. **Resolve icon** via `internal/resolver`:

     - Parse HTML `<link rel="icon">`, `apple-touch-icon`, `mask-icon`; fallback to `/favicon.ico`.

  4. **Cloud delivery** via `internal/cloud`:

     - Build a **Cloudinary Remote Fetch** URL
       `https://res.cloudinary.com/<cloud>/image/fetch/f_auto,q_auto/<source_url>`.

  5. **Persist & cache**:

     - Upsert metadata in **Postgres** (`icons` table).
     - Store Cloudinary URL in **Redis** with TTL (`CACHE_TTL_SECONDS`).

  6. **Respond**: **302 Redirect** to Cloudinary with `Cache-Control` and permissive CORS.

- **Routing & Deployment**

  - Public access: `/api/v1/icon` by default.
  - Optional rewrites in `vercel.json` map `/v1/*` and `/healthz` → `/api/index`.

- **Data Model & Cache**

  - **Postgres `icons`**: `domain` (PK), `icon_url` (Cloudinary), `source_url`, `etag`, `width`, `height`, `content_type`, `updated_at`.
  - **Redis**: `icon:<domain>` → `icon_url` (TTL = `CACHE_TTL_SECONDS`).

## 🚦 API Endpoints

- `GET /v1/icon?domain=example.com`
  → Redirects (302) to Cloudinary URL for <img> usage.

- `POST /v1/refresh (protected)`
  → Forces re-crawl and refresh of icon.

> Default route on Vercel is /api/v1/icon. This repo uses vercel.json to rewrite /v1/icon → /api/v1/icon so your public URL stays clean.

## ⚡ Quickstart

### 1. Clone the repository

```bash
git clone https://github.com/kudanilll/favget.git
cd favget
```

### 2. Set environment variables

#### 2.1 Copy the sample and fill your credentials:

```bash
cp .env.example .env
```

#### 2.2 Edit `.env` with your credentials:

```bash
PORT=8080
DATABASE_URL=postgres://user:pass@host:port/db?sslmode=require
REDIS_URL=rediss://default:password@host:port
CLOUDINARY_URL=cloudinary://API_KEY:API_SECRET@CLOUD_NAME
APP_ENV=dev
CACHE_TTL_SECONDS=86400
RATE_LIMIT_RPS=10
```

#### 2.3 Make sure `.env` is ignored by git:

```bash
echo ".env" >> .gitignore
```

### 3. Run locally

#### Using Vercel CLI:

```bash
vercel dev
curl -i "http://localhost:3000/v1/icon?domain=github.com"
```

#### Local Server:

```bash
go run ./cmd/server
curl -i "http://localhost:8080/v1/icon?domain=github.com"
```

### 4. Deploy to Vercel

#### Option A — GitHub integration (recommended)

1. Push this repo to GitHub.
2. Import the repo in Vercel Dashboard.
3. In Settings → Environment Variables, add:

   - CLOUDINARY_CLOUD_NAME (required)
   - DATABASE_URL, REDIS_URL (optional)

4. (Optional) Set region close to your users (e.g., sin1) via vercel.json.
5. Click Deploy.

#### Option B — Vercel CLI

```bash
vercel                                   # first-time setup (preview)
vercel --prod                            # deploy to production
```

Your public endpoint will be:

```text
https://<your-vercel-domain>/v1/icon?domain=github.com
```

### 5. vercel.json

This repo uses the following `vercel.json`:

```json
{
  "$schema": "https://openapi.vercel.sh/vercel.json",
  "routes": [
    { "src": "^/v1/(.*)$", "dest": "/api/index" },
    { "src": "^/healthz$", "dest": "/api/index" }
  ],
  "build": {
    "env": {
      "GO_BUILD_FLAGS": "-ldflags '-s -w'"
    }
  }
}
```

- routes: rewrites `/v1/icon` → `/api/v1/icon`
- `build.env.GO_BUILD_FLAGS`: optimizes Go binary size

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

## ☕ Support

If you appreciate my work, you can [**buy me a coffee**](https://www.buymeacoffee.com/kudanil) and share your feedback! Your support helps me continue to improve Favget.

## 📄 License

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

Built with ❤️ using Go, Vercel, Neon, Upstash, and Cloudinary.
