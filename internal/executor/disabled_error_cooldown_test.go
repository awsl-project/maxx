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

type disabledCooldownStreamRetryAdapter struct {
	calls int
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

func TestDispatchDoesNotRetryCommittedStreamReadErrorWhenErrorCooldownEnabled(t *testing.T) {
	c, adapter, _, proxyRepo := newDisabledCooldownStreamDispatchCtx(false)
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
	e := newDisabledCooldownStreamTestExecutor(&codexGuardProxyRequestRepo{}, &recordingAttemptRepo{})

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
	if proxyReq.Status != "CANCELLED" {
		t.Fatalf("proxy request status = %q, want CANCELLED", proxyReq.Status)
	}
	if proxyReq.Error != "client disconnected during retry wait" {
		t.Fatalf("proxy request error = %q, want client disconnected during retry wait", proxyReq.Error)
	}
}

func newDisabledCooldownStreamTestExecutor(proxyRepo *codexGuardProxyRequestRepo, attemptRepo *recordingAttemptRepo) *Executor {
	return &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      attemptRepo,
		modelMappingRepo: &codexGuardModelMappingRepo{},
		settingsRepo:     &codexGuardSettingsRepo{},
		converter:        converter.GetGlobalRegistry(),
	}
}

func newDisabledCooldownStreamDispatchCtx(disableErrorCooldown bool) (*flow.Ctx, *disabledCooldownStreamRetryAdapter, *recordingAttemptRepo, *codexGuardProxyRequestRepo) {
	proxyRepo := &codexGuardProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	adapter := &disabledCooldownStreamRetryAdapter{}
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
				RetryConfig:     &domain.RetryConfig{MaxRetries: 1, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	return c, adapter, attemptRepo, proxyRepo
}
