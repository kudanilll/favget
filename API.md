# Favget API Reference

Favget provides a lightweight, high-throughput REST API for resolving, caching, and serving website favicons and icons.

- **Base URL (Local)**: `http://localhost:8080`
- **Specification**: OpenAPI 3.0 compatible (see [OpenAPI Specification](#openapi-specification) below)

---

## Authentication

All endpoints require authentication **except** `/healthz` and `/`.

In production (`APP_ENV=production`), incoming requests without valid authentication return `401 Unauthorized`.

### Supported Methods (in order of preference)

1. **HTTP Authorization Header (Recommended)**
   ```http
   Authorization: Bearer <API_KEY>
   ```
2. **Custom Header**
   ```http
   X-API-Key: <API_KEY>
   ```
3. **Query Parameter (Discouraged in production)**
   ```http
   ?api_key=<API_KEY>
   ```
   or
   ```http
   ?apikey=<API_KEY>
   ```

> **Security Note:** Multiple keys can be configured via comma-separation (`API_KEY="key1,key2"`) to enable seamless zero-downtime key rotation.

---

## Endpoints

### 1. Resolve Best Icon

Resolves the best icon for a target domain, stores metadata in PostgreSQL, caches the result in Redis, and issues an immediate HTTP 302 Redirect to the optimized Cloudinary CDN URL.

```http
GET /v1/icon
```

#### Query Parameters

| Parameter | Type     | Required | Description                       | Example      |
| --------- | -------- | -------- | --------------------------------- | ------------ |
| `domain`  | `string` | **Yes**  | Target website domain or hostname | `github.com` |

#### Request Headers

| Header          | Description                                             |
| --------------- | ------------------------------------------------------- |
| `Authorization` | `Bearer <API_KEY>` (if authenticating via Bearer token) |
| `X-API-Key`     | `<API_KEY>` (alternative header authentication)         |

#### Responses

- **`302 Found`**: Icon resolved successfully.
  - **Header `Location`**: Direct Cloudinary delivery URL (`https://res.cloudinary.com/<cloud>/image/fetch/f_auto,q_auto/<source_url>`).
  - **Header `Cache-Control`**: `public, max-age=86400, stale-while-revalidate=604800`
- **`400 Bad Request`**: Target domain is malformed, contains URI paths/queries, or exceeds length limits (`invalid domain`).
- **`401 Unauthorized`**: Missing or invalid API key. Header `WWW-Authenticate: Bearer realm="favget"` included.
- **`404 Not Found`**: No icon could be discovered or candidate was rejected (`icon not found`).
- **`429 Too Many Requests`**: Rate limit exceeded for client IP/Key. Header `Retry-After: <seconds>` included.

#### Example

```bash
curl -i "http://localhost:8080/v1/icon?domain=github.com" \
  -H "Authorization: Bearer my-secret-api-key"
```

**Response:**

```http
HTTP/1.1 302 Found
Location: https://res.cloudinary.com/demo/image/fetch/f_auto,q_auto/https://github.githubassets.com/favicons/favicon.svg
Cache-Control: public, max-age=86400, stale-while-revalidate=604800
Access-Control-Allow-Origin: *
```

---

### 2. Service Index

Returns service status, environment information, and available routes.

```http
GET /
```

#### Query Parameters

| Parameter | Type     | Required | Description                                                            |
| --------- | -------- | -------- | ---------------------------------------------------------------------- |
| `format`  | `string` | No       | Set `format=html` or pass `Accept: text/html` for a minimal HTML view. |

#### Responses

- **`200 OK`**: Service details JSON (default) or HTML.

```json
{
  "service": "Favget",
  "env": "production",
  "author": "Achmad Daniel Syahputra",
  "contact": "https://www.kudaniel.my.id",
  "repo": "https://github.com/kudanilll/favget",
  "routes": [
    {
      "method": "GET",
      "path": "/healthz",
      "auth": "none",
      "description": "Health probe"
    },
    {
      "method": "GET",
      "path": "/v1/icon",
      "auth": "required (API key)",
      "description": "Resolve best icon for a domain and redirect to optimized Cloudinary URL",
      "example": "curl -i \"https://<host>/v1/icon?domain=github.com\" -H \"Authorization: Bearer <API_KEY>\""
    }
  ]
}
```

---

### 3. Health Probe

Liveness probe for load balancers, Kubernetes, and uptime checkers.

```http
GET /healthz
```

#### Authentication

None (Public endpoint).

#### Responses

- **`200 OK`**: Service is healthy.

---

## Rate Limiting

Rate limiting is enforced when `REDIS_URL` is configured and `RATE_LIMIT_RPS > 0` (default: 10 RPS per IP/Key combination):

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 42
Content-Type: text/plain; charset=utf-8

rate limit exceeded
```

---

## OpenAPI Specification

You can copy this OpenAPI 3.0 YAML specification directly into [Swagger Editor](https://editor.swagger.io/), Postman, or Scalar:

```yaml
openapi: 3.0.3
info:
  title: Favget API
  description: High-performance backend service for fetching and delivering favicons with CDN support.
  version: 1.0.0
  contact:
    name: Achmad Daniel Syahputra
    url: https://www.kudaniel.my.id
servers:
  - url: http://localhost:8080
    description: Local development server
paths:
  /v1/icon:
    get:
      summary: Resolve Best Icon
      description: Resolves the best icon for the given domain and redirects (302) to an optimized Cloudinary CDN URL.
      security:
        - BearerAuth: []
        - ApiKeyAuth: []
        - QueryAuth: []
      parameters:
        - name: domain
          in: query
          required: true
          description: Target domain to fetch icon for (e.g. github.com)
          schema:
            type: string
            example: github.com
      responses:
        "302":
          description: Redirect to the Cloudinary CDN icon URL.
          headers:
            Location:
              schema:
                type: string
                format: uri
              description: Target Cloudinary CDN URL
            Cache-Control:
              schema:
                type: string
                example: public, max-age=86400, stale-while-revalidate=604800
        "400":
          description: Invalid or malformed domain.
        "401":
          description: Unauthorized - missing or invalid API key.
        "404":
          description: Icon not found.
        "429":
          description: Rate limit exceeded.
          headers:
            Retry-After:
              schema:
                type: integer
              description: Seconds to wait before retrying.

  /healthz:
    get:
      summary: Health Probe
      description: Returns 200 OK if the service is alive.
      responses:
        "200":
          description: Service is healthy.

  /:
    get:
      summary: Service Index
      description: Returns information about the service and available routes.
      parameters:
        - name: format
          in: query
          required: false
          schema:
            type: string
            enum: [json, html]
      responses:
        "200":
          description: Service index payload.
          content:
            application/json:
              schema:
                type: object
            text/html:
              schema:
                type: string

components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
    ApiKeyAuth:
      type: apiKey
      in: header
      name: X-API-Key
    QueryAuth:
      type: apiKey
      in: query
      name: api_key
```
