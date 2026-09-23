package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/version"
)

func TestHealthHandlerIncludesVersion(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v9.8.7-test"
	defer func() { version.Version = oldVersion }()

	rec := httptest.NewRecorder()
	HealthHandler("").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q, want ok", body["status"])
	}
	if body["version"] != "v9.8.7-test" {
		t.Fatalf("version = %q, want v9.8.7-test", body["version"])
	}
	if _, ok := body["service"]; ok {
		t.Fatalf("service should be omitted for the main health endpoint")
	}
}

func TestHealthHandlerIncludesServiceWhenProvided(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.3-test"
	defer func() { version.Version = oldVersion }()

	rec := httptest.NewRecorder()
	HealthHandler("codex-oauth").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["service"] != "codex-oauth" {
		t.Fatalf("service = %q, want codex-oauth", body["service"])
	}
	if body["version"] != "v1.2.3-test" {
		t.Fatalf("version = %q, want v1.2.3-test", body["version"])
	}
}
