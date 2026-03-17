package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "bad request")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var payload map[string]map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["error"]["message"] != "bad request" {
		t.Fatalf("payload = %v, want error message", payload)
	}
}

func TestWriteProxyErrorPreservesStatusAndRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	until := time.Now().Add(2 * time.Second)
	writeProxyError(rec, &domain.ProxyError{
		Err:            domain.ErrUpstreamError,
		Message:        "upstream returned status 429",
		Retryable:      true,
		HTTPStatusCode: http.StatusTooManyRequests,
		CooldownUntil:  &until,
	})

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatal("expected Retry-After header")
	}
}
