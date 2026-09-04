package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/cliproxyerr"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

// classifiedErrorAdapter always returns a pre-classified ProxyError and counts
// how many times Execute is invoked. It mirrors what the real custom adapter's
// classifyHTTPError produces for a given upstream status, so dispatch-level
// retry/failover behaviour can be exercised end-to-end.
type classifiedErrorAdapter struct {
	calls int
	build func() *domain.ProxyError
}

func (a *classifiedErrorAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *classifiedErrorAdapter) Execute(_ *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	// Return a genuine nil error on success — returning a typed-nil
	// *domain.ProxyError would produce a non-nil error interface and be
	// mistaken for a failure.
	if pe := a.build(); pe != nil {
		return pe
	}
	return nil
}

func newAmplificationDispatchCtx(disableErrorCooldown bool, adapter *classifiedErrorAdapter) (*flow.Ctx, *recordingProxyRequestRepo) {
	return newMultiRouteAmplificationDispatchCtx(1, disableErrorCooldown, adapter)
}

// newMultiRouteAmplificationDispatchCtx builds a dispatch context fronted by
// routeCount routes that all share the same failing adapter, so a test can
// exercise failover fan-out as well as single-provider retry behaviour.
func newMultiRouteAmplificationDispatchCtx(routeCount int, disableErrorCooldown bool, adapter *classifiedErrorAdapter) (*flow.Ctx, *recordingProxyRequestRepo) {
	proxyRepo := &recordingProxyRequestRepo{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         900,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	routes := make([]*router.MatchedRoute, 0, routeCount)
	for i := range routeCount {
		routes = append(routes, &router.MatchedRoute{
			Route: &domain.Route{ID: uint64(10 + i), TenantID: domain.DefaultTenantID, ProviderID: uint64(20 + i), ClientType: domain.ClientTypeOpenAI},
			Provider: &domain.Provider{
				ID:       uint64(20 + i),
				TenantID: domain.DefaultTenantID,
				Type:     "custom",
				Name:     "custom-disabled-cooldown-amplification",
				Config:   &domain.ProviderConfig{DisableErrorCooldown: disableErrorCooldown},
			},
			ProviderAdapter: adapter,
			// InitialInterval 0 reproduces the production config (empty
			// retry_configs → 0 backoff) that let the loop spin at ~10/sec.
			RetryConfig: &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
		})
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "gpt-image-model",
		routes:       routes,
	}
	c.Set(flow.KeyExecutorState, state)
	return c, proxyRepo
}

// TestDispatchDoesNotAmplify404ToolUseError reproduces proxy_request_id=54414:
// a 404 "No endpoints found that support tool use" against an image provider
// with disableErrorCooldown. Before the fix this retried 151,780 times; it must
// now fail after a single upstream call.
func TestDispatchDoesNotAmplify404ToolUseError(t *testing.T) {
	adapter := &classifiedErrorAdapter{build: func() *domain.ProxyError {
		// Mirrors custom.classifyHTTPError(404, "No endpoints found that
		// support tool use"): no "model"/"not found" substring → ScopeEndpoint,
		// Retryable=false.
		pe := domain.NewProxyErrorWithMessage(
			errors.New(`{"error":{"message":"No endpoints found that support tool use"}}`),
			false,
			"upstream returned status 404",
		)
		pe.Scope = domain.ScopeEndpoint
		pe.Reason = domain.CooldownReasonServerError
		pe.HTTPStatusCode = http.StatusNotFound
		return pe
	}}
	c, proxyRepo := newAmplificationDispatchCtx(true, adapter)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch to fail on non-retryable 404")
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1 (404 tool-use must not amplify)", adapter.calls)
	}
	if last := lastProxyStatus(proxyRepo); last != "FAILED" {
		t.Fatalf("final proxy status = %q, want FAILED", last)
	}
}

// TestDispatchDoesNotAmplifyInvalidImageError reproduces the 400 "invalid
// image" case (11 requests → 4,321 calls before the fix).
func TestDispatchDoesNotAmplifyInvalidImageError(t *testing.T) {
	adapter := &classifiedErrorAdapter{build: func() *domain.ProxyError {
		pe := domain.NewProxyErrorWithMessage(
			errors.New(`{"error":{"message":"The image data you provided does not represent a valid image"}}`),
			false,
			"upstream returned status 400",
		)
		pe.Scope = domain.ScopeRequest
		pe.HTTPStatusCode = http.StatusBadRequest
		return pe
	}}
	c, proxyRepo := newAmplificationDispatchCtx(true, adapter)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch to fail on non-retryable 400")
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1 (invalid-image 400 must not amplify)", adapter.calls)
	}
	if last := lastProxyStatus(proxyRepo); last != "FAILED" {
		t.Fatalf("final proxy status = %q, want FAILED", last)
	}
}

// newAlways500Adapter returns an adapter whose every call fails with a
// retryable 5xx — under disableErrorCooldown this is the shape that used to
// retry until the global ceiling stopped it.
func newAlways500Adapter() *classifiedErrorAdapter {
	return &classifiedErrorAdapter{build: func() *domain.ProxyError {
		pe := domain.NewProxyErrorWithMessage(
			errors.New("upstream returned 500"),
			true,
			"upstream returned status 500",
		)
		pe.Scope = domain.ScopeProvider
		pe.Reason = domain.CooldownReasonServerError
		pe.HTTPStatusCode = http.StatusInternalServerError
		return pe
	}}
}

// TestDispatchCapsRetriesWhenErrorCooldownDisabled pins the per-provider budget:
// disableErrorCooldown still buys far more retries than the RetryConfig allows,
// but no longer an unbounded loop against one provider.
func TestDispatchCapsRetriesWhenErrorCooldownDisabled(t *testing.T) {
	adapter := newAlways500Adapter()
	c, proxyRepo := newAmplificationDispatchCtx(true, adapter)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch to fail once the per-provider cap is hit")
	}
	if adapter.calls != maxAttemptsPerProviderWithoutErrorCooldown {
		t.Fatalf("adapter calls = %d, want %d (per-provider cap)", adapter.calls, maxAttemptsPerProviderWithoutErrorCooldown)
	}
	if last := lastProxyStatus(proxyRepo); last != "FAILED" {
		t.Fatalf("final proxy status = %q, want FAILED", last)
	}
}

// TestDispatchHardCapsRunawayRetryableErrors is the backstop test: even a
// genuinely-retryable error that (via a classification bug or misconfiguration)
// never stops retrying must be bounded by the hard per-request ceiling, instead
// of running for hours. Enough routes are matched that the per-provider cap
// alone would allow more calls than the ceiling permits.
func TestDispatchHardCapsRunawayRetryableErrors(t *testing.T) {
	routeCount := maxUpstreamAttemptsPerRequest/maxAttemptsPerProviderWithoutErrorCooldown + 1
	adapter := newAlways500Adapter()
	c, proxyRepo := newMultiRouteAmplificationDispatchCtx(routeCount, true, adapter)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch to fail once the hard cap is hit")
	}
	if adapter.calls != maxUpstreamAttemptsPerRequest {
		t.Fatalf("adapter calls = %d, want %d (hard ceiling)", adapter.calls, maxUpstreamAttemptsPerRequest)
	}
	if last := lastProxyStatus(proxyRepo); last != "FAILED" {
		t.Fatalf("final proxy status = %q, want FAILED", last)
	}
}

// cliProxySDKErr mimics the CLIProxyAPI SDK's status-carrying error: the
// message is the raw upstream body and StatusCode reports the HTTP status.
type cliProxySDKErr struct {
	body string
	code int
}

func (e cliProxySDKErr) Error() string   { return e.body }
func (e cliProxySDKErr) StatusCode() int { return e.code }

// TestDispatchDoesNotRetryUsageLimitUnderDisabledCooldown reproduces
// proxy_request 75010 end to end, through the real adapter-side classifier: a
// Codex account whose quota resets in 2.6 days answered "usage_limit_reached",
// which the SDK reports as HTTP 429. Treating that as a transient throttle got
// it retried 100 more times. It must now cost exactly one upstream call.
func TestDispatchDoesNotRetryUsageLimitUnderDisabledCooldown(t *testing.T) {
	const usageLimitBody = `{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","plan_type":"prolite","resets_at":1788747900,"resets_in_seconds":227596}}`
	adapter := &classifiedErrorAdapter{build: func() *domain.ProxyError {
		return cliproxyerr.Classify(
			cliProxySDKErr{body: usageLimitBody, code: http.StatusTooManyRequests},
			"gpt-5.6-sol",
			"executor stream request failed",
			domain.ScopeProvider, domain.CooldownReasonServerError,
		)
	}}
	c, proxyRepo := newAmplificationDispatchCtx(true, adapter)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch to fail on an exhausted usage limit")
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1 (a multi-day usage limit must not be retried)", adapter.calls)
	}
	if last := lastProxyStatus(proxyRepo); last != "FAILED" {
		t.Fatalf("final proxy status = %q, want FAILED", last)
	}
}

// TestDispatchCapsRateLimitRetriesUnderDisabledCooldown covers the other 191
// attempts on proxy_request 75010: a plain 429 stays retryable, but under
// disableErrorCooldown it must exhaust the per-provider budget rather than loop
// until the global ceiling.
func TestDispatchCapsRateLimitRetriesUnderDisabledCooldown(t *testing.T) {
	adapter := &classifiedErrorAdapter{build: func() *domain.ProxyError {
		return cliproxyerr.Classify(
			cliProxySDKErr{body: `{"detail":"Rate limit exceeded"}`, code: http.StatusTooManyRequests},
			"gpt-5.6-sol",
			"executor stream request failed",
			domain.ScopeProvider, domain.CooldownReasonServerError,
		)
	}}
	c, proxyRepo := newAmplificationDispatchCtx(true, adapter)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch to fail once the rate limit persisted")
	}
	if adapter.calls != maxAttemptsPerProviderWithoutErrorCooldown {
		t.Fatalf("adapter calls = %d, want %d (per-provider cap)", adapter.calls, maxAttemptsPerProviderWithoutErrorCooldown)
	}
	if last := lastProxyStatus(proxyRepo); last != "FAILED" {
		t.Fatalf("final proxy status = %q, want FAILED", last)
	}
}

// TestDispatchStillRetriesServerErrorUnderCap makes sure the fix does not
// over-correct: a 5xx that eventually succeeds must still be retried/failed over
// (not treated as a terminal client error).
func TestDispatchStillRetriesServerErrorUnderCap(t *testing.T) {
	calls := 0
	adapter := &classifiedErrorAdapter{build: func() *domain.ProxyError {
		calls++
		if calls > 2 {
			return nil // succeed on the 3rd attempt
		}
		pe := domain.NewProxyErrorWithMessage(
			errors.New("upstream returned 503"),
			true,
			"upstream returned status 503",
		)
		pe.Scope = domain.ScopeProvider
		pe.Reason = domain.CooldownReasonServerError
		pe.HTTPStatusCode = http.StatusServiceUnavailable
		return pe
	}}
	c, proxyRepo := newAmplificationDispatchCtx(true, adapter)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 3 {
		t.Fatalf("adapter calls = %d, want 3 (retry until success)", adapter.calls)
	}
	if last := lastProxyStatus(proxyRepo); last != "COMPLETED" {
		t.Fatalf("final proxy status = %q, want COMPLETED", last)
	}
}

// TestDispatchStopsPromptlyWhenInboundContextCanceled verifies the retry/
// failover loop terminates soon after the inbound client disconnects, rather
// than continuing to hammer upstream for hours.
func TestDispatchStopsPromptlyWhenInboundContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         901,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	// The adapter cancels the inbound context on its first call, then returns a
	// retryable error. Under disableErrorCooldown this error would otherwise be
	// retried forever; the loop must instead observe the cancellation and stop.
	// Cancelling from inside Execute (rather than a spinning reader goroutine)
	// keeps the call counter race-free.
	adapter := &classifiedErrorAdapter{build: func() *domain.ProxyError {
		cancel()
		pe := domain.NewProxyErrorWithMessage(
			errors.New("upstream returned 500"),
			true,
			"upstream returned status 500",
		)
		pe.Scope = domain.ScopeProvider
		pe.Reason = domain.CooldownReasonServerError
		pe.HTTPStatusCode = http.StatusInternalServerError
		return pe
	}}
	state := &execState{
		ctx:          ctx,
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "gpt-4o",
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 20, ClientType: domain.ClientTypeOpenAI},
				Provider: &domain.Provider{
					ID:       20,
					TenantID: domain.DefaultTenantID,
					Type:     "custom",
					Name:     "custom-disabled-cooldown-cancel",
					Config:   &domain.ProviderConfig{DisableErrorCooldown: true},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	e := newDisabledCooldownStreamTestExecutor(&recordingProxyRequestRepo{}, &recordingAttemptRepo{})

	done := make(chan struct{})
	go func() {
		e.dispatch(c)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("dispatch did not stop after context cancellation; adapter calls=%d", adapter.calls)
	}

	if !errors.Is(c.Err, context.Canceled) {
		t.Fatalf("dispatch error = %v, want context.Canceled", c.Err)
	}
	// The loop must halt on cancellation, nowhere near the hard cap. Allowing a
	// tiny margin for a race where the select picks the zero-wait branch once
	// more before the top-of-loop ctx check fires.
	if adapter.calls > 3 {
		t.Fatalf("adapter calls = %d; loop should have stopped promptly on cancellation", adapter.calls)
	}
}

func lastProxyStatus(repo *recordingProxyRequestRepo) string {
	if len(repo.updated) == 0 {
		return ""
	}
	return repo.updated[len(repo.updated)-1].Status
}
