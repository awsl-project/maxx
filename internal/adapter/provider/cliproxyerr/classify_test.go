package cliproxyerr

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// sdkErr mimics the CLIProxyAPI SDK's statusErr: its message is the raw
// upstream body, and it optionally exposes the HTTP status and a retry hint.
type sdkErr struct {
	body       string
	code       int
	retryAfter *time.Duration
}

func (e sdkErr) Error() string              { return e.body }
func (e sdkErr) StatusCode() int            { return e.code }
func (e sdkErr) RetryAfter() *time.Duration { return e.retryAfter }

// usageLimitReset is the reset horizon proxy_request 75010 was given, 2.6 days
// out. The upstream reports it as an absolute instant, so the fixture builds it
// relative to now: a hard-coded timestamp would go stale and silently stop
// exercising the cooldown path it is meant to pin.
const usageLimitReset = 227596 * time.Second

// usageLimitBody renders the payload proxy_request 75010 received 9 times and
// then retried 100 more times, with its reset pointed at resetsAt.
func usageLimitBody(resetsAt time.Time) string {
	return fmt.Sprintf(`{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","plan_type":"prolite","resets_at":%d,"eligible_promo":null,"resets_in_seconds":%d}}`,
		resetsAt.Unix(), int(time.Until(resetsAt).Seconds()))
}

func TestClassifyUsageLimitReachedIsNotRetryable(t *testing.T) {
	resetsAt := time.Now().Add(usageLimitReset)
	// The SDK rewrites usage_limit_reached to HTTP 429, so status alone would
	// call this a transient throttle worth retrying.
	proxyErr := Classify(sdkErr{body: usageLimitBody(resetsAt), code: 429}, "gpt-5.6-sol", "executor stream request failed",
		domain.ScopeProvider, domain.CooldownReasonServerError)

	if proxyErr.Retryable {
		t.Error("usage_limit_reached must not be retryable")
	}
	if proxyErr.Scope != domain.ScopeKey {
		t.Errorf("scope = %q, want %q", proxyErr.Scope, domain.ScopeKey)
	}
	if proxyErr.Reason != domain.CooldownReasonQuotaExhausted {
		t.Errorf("reason = %q, want %q", proxyErr.Reason, domain.CooldownReasonQuotaExhausted)
	}
	if proxyErr.CooldownUntil == nil {
		t.Fatal("expected a cooldown deadline from resets_at")
	}
	if got := proxyErr.CooldownUntil.Unix(); got != resetsAt.Unix() {
		t.Errorf("cooldown until = %d, want %d (resets_at)", got, resetsAt.Unix())
	}
	// A 2.6-day reset must never become an in-request retry wait.
	if proxyErr.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0 (must fail over, not park the client)", proxyErr.RetryAfter)
	}
}

func TestClassifyUsageLimitFromResetsInSecondsOnly(t *testing.T) {
	body := `{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","resets_in_seconds":600}}`
	proxyErr := Classify(sdkErr{body: body, code: 429}, "gpt-5.6-sol", "msg", domain.ScopeProvider, domain.CooldownReasonServerError)

	if proxyErr.CooldownUntil == nil {
		t.Fatal("expected a cooldown deadline from resets_in_seconds")
	}
	if d := time.Until(*proxyErr.CooldownUntil); d < 9*time.Minute || d > 11*time.Minute {
		t.Errorf("cooldown in %v, want ~10m", d)
	}
}

func TestClassifyUsageLimitFallsBackWhenResetsAtIsStale(t *testing.T) {
	// resets_at is absolute and goes stale under clock skew between us and the
	// upstream; the relative resets_in_seconds must still park the key.
	body := `{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","resets_at":1,"resets_in_seconds":600}}`
	proxyErr := Classify(sdkErr{body: body, code: 429}, "gpt-5.6-sol", "msg", domain.ScopeProvider, domain.CooldownReasonServerError)

	if proxyErr.CooldownUntil == nil {
		t.Fatal("a stale resets_at must not swallow the resets_in_seconds fallback")
	}
	if d := time.Until(*proxyErr.CooldownUntil); d < 9*time.Minute || d > 11*time.Minute {
		t.Errorf("cooldown in %v, want ~10m", d)
	}
}

func TestClassifyRateLimitStaysRetryable(t *testing.T) {
	// The other 191 attempts on proxy_request 75010.
	proxyErr := Classify(sdkErr{body: `{"detail":"Rate limit exceeded"}`, code: 429}, "gpt-5.6-sol", "msg",
		domain.ScopeProvider, domain.CooldownReasonServerError)

	if !proxyErr.Retryable {
		t.Error("a plain rate limit should stay retryable")
	}
	if proxyErr.Scope != domain.ScopeKey {
		t.Errorf("scope = %q, want %q (cooldown the key, not the provider)", proxyErr.Scope, domain.ScopeKey)
	}
	if proxyErr.Reason != domain.CooldownReasonRateLimitExceeded {
		t.Errorf("reason = %q, want %q", proxyErr.Reason, domain.CooldownReasonRateLimitExceeded)
	}
	if proxyErr.CooldownUntil == nil {
		t.Error("expected a default rate-limit cooldown")
	}
}

func TestClassifyRateLimitBodyWithoutStatus(t *testing.T) {
	// CLIProxyAPI does not always attach a status; the body alone must suffice.
	proxyErr := Classify(errors.New(`{"detail":"Rate limit exceeded"}`), "gpt-5.6-sol", "msg",
		domain.ScopeProvider, domain.CooldownReasonServerError)

	if proxyErr.Scope != domain.ScopeKey || proxyErr.Reason != domain.CooldownReasonRateLimitExceeded {
		t.Errorf("scope/reason = %q/%q, want key/rate_limit_exceeded", proxyErr.Scope, proxyErr.Reason)
	}
}

func TestClassifyStatusCodes(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		scope     domain.ErrorScope
		reason    domain.CooldownReason
		retryable bool
	}{
		{"bad request", 400, `{"error":{"message":"bad payload"}}`, domain.ScopeRequest, domain.CooldownReasonServerError, false},
		{"payload too large", 413, `{}`, domain.ScopeRequest, domain.CooldownReasonServerError, false},
		{"unauthorized", 401, `{}`, domain.ScopeKey, domain.CooldownReasonAuthFailure, false},
		{"forbidden", 403, `{}`, domain.ScopeKey, domain.CooldownReasonAuthFailure, false},
		{"payment required", 402, `{}`, domain.ScopeKey, domain.CooldownReasonQuotaExhausted, false},
		{"not found", 404, `{"error":{"message":"no such endpoint"}}`, domain.ScopeEndpoint, domain.CooldownReasonServerError, false},
		{"request timeout", 408, `{}`, domain.ScopeProvider, domain.CooldownReasonNetworkError, true},
		{"server error", 500, `{}`, domain.ScopeProvider, domain.CooldownReasonServerError, true},
		{"bad gateway", 502, `{}`, domain.ScopeProvider, domain.CooldownReasonServerError, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyErr := Classify(sdkErr{body: tt.body, code: tt.status}, "gpt-5.6-sol", "msg",
				domain.ScopeProvider, domain.CooldownReasonServerError)
			if proxyErr.HTTPStatusCode != tt.status {
				t.Errorf("status = %d, want %d", proxyErr.HTTPStatusCode, tt.status)
			}
			if proxyErr.Scope != tt.scope {
				t.Errorf("scope = %q, want %q", proxyErr.Scope, tt.scope)
			}
			if proxyErr.Retryable != tt.retryable {
				t.Errorf("retryable = %v, want %v", proxyErr.Retryable, tt.retryable)
			}
			// ScopeRequest carries no cooldown reason; it never reaches a cooldown.
			if tt.scope != domain.ScopeRequest && proxyErr.Reason != tt.reason {
				t.Errorf("reason = %q, want %q", proxyErr.Reason, tt.reason)
			}
		})
	}
}

func TestClassifyModelNotSupportedIsModelScoped(t *testing.T) {
	body := `{"detail":"The model is not supported when using Codex with a ChatGPT account"}`
	proxyErr := Classify(sdkErr{body: body, code: 400}, "gpt-5.6-sol", "msg",
		domain.ScopeProvider, domain.CooldownReasonServerError)

	if proxyErr.Scope != domain.ScopeModel {
		t.Errorf("scope = %q, want %q (must not freeze the whole provider)", proxyErr.Scope, domain.ScopeModel)
	}
	if proxyErr.Reason != domain.CooldownReasonModelUnavailable {
		t.Errorf("reason = %q, want %q", proxyErr.Reason, domain.CooldownReasonModelUnavailable)
	}
	if proxyErr.Model != "gpt-5.6-sol" {
		t.Errorf("model = %q, want the mapped model as the cooldown key", proxyErr.Model)
	}
	if proxyErr.Retryable {
		t.Error("an unsupported model must not be retried on the same provider")
	}
}

func TestClassifyKeepsFallbackForUnreadableErrors(t *testing.T) {
	// A bare transport error: no status, no JSON body. The caller's fallback
	// classification must survive so genuine outages still trip the cooldown.
	proxyErr := Classify(errors.New("connection reset by peer"), "gpt-5.6-sol", "stream chunk error",
		domain.ScopeProvider, domain.CooldownReasonNetworkError)

	if proxyErr.Scope != domain.ScopeProvider || proxyErr.Reason != domain.CooldownReasonNetworkError {
		t.Errorf("scope/reason = %q/%q, want the fallback provider/network_error", proxyErr.Scope, proxyErr.Reason)
	}
	if !proxyErr.Retryable {
		t.Error("an unclassified transport error should stay retryable")
	}
	if proxyErr.HTTPStatusCode != 0 {
		t.Errorf("status = %d, want 0", proxyErr.HTTPStatusCode)
	}
}

func TestClassifyRetryHintBounds(t *testing.T) {
	short := 5 * time.Second
	long := 48 * time.Hour

	shortErr := Classify(sdkErr{body: `{"detail":"Rate limit exceeded"}`, code: 429, retryAfter: &short},
		"gpt-5.6-sol", "msg", domain.ScopeProvider, domain.CooldownReasonServerError)
	if shortErr.RetryAfter != short {
		t.Errorf("RetryAfter = %v, want %v (short hints are worth waiting out)", shortErr.RetryAfter, short)
	}

	longErr := Classify(sdkErr{body: `{"detail":"Rate limit exceeded"}`, code: 429, retryAfter: &long},
		"gpt-5.6-sol", "msg", domain.ScopeProvider, domain.CooldownReasonServerError)
	if longErr.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0 (a 48h hint must fail over, not block the client)", longErr.RetryAfter)
	}
	if longErr.CooldownUntil == nil {
		t.Fatal("a long hint should still become cooldown state")
	}
	if d := time.Until(*longErr.CooldownUntil); d > maxCooldownHorizon {
		t.Errorf("cooldown in %v, want clamped to %v", d, maxCooldownHorizon)
	}
}

func TestClassifyNilError(t *testing.T) {
	proxyErr := Classify(nil, "gpt-5.6-sol", "msg", domain.ScopeProvider, domain.CooldownReasonServerError)
	if proxyErr == nil {
		t.Fatal("Classify(nil) must still return a ProxyError")
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Errorf("scope = %q, want the fallback", proxyErr.Scope)
	}
}
