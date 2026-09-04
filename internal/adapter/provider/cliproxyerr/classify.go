// Package cliproxyerr classifies errors returned by the CLIProxyAPI SDK
// executors into maxx's ProxyError scope/reason/retryable model.
//
// The SDK surfaces upstream failures as opaque errors whose message is the raw
// upstream response body, and optionally exposes the HTTP status and a retry
// hint through the StatusCode()/RetryAfter() interfaces. The cliproxyapi_*
// adapters used to ignore all of that and label every failure a retryable
// provider-side server error, so a rate-limited or usage-capped account was
// retried until the executor's global attempt ceiling cut the request off —
// proxy_request 75010 burned 200 upstream calls in 99 seconds that way, 100 of
// them after the upstream had already said the quota resets in 2.6 days.
package cliproxyerr

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

const (
	// maxRetryAfterHint caps how long an upstream retry hint may stall the
	// inbound request inside the dispatch retry wait. A usage-limit reset is
	// hours or days out; parking the client connection that long is never
	// right, so longer hints are recorded as cooldown state only and the
	// request fails over to another provider instead.
	maxRetryAfterHint = 30 * time.Second

	// defaultRateLimitCooldown matches the native codex adapter's fallback for
	// a 429 that carries no explicit reset hint.
	defaultRateLimitCooldown = time.Minute

	// maxCooldownHorizon bounds how far out an upstream-reported reset may push
	// a cooldown, so one bogus timestamp cannot park a key indefinitely.
	maxCooldownHorizon = 7 * 24 * time.Hour
)

// Classify wraps err as a ProxyError carrying msg, refining scope, reason and
// retryability from the HTTP status and error body the SDK reports. model names
// the mapped model that produced the error, used as the cooldown key when the
// failure turns out to be model-scoped. fallbackScope and fallbackReason apply
// when the error carries no signal we can read confidently.
func Classify(err error, model, msg string, fallbackScope domain.ErrorScope, fallbackReason domain.CooldownReason) *domain.ProxyError {
	proxyErr := domain.NewProxyErrorWithMessage(err, true, msg)
	proxyErr.Scope = fallbackScope
	proxyErr.Reason = fallbackReason
	proxyErr.Model = model
	if err == nil {
		return proxyErr
	}

	body := err.Error()
	proxyErr.HTTPStatusCode = statusCodeOf(err)
	applyRetryHint(proxyErr, err)

	// The body is the more specific signal and wins: the SDK rewrites a
	// usage-limit rejection to HTTP 429, which the status rules alone would
	// mistake for a transient rate limit worth hammering.
	if classifyBody(proxyErr, body) {
		return proxyErr
	}
	classifyStatus(proxyErr, body)
	return proxyErr
}

// statusCodeOf reads the HTTP status the SDK attached to err, if any.
func statusCodeOf(err error) int {
	var statusErr interface{ StatusCode() int }
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode()
	}
	return 0
}

// applyRetryHint transfers the SDK's Retry-After hint onto proxyErr. The hint
// always becomes cooldown state; it only becomes an in-request retry delay when
// it is short enough to be worth waiting out.
func applyRetryHint(proxyErr *domain.ProxyError, err error) {
	var retryAfterErr interface{ RetryAfter() *time.Duration }
	if !errors.As(err, &retryAfterErr) {
		return
	}
	hint := retryAfterErr.RetryAfter()
	if hint == nil || *hint <= 0 {
		return
	}
	setCooldownUntil(proxyErr, time.Now().Add(*hint))
	if *hint <= maxRetryAfterHint {
		proxyErr.RetryAfter = *hint
	}
}

// classifyBody refines proxyErr from the upstream JSON error body. It reports
// false when the body carries no unambiguous signal, leaving the decision to
// the status code. Patterns are kept narrow on purpose — anything we cannot
// read confidently stays at the caller's fallback so genuine outages still trip
// the wider provider cooldown.
func classifyBody(proxyErr *domain.ProxyError, body string) bool {
	if !gjson.Valid(body) {
		return false
	}
	code := firstString(body, "error.code", "code")
	errType := firstString(body, "error.type", "type")
	msg := firstString(body, "error.message", "message", "detail", "error")

	// Usage/quota exhaustion: the upstream told us when it resets, hours or
	// days out. Retrying the same key before then cannot succeed.
	if code == "usage_limit_reached" || errType == "usage_limit_reached" ||
		code == "insufficient_quota" || code == "billing_hard_limit_reached" ||
		containsAny(msg, "usage limit has been reached", "usage limit reached",
			"insufficient quota", "quota exceeded", "exceeded your current quota",
			"exceeded your quota", "out of quota", "exhausted your quota") {
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonQuotaExhausted
		proxyErr.Retryable = false
		applyQuotaReset(proxyErr, body)
		return true
	}

	// Model-level: do not fail the whole provider over one unavailable model.
	if code == "model_not_found" || code == "model_not_supported" ||
		containsAny(msg, "model is not supported", "model not supported",
			"model is not available", "model not available", "no access to the model",
			"does not have access to model", "does not exist or you do not have access") {
		proxyErr.Scope = domain.ScopeModel
		proxyErr.Reason = domain.CooldownReasonModelUnavailable
		proxyErr.Retryable = false
		return true
	}

	// Key-level auth: the credential is wrong, not the upstream.
	if code == "invalid_api_key" || code == "unauthorized" || code == "permission_denied" ||
		containsAny(msg, "invalid api key", "unauthorized", "authentication failed") {
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		proxyErr.Retryable = false
		return true
	}

	// Rate limiting is genuinely transient: keep it retryable, but scope the
	// cooldown to the key so other providers stay usable.
	if code == "rate_limit_exceeded" || errType == "rate_limit_exceeded" ||
		strings.Contains(msg, "rate limit") {
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonRateLimitExceeded
		proxyErr.Retryable = true
		ensureRateLimitCooldown(proxyErr)
		return true
	}

	return false
}

// classifyStatus maps the upstream HTTP status onto scope/reason/retryable,
// mirroring the native codex adapter's classifyCodexHTTPError. A zero status
// means the SDK gave us nothing to go on, so the caller's fallback stands.
func classifyStatus(proxyErr *domain.ProxyError, body string) {
	switch status := proxyErr.HTTPStatusCode; {
	case status == 0:
		return

	case status == http.StatusBadRequest,
		status == http.StatusRequestEntityTooLarge,
		status == http.StatusUnsupportedMediaType,
		status == http.StatusUnprocessableEntity:
		// The request itself is the problem — no provider will accept it.
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false

	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		proxyErr.Retryable = false

	case status == http.StatusPaymentRequired:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonQuotaExhausted
		proxyErr.Retryable = false

	case status == http.StatusNotFound:
		if proxyErr.Model != "" && strings.Contains(strings.ToLower(body), "model") {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Reason = domain.CooldownReasonModelUnavailable
		} else {
			proxyErr.Scope = domain.ScopeEndpoint
			proxyErr.Reason = domain.CooldownReasonServerError
		}
		proxyErr.Retryable = false

	case status == http.StatusTooManyRequests:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonRateLimitExceeded
		proxyErr.Retryable = true
		ensureRateLimitCooldown(proxyErr)

	case status == http.StatusRequestTimeout:
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonNetworkError
		proxyErr.Retryable = true

	case status >= 500:
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonServerError
		proxyErr.Retryable = true

	case status >= 400:
		// Any other 4xx is a client-side reject; retrying only amplifies load.
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false
	}
}

// applyQuotaReset records the upstream's own reset time as the cooldown
// deadline, so the key is skipped until it can actually serve again.
func applyQuotaReset(proxyErr *domain.ProxyError, body string) {
	for _, path := range []string{"error.resets_at", "resets_at"} {
		if v := gjson.Get(body, path); v.Exists() && v.Int() > 0 {
			setCooldownUntil(proxyErr, time.Unix(v.Int(), 0))
			return
		}
	}
	for _, path := range []string{"error.resets_in_seconds", "resets_in_seconds"} {
		if v := gjson.Get(body, path); v.Exists() && v.Int() > 0 {
			setCooldownUntil(proxyErr, time.Now().Add(time.Duration(v.Int())*time.Second))
			return
		}
	}
}

// ensureRateLimitCooldown gives a rate limit without an explicit reset hint a
// short default cooldown rather than none at all.
func ensureRateLimitCooldown(proxyErr *domain.ProxyError) {
	if proxyErr.CooldownUntil == nil && proxyErr.RetryAfter <= 0 {
		setCooldownUntil(proxyErr, time.Now().Add(defaultRateLimitCooldown))
	}
}

// setCooldownUntil stores until as the cooldown deadline, ignoring times that
// are already past and clamping ones absurdly far out.
func setCooldownUntil(proxyErr *domain.ProxyError, until time.Time) {
	now := time.Now()
	if !until.After(now) {
		return
	}
	if horizon := now.Add(maxCooldownHorizon); until.After(horizon) {
		until = horizon
	}
	proxyErr.CooldownUntil = &until
}

// firstString returns the first non-empty string among the given JSON paths,
// lowercased for comparison against the pattern tables above.
func firstString(body string, paths ...string) string {
	for _, path := range paths {
		if v := gjson.Get(body, path); v.Type == gjson.String && v.String() != "" {
			return strings.ToLower(v.String())
		}
	}
	return ""
}

func containsAny(s string, patterns ...string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
