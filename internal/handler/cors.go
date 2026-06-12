package handler

import (
	"net/http"
	"strings"
)

// CORSConfig holds the cross-origin policy for the HTTP server. It is populated
// from the MAXX_CORS_ALLOW_ORIGINS environment variable so that a frontend
// hosted on a different origin (e.g. a static build served from a CDN) can talk
// to this backend.
type CORSConfig struct {
	// AllowOrigins is the set of permitted request origins. A single "*" entry
	// allows any origin. An empty slice disables CORS entirely (same-origin
	// only), which is the default.
	AllowOrigins []string
}

// ParseCORSOrigins parses a comma-separated origin list (the value of
// MAXX_CORS_ALLOW_ORIGINS) into a CORSConfig. Whitespace around entries is
// trimmed and empty entries are dropped. A trailing slash is stripped from each
// entry so a configured "https://ui.example.com/" still matches the browser's
// slash-less "Origin: https://ui.example.com" header.
func ParseCORSOrigins(raw string) CORSConfig {
	var origins []string
	for _, part := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if trimmed != "*" {
			trimmed = strings.TrimRight(trimmed, "/")
		}
		origins = append(origins, trimmed)
	}
	return CORSConfig{AllowOrigins: origins}
}

// HasWildcard reports whether any configured origin is the "*" wildcard.
func (c CORSConfig) HasWildcard() bool {
	for _, o := range c.AllowOrigins {
		if o == "*" {
			return true
		}
	}
	return false
}

// Enabled reports whether any origins are configured. When false, CORSMiddleware
// is a no-op pass-through.
func (c CORSConfig) Enabled() bool {
	return len(c.AllowOrigins) > 0
}

// allows reports whether the given request Origin is permitted.
func (c CORSConfig) allows(origin string) bool {
	for _, allowed := range c.AllowOrigins {
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

// CORSMiddleware wraps a handler with cross-origin headers driven by cfg. When
// cfg has no origins it returns next unchanged so there is zero overhead in the
// common same-origin deployment. Requests carry a Bearer token rather than
// cookies, so the origin is reflected without Allow-Credentials.
func CORSMiddleware(cfg CORSConfig, next http.Handler) http.Handler {
	if !cfg.Enabled() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Advertise that the response varies by Origin whenever one is present —
		// even for disallowed origins — so a shared cache never reuses a no-CORS
		// response for an allowlisted origin (or vice versa).
		if origin != "" {
			w.Header().Add("Vary", "Origin")
		}
		allowed := origin != "" && cfg.allows(origin)
		isPreflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""

		if allowed {
			// Reflect the concrete origin (even for "*") so the response stays
			// valid if a caller later adds credentials, and so Vary is honored.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

			reqHeaders := r.Header.Get("Access-Control-Request-Headers")
			if reqHeaders == "" {
				reqHeaders = "Authorization, Content-Type"
			}
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		// Short-circuit only valid preflights from an allowed origin. Disallowed
		// origins (and non-preflight OPTIONS) fall through to normal handling so
		// they get no CORS headers — the browser then blocks the cross-origin
		// read — and so unknown routes/methods still surface their real status.
		if isPreflight && allowed {
			// The Allow-Headers value is reflected from the request, so the
			// response varies on the preflight request headers/method too.
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
