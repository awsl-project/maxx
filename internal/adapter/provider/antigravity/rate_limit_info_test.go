package antigravity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func TestParseRateLimitInfoRateLimitDoesNotUseQuotaDefault(t *testing.T) {
	adapter := &AntigravityAdapter{}
	body := []byte(`{
		"error": {
			"code": 429,
			"message": "Resource has been exhausted. Requests per minute exceeded.",
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED"},
				{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"3s"}
			]
		}
	}`)

	scope, reason, until, updateChan := adapter.parseRateLimitInfo(context.Background(), body, nil)
	if scope != domain.ScopeKey {
		t.Fatalf("scope = %q, want %q", scope, domain.ScopeKey)
	}
	if reason != domain.CooldownReasonRateLimitExceeded {
		t.Fatalf("reason = %q, want %q", reason, domain.CooldownReasonRateLimitExceeded)
	}
	if until != nil {
		t.Fatalf("rate limit should not force quota cooldown until, got %v", until)
	}
	if updateChan != nil {
		t.Fatal("rate limit should not start quota async update")
	}
}

func TestParseRateLimitInfoQuotaWithoutResetUsesDefault(t *testing.T) {
	adapter := &AntigravityAdapter{}
	provider := &domain.Provider{Config: &domain.ProviderConfig{}}
	body := []byte(`{
		"error": {
			"code": 429,
			"message": "Quota exhausted for quota metric",
			"status": "RESOURCE_EXHAUSTED",
			"details": [{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED"}]
		}
	}`)

	scope, reason, until, updateChan := adapter.parseRateLimitInfo(context.Background(), body, provider)
	if scope != domain.ScopeKey {
		t.Fatalf("scope = %q, want %q", scope, domain.ScopeKey)
	}
	if reason != domain.CooldownReasonQuotaExhausted {
		t.Fatalf("reason = %q, want %q", reason, domain.CooldownReasonQuotaExhausted)
	}
	if until == nil || time.Until(*until) < 55*time.Second || time.Until(*until) > 65*time.Second {
		t.Fatalf("quota fallback until = %v, want about 60s", until)
	}
	if updateChan != nil {
		t.Fatal("nil antigravity config should not start quota async update")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestExecuteRateLimitResponsePreservesShortRetryInfo(t *testing.T) {
	var sawUpstreamRequest bool

	provider := &domain.Provider{
		ID:   9001,
		Name: "mock-antigravity-rate-limit",
		Type: "antigravity",
		Config: &domain.ProviderConfig{Antigravity: &domain.ProviderConfigAntigravity{
			Email:        "mock@example.com",
			RefreshToken: "unused-in-test",
			ProjectID:    "mock-project",
			Endpoint:     "https://cloudcode-pa.googleapis.com",
		}},
	}
	adapter := &AntigravityAdapter{
		provider: provider,
		tokenCache: &TokenCache{
			AccessToken: "test-access-token",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			sawUpstreamRequest = true
			if got := r.Header.Get("Authorization"); got != "Bearer test-access-token" {
				t.Fatalf("Authorization = %q, want bearer token", got)
			}
			if !strings.Contains(r.URL.String(), "cloudcode-pa.googleapis.com") {
				t.Fatalf("upstream URL = %q, want Antigravity endpoint", r.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"error": {
						"code": 429,
						"message": "Resource exhausted. Requests per minute exceeded.",
						"status": "RESOURCE_EXHAUSTED",
						"details": [
							{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED"},
							{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"3s"}
						]
					}
				}`)),
			}, nil
		})},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(`{}`))
	c := flow.NewCtx(httptest.NewRecorder(), req)
	c.Set(flow.KeyClientType, domain.ClientTypeGemini)
	c.Set(flow.KeyRequestModel, "gemini-2.5-pro")
	c.Set(flow.KeyMappedModel, "gemini-2.5-pro")
	c.Set(flow.KeyRequestBody, []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`))

	err := adapter.Execute(c, provider)
	if !sawUpstreamRequest {
		t.Fatalf("expected adapter to call mock Antigravity upstream, got err=%T %v", err, err)
	}
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error type = %T, want *domain.ProxyError: %v", err, err)
	}
	if proxyErr.Scope != domain.ScopeKey {
		t.Fatalf("Scope = %q, want %q", proxyErr.Scope, domain.ScopeKey)
	}
	if proxyErr.Reason != domain.CooldownReasonRateLimitExceeded {
		t.Fatalf("Reason = %q, want %q", proxyErr.Reason, domain.CooldownReasonRateLimitExceeded)
	}
	if proxyErr.CooldownUntil != nil {
		t.Fatalf("CooldownUntil = %v, want nil for request-rate throttle", proxyErr.CooldownUntil)
	}
	if proxyErr.RetryAfter < 3*time.Second || proxyErr.RetryAfter > 4*time.Second {
		t.Fatalf("RetryAfter = %v, want RetryInfo-derived short delay around 3s", proxyErr.RetryAfter)
	}
}
