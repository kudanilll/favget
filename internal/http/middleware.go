package httpx

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// APIKeyAuth returns a middleware that enforces API-key authentication.
//
// How it works:
//   - If "keys" is empty, the middleware becomes a no-op (backward compatible).
//   - Otherwise, the request must present a valid key using one of:
//     1) Header: Authorization: Bearer <key>  (preferred)
//     2) Header: X-API-Key: <key>
//     3) Query string: ?api_key=<key> or ?apikey=<key>  (discouraged)
//
// Notes:
//   - Comparison is constant-time to avoid timing side channels.
//   - The middleware only checks *presence* and *validity* of the key; it does
//     not implement quotas/rate limits. Combine with your rate limiter if needed.
func APIKeyAuth(keys []string) func(next http.Handler) http.Handler {
	// Normalize keys and pre-encode for constant-time comparison.
	var bkeys [][]byte
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k != "" {
			bkeys = append(bkeys, []byte(k))
		}
	}

	// No keys configured → pass-through.
	if len(bkeys) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prefer Authorization: Bearer <key>
			var provided string
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				provided = strings.TrimSpace(auth[len("Bearer "):])
			}

			// Fallback: X-API-Key header
			if provided == "" {
				provided = strings.TrimSpace(r.Header.Get("X-API-Key"))
			}

			// Last resort: query parameter (useful for quick tests; avoid in production)
			if provided == "" {
				provided = strings.TrimSpace(r.URL.Query().Get("api_key"))
				if provided == "" {
					provided = strings.TrimSpace(r.URL.Query().Get("apikey"))
				}
			}

			if provided == "" {
				// Do not disclose whether the server actually has keys configured.
				w.Header().Set("WWW-Authenticate", `Bearer realm="favget"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ok := false
			pb := []byte(provided)
			for _, kb := range bkeys {
				// Constant-time equality check
				if subtle.ConstantTimeCompare(pb, kb) == 1 {
					ok = true
					break
				}
			}
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
