package e2e_test

import (
	"net/http"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/health")
	AssertStatus(t, resp, http.StatusOK)

	var result struct {
		Status       string            `json:"status"`
		Dependencies map[string]string `json:"dependencies"`
	}
	DecodeJSON(t, resp, &result)

	if result.Status != "ok" {
		t.Fatalf("Expected status 'ok', got %q", result.Status)
	}
	if result.Dependencies["database"] != "ok" {
		t.Fatalf("Expected database dependency to be ok, got %q", result.Dependencies["database"])
	}
}

func TestHealthCheckReturnsServiceUnavailableWhenDatabaseIsDown(t *testing.T) {
	env := NewTestEnv(t)

	if err := env.DB.Close(); err != nil {
		t.Fatalf("Close database: %v", err)
	}

	resp := env.UnauthGet("/health")
	AssertStatus(t, resp, http.StatusServiceUnavailable)

	var result struct {
		Status       string            `json:"status"`
		Dependencies map[string]string `json:"dependencies"`
	}
	DecodeJSON(t, resp, &result)

	if result.Status != "error" {
		t.Fatalf("Expected status 'error', got %q", result.Status)
	}
	if result.Dependencies["database"] != "error" {
		t.Fatalf("Expected database dependency to be error, got %q", result.Dependencies["database"])
	}
}
