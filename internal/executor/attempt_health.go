package executor

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/health"
)

func (e *Executor) normalizeAttemptError(parentCtx context.Context, attemptCtx context.Context, timeoutErr error, err error, responseStarted bool) error {
	if timeoutErr == nil && err == nil {
		return nil
	}
	if parentCtx != nil && parentCtx.Err() != nil {
		if err != nil {
			return err
		}
		return parentCtx.Err()
	}
	if responseStarted {
		return normalizeResponseStartedError(attemptCtx, timeoutErr, err)
	}
	if timeoutErr != nil {
		if proxyErr, ok := err.(*domain.ProxyError); ok {
			proxyErr.Retryable = true
			proxyErr.IsNetworkError = true
			if proxyErr.HTTPStatusCode == 0 {
				proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
			}
			proxyErr.Err = timeoutErr
			return proxyErr
		}

		proxyErr := domain.NewProxyErrorWithMessage(timeoutErr, true, "provider attempt timed out")
		proxyErr.IsNetworkError = true
		proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
		return proxyErr
	}
	if attemptCtx == nil || !errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return err
	}

	if proxyErr, ok := err.(*domain.ProxyError); ok {
		proxyErr.Retryable = true
		proxyErr.IsNetworkError = true
		if proxyErr.HTTPStatusCode == 0 {
			proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
		}
		if !errors.Is(proxyErr.Err, context.DeadlineExceeded) {
			proxyErr.Err = context.DeadlineExceeded
		}
		return proxyErr
	}

	proxyErr := domain.NewProxyErrorWithMessage(context.DeadlineExceeded, true, "provider attempt timed out")
	proxyErr.IsNetworkError = true
	proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
	return proxyErr
}

func normalizeResponseStartedError(attemptCtx context.Context, timeoutErr error, err error) error {
	if timeoutErr != nil {
		if proxyErr, ok := err.(*domain.ProxyError); ok {
			proxyErr.Retryable = false
			proxyErr.IsNetworkError = true
			if proxyErr.HTTPStatusCode == 0 {
				proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
			}
			proxyErr.Err = timeoutErr
			return proxyErr
		}

		proxyErr := domain.NewProxyErrorWithMessage(timeoutErr, false, "provider attempt timed out after response started")
		proxyErr.IsNetworkError = true
		proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
		return proxyErr
	}
	if attemptCtx != nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		if proxyErr, ok := err.(*domain.ProxyError); ok {
			proxyErr.Retryable = false
			proxyErr.IsNetworkError = true
			if proxyErr.HTTPStatusCode == 0 {
				proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
			}
			if !errors.Is(proxyErr.Err, context.DeadlineExceeded) {
				proxyErr.Err = context.DeadlineExceeded
			}
			return proxyErr
		}

		proxyErr := domain.NewProxyErrorWithMessage(context.DeadlineExceeded, false, "provider attempt timed out after response started")
		proxyErr.IsNetworkError = true
		proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
		return proxyErr
	}
	if proxyErr, ok := err.(*domain.ProxyError); ok {
		proxyErr.Retryable = false
		return proxyErr
	}
	return err
}

func (e *Executor) shouldRecordAttemptHealth(parentCtx context.Context) bool {
	if e.healthTracker == nil {
		return false
	}
	return parentCtx == nil || parentCtx.Err() == nil
}

func (e *Executor) recordAttemptHealth(
	providerID uint64,
	clientType domain.ClientType,
	attempt *domain.ProxyUpstreamAttempt,
	statusCode int,
	err error,
) {
	if e.healthTracker == nil || attempt == nil {
		return
	}

	result := health.AttemptResult{
		ProviderID: providerID,
		ClientType: string(clientType),
		Success:    err == nil,
		StartedAt:  attempt.StartTime,
		Duration:   attempt.Duration,
		TTFT:       attempt.TTFT,
		StatusCode: statusCode,
	}

	if err == nil {
		e.healthTracker.Record(result)
		return
	}

	if proxyErr, ok := err.(*domain.ProxyError); ok {
		if result.StatusCode == 0 {
			result.StatusCode = proxyErr.HTTPStatusCode
		}
		result.IsTimeout = errors.Is(proxyErr.Err, context.DeadlineExceeded) ||
			errors.Is(proxyErr.Err, domain.ErrFirstByteTimeout) ||
			errors.Is(proxyErr.Err, domain.ErrStreamIdleTimeout)
		result.IsNetwork = proxyErr.IsNetworkError || result.IsTimeout
		result.IsRateLimit = proxyErr.RateLimitInfo != nil ||
			proxyErr.RetryAfter > 0 ||
			result.StatusCode == http.StatusTooManyRequests
		result.IsServerErr = proxyErr.IsServerError || result.StatusCode >= http.StatusInternalServerError
	} else {
		result.IsTimeout = errors.Is(err, context.DeadlineExceeded)
		result.IsNetwork = result.IsTimeout
	}

	e.healthTracker.Record(result)
}

func (e *Executor) capRetryWait(wait time.Duration, requestStart time.Time, fromRetryAfter bool) time.Duration {
	if wait <= 0 {
		return 0
	}
	if fromRetryAfter && e.attemptBudget.MaxRetryAfter > 0 && wait > e.attemptBudget.MaxRetryAfter {
		wait = e.attemptBudget.MaxRetryAfter
	}

	wait = e.attemptBudget.ClampRetryWait(wait)
	if e.attemptBudget.RequestTimeout > 0 {
		remaining := e.attemptBudget.RequestRemainingSince(requestStart)
		if remaining <= 0 {
			return 0
		}
		if wait > remaining {
			return remaining
		}
	}
	return wait
}

func (e *Executor) releaseHalfOpenProbeIfNeeded(
	providerID uint64,
	clientType domain.ClientType,
	attempt *domain.ProxyUpstreamAttempt,
	recordedHealth bool,
) {
	if recordedHealth || e.healthTracker == nil || attempt == nil {
		return
	}
	e.healthTracker.ReleaseHalfOpenProbe(providerID, string(clientType), attempt.StartTime)
}
