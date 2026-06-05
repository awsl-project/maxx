package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCORSOrigins(t *testing.T) {
	cases := []struct {
		raw     string
		want    []string
		enabled bool
	}{
		{"", nil, false},
		{"   ", nil, false},
		{"*", []string{"*"}, true},
		{"https://a.com, https://b.com ", []string{"https://a.com", "https://b.com"}, true},
		{"https://a.com,,", []string{"https://a.com"}, true},
		// Trailing slashes are stripped so they match the slash-less Origin header.
		{"https://a.com/", []string{"https://a.com"}, true},
		{"https://a.com/// , https://b.com/", []string{"https://a.com", "https://b.com"}, true},
	}
	for _, c := range cases {
		got := ParseCORSOrigins(c.raw)
		if got.Enabled() != c.enabled {
			t.Errorf("ParseCORSOrigins(%q).Enabled() = %v, want %v", c.raw, got.Enabled(), c.enabled)
		}
		if len(got.AllowOrigins) != len(c.want) {
			t.Errorf("ParseCORSOrigins(%q) = %v, want %v", c.raw, got.AllowOrigins, c.want)
			continue
		}
		for i := range c.want {
			if got.AllowOrigins[i] != c.want[i] {
				t.Errorf("ParseCORSOrigins(%q)[%d] = %q, want %q", c.raw, i, got.AllowOrigins[i], c.want[i])
			}
		}
	}
}

func TestCORSMiddlewareDisabledIsPassthrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := CORSMiddleware(CORSConfig{}, next)
	// When disabled, the middleware must not add any CORS headers.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://x.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disabled CORS should not set Allow-Origin header")
	}
	if rec.Code != http.StatusTeapot {
		t.Fatalf("disabled CORS should pass through to next handler, got %d", rec.Code)
	}
}

func TestCORSMiddlewareAllowsConfiguredOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := CORSMiddleware(ParseCORSOrigins("https://app.example.com"), next)

	// Allowed origin gets reflected.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Allow-Origin = %q, want reflected origin", got)
	}

	// Disallowed origin gets no CORS header.
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	req2.Header.Set("Origin", "https://evil.example.com")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty for disallowed origin", got)
	}
}

func TestCORSMiddlewarePreflightShortCircuits(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	h := CORSMiddleware(ParseCORSOrigins("*"), next)

	req := httptest.NewRequest(http.MethodOptions, "/api/admin/providers", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if called {
		t.Fatal("preflight should not reach the next handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://anything.example.com" {
		t.Fatalf("wildcard Allow-Origin = %q, want reflected origin", got)
	}
	// Preflight responses must vary on the request headers/method they reflect.
	vary := rec.Header().Values("Vary")
	for _, want := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
		if !containsStr(vary, want) {
			t.Fatalf("preflight Vary=%v missing %q", vary, want)
		}
	}
}

func TestParseCORSOriginsTrailingSlashMatches(t *testing.T) {
	// A configured origin with a trailing slash still matches the slash-less
	// Origin header the browser sends.
	h := CORSMiddleware(ParseCORSOrigins("https://ui.example.com/"), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://ui.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ui.example.com" {
		t.Fatalf("Allow-Origin = %q, want match despite configured trailing slash", got)
	}
}

func TestCORSMiddlewareDisallowedPreflightFallsThrough(t *testing.T) {
	// A preflight from a disallowed origin must NOT be short-circuited with 204;
	// it falls through to the next handler with no CORS headers so the browser
	// blocks it and the real route status is preserved.
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := CORSMiddleware(ParseCORSOrigins("https://ui.example.com"), next)

	req := httptest.NewRequest(http.MethodOptions, "/api/admin/providers", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "DELETE")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("disallowed preflight status = %d, want pass-through 418", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed preflight should have no Allow-Origin, got %q", got)
	}
}

func containsStr(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
