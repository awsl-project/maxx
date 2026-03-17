package custom

import (
	"net/http"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestApplyRateLimitRetryHintsUsesRetryAfterHeader(t *testing.T) {
	proxyErr := &domain.ProxyError{}
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Set("Retry-After", "3")
	rateLimitInfo := &domain.RateLimitInfo{
		QuotaResetTime: time.Now().Add(10 * time.Second),
	}

	applyRateLimitRetryHints(proxyErr, resp, rateLimitInfo)

	if proxyErr.RetryAfter < 3*time.Second || proxyErr.RetryAfter > 4*time.Second {
		t.Fatalf("RetryAfter = %v, want about 3s", proxyErr.RetryAfter)
	}
	if proxyErr.CooldownUntil == nil {
		t.Fatal("CooldownUntil should be set")
	}
}

func TestExtractStructuredResetTimeFindsNestedQuotaResetTime(t *testing.T) {
	body := []byte(`{"error":{"details":[{"metadata":{"QuotaResetTime":"2026-03-17T13:20:00Z"}}]}}`)

	got := extractStructuredResetTime(body)
	want := time.Date(2026, 3, 17, 13, 20, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("reset time = %v, want %v", got, want)
	}
}
