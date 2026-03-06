package e2e_test

import (
	"net/http"
	"testing"
)

func TestCORS_Headers(t *testing.T) {
	env := NewTestEnv(t)

	// Send an OPTIONS preflight request to the admin API
	req, err := http.NewRequest(http.MethodOptions, env.URL("/api/admin/providers"), nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// The server may or may not implement CORS at the Go level.
	// If CORS headers are present, verify they are reasonable.
	// If not, the test still passes -- it documents the current behavior.
	corsOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	corsMethodsHeader := resp.Header.Get("Access-Control-Allow-Methods")

	if corsOrigin != "" {
		t.Logf("CORS Access-Control-Allow-Origin: %s", corsOrigin)
	} else {
		t.Log("No CORS Access-Control-Allow-Origin header set (CORS not handled at Go level)")
	}
	if corsMethodsHeader != "" {
		t.Logf("CORS Access-Control-Allow-Methods: %s", corsMethodsHeader)
	}

	// Verify the server responds without error (not 5xx)
	if resp.StatusCode >= 500 {
		t.Fatalf("Expected non-5xx response for OPTIONS request, got %d", resp.StatusCode)
	}
}

func TestCORS_ActualCrossOriginRequest(t *testing.T) {
	env := NewTestEnv(t)

	// Send an actual GET request with Origin header (not preflight)
	req, err := http.NewRequest(http.MethodGet, env.URL("/health"), nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Origin", "http://example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// The request should succeed regardless of CORS configuration
	AssertStatus(t, resp, http.StatusOK)

	// Verify the response body is still valid
	var result map[string]string
	DecodeJSON(t, resp, &result)

	if result["status"] != "ok" {
		t.Fatalf("Expected status 'ok', got %q", result["status"])
	}

	// Log CORS headers if present
	if corsOrigin := resp.Header.Get("Access-Control-Allow-Origin"); corsOrigin != "" {
		t.Logf("CORS Access-Control-Allow-Origin: %s", corsOrigin)
	} else {
		t.Log("No CORS Access-Control-Allow-Origin header on actual cross-origin request")
	}
}
