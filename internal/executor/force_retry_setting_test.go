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

func newHTTP408UpstreamConnectError() *domain.ProxyError {
	proxyErr := domain.NewProxyErrorWithMessage(errors.New("upstream error: failed to connect to upstream: upstream error"), true, "upstream returned status 408")
	proxyErr.Scope = domain.ScopeEndpoint
	proxyErr.Reason = domain.CooldownReasonNetworkError
	proxyErr.HTTPStatusCode = http.StatusRequestTimeout
	return proxyErr
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

func TestDispatchForceRetryUpstreamErrorsAddsOneRetryBudgetForConnectionError(t *testing.T) {
	retryErr := domain.NewUpstreamConnectionError("failed to connect to upstream")

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{errs: []error{retryErr}},
		&domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0, ForceRetryUpstreamErrors: true},
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

func TestDispatchRetryableUpstreamConnectionErrorGetsOneRetryBudget(t *testing.T) {
	retryErr := domain.NewUpstreamConnectionError("failed to connect to upstream")

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{errs: []error{retryErr, nil}},
		&domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0, ForceRetryUpstreamErrors: false},
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

func TestDispatchRetryableHTTP408ConnectErrorGetsOneRetryBudget(t *testing.T) {
	retryErr := newHTTP408UpstreamConnectError()

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{errs: []error{retryErr, nil}},
		&domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0, ForceRetryUpstreamErrors: false},
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

func TestDispatchHTTP408ConnectErrorFallsThroughAfterRetryBudget(t *testing.T) {
	first := &forceRetrySequenceAdapter{errs: []error{newHTTP408UpstreamConnectError(), newHTTP408UpstreamConnectError()}}
	second := &forceRetrySequenceAdapter{}
	_, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		first,
		&domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0, ForceRetryUpstreamErrors: false},
	)
	storedState, ok := c.Get(flow.KeyExecutorState)
	if !ok {
		t.Fatal("executor state missing")
	}
	state := storedState.(*execState)
	state.routes = append(state.routes, &router.MatchedRoute{
		Route:           &domain.Route{ID: 11, TenantID: domain.DefaultTenantID, ProviderID: 21, ClientType: domain.ClientTypeOpenAI},
		Provider:        &domain.Provider{ID: 21, TenantID: domain.DefaultTenantID, Type: "custom", Name: "custom-success"},
		ProviderAdapter: second,
		RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	})

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if first.calls != 2 || second.calls != 1 {
		t.Fatalf("adapter calls first=%d second=%d, want 2/1", first.calls, second.calls)
	}
	if proxyReq.Status != "COMPLETED" {
		t.Fatalf("proxy request status = %q, want COMPLETED", proxyReq.Status)
	}
	if proxyReq.ProviderID != 21 {
		t.Fatalf("final provider ID = %d, want 21", proxyReq.ProviderID)
	}
}

func TestDispatchRetryOpenAIPolicyFlaggedPromptRetriesOnceWhenEnabled(t *testing.T) {
	policyErr := domain.NewProxyErrorWithMessage(errors.New("SSE error (code=0): Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt: https://platform.openai.com/docs/guides/reasoning#advice-on-prompting"), false, "Invalid prompt: your prompt was flagged as potentially violating our usage policy")
	policyErr.Scope = domain.ScopeProvider
	policyErr.Reason = domain.CooldownReasonServerError

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{errs: []error{policyErr}},
		&domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	)
	storedState, ok := c.Get(flow.KeyExecutorState)
	if !ok {
		t.Fatal("executor state missing")
	}
	state := storedState.(*execState)
	state.routes[0].Provider.Config = &domain.ProviderConfig{RetryOpenAIPolicyFlaggedPrompt: true}

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 2 {
		t.Fatalf("adapter calls = %d, want one retry", adapter.calls)
	}
	if proxyReq.Status != "COMPLETED" {
		t.Fatalf("proxy request status = %q, want COMPLETED", proxyReq.Status)
	}
}

func TestDispatchRetryOpenAIPolicyFlaggedPromptStopsAfterOneRetry(t *testing.T) {
	policyErr := domain.NewProxyErrorWithMessage(errors.New("SSE error (code=0): Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt"), false, "Invalid prompt: your prompt was flagged as potentially violating our usage policy")
	policyErr.Scope = domain.ScopeProvider
	policyErr.Reason = domain.CooldownReasonServerError

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{errs: []error{policyErr, policyErr, nil}},
		&domain.RetryConfig{MaxRetries: 3, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	)
	storedState, ok := c.Get(flow.KeyExecutorState)
	if !ok {
		t.Fatal("executor state missing")
	}
	state := storedState.(*execState)
	state.routes[0].Provider.Config = &domain.ProviderConfig{RetryOpenAIPolicyFlaggedPrompt: true}
	second := &forceRetrySequenceAdapter{}
	state.routes = append(state.routes, &router.MatchedRoute{
		Route:           &domain.Route{ID: 11, TenantID: domain.DefaultTenantID, ProviderID: 21, ClientType: domain.ClientTypeOpenAI},
		Provider:        &domain.Provider{ID: 21, TenantID: domain.DefaultTenantID, Type: "custom", Name: "custom-success"},
		ProviderAdapter: second,
		RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected policy flagged prompt error after one retry")
	}
	if adapter.calls != 2 {
		t.Fatalf("adapter calls = %d, want exactly 2", adapter.calls)
	}
	if second.calls != 0 {
		t.Fatalf("second route calls = %d, want 0", second.calls)
	}
	if proxyReq.Status != "FAILED" {
		t.Fatalf("proxy request status = %q, want FAILED", proxyReq.Status)
	}
}

func TestDispatchRetryOpenAIPolicyFlaggedPromptDisabledDoesNotRetry(t *testing.T) {
	policyErr := domain.NewProxyErrorWithMessage(errors.New("SSE error (code=0): Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt"), false, "Invalid prompt: your prompt was flagged as potentially violating our usage policy")
	policyErr.Scope = domain.ScopeProvider
	policyErr.Reason = domain.CooldownReasonServerError

	adapter, proxyReq, c, e := newForceRetryDispatchHarness(
		t,
		false,
		&forceRetrySequenceAdapter{errs: []error{policyErr, nil}},
		&domain.RetryConfig{MaxRetries: 1, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	)
	storedState, ok := c.Get(flow.KeyExecutorState)
	if !ok {
		t.Fatal("executor state missing")
	}
	state := storedState.(*execState)
	state.routes[0].Provider.Config = &domain.ProviderConfig{RetryOpenAIPolicyFlaggedPrompt: false}
	second := &forceRetrySequenceAdapter{}
	state.routes = append(state.routes, &router.MatchedRoute{
		Route:           &domain.Route{ID: 11, TenantID: domain.DefaultTenantID, ProviderID: 21, ClientType: domain.ClientTypeOpenAI},
		Provider:        &domain.Provider{ID: 21, TenantID: domain.DefaultTenantID, Type: "custom", Name: "custom-success"},
		ProviderAdapter: second,
		RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected non-retryable policy flagged prompt error")
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if second.calls != 0 {
		t.Fatalf("second route calls = %d, want 0", second.calls)
	}
	if proxyReq.Status != "FAILED" {
		t.Fatalf("proxy request status = %q, want FAILED", proxyReq.Status)
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

func TestEnsureRetryableUpstreamErrorHasBudgetBoundaries(t *testing.T) {
	baseErr := func() *domain.ProxyError {
		return domain.NewUpstreamConnectionError("failed to connect to upstream")
	}
	nonRetryable := baseErr()
	nonRetryable.Retryable = false
	keyScoped := baseErr()
	keyScoped.Scope = domain.ScopeKey
	badRequest := baseErr()
	badRequest.HTTPStatusCode = http.StatusBadRequest
	rateLimited := baseErr()
	rateLimited.HTTPStatusCode = http.StatusTooManyRequests
	http408Connect := newHTTP408UpstreamConnectError()
	http408ConnectWithoutWrappedCause := newHTTP408UpstreamConnectError()
	http408ConnectWithoutWrappedCause.Err = errors.New("upstream error: failed to connect to upstream")
	http408NonConnect := newHTTP408UpstreamConnectError()
	http408NonConnect.Err = errors.New("upstream error: request timeout waiting for model output")
	nonNetwork := baseErr()
	nonNetwork.Reason = domain.CooldownReasonServerError
	alreadyBudgeted := baseErr()
	responseRead := baseErr()
	responseRead.UpstreamFailurePhase = domain.UpstreamFailurePhaseResponseRead
	unknownPhase := baseErr()
	unknownPhase.UpstreamFailurePhase = domain.UpstreamFailurePhaseUnknown
	nonConnectMessage := baseErr()
	nonConnectMessage.Message = "invalid provider proxy configuration"

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	deadlineCtx, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer deadlineCancel()

	cases := []struct {
		name                      string
		maxRetries                int
		proxyErr                  *domain.ProxyError
		ctx                       context.Context
		responseCommitted         bool
		attempt                   int
		allowPolicyRetryExtension bool
		want                      int
	}{
		{name: "adds one budget for safe connection error", maxRetries: 0, proxyErr: baseErr(), want: 1},
		{name: "keeps existing budget", maxRetries: 2, proxyErr: alreadyBudgeted, want: 2},
		{name: "keeps exhausted configured retry budget without policy retry", maxRetries: 1, attempt: 1, proxyErr: baseErr(), want: 1},
		{name: "raises budget above current attempt after policy retry", maxRetries: 1, attempt: 1, allowPolicyRetryExtension: true, proxyErr: baseErr(), want: 2},
		{name: "ignores non retryable error", maxRetries: 0, proxyErr: nonRetryable, want: 0},
		{name: "rejects key scoped error", maxRetries: 0, proxyErr: keyScoped, want: 0},
		{name: "rejects ordinary four hundred error", maxRetries: 0, proxyErr: badRequest, want: 0},
		{name: "does not treat rate limit as connection budget", maxRetries: 0, proxyErr: rateLimited, want: 0},
		{name: "adds one budget for http 408 wrapped connection error", maxRetries: 0, proxyErr: http408Connect, want: 1},
		{name: "adds one budget for http 408 connection message without wrapped cause", maxRetries: 0, proxyErr: http408ConnectWithoutWrappedCause, want: 1},
		{name: "rejects http 408 without connection message", maxRetries: 0, proxyErr: http408NonConnect, want: 0},
		{name: "rejects non network upstream error", maxRetries: 0, proxyErr: nonNetwork, want: 0},
		{name: "rejects response read upstream error", maxRetries: 0, proxyErr: responseRead, want: 0},
		{name: "rejects unknown upstream failure phase", maxRetries: 0, proxyErr: unknownPhase, want: 0},
		{name: "rejects connect phase without connection message", maxRetries: 0, proxyErr: nonConnectMessage, want: 0},
		{name: "rejects canceled context", maxRetries: 0, proxyErr: baseErr(), ctx: canceledCtx, want: 0},
		{name: "rejects deadline context", maxRetries: 0, proxyErr: baseErr(), ctx: deadlineCtx, want: 0},
		{name: "rejects committed response", maxRetries: 0, proxyErr: baseErr(), responseCommitted: true, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureRetryableUpstreamErrorHasBudget(tc.maxRetries, tc.attempt, tc.allowPolicyRetryExtension, tc.proxyErr, tc.ctx, tc.responseCommitted)
			if got != tc.want {
				t.Fatalf("budget = %d, want %d", got, tc.want)
			}
		})
	}
}
