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

func TestClassifyHTTPError402InsufficientBalanceIsKeyQuota(t *testing.T) {
	headers := make(http.Header)
	body := []byte(`{"error":{"message":"Insufficient balance","type":"bad_response_status_code","code":"bad_response_status_code"}}`)

	proxyErr := classifyHTTPError(http.StatusPaymentRequired, body, headers, domain.ClientTypeOpenAI, "gpt-4")

	if proxyErr.Scope != domain.ScopeKey {
		t.Fatalf("Scope = %v, want ScopeKey", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonQuotaExhausted {
		t.Fatalf("Reason = %v, want CooldownReasonQuotaExhausted", proxyErr.Reason)
	}
	if proxyErr.Retryable {
		t.Fatal("402 insufficient balance should not retry the same provider")
	}
}

// OpenRouter returns 402 when a single request's max_tokens exceeds what the
// remaining credits can cover, even though the key still works for cheaper
// requests. This must be request-scoped (no cooldown) so one oversized request
// can't take the whole provider — a shared fallback — offline for everyone.
func TestClassifyHTTPError402PerRequestAffordabilityIsRequestScoped(t *testing.T) {
	headers := make(http.Header)
	body := []byte(`{"error":{"message":"This request requires more credits, or fewer max_tokens. You requested up to 16000 tokens, but can only afford 7217. To increase, visit https://openrouter.ai/settings/credits and add more credits","code":402,"metadata":{"limit_source":"openrouter_credits"}}}`)

	proxyErr := classifyHTTPError(http.StatusPaymentRequired, body, headers, domain.ClientTypeOpenAI, "openai/gpt-5.5")

	if proxyErr.Scope != domain.ScopeRequest {
		t.Fatalf("Scope = %v, want ScopeRequest (must not cool the provider)", proxyErr.Scope)
	}
	if proxyErr.Reason == domain.CooldownReasonQuotaExhausted {
		t.Fatal("per-request affordability 402 must not be quota_exhausted (that cools the key)")
	}
	if proxyErr.Retryable {
		t.Fatal("oversized request should not retry the same provider")
	}
}

// Even if the affordability 402 arrives with a Retry-After header (parsed before
// classification), the request-scoped branch must clear the cooldown metadata so a
// single oversized request can't cool the shared provider.
func TestClassifyHTTPError402PerRequestAffordabilityClearsRetryAfter(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Retry-After", "30")
	body := []byte(`{"error":{"message":"This request requires more credits, or fewer max_tokens. You requested up to 16000 tokens, but can only afford 7217","code":402}}`)

	proxyErr := classifyHTTPError(http.StatusPaymentRequired, body, headers, domain.ClientTypeOpenAI, "openai/gpt-5.5")

	if proxyErr.Scope != domain.ScopeRequest {
		t.Fatalf("Scope = %v, want ScopeRequest", proxyErr.Scope)
	}
	if proxyErr.RetryAfter != 0 {
		t.Fatalf("RetryAfter = %v, want 0 (request-scoped must not cool the provider)", proxyErr.RetryAfter)
	}
	if proxyErr.CooldownUntil != nil {
		t.Fatalf("CooldownUntil = %v, want nil", *proxyErr.CooldownUntil)
	}
}

func TestIsPerRequestAffordability402(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"openrouter fewer max_tokens", "requires more credits, or fewer max_tokens", true},
		{"can only afford", "you requested up to 16000 tokens, but can only afford 7217", true},
		{"more credits + max_tokens", "needs more credits for these max_tokens", true},
		{"can only afford without token context", "your plan can only afford the basic tier", false},
		{"more credits without token context", "please add more credits to your account", false},
		{"insufficient balance", "insufficient balance", false},
		{"no remaining balance", "account has no remaining balance", false},
		{"generic payment", "payment required", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPerRequestAffordability402(tc.body); got != tc.want {
				t.Errorf("isPerRequestAffordability402(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
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

func TestClassifyHTTPErrorRequestTimeoutIsRetryableNetworkError(t *testing.T) {
	proxyErr := classifyHTTPError(http.StatusRequestTimeout, []byte(`failed to connect to upstream: upstream error`), http.Header{}, domain.ClientTypeOpenAI, "gpt-4")

	if proxyErr.Scope != domain.ScopeEndpoint {
		t.Fatalf("scope = %q, want endpoint", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonNetworkError {
		t.Fatalf("reason = %q, want network_error", proxyErr.Reason)
	}
	if !proxyErr.Retryable {
		t.Fatal("408 upstream timeout should be retryable")
	}
}

func TestClassifyHTTPError422InvalidModelIsModelScoped(t *testing.T) {
	body := []byte(`{"error":{"message":"model not found: moonshotai/kimi-k3 (no channel candidates remain; applied filters: api_key.binding_mode=manual, api_key.channelIDs, api_format=openai/chat_completions, stream=true)","type":"invalid_model_error"}}`)

	proxyErr := classifyHTTPError(http.StatusUnprocessableEntity, body, http.Header{}, domain.ClientTypeOpenAI, "moonshotai/kimi-k3")

	if proxyErr.Scope != domain.ScopeModel {
		t.Fatalf("Scope = %v, want ScopeModel", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonModelUnavailable {
		t.Fatalf("Reason = %v, want CooldownReasonModelUnavailable", proxyErr.Reason)
	}
	if proxyErr.Model != "moonshotai/kimi-k3" {
		t.Fatalf("Model = %q, want moonshotai/kimi-k3", proxyErr.Model)
	}
	if proxyErr.Retryable {
		t.Fatal("422 invalid_model_error should not retry the same provider")
	}
}

func TestClassifyHTTPError422ValidationRemainsRequestScoped(t *testing.T) {
	body := []byte(`{"error":{"message":"messages: field required","type":"invalid_request_error"}}`)

	proxyErr := classifyHTTPError(http.StatusUnprocessableEntity, body, http.Header{}, domain.ClientTypeOpenAI, "moonshotai/kimi-k3")

	if proxyErr.Scope != domain.ScopeRequest {
		t.Fatalf("Scope = %v, want ScopeRequest", proxyErr.Scope)
	}
	if proxyErr.Retryable {
		t.Fatal("422 validation error should not be retryable")
	}
}
