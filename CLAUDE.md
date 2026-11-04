# Favget Architecture & Insights

Based on an analysis of the Favget codebase, this document provides an overview of the project's structure, architecture, and goals.

## Goals

Favget is designed as a high-performance backend service for fetching and delivering favicons (and other website icons) with CDN support. Its primary goals are:

- **Speed & Reliability**: Delivering icons quickly using Cloudinary and Redis caching.
- **Scalability**: Capable of handling high volumes of requests with minimal latency.
- **Security**: Protecting against SSRF (Server-Side Request Forgery) and enforcing API key authentication in production.
- **Simplicity**: Offering a straightforward API (`GET /v1/icon?domain=...`) while managing complex internal processes like fetching, caching, and CDN uploading.

## Architecture Overview

Favget's architecture is composed of several independent packages orchestrated by the `pkg/app` layer.

### Core Components

- **`cmd/server/main.go`**: The entry point. It sets up the server, handles graceful shutdowns, and integrates the HTTP handler.
- **`pkg/app/app.go`**: Acts as the orchestrator. It initializes dependencies (DB, Redis, Cloudinary, Resolver) and injects them into the HTTP server struct.
- **`internal/http/`**: Defines the HTTP server, routes, and handlers. The primary endpoint (`/v1/icon`) uses a singleflight pattern to prevent duplicate concurrent resolutions for the same domain.
- **`internal/config/`**: Manages environment variables. Enforces the presence of an API key in production mode.
- **`internal/resolver/`**: Handles the core logic of fetching and parsing HTML to find the best icon (supporting `<link rel="icon">`, `apple-touch-icon`, etc.). It includes critical SSRF protections by validating IPs before fetching.
- **`internal/cloud/`**: Integrates with Cloudinary for uploading and serving optimized icons.
- **`internal/store/`**: Manages PostgreSQL interactions for persistent storage of icon metadata.
- **`internal/cache/`**: Wraps Redis for fast lookups, rate limiting, and negative caching.

### Request Flow (`GET /v1/icon`)

1. **Validation**: The domain is normalized.
2. **Cache Check (Redis)**: Checks for a cached icon URL (positive cache) or a recent miss (negative cache).
3. **Database Check (Postgres)**: If not in Redis, checks the DB. If found, backfills Redis and redirects.
4. **Resolution (Cold Path)**: Uses `singleflight` to prevent duplicate concurrent requests.
   - Fetches the target domain's HTML and parses icon links.
   - Falls back to `/favicon.ico` if necessary.
5. **Cloud Upload**: Uploads the resolved icon to Cloudinary.
6. **Persistence**: Saves metadata to Postgres and caches the Cloudinary URL in Redis.
7. **Response**: Issues a 302 redirect to the Cloudinary URL.

## Best Practices & Observations

- **Dependency Injection**: The `pkg/app` layer effectively uses DI, making testing and maintenance easier.
- **Concurrency Control**: The use of `singleflight` in `handleIcon` is a great practice to prevent stampedes when a cache miss occurs for a high-traffic domain.
- **Security**: The robust SSRF protection and mandatory API keys in production are critical security features.
- **Graceful Shutdown**: The `main.go` implementation correctly handles OS signals for graceful termination.
- **Graceful Degradation**: Redis is optional. If not configured, the service falls back to DB and direct resolution without crashing.

## AI Assistant Instructions

- **End-of-Chat Report**: At the end of every single response, you MUST include a brief report listing the Skills, MCPs, and Plugins (tools) you used during that interaction.
