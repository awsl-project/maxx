package executor

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// forceRetryUpstreamErrorsEnabled reports whether the admin opted into
// retrying upstream/provider failures that were previously classified as
// non-retryable. It intentionally defaults to false: the normal per-error
// retry classification remains the safe default.
func (e *Executor) forceRetryUpstreamErrorsEnabled() bool {
	if e == nil || e.settingsRepo == nil {
		return false
	}
	value, err := e.settingsRepo.Get(domain.SettingKeyForceRetryUpstreamErrors)
	return err == nil && strings.EqualFold(strings.TrimSpace(value), "true")
}

// forceRetryUpstreamErrorIfSafe upgrades only upstream/provider-side failures
// to retryable when the admin setting is enabled. Hard safety boundaries stay
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
