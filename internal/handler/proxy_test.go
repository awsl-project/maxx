package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	got := rec.Header().Get("Retry-After")
	if got == "" {
		t.Fatal("expected Retry-After header")
	}
	sec, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("Retry-After = %q, parse error: %v", got, err)
	}
	if sec < 1 || sec > 2 {
		t.Fatalf("Retry-After = %d, want 1 or 2", sec)
	}
}

func TestWriteStreamErrorPreservesStatusAndRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	until := time.Now().Add(2 * time.Second)
	writeStreamError(rec, &domain.ProxyError{
		Err:            domain.ErrUpstreamError,
		Message:        "upstream returned status 429",
		Retryable:      true,
		HTTPStatusCode: http.StatusTooManyRequests,
		CooldownUntil:  &until,
	})

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	got := rec.Header().Get("Retry-After")
	if got == "" {
		t.Fatal("expected Retry-After header")
	}
	sec, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("Retry-After = %q, parse error: %v", got, err)
	}
	if sec < 1 || sec > 2 {
		t.Fatalf("Retry-After = %d, want 1 or 2", sec)
	}
	if !strings.Contains(rec.Body.String(), `"type":"error"`) {
		t.Fatalf("stream body = %q, want error event", rec.Body.String())
	}
}
