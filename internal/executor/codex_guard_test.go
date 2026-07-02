package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/codexguard"
	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/router"
)

type codexGuardProxyRequestRepo struct {
	updated []*domain.ProxyRequest
}

func (r *codexGuardProxyRequestRepo) Create(req *domain.ProxyRequest) error { return nil }

func (r *codexGuardProxyRequestRepo) Update(req *domain.ProxyRequest) error {
	snap := *req
	r.updated = append(r.updated, &snap)
	return nil
}

func (r *codexGuardProxyRequestRepo) GetByID(uint64, uint64) (*domain.ProxyRequest, error) {
	return nil, nil
}

func (r *codexGuardProxyRequestRepo) List(uint64, int, int) ([]*domain.ProxyRequest, error) {
	return nil, nil
}

func (r *codexGuardProxyRequestRepo) ListCursor(uint64, int, uint64, uint64, *repository.ProxyRequestFilter) ([]*domain.ProxyRequest, error) {
	return nil, nil
}

func (r *codexGuardProxyRequestRepo) ListActive(uint64) ([]*domain.ProxyRequest, error) {
	return nil, nil
}

func (r *codexGuardProxyRequestRepo) Count(uint64) (int64, error) { return 0, nil }

func (r *codexGuardProxyRequestRepo) CountWithFilter(uint64, *repository.ProxyRequestFilter) (int64, error) {
	return 0, nil
}

func (r *codexGuardProxyRequestRepo) GetErrorStats(uint64, *repository.ProxyRequestFilter) (*repository.ProxyRequestErrorStats, error) {
	return nil, nil
}

func (r *codexGuardProxyRequestRepo) CountFailedWithFilter(uint64, *repository.ProxyRequestFilter) (int64, error) {
	return 0, nil
}

func (r *codexGuardProxyRequestRepo) DeleteFailedWithFilter(uint64, *repository.ProxyRequestFilter) (int64, int64, error) {
	return 0, 0, nil
}

func (r *codexGuardProxyRequestRepo) UpdateProjectIDBySessionID(uint64, string, uint64) (int64, error) {
	return 0, nil
}

func (r *codexGuardProxyRequestRepo) MarkStaleAsFailed([]string) (int64, error) {
	return 0, nil
}

func (r *codexGuardProxyRequestRepo) FixFailedRequestsWithoutEndTime() (int64, error) {
	return 0, nil
}

func (r *codexGuardProxyRequestRepo) DeleteOlderThan(time.Time) (int64, error) {
	return 0, nil
}

func (r *codexGuardProxyRequestRepo) HasRecentRequests(time.Time) (bool, error) {
	return false, nil
}

func (r *codexGuardProxyRequestRepo) GetProjectUsageSummaries(uint64, time.Time, ...uint64) (map[uint64]domain.ProjectUsageSummary, error) {
	return nil, nil
}

func (r *codexGuardProxyRequestRepo) UpdateCost(uint64, uint64) error { return nil }

func (r *codexGuardProxyRequestRepo) UpdateCostAtomically(uint64, uint64, map[uint64]domain.AttemptCostUpdate) error {
	return nil
}

func (r *codexGuardProxyRequestRepo) RecalculateCostsFromAttempts() (int64, error) {
	return 0, nil
}

func (r *codexGuardProxyRequestRepo) RecalculateCostsFromAttemptsWithProgress(chan<- domain.Progress) (int64, error) {
	return 0, nil
}

func (r *codexGuardProxyRequestRepo) ClearDetailOlderThan(time.Time, []string) (int64, error) {
	return 0, nil
}

type codexGuardSettingsRepo struct {
	values map[string]string
	getErr error
}

func (r *codexGuardSettingsRepo) Get(key string) (string, error) {
	if r.getErr != nil {
		return "", r.getErr
	}
	if r.values == nil {
		return "", nil
	}
	return r.values[key], nil
}

func (r *codexGuardSettingsRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *codexGuardSettingsRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }

func (r *codexGuardSettingsRepo) Delete(key string) error { return nil }

type codexGuardModelMappingRepo struct{}

func (r *codexGuardModelMappingRepo) Create(*domain.ModelMapping) error { return nil }
func (r *codexGuardModelMappingRepo) Update(*domain.ModelMapping) error { return nil }
func (r *codexGuardModelMappingRepo) Delete(uint64, uint64) error       { return nil }
func (r *codexGuardModelMappingRepo) GetByID(uint64, uint64) (*domain.ModelMapping, error) {
	return nil, nil
}
func (r *codexGuardModelMappingRepo) List(uint64) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *codexGuardModelMappingRepo) ListEnabled(uint64) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *codexGuardModelMappingRepo) ListByClientType(uint64, domain.ClientType) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *codexGuardModelMappingRepo) ListByQuery(uint64, *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *codexGuardModelMappingRepo) Count(uint64) (int, error) { return 0, nil }
func (r *codexGuardModelMappingRepo) DeleteAll(uint64) error    { return nil }
func (r *codexGuardModelMappingRepo) ClearAll(uint64) error     { return nil }
func (r *codexGuardModelMappingRepo) SeedDefaults(uint64) error { return nil }

type codexGuardSequenceAdapter struct {
	errors []error
	calls  int
}

func (a *codexGuardSequenceAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

func (a *codexGuardSequenceAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	if a.calls <= len(a.errors) && a.errors[a.calls-1] != nil {
		return a.errors[a.calls-1]
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write([]byte(`{"ok":true}`))
	return nil
}

func TestGetCodexGuardConfig(t *testing.T) {
	t.Run("unset uses defaults", func(t *testing.T) {
		e := &Executor{settingsRepo: &codexGuardSettingsRepo{}}

		got := e.getCodexGuardConfig()

		if got.Enabled {
			t.Fatal("Enabled = true, want false")
		}
		if got.MaxAttempts != codexguard.DefaultConfig().MaxAttempts {
			t.Fatalf("MaxAttempts = %d, want default", got.MaxAttempts)
		}
	})

	t.Run("settings read error disables guard", func(t *testing.T) {
		e := &Executor{settingsRepo: &codexGuardSettingsRepo{getErr: errors.New("read failed")}}

		got := e.getCodexGuardConfig()

		if got.Enabled {
			t.Fatal("Enabled = true, want false")
		}
	})

	t.Run("invalid stored config disables guard", func(t *testing.T) {
		e := &Executor{settingsRepo: &codexGuardSettingsRepo{values: map[string]string{
			domain.SettingKeyCodexReasoningGuard: `{"max_attempts":0}`,
		}}}

		got := e.getCodexGuardConfig()

		if got.Enabled {
			t.Fatal("Enabled = true, want false")
		}
	})
}

func TestDispatchRetriesCodexReasoningGuardWithoutOrdinaryRetryBudget(t *testing.T) {
	guardErr := newTestCodexGuardProxyError()
	proxyRepo := &codexGuardProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	adapter := &codexGuardSequenceAdapter{errors: []error{guardErr}}
	e := newCodexGuardTestExecutor(proxyRepo, attemptRepo, enabledCodexGuardSettings())

	c := newCodexGuardDispatchCtx(adapter)
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
	if body := c.Writer.(*httptest.ResponseRecorder).Body.String(); body != `{"ok":true}` {
		t.Fatalf("client body = %q, want success body", body)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request update, got %#v", proxyRepo.updated)
	}
}

func TestDispatchStopsAfterCodexReasoningGuardAttemptsExhausted(t *testing.T) {
	guardErr := newTestCodexGuardProxyError()
	proxyRepo := &codexGuardProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	adapter := &codexGuardSequenceAdapter{errors: []error{guardErr, guardErr}}
	e := newCodexGuardTestExecutor(proxyRepo, attemptRepo, enabledCodexGuardSettings())

	c := newCodexGuardDispatchCtx(adapter)
	e.dispatch(c)

	if c.Err == nil {
		t.Fatalf("expected dispatch error")
	}
	if !codexguard.IsReasoningGuardError(c.Err) {
		t.Fatalf("expected reasoning guard error, got %v", c.Err)
	}
	if adapter.calls != 2 {
		t.Fatalf("adapter calls = %d, want 2", adapter.calls)
	}
	if len(attemptRepo.created) != 2 {
		t.Fatalf("created attempts = %d, want 2", len(attemptRepo.created))
	}
	if got := c.Writer.(*httptest.ResponseRecorder).Body.String(); got != "" {
		t.Fatalf("expected no client body to be written by dispatch, got %q", got)
	}
	if len(proxyRepo.updated) == 0 {
		t.Fatalf("expected proxy request updates")
	}
	last := proxyRepo.updated[len(proxyRepo.updated)-1]
	if last.Status != "FAILED" {
		t.Fatalf("final proxy request status = %q, want FAILED", last.Status)
	}
	if last.StatusCode != 502 {
		t.Fatalf("final proxy request status code = %d, want 502", last.StatusCode)
	}
}

func enabledCodexGuardSettings() map[string]string {
	return map[string]string{
		domain.SettingKeyCodexReasoningGuard: `{"enabled":true,"blocked_reasoning_tokens":[516,1034,1552],"max_attempts":2,"status_code":502,"error_code":"reasoning_guard_triggered","mode":"non_stream"}`,
	}
}

func newCodexGuardTestExecutor(
	proxyRepo *codexGuardProxyRequestRepo,
	attemptRepo *recordingAttemptRepo,
	settings map[string]string,
) *Executor {
	return &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      attemptRepo,
		modelMappingRepo: &codexGuardModelMappingRepo{},
		settingsRepo:     &codexGuardSettingsRepo{values: settings},
		converter:        converter.GetGlobalRegistry(),
	}
}

func newCodexGuardDispatchCtx(adapter *codexGuardSequenceAdapter) *flow.Ctx {
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(context.Background())
	c := flow.NewCtx(httptest.NewRecorder(), req)

	proxyReq := &domain.ProxyRequest{
		ID:         100,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeCodex,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeCodex,
		requestModel: "gpt-5",
		routes: []*router.MatchedRoute{
			{
				Route:           &domain.Route{ID: 10, TenantID: domain.DefaultTenantID, ProviderID: 20, ClientType: domain.ClientTypeCodex},
				Provider:        &domain.Provider{ID: 20, TenantID: domain.DefaultTenantID, Type: "codex", Name: "codex"},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	return c
}

func newTestCodexGuardProxyError() *domain.ProxyError {
	cfg := codexguard.DefaultConfig()
	guardErr := codexguard.NewReasoningGuardError(516, cfg)
	proxyErr := domain.NewProxyErrorWithMessage(guardErr, false, "codex reasoning guard triggered")
	proxyErr.Scope = domain.ScopeRequest
	proxyErr.HTTPStatusCode = cfg.StatusCode
	proxyErr.Code = cfg.ErrorCode
	return proxyErr
}
