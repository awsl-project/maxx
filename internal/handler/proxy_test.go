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
	"github.com/awsl-project/maxx/internal/flow"
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

func TestWriteDispatchErrorSkipsWhenResponseAlreadyStarted(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := newResponseStateWriter(recorder)
	ctx := flow.NewCtx(writer, httptest.NewRequest("POST", "/v1/messages", nil))

	if _, err := ctx.Writer.Write([]byte("chunk-1")); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	handler := &ProxyHandler{}
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrStreamIdleTimeout, false, "stream stalled")
	handler.writeDispatchError(ctx, proxyErr, true)

	if body := recorder.Body.String(); body != "chunk-1" {
		t.Fatalf("response body = %q, want original partial response only", body)
	}
}

func TestWriteProxyErrorUsesProxyHTTPStatusCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrFirstByteTimeout, true, "first token timeout")
	proxyErr.HTTPStatusCode = http.StatusGatewayTimeout

	writeProxyError(recorder, proxyErr)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusGatewayTimeout)
	}
}

func TestWriteStreamErrorUsesProxyHTTPStatusCodeBeforeStreamStarts(t *testing.T) {
	recorder := httptest.NewRecorder()
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrFirstByteTimeout, true, "first token timeout")
	proxyErr.HTTPStatusCode = http.StatusGatewayTimeout

	writeStreamError(recorder, proxyErr)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusGatewayTimeout)
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
