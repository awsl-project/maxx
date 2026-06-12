package custom

import (
	"net/http"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestClassifyHTTPError429UsesRetryAfterHeader(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Retry-After", "3")

	proxyErr := classifyHTTPError(429, []byte(`{"error":{"message":"rate limited"}}`), headers, domain.ClientTypeOpenAI, "gpt-4")

	if proxyErr.RetryAfter < 3*time.Second || proxyErr.RetryAfter > 4*time.Second {
		t.Fatalf("RetryAfter = %v, want about 3s", proxyErr.RetryAfter)
	}
	if proxyErr.CooldownUntil == nil {
		t.Fatal("CooldownUntil should be set")
	}
	if proxyErr.Scope != domain.ScopeKey {
		t.Fatalf("Scope = %v, want ScopeKey", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonRateLimitExceeded {
		t.Fatalf("Reason = %v, want CooldownReasonRateLimitExceeded", proxyErr.Reason)
	}
}

func TestClassifyHTTPError429QuotaExhausted(t *testing.T) {
	headers := make(http.Header)
	body := []byte(`{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`)

	proxyErr := classifyHTTPError(429, body, headers, domain.ClientTypeOpenAI, "gpt-4")

	if proxyErr.Reason != domain.CooldownReasonQuotaExhausted {
		t.Fatalf("Reason = %v, want CooldownReasonQuotaExhausted", proxyErr.Reason)
	}
}

func TestClassifyHTTPError401AuthFailure(t *testing.T) {
	headers := make(http.Header)
	proxyErr := classifyHTTPError(401, []byte(`{"error":{"message":"invalid api key"}}`), headers, domain.ClientTypeOpenAI, "gpt-4")

	if proxyErr.Scope != domain.ScopeKey {
		t.Fatalf("Scope = %v, want ScopeKey", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonAuthFailure {
		t.Fatalf("Reason = %v, want CooldownReasonAuthFailure", proxyErr.Reason)
	}
	if proxyErr.Retryable {
		t.Fatal("401 should not be retryable")
	}
}

func TestClassifyHTTPError503ModelOverloaded(t *testing.T) {
	headers := make(http.Header)
	proxyErr := classifyHTTPError(503, []byte(`{"error":{"message":"model is overloaded"}}`), headers, domain.ClientTypeClaude, "claude-3")

	if proxyErr.Scope != domain.ScopeModel {
		t.Fatalf("Scope = %v, want ScopeModel", proxyErr.Scope)
	}
	if proxyErr.Model != "claude-3" {
		t.Fatalf("Model = %v, want claude-3", proxyErr.Model)
	}
}

func TestParseRetryAfterHeaderSkipsExpiredHTTPDate(t *testing.T) {
	retryAfter, until := parseRetryAfterHeader(time.Now().Add(-1 * time.Minute).UTC().Format(http.TimeFormat))
	if retryAfter != 0 {
		t.Fatalf("RetryAfter = %v, want 0", retryAfter)
	}
	if until != nil {
		t.Fatalf("CooldownUntil = %v, want nil", *until)
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
