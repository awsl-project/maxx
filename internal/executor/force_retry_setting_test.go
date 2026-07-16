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

func TestDispatchForceRetryUpstreamErrorsSettingRetriesProviderError(t *testing.T) {
	retryErr := domain.NewProxyErrorWithMessage(errors.New("upstream error"), false, "failed to connect to upstream")
	retryErr.Scope = domain.ScopeProvider
	retryErr.Reason = domain.CooldownReasonNetworkError

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		true,
		&forceRetrySequenceAdapter{errs: []error{retryErr}},
		&domain.RetryConfig{MaxRetries: 1, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
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
		proxyRequestRepo: &codexGuardProxyRequestRepo{},
		attemptRepo:      &recordingAttemptRepo{},
		modelMappingRepo: &codexGuardModelMappingRepo{},
		settingsRepo: &forceRetrySettingsRepo{values: map[string]string{
			domain.SettingKeyForceRetryUpstreamErrors: map[bool]string{true: "true", false: "false"}[forceRetry],
		}},
		converter: converter.GetGlobalRegistry(),
	}
	return adapter, proxyReq, c, e
}
