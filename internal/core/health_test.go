package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/version"
)

func TestWriteHealthResponse(t *testing.T) {
	oldVersion := version.Version
	oldCommit := version.Commit
	oldBuildTime := version.BuildTime
	t.Cleanup(func() {
		version.Version = oldVersion
		version.Commit = oldCommit
		version.BuildTime = oldBuildTime
	})

	version.Version = "1.2.3"
	version.Commit = "abc1234"
	version.BuildTime = "2026-03-05T00:00:00Z"

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteHealthResponse(w)
	})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", got)
	}

	var resp HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %s", resp.Status)
	}
	if resp.Version != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %s", resp.Version)
	}
	if resp.Commit != "abc1234" {
		t.Fatalf("expected commit abc1234, got %s", resp.Commit)
	}
	if resp.BuildTime != "2026-03-05T00:00:00Z" {
		t.Fatalf("expected build_time 2026-03-05T00:00:00Z, got %s", resp.BuildTime)
	}
}
