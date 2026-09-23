package executor

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// forceRetryUpstreamErrorsEnabled reports whether the matched retry policy opts
// into retrying upstream/provider failures that were previously classified as
// non-retryable. It intentionally defaults to false: the normal per-error retry
// classification remains the safe default.
func (e *Executor) forceRetryUpstreamErrorsEnabled(config *domain.RetryConfig) bool {
	return config != nil && config.ForceRetryUpstreamErrors
}

// effectiveMaxRetries preserves the configured retry budget by default. When a
// retry policy explicitly opts into ForceRetryUpstreamErrors, make that opt-in
// meaningful even for configs with MaxRetries=0 by allowing one safe upstream
// retry; request/key/client guards still decide whether a specific error can use
// that budget.
func effectiveMaxRetries(config *domain.RetryConfig) int {
	if config == nil {
		return 0
	}
	if config.ForceRetryUpstreamErrors && config.MaxRetries < 1 {
		return 1
	}
	return config.MaxRetries
}

// forceRetryUpstreamErrorIfSafe upgrades only upstream/provider-side failures
// to retryable when the matched retry policy enables it. Hard safety boundaries stay
// intact: request/client errors, auth/key errors, canceled request contexts,
// committed responses that are not explicitly safe to retry, and exhausted
// retry budgets are still handled by the dispatch loop.
func forceRetryUpstreamErrorIfSafe(proxyErr *domain.ProxyError, ctx context.Context, responseCommitted bool, enabled bool) bool {
	if !enabled || proxyErr == nil || proxyErr.Retryable {
		return false
	}
	if !safeUpstreamRetryBoundary(proxyErr, ctx, responseCommitted) {
		return false
	}

	proxyErr.Retryable = true
	return true
}

func safeUpstreamRetryBoundary(proxyErr *domain.ProxyError, ctx context.Context, responseCommitted bool) bool {
	if proxyErr == nil {
		return false
	}
	if ctx != nil && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return false
	}
	if responseCommitted && !shouldRetryCommittedResponseError(proxyErr) {
		return false
	}
	if proxyErr.Scope == domain.ScopeRequest || proxyErr.Scope == domain.ScopeKey {
		return false
	}
	if proxyErr.HTTPStatusCode >= http.StatusBadRequest && proxyErr.HTTPStatusCode < http.StatusInternalServerError && proxyErr.HTTPStatusCode != http.StatusTooManyRequests {
		return false
	}

	switch proxyErr.Scope {
	case domain.ScopeProvider, domain.ScopeEndpoint, domain.ScopeModel:
		return true
	default:
		return false
	}
}

func ensureRetryableUpstreamErrorHasBudget(maxRetries int, attempt int, allowPolicyRetryExtension bool, proxyErr *domain.ProxyError, ctx context.Context, responseCommitted bool) int {
	if proxyErr == nil || !proxyErr.Retryable || !safeUpstreamRetryBoundary(proxyErr, ctx, responseCommitted) {
		return maxRetries
	}
	if proxyErr.HTTPStatusCode != 0 || proxyErr.Reason != domain.CooldownReasonNetworkError || !errors.Is(proxyErr.Err, domain.ErrUpstreamError) {
		return maxRetries
	}
	if proxyErr.UpstreamFailurePhase != domain.UpstreamFailurePhaseConnect || !strings.HasPrefix(proxyErr.Message, "failed to connect") {
		return maxRetries
	}
	neededBudget := 1
	if allowPolicyRetryExtension {
		neededBudget = attempt + 1
	}
	if maxRetries >= neededBudget {
		return maxRetries
	}
	return neededBudget
}

func proxyErrorScopeForLog(proxyErr *domain.ProxyError) domain.ErrorScope {
	if proxyErr == nil {
		return ""
	}
	return proxyErr.Scope
}

func proxyErrorReasonForLog(proxyErr *domain.ProxyError) domain.CooldownReason {
	if proxyErr == nil {
		return ""
	}
	return proxyErr.Reason
}
