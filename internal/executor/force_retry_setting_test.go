package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

type forceRetrySettingsRepo struct {
	values map[string]string
}

func (r *forceRetrySettingsRepo) Get(key string) (string, error) {
	if r != nil && r.values != nil {
		if value, ok := r.values[key]; ok {
			return value, nil
		}
	}
	return "", domain.ErrNotFound
}

func (r *forceRetrySettingsRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *forceRetrySettingsRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }
func (r *forceRetrySettingsRepo) Delete(key string) error                  { return nil }

type forceRetrySequenceAdapter struct {
	errs   []error
	calls  int
	onCall func()
}

func (a *forceRetrySequenceAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *forceRetrySequenceAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	if a.onCall != nil {
		a.onCall()
	}
	if a.calls <= len(a.errs) && a.errs[a.calls-1] != nil {
		return a.errs[a.calls-1]
	}
	_, _ = c.Writer.Write([]byte(`{"ok":true}`))
	return nil
}

func TestDispatchForceRetryUpstreamErrorsRetryConfigRetriesProviderError(t *testing.T) {
	retryErr := domain.NewProxyErrorWithMessage(errors.New("upstream error"), false, "failed to connect to upstream")
	retryErr.Scope = domain.ScopeProvider
	retryErr.Reason = domain.CooldownReasonNetworkError

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{errs: []error{retryErr}},
		&domain.RetryConfig{MaxRetries: 1, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0, ForceRetryUpstreamErrors: true},
	)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 2 {
		t.Fatalf("adapter calls = %d, want 2", adapter.calls)
	}
	if proxyReq.Status != "COMPLETED" {
		t.Fatalf("proxy request status = %q, want COMPLETED", proxyReq.Status)
	}
	if proxyReq.ProxyUpstreamAttemptCount != 2 {
		t.Fatalf("attempt count = %d, want 2", proxyReq.ProxyUpstreamAttemptCount)
	}
}

func TestDispatchForceRetryUpstreamErrorsSettingOffPreservesNonRetryableProviderError(t *testing.T) {
	retryErr := domain.NewProxyErrorWithMessage(errors.New("upstream error"), false, "failed to connect to upstream")
	retryErr.Scope = domain.ScopeProvider
	retryErr.Reason = domain.CooldownReasonNetworkError

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{errs: []error{retryErr, nil}},
		&domain.RetryConfig{MaxRetries: 1, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected non-retryable provider error")
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if proxyReq.Status != "FAILED" {
		t.Fatalf("proxy request status = %q, want FAILED", proxyReq.Status)
	}
}

func TestDispatchForceRetryUpstreamErrorsDoesNotOverrideRequestScopedError(t *testing.T) {
	requestErr := domain.NewProxyErrorWithMessage(errors.New("bad request"), false, "invalid request")
	requestErr.Scope = domain.ScopeRequest
	requestErr.HTTPStatusCode = http.StatusBadRequest

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		true,
		&forceRetrySequenceAdapter{errs: []error{requestErr, nil}},
		&domain.RetryConfig{MaxRetries: 1, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected request-scoped error")
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if proxyReq.Status != "FAILED" {
		t.Fatalf("proxy request status = %q, want FAILED", proxyReq.Status)
	}
}

func TestDispatchForceRetryUpstreamErrorsDoesNotOverrideCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	retryErr := domain.NewProxyErrorWithMessage(errors.New("upstream error"), false, "failed to connect to upstream")
	retryErr.Scope = domain.ScopeProvider
	retryErr.Reason = domain.CooldownReasonNetworkError
	sequence := &forceRetrySequenceAdapter{
		errs: []error{retryErr, nil},
		onCall: func() {
			cancel()
		},
	}

	adapter, proxyReq, c, e := newForceRetryDispatchHarnessWithContext(
		t,
		ctx,
		true,
		sequence,
		&domain.RetryConfig{MaxRetries: 1, InitialInterval: time.Hour, BackoffRate: 1, MaxInterval: time.Hour},
	)

	e.dispatch(c)

	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if !errors.Is(c.Err, context.Canceled) {
		t.Fatalf("dispatch error = %v, want context.Canceled", c.Err)
	}
	if proxyReq.ProxyUpstreamAttemptCount != 1 {
		t.Fatalf("attempt count = %d, want 1", proxyReq.ProxyUpstreamAttemptCount)
	}
}

func TestDispatchProviderConcurrencyLimitFailsBeforeUpstreamWithExplicit429(t *testing.T) {
	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{},
		&domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	)
	e.router = router.NewRouter(nil, nil, nil, nil, nil)
	storedState, ok := c.Get(flow.KeyExecutorState)
	if !ok {
		t.Fatal("executor state missing")
	}
	provider := storedState.(*execState).routes[0].Provider
	provider.MaxConcurrency = 1
	release, acquired := e.router.TryAcquireProvider(provider)
	if !acquired {
		t.Fatal("failed to acquire provider slot for test setup")
	}
	defer release()

	e.dispatch(c)

	if adapter.calls != 0 {
		t.Fatalf("adapter calls = %d, want 0", adapter.calls)
	}
	if proxyReq.ProxyUpstreamAttemptCount != 0 {
		t.Fatalf("attempt count = %d, want 0", proxyReq.ProxyUpstreamAttemptCount)
	}
	if proxyReq.Status != "FAILED" {
		t.Fatalf("proxy request status = %q, want FAILED", proxyReq.Status)
	}
	if proxyReq.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want %d", proxyReq.StatusCode, http.StatusTooManyRequests)
	}
	if c.Err == nil || !errors.Is(c.Err, domain.ErrNoAvailableProviders) {
		t.Fatalf("dispatch error = %v, want ErrNoAvailableProviders", c.Err)
	}
	var proxyErr *domain.ProxyError
	if !errors.As(c.Err, &proxyErr) {
		t.Fatalf("dispatch error type = %T, want ProxyError", c.Err)
	}
	if proxyErr.Reason != domain.CooldownReasonConcurrentLimit {
		t.Fatalf("proxy error reason = %q, want %q", proxyErr.Reason, domain.CooldownReasonConcurrentLimit)
	}
}

func newForceRetryDispatchHarness(
	t *testing.T,
	forceRetry bool,
	adapter *forceRetrySequenceAdapter,
	retryConfig *domain.RetryConfig,
) (*forceRetrySequenceAdapter, *domain.ProxyRequest, *flow.Ctx, *Executor) {
	t.Helper()
	return newForceRetryDispatchHarnessWithContext(t, context.Background(), forceRetry, adapter, retryConfig)
}

func newForceRetryDispatchHarnessWithContext(
	t *testing.T,
	ctx context.Context,
	forceRetry bool,
	adapter *forceRetrySequenceAdapter,
	retryConfig *domain.RetryConfig,
) (*forceRetrySequenceAdapter, *domain.ProxyRequest, *flow.Ctx, *Executor) {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         303,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	state := &execState{
		ctx:          ctx,
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "gpt-4o",
		routes: []*router.MatchedRoute{
			{
				Route:           &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 20, ClientType: domain.ClientTypeOpenAI},
				Provider:        &domain.Provider{ID: 20, TenantID: domain.DefaultTenantID, Type: "custom", Name: "custom-force-retry"},
				ProviderAdapter: adapter,
				RetryConfig:     retryConfig,
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	e := &Executor{
		proxyRequestRepo: &recordingProxyRequestRepo{},
		attemptRepo:      &recordingAttemptRepo{},
		modelMappingRepo: &stubModelMappingRepo{},
		settingsRepo: &forceRetrySettingsRepo{values: map[string]string{
			domain.SettingKeyForceRetryUpstreamErrors: map[bool]string{true: "true", false: "false"}[forceRetry],
		}},
		converter: converter.GetGlobalRegistry(),
	}
	return adapter, proxyReq, c, e
}

func TestDispatchModelScopedNonRetryableErrorFailsOverToNextRoute(t *testing.T) {
	modelErr := domain.NewProxyErrorWithMessage(errors.New(`{"error":{"message":"model not found: moonshotai/kimi-k3"}}`), false, "upstream returned status 422")
	modelErr.Scope = domain.ScopeModel
	modelErr.Reason = domain.CooldownReasonModelUnavailable
	modelErr.Model = "moonshotai/kimi-k3"
	modelErr.HTTPStatusCode = http.StatusUnprocessableEntity

	first := &forceRetrySequenceAdapter{errs: []error{modelErr}}
	second := &forceRetrySequenceAdapter{}
	_, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		first,
		&domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	)
	storedState, ok := c.Get(flow.KeyExecutorState)
	if !ok {
		t.Fatal("executor state missing")
	}
	state := storedState.(*execState)
	state.requestModel = "gpt-5"
	state.routes = append(state.routes, &router.MatchedRoute{
		Route:           &domain.Route{ID: 11, TenantID: domain.DefaultTenantID, ProviderID: 21, ClientType: domain.ClientTypeOpenAI},
		Provider:        &domain.Provider{ID: 21, TenantID: domain.DefaultTenantID, Type: "custom", Name: "custom-success"},
		ProviderAdapter: second,
		RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	})

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch error = %v", c.Err)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("adapter calls first=%d second=%d, want 1/1", first.calls, second.calls)
	}
	if proxyReq.Status != "COMPLETED" {
		t.Fatalf("proxy request status = %q, want COMPLETED", proxyReq.Status)
	}
	if proxyReq.ProviderID != 21 {
		t.Fatalf("final provider ID = %d, want 21", proxyReq.ProviderID)
	}
}
