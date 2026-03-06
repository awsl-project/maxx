package e2e_test

import (
	"net/http"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/health")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]string
	DecodeJSON(t, resp, &result)

	if result["status"] != "ok" {
		t.Fatalf("Expected status 'ok', got %q", result["status"])
	}
}
