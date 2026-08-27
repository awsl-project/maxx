package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

type disabledCooldownStreamRetryAdapter struct {
	calls int
}

type disabledCooldownHTTPErrorAdapter struct {
	calls        int
	succeedAfter int
}

type interruptedResponseStreamRetryAdapter struct {
	calls *int
}

func (a *disabledCooldownStreamRetryAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *disabledCooldownStreamRetryAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	if a.calls == 1 {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = c.Writer.Write([]byte("data: partial\n\n"))
		proxyErr := domain.NewProxyErrorWithMessage(
			errors.New("stream error: stream ID 1; INTERNAL_ERROR; received from peer"),
			false,
			"upstream stream read error after response started",
		)
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonNetworkError
		return proxyErr
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	_, _ = c.Writer.Write([]byte("data: fallback\n\ndata: [DONE]\n\n"))
	return nil
}

func (a *disabledCooldownHTTPErrorAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a interruptedResponseStreamRetryAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *disabledCooldownHTTPErrorAdapter) Execute(_ *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	if a.succeedAfter > 0 && a.calls > a.succeedAfter {
		return nil
	}
	proxyErr := domain.NewProxyErrorWithMessage(errors.New("upstream returned 500"), false, "upstream returned 500")
	proxyErr.Scope = domain.ScopeProvider
	proxyErr.Reason = domain.CooldownReasonServerError
	proxyErr.HTTPStatusCode = http.StatusInternalServerError
	return proxyErr
}

func (a interruptedResponseStreamRetryAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	*a.calls = *a.calls + 1
	if *a.calls == 1 {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = c.Writer.Write([]byte("data: partial\n\n"))
		proxyErr := domain.NewProxyErrorWithMessage(
			errors.New("stream transport reset"),
			false,
			"Upstream response stream was interrupted",
		)
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonNetworkError
		return proxyErr
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	_, _ = c.Writer.Write([]byte("data: fallback\n\ndata: [DONE]\n\n"))
	return nil
}

func TestDispatchRetriesCommittedStreamReadErrorWhenErrorCooldownDisabled(t *testing.T) {
	c, adapter, attemptRepo, proxyRepo := newDisabledCooldownStreamDispatchCtx(true)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 2 {
		t.Fatalf("adapter calls = %d, want 2", adapter.calls)
	}
	if len(attemptRepo.created) != 2 {
		t.Fatalf("created attempts = %d, want 2", len(attemptRepo.created))
	}
	if got := attemptRepo.updated[0].Status; got != "FAILED" {
		t.Fatalf("first attempt status = %q, want FAILED", got)
	}
	if got := attemptRepo.updated[len(attemptRepo.updated)-1].Status; got != "COMPLETED" {
		t.Fatalf("final attempt status = %q, want COMPLETED", got)
	}
	if got := c.Writer.(*httptest.ResponseRecorder).Body.String(); got != "data: partial\n\ndata: fallback\n\ndata: [DONE]\n\n" {
		t.Fatalf("client body = %q", got)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request update, got %#v", proxyRepo.updated)
	}
}

func TestDispatchRetriesCommittedStreamReadErrorWhenErrorCooldownEnabled(t *testing.T) {
	c, adapter, _, proxyRepo := newDisabledCooldownStreamDispatchCtx(false)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 2 {
		t.Fatalf("adapter calls = %d, want 2", adapter.calls)
	}
	if got := c.Writer.(*httptest.ResponseRecorder).Body.String(); got != "data: partial\n\ndata: fallback\n\ndata: [DONE]\n\n" {
		t.Fatalf("client body = %q", got)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request update, got %#v", proxyRepo.updated)
	}
}

func TestDispatchRetriesInterruptedResponseStreamError(t *testing.T) {
	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	calls := 0
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         101,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "gpt-4o",
		isStream:     true,
		routes: []*router.MatchedRoute{{
			Route: &domain.Route{ID: 11, TenantID: domain.DefaultTenantID, ProviderID: 21, ClientType: domain.ClientTypeOpenAI},
			Provider: &domain.Provider{
				ID:       21,
				TenantID: domain.DefaultTenantID,
				Type:     "custom",
				Name:     "custom-interrupted-stream",
				Config:   &domain.ProviderConfig{DisableErrorCooldown: false},
			},
			ProviderAdapter: interruptedResponseStreamRetryAdapter{calls: &calls},
			RetryConfig:     &domain.RetryConfig{MaxRetries: 1, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
		}},
	}
	c.Set(flow.KeyExecutorState, state)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if calls != 2 {
		t.Fatalf("adapter calls = %d, want 2", calls)
	}
	if got := c.Writer.(*httptest.ResponseRecorder).Body.String(); got != "data: partial\n\ndata: fallback\n\ndata: [DONE]\n\n" {
		t.Fatalf("client body = %q", got)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request update, got %#v", proxyRepo.updated)
	}
}

func TestDispatchDoesNotRetryCommittedStreamReadErrorWithoutRetryBudget(t *testing.T) {
	c, adapter, _, proxyRepo := newDisabledCooldownStreamDispatchCtx(false, 0)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected committed stream read error")
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if got := c.Writer.(*httptest.ResponseRecorder).Body.String(); got != "data: partial\n\n" {
		t.Fatalf("client body = %q", got)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "FAILED" {
		t.Fatalf("expected failed proxy request update, got %#v", proxyRepo.updated)
	}
}

type smartMappingRetryAdapter struct {
	models []string
}

func (a *smartMappingRetryAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *smartMappingRetryAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	model := flow.GetMappedModel(c)
	a.models = append(a.models, model)
	if model == "mapped-b" {
		return nil
	}
	proxyErr := domain.NewProxyErrorWithMessage(errors.New("upstream returned 500"), true, "upstream returned 500")
	proxyErr.Scope = domain.ScopeProvider
	proxyErr.Reason = domain.CooldownReasonServerError
	proxyErr.HTTPStatusCode = http.StatusInternalServerError
	return proxyErr
}

func TestDispatchDisableErrorCooldownRetriesHTTPErrorBeyondRetryBudget(t *testing.T) {
	c, adapter, _, proxyRepo := newDisabledCooldownHTTPErrorDispatchCtx(true, 0, 3)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 4 {
		t.Fatalf("adapter calls = %d, want 4; disableErrorCooldown should retry beyond MaxRetries", adapter.calls)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request update, got %#v", proxyRepo.updated)
	}
}

func TestDispatchSmartMappingRetrySwitchesMappedModelAfterLimit(t *testing.T) {
	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	adapter := &smartMappingRetryAdapter{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         103,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "requested-model",
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 22, ClientType: domain.ClientTypeOpenAI},
				Provider: &domain.Provider{
					ID:       22,
					TenantID: domain.DefaultTenantID,
					Type:     "custom",
					Name:     "custom-smart-mapping",
					Config: &domain.ProviderConfig{
						DisableErrorCooldown:     true,
						SmartMappingRetryEnabled: true,
						SmartMappingRetryLimit:   2,
					},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, attemptRepo)
	e.modelMappingRepo = &stubModelMappingRepo{mappings: []*domain.ModelMapping{
		{Pattern: "requested-*", Target: "mapped-a"},
		{Pattern: "requested-*", Target: "mapped-b"},
	}}

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	want := []string{"mapped-a", "mapped-a", "mapped-b"}
	if len(adapter.models) != len(want) {
		t.Fatalf("adapter models = %#v, want %#v", adapter.models, want)
	}
	for i := range want {
		if adapter.models[i] != want[i] {
			t.Fatalf("adapter models = %#v, want %#v", adapter.models, want)
		}
	}
	if len(attemptRepo.created) != len(want) {
		t.Fatalf("created attempts = %d, want %d", len(attemptRepo.created), len(want))
	}
	for i, model := range want {
		if attemptRepo.created[i].MappedModel != model {
			t.Fatalf("attempt %d mapped model = %q, want %q", i, attemptRepo.created[i].MappedModel, model)
		}
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request update, got %#v", proxyRepo.updated)
	}
}

type configurableSmartMappingRetryAdapter struct {
	succeedOn string
	models    []string
}

func (a *configurableSmartMappingRetryAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *configurableSmartMappingRetryAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	model := flow.GetMappedModel(c)
	a.models = append(a.models, model)
	if model == a.succeedOn {
		return nil
	}
	proxyErr := domain.NewProxyErrorWithMessage(errors.New("upstream returned 500"), true, "upstream returned 500")
	proxyErr.Scope = domain.ScopeProvider
	proxyErr.Reason = domain.CooldownReasonServerError
	proxyErr.HTTPStatusCode = http.StatusInternalServerError
	return proxyErr
}

func newSmartMappingRetryDispatchCtx(proxyReqID uint64, adapter provider.ProviderAdapter) *flow.Ctx {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         proxyReqID,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "requested-model",
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 22, ClientType: domain.ClientTypeOpenAI},
				Provider: &domain.Provider{
					ID:       22,
					TenantID: domain.DefaultTenantID,
					Type:     "custom",
					Name:     "custom-smart-mapping-last-success",
					Config: &domain.ProviderConfig{
						DisableErrorCooldown:     true,
						SmartMappingRetryEnabled: true,
						SmartMappingRetryLimit:   1,
					},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	return c
}

func TestDispatchSmartMappingRetryStartsWithLastSuccessfulMappedModel(t *testing.T) {
	proxyRepo := &recordingProxyRequestRepo{}
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})
	e.modelMappingRepo = &stubModelMappingRepo{mappings: []*domain.ModelMapping{
		{Pattern: "requested-*", Target: "mapped-a"},
		{Pattern: "requested-*", Target: "mapped-b"},
		{Pattern: "requested-*", Target: "mapped-c"},
	}}

	firstAdapter := &configurableSmartMappingRetryAdapter{succeedOn: "mapped-b"}
	firstCtx := newSmartMappingRetryDispatchCtx(201, firstAdapter)
	e.dispatch(firstCtx)
	if firstCtx.Err != nil {
		t.Fatalf("first dispatch returned error: %v", firstCtx.Err)
	}
	if want := []string{"mapped-a", "mapped-b"}; !reflect.DeepEqual(firstAdapter.models, want) {
		t.Fatalf("first dispatch models = %#v, want %#v", firstAdapter.models, want)
	}

	secondAdapter := &configurableSmartMappingRetryAdapter{succeedOn: "mapped-b"}
	secondCtx := newSmartMappingRetryDispatchCtx(202, secondAdapter)
	e.dispatch(secondCtx)
	if secondCtx.Err != nil {
		t.Fatalf("second dispatch returned error: %v", secondCtx.Err)
	}
	if want := []string{"mapped-b"}; !reflect.DeepEqual(secondAdapter.models, want) {
		t.Fatalf("second dispatch models = %#v, want %#v", secondAdapter.models, want)
	}
}

func TestDispatchSmartMappingRetryIgnoresLastSuccessAfterCandidateListChanges(t *testing.T) {
	proxyRepo := &recordingProxyRequestRepo{}
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, &recordingAttemptRepo{})
	e.modelMappingRepo = &stubModelMappingRepo{mappings: []*domain.ModelMapping{
		{Pattern: "requested-*", Target: "mapped-a"},
		{Pattern: "requested-*", Target: "mapped-b"},
	}}

	firstAdapter := &configurableSmartMappingRetryAdapter{succeedOn: "mapped-b"}
	firstCtx := newSmartMappingRetryDispatchCtx(203, firstAdapter)
	e.dispatch(firstCtx)
	if firstCtx.Err != nil {
		t.Fatalf("first dispatch returned error: %v", firstCtx.Err)
	}

	e.modelMappingRepo = &stubModelMappingRepo{mappings: []*domain.ModelMapping{
		{Pattern: "requested-*", Target: "mapped-a"},
		{Pattern: "requested-*", Target: "mapped-c"},
	}}
	secondAdapter := &configurableSmartMappingRetryAdapter{succeedOn: "mapped-c"}
	secondCtx := newSmartMappingRetryDispatchCtx(204, secondAdapter)
	e.dispatch(secondCtx)
	if secondCtx.Err != nil {
		t.Fatalf("second dispatch returned error: %v", secondCtx.Err)
	}
	if want := []string{"mapped-a", "mapped-c"}; !reflect.DeepEqual(secondAdapter.models, want) {
		t.Fatalf("second dispatch models = %#v, want %#v", secondAdapter.models, want)
	}
}

type noProviderSwitchSmartMappingAdapter struct {
	models       []string
	succeedAfter int
}

func (a *noProviderSwitchSmartMappingAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *noProviderSwitchSmartMappingAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	model := flow.GetMappedModel(c)
	a.models = append(a.models, model)
	if a.succeedAfter > 0 && len(a.models) >= a.succeedAfter {
		return nil
	}
	proxyErr := domain.NewProxyErrorWithMessage(errors.New("upstream returned 500"), true, "upstream returned 500")
	proxyErr.Scope = domain.ScopeProvider
	proxyErr.Reason = domain.CooldownReasonServerError
	proxyErr.HTTPStatusCode = http.StatusInternalServerError
	return proxyErr
}

func TestDispatchDisableErrorCooldownKeepsSmartMappingRetryOnSameProvider(t *testing.T) {
	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	firstAdapter := &noProviderSwitchSmartMappingAdapter{succeedAfter: 3}
	secondAdapter := &noProviderSwitchSmartMappingAdapter{succeedAfter: 1}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         104,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "requested-model",
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 22, ClientType: domain.ClientTypeOpenAI},
				Provider: &domain.Provider{
					ID:       22,
					TenantID: domain.DefaultTenantID,
					Type:     "custom",
					Name:     "custom-smart-mapping-no-switch",
					Config: &domain.ProviderConfig{
						DisableErrorCooldown:     true,
						SmartMappingRetryEnabled: true,
						SmartMappingRetryLimit:   1,
					},
				},
				ProviderAdapter: firstAdapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
			{
				Route: &domain.Route{ID: 11, TenantID: domain.DefaultTenantID, ProviderID: 23, ClientType: domain.ClientTypeOpenAI},
				Provider: &domain.Provider{
					ID:       23,
					TenantID: domain.DefaultTenantID,
					Type:     "custom",
					Name:     "custom-must-not-be-used",
				},
				ProviderAdapter: secondAdapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	e := newDisabledCooldownStreamTestExecutor(proxyRepo, attemptRepo)
	e.modelMappingRepo = &stubModelMappingRepo{mappings: []*domain.ModelMapping{
		{Pattern: "requested-*", Target: "mapped-a"},
		{Pattern: "requested-*", Target: "mapped-b"},
	}}

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	want := []string{"mapped-a", "mapped-b", "mapped-a"}
	if len(firstAdapter.models) != len(want) {
		t.Fatalf("first provider models = %#v, want %#v", firstAdapter.models, want)
	}
	for i := range want {
		if firstAdapter.models[i] != want[i] {
			t.Fatalf("first provider models = %#v, want %#v", firstAdapter.models, want)
		}
	}
	if len(secondAdapter.models) != 0 {
		t.Fatalf("second provider was called despite disable switch: %#v", secondAdapter.models)
	}
	if proxyReq.ProviderID != 22 {
		t.Fatalf("final provider id = %d, want 22", proxyReq.ProviderID)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request update, got %#v", proxyRepo.updated)
	}
}

type canceledContextRetryAdapter struct {
	calls int
}

func (a *canceledContextRetryAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *canceledContextRetryAdapter) Execute(*flow.Ctx, *domain.Provider) error {
	a.calls++
	proxyErr := domain.NewProxyErrorWithMessage(errors.New("upstream retryable error"), true, "upstream retryable error")
	proxyErr.Scope = domain.ScopeProvider
	proxyErr.Reason = domain.CooldownReasonNetworkError
	return proxyErr
}

func TestDispatchDoesNotRetryAfterRequestContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         101,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	adapter := &canceledContextRetryAdapter{}
	state := &execState{
		ctx:          ctx,
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "gpt-4o",
		routes: []*router.MatchedRoute{
			{
				Route:           &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 20, ClientType: domain.ClientTypeOpenAI},
				Provider:        &domain.Provider{ID: 20, TenantID: domain.DefaultTenantID, Type: "custom", Name: "custom-cancelled"},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 1, InitialInterval: time.Hour, BackoffRate: 1, MaxInterval: time.Hour},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	e := newDisabledCooldownStreamTestExecutor(&recordingProxyRequestRepo{}, &recordingAttemptRepo{})

	go func() {
		for adapter.calls == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	e.dispatch(c)

	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if !errors.Is(c.Err, context.Canceled) {
		t.Fatalf("dispatch error = %v, want context.Canceled", c.Err)
	}
	if proxyReq.Status != "FAILED" {
		t.Fatalf("proxy request status = %q, want FAILED", proxyReq.Status)
	}
	if proxyReq.Error != "upstream retryable error: upstream retryable error" {
		t.Fatalf("proxy request error = %q, want upstream error", proxyReq.Error)
	}
}

func TestRequestFailureStatusOnlyCancelsForClientDisconnectEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	upstreamErr := domain.NewProxyErrorWithMessage(errors.New("upstream retryable error"), true, "upstream retryable error")
	upstreamErr.Scope = domain.ScopeProvider
	status, msg := requestFailureStatusAndError(ctx, upstreamErr)
	if status != "FAILED" {
		t.Fatalf("status without disconnect evidence = %q, want FAILED", status)
	}
	if msg != "upstream retryable error: upstream retryable error" {
		t.Fatalf("message without disconnect evidence = %q", msg)
	}

	clientCancelledErr := domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
	clientCancelledErr.Scope = domain.ScopeRequest
	status, msg = requestFailureStatusAndError(ctx, clientCancelledErr)
	if status != "CANCELLED" {
		t.Fatalf("status for client cancellation labelled client disconnected = %q, want CANCELLED", status)
	}
	if msg != "client disconnected" {
		t.Fatalf("message for client cancellation labelled client disconnected = %q", msg)
	}

	disconnectErr := domain.NewProxyErrorWithMessage(errors.New("write tcp: broken pipe"), false, "client disconnected")
	disconnectErr.Scope = domain.ScopeRequest
	status, msg = requestFailureStatusAndError(ctx, disconnectErr)
	if status != "CANCELLED" {
		t.Fatalf("status with disconnect evidence = %q, want CANCELLED", status)
	}
	if msg != "client disconnected" {
		t.Fatalf("message with disconnect evidence = %q", msg)
	}
}

func TestDisabledErrorCooldownDoesNotRetryBedrockAdaptiveThinkingSchemaError(t *testing.T) {
	proxyErr := domain.NewProxyErrorWithMessage(
		errors.New(`InvokeModelWithResponseStream: operation error Bedrock Runtime: InvokeModelWithResponseStream, https response error StatusCode: 400, ValidationException: "..enabled" is not supported for this model. Use "..adaptive" and "output_config.effort" to control thinking behavior.`),
		false,
		"upstream returned status 400",
	)
	proxyErr.Scope = domain.ScopeRequest
	proxyErr.HTTPStatusCode = http.StatusBadRequest

	if isDisabledErrorCooldownRetryableError(proxyErr) {
		t.Fatal("Bedrock adaptive-thinking schema error should not be retryable")
	}

	applyDisabledErrorCooldownRetryPolicy(
		&domain.Provider{Config: &domain.ProviderConfig{DisableErrorCooldown: true}},
		proxyErr,
	)
	if proxyErr.Retryable {
		t.Fatal("disableErrorCooldown should not force-retry Bedrock adaptive-thinking schema errors")
	}
}

func TestDisabledErrorCooldownStillRetriesOrdinaryHTTP400(t *testing.T) {
	proxyErr := domain.NewProxyErrorWithMessage(
		errors.New(`{"error":{"message":"temporary upstream 400"}}`),
		false,
		"upstream returned status 400",
	)
	proxyErr.Scope = domain.ScopeRequest
	proxyErr.HTTPStatusCode = http.StatusBadRequest

	if !isDisabledErrorCooldownRetryableError(proxyErr) {
		t.Fatal("ordinary HTTP 400 should still follow disableErrorCooldown retry policy")
	}
}

func newDisabledCooldownStreamTestExecutor(proxyRepo *recordingProxyRequestRepo, attemptRepo *recordingAttemptRepo) *Executor {
	return &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      attemptRepo,
		modelMappingRepo: &stubModelMappingRepo{},
		settingsRepo:     &stubExecutorSettingsRepo{},
		converter:        converter.GetGlobalRegistry(),
	}
}

func newDisabledCooldownStreamDispatchCtx(disableErrorCooldown bool, maxRetriesOverride ...int) (*flow.Ctx, *disabledCooldownStreamRetryAdapter, *recordingAttemptRepo, *recordingProxyRequestRepo) {
	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	adapter := &disabledCooldownStreamRetryAdapter{}
	maxRetries := 1
	if len(maxRetriesOverride) > 0 {
		maxRetries = maxRetriesOverride[0]
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         100,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "gpt-4o",
		isStream:     true,
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 20, ClientType: domain.ClientTypeOpenAI},
				Provider: &domain.Provider{
					ID:       20,
					TenantID: domain.DefaultTenantID,
					Type:     "custom",
					Name:     "custom-disabled-cooldown",
					Config:   &domain.ProviderConfig{DisableErrorCooldown: disableErrorCooldown},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: maxRetries, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	return c, adapter, attemptRepo, proxyRepo
}

func newDisabledCooldownHTTPErrorDispatchCtx(disableErrorCooldown bool, maxRetries int, succeedAfter int) (*flow.Ctx, *disabledCooldownHTTPErrorAdapter, *recordingAttemptRepo, *recordingProxyRequestRepo) {
	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	adapter := &disabledCooldownHTTPErrorAdapter{succeedAfter: succeedAfter}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         102,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeOpenAI,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeOpenAI,
		requestModel: "gpt-4o",
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 21, ClientType: domain.ClientTypeOpenAI},
				Provider: &domain.Provider{
					ID:       21,
					TenantID: domain.DefaultTenantID,
					Type:     "custom",
					Name:     "custom-disabled-cooldown-http-error",
					Config:   &domain.ProviderConfig{DisableErrorCooldown: disableErrorCooldown},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: maxRetries, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	return c, adapter, attemptRepo, proxyRepo
}
