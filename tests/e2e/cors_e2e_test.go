package e2e_test

import (
	"net/http"
	"testing"
)

// originReq issues a request to env carrying an Origin header (and, when
// preflightMethod is set, the Access-Control-Request-Method preflight header
// plus the OPTIONS method). An empty token sends the request unauthenticated.
func originReq(t *testing.T, env *TestEnv, method, path, origin, token, preflightMethod string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, env.URL(path), nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if preflightMethod != "" {
		req.Header.Set("Access-Control-Request-Method", preflightMethod)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// TestCORS_DisabledByDefault verifies the default deployment adds no CORS
// headers, even when a request carries an Origin.
func TestCORS_DisabledByDefault(t *testing.T) {
	env := NewTestEnv(t)

	resp := originReq(t, env, http.MethodGet, "/health", "https://ui.example.com", "", "")
	AssertStatus(t, resp, http.StatusOK)
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("CORS disabled by default, unexpected Allow-Origin=%q", got)
	}
}

// TestCORS_PreflightAndActualRequest verifies that, with an allowed origin
// configured, preflight requests short-circuit with 204 + reflected origin and
// actual API responses carry the Allow-Origin header.
func TestCORS_PreflightAndActualRequest(t *testing.T) {
	const origin = "https://ui.example.com"
	env := newTestEnv(t, testEnvOptions{corsOrigins: origin})

	// Preflight OPTIONS against a real API route is handled by the middleware.
	resp := originReq(t, env, http.MethodOptions, "/api/admin/providers", origin, "", http.MethodGet)
	AssertStatus(t, resp, http.StatusNoContent)
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("preflight Allow-Origin=%q, want %q", got, origin)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatalf("preflight missing Access-Control-Allow-Methods")
	}
	resp.Body.Close()

	// The actual authenticated request succeeds and carries the CORS header.
	resp = originReq(t, env, http.MethodGet, "/api/admin/providers", origin, env.Token, "")
	AssertStatus(t, resp, http.StatusOK)
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("response Allow-Origin=%q, want %q", got, origin)
	}
	resp.Body.Close()
}

// TestCORS_DisallowedOrigin verifies a non-allowlisted origin receives no CORS
// headers (the browser blocks it) while the server still processes the request.
func TestCORS_DisallowedOrigin(t *testing.T) {
	env := newTestEnv(t, testEnvOptions{corsOrigins: "https://ui.example.com"})

	resp := originReq(t, env, http.MethodGet, "/health", "https://evil.example.com", "", "")
	AssertStatus(t, resp, http.StatusOK)
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin should get no Allow-Origin, got %q", got)
	}
}

// TestCORS_WildcardReflectsAnyOrigin verifies "*" reflects whatever origin the
// request presents.
func TestCORS_WildcardReflectsAnyOrigin(t *testing.T) {
	env := newTestEnv(t, testEnvOptions{corsOrigins: "*"})

	const origin = "https://anything.example.com"
	resp := originReq(t, env, http.MethodGet, "/health", origin, "", "")
	AssertStatus(t, resp, http.StatusOK)
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("wildcard Allow-Origin=%q, want reflected %q", got, origin)
	}
}
