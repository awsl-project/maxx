package executor

import (
	"context"
	"errors"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
)

// forceRetryUpstreamErrorsEnabled reports whether the matched retry policy opts
// into retrying upstream/provider failures that were previously classified as
// non-retryable. It intentionally defaults to false: the normal per-error retry
// classification remains the safe default.
func (e *Executor) forceRetryUpstreamErrorsEnabled(config *domain.RetryConfig) bool {
	return config != nil && config.ForceRetryUpstreamErrors
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
		proxyErr.Retryable = true
		return true
	default:
		return false
	}
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
