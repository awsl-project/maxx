package e2e_test

import (
	"net/http"
	"testing"
)

// TestCORS_Preflight verifies a cross-origin preflight (OPTIONS) request is
// answered by the CORS middleware with 204 and the appropriate allow headers.
func TestCORS_Preflight(t *testing.T) {
	const origin = "https://ui.example.com"
	env := newTestEnv(t, testEnvOptions{corsOrigins: origin})

	resp := originReq(t, env, http.MethodOptions, "/api/admin/providers", origin, "", http.MethodPost)
	AssertStatus(t, resp, http.StatusNoContent)
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("preflight Allow-Origin=%q, want %q", got, origin)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatalf("preflight missing Access-Control-Allow-Methods")
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatalf("preflight missing Access-Control-Allow-Headers")
	}
}

// TestCORS_ActualCrossOriginRequest verifies an actual cross-origin request to a
// real endpoint succeeds and carries the Access-Control-Allow-Origin header.
func TestCORS_ActualCrossOriginRequest(t *testing.T) {
	const origin = "https://ui.example.com"
	env := newTestEnv(t, testEnvOptions{corsOrigins: origin})

	resp := originReq(t, env, http.MethodGet, "/health", origin, "", "")
	AssertStatus(t, resp, http.StatusOK)
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("response Allow-Origin=%q, want %q", got, origin)
	}
}
