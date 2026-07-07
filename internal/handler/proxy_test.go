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

func TestWriteRateLimitError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeRateLimitError(rec, "API token concurrent request limit exceeded", 1)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}

	var payload map[string]map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["error"]["type"] != "rate_limit_error" {
		t.Fatalf("payload = %v, want rate_limit_error", payload)
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

func TestWriteStreamRateLimitError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeStreamRateLimitError(rec, "API token concurrent request limit exceeded", 1)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if !strings.Contains(rec.Body.String(), `"type":"rate_limit_error"`) {
		t.Fatalf("stream body = %q, want rate_limit_error", rec.Body.String())
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

func TestUserPanelAPITokenProjectBindingGuard(t *testing.T) {
	userPanelToken := &domain.APIToken{
		Description: userPanelAPITokenDescription(123),
		ProjectID:   42,
	}
	regularToken := &domain.APIToken{
		Description: "regular token",
		ProjectID:   42,
	}

	if !isUserPanelAPIToken(userPanelToken) {
		t.Fatal("expected managed user panel token to be detected")
	}
	if canAPITokenUseProjectBinding(userPanelToken) {
		t.Fatal("user panel token must not use header/token/session project binding")
	}
	if got, ok := apiTokenProjectBinding(userPanelToken, 0); ok || got != 0 {
		t.Fatalf("user panel token project binding = (%d, %v), want (0, false)", got, ok)
	}
	if isUserPanelAPIToken(regularToken) {
		t.Fatal("regular token must not be treated as user panel token")
	}
	if !canAPITokenUseProjectBinding(regularToken) {
		t.Fatal("regular token should keep existing project binding behavior")
	}
	if got, ok := apiTokenProjectBinding(regularToken, 0); !ok || got != regularToken.ProjectID {
		t.Fatalf("regular token project binding = (%d, %v), want (%d, true)", got, ok, regularToken.ProjectID)
	}
	if got, ok := apiTokenProjectBinding(regularToken, 7); ok || got != 7 {
		t.Fatalf("existing project binding = (%d, %v), want (7, false)", got, ok)
	}
	if !canAPITokenUseProjectBinding(nil) {
		t.Fatal("nil token should keep unauthenticated/default project behavior")
	}
}
