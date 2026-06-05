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
}
