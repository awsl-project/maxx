package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

// staticErrorAdapter is a provider adapter that always returns a fixed error,
// modeling one hop of a cross-provider fallthrough.
type staticErrorAdapter struct {
	err   error
	calls int
}

func (a *staticErrorAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *staticErrorAdapter) Execute(_ *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	return a.err
}

// TestDispatchSurfacesFirstUpstreamErrorAfterCatchAllFallthrough reproduces the
// real incident: a request is routed to the fal provider first, which returns a
// meaningful "403 User is locked / balance" error; maxx fails over to an
// OpenAI-compatible catch-all (OpenRouter) which returns an unrelated "404 no
// model found". Before the fix the client saw ONLY the 404, masking the real
// cause. The surfaced final error must now reference the first provider's real
// 403 balance error too.
func TestDispatchSurfacesFirstUpstreamErrorAfterCatchAllFallthrough(t *testing.T) {
	const falProviderID = uint64(12)
	const catchAllProviderID = uint64(1)
	cooldown.Default().ClearCooldown(falProviderID, "", "")
	cooldown.Default().ClearCooldown(catchAllProviderID, "", "")
	defer cooldown.Default().ClearCooldown(falProviderID, "", "")
	defer cooldown.Default().ClearCooldown(catchAllProviderID, "", "")

	// First provider (fal): meaningful, retryable key-scoped error → fails over.
	falErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeKey, domain.CooldownReasonQuotaExhausted)
	falErr.Message = `fal returned status 403: {"detail":"User is locked. Reason: TOP_UP."}`
	falErr.HTTPStatusCode = http.StatusForbidden
	falAdapter := &staticErrorAdapter{err: falErr}

	// Catch-all provider (OpenRouter): unrelated 404, terminal.
	catchAllErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeModel, domain.CooldownReasonModelUnavailable)
	catchAllErr.Message = `404 {"error":{"message":"No model found for \"fal-ai/flux/schnell\"","code":404}}`
	catchAllErr.HTTPStatusCode = http.StatusNotFound
	catchAllErr.Retryable = false
	catchAllAdapter := &staticErrorAdapter{err: catchAllErr}

	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	e := &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      attemptRepo,
		modelMappingRepo: &stubModelMappingRepo{},
		settingsRepo:     &stubExecutorSettingsRepo{},
		converter:        converter.GetGlobalRegistry(),
	}

	const uri = "/v1/chat/completions"
	body := `{"model":"fal-ai/flux/schnell"}`
	req := httptest.NewRequest(http.MethodPost, uri, strings.NewReader(body)).WithContext(context.Background())
	c := flow.NewCtx(httptest.NewRecorder(), req)

	proxyReq := &domain.ProxyRequest{
		ID:           303,
		TenantID:     domain.DefaultTenantID,
		ClientType:   domain.ClientTypeOpenAI,
		RequestModel: "fal-ai/flux/schnell",
		Status:       "IN_PROGRESS",
		StartTime:    time.Now(),
	}
	noRetry := &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0}
	state := &execState{
		ctx:                 context.Background(),
		proxyReq:            proxyReq,
		tenantID:            domain.DefaultTenantID,
		clientType:          domain.ClientTypeOpenAI,
		requestModel:        "fal-ai/flux/schnell",
		requestBody:         []byte(body),
		originalRequestBody: []byte(body),
		requestHeaders:      http.Header{"Content-Type": []string{"application/json"}},
		requestURI:          uri,
		routes: []*router.MatchedRoute{
			{
				Route:           &domain.Route{ID: 1, TenantID: domain.DefaultTenantID, ProviderID: falProviderID, ClientType: domain.ClientTypeOpenAI},
				Provider:        &domain.Provider{ID: falProviderID, TenantID: domain.DefaultTenantID, Type: "fal", Name: "fal"},
				ProviderAdapter: falAdapter,
				RetryConfig:     noRetry,
			},
			{
				Route:           &domain.Route{ID: 2, TenantID: domain.DefaultTenantID, ProviderID: catchAllProviderID, ClientType: domain.ClientTypeOpenAI},
				Provider:        &domain.Provider{ID: catchAllProviderID, TenantID: domain.DefaultTenantID, Type: "openrouter", Name: "openrouter"},
				ProviderAdapter: catchAllAdapter,
				RetryConfig:     noRetry,
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch to fail after both providers errored")
	}
	if falAdapter.calls != 1 {
		t.Fatalf("fal adapter calls = %d, want 1", falAdapter.calls)
	}
	if catchAllAdapter.calls != 1 {
		t.Fatalf("catch-all adapter calls = %d, want 1 (fallthrough must still occur)", catchAllAdapter.calls)
	}

	surfaced := c.Err.Error()
	// The real first-provider cause must be observable in the surfaced error.
	if !strings.Contains(surfaced, "provider 12") {
		t.Fatalf("surfaced error does not reference first provider (12): %q", surfaced)
	}
	if !strings.Contains(surfaced, "User is locked") {
		t.Fatalf("surfaced error masked the real 403 balance cause: %q", surfaced)
	}
	// The final catch-all 404 should still be present (we augment, not replace).
	if !strings.Contains(surfaced, "No model found") {
		t.Fatalf("surfaced error dropped the final catch-all error: %q", surfaced)
	}

	// The persisted proxy request error must carry the surfaced (augmented) text.
	if len(proxyRepo.updated) == 0 {
		t.Fatal("expected proxy request updates")
	}
	finalReq := proxyRepo.updated[len(proxyRepo.updated)-1]
	if finalReq.Status != "FAILED" {
		t.Fatalf("final proxy request status = %q, want FAILED", finalReq.Status)
	}
	if !strings.Contains(finalReq.Error, "User is locked") {
		t.Fatalf("persisted request error masked the real cause: %q", finalReq.Error)
	}

	// No secrets: the summary is built only from provider ids + upstream
	// status/message, never from headers/keys.
	if strings.Contains(surfaced, "Authorization") || strings.Contains(surfaced, "Bearer ") {
		t.Fatalf("surfaced error unexpectedly contains auth material: %q", surfaced)
	}
}

// TestDispatchSingleProviderErrorNotAugmented ensures the surfacing logic only
// fires on genuine cross-provider fallthrough — a single provider's error is
// returned verbatim, with no "first upstream" note (nothing was masked).
func TestDispatchSingleProviderErrorNotAugmented(t *testing.T) {
	const providerID = uint64(77)
	cooldown.Default().ClearCooldown(providerID, "", "")
	defer cooldown.Default().ClearCooldown(providerID, "", "")

	onlyErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeModel, domain.CooldownReasonModelUnavailable)
	onlyErr.Message = `404 {"error":{"message":"boom"}}`
	onlyErr.HTTPStatusCode = http.StatusNotFound
	onlyErr.Retryable = false
	adapter := &staticErrorAdapter{err: onlyErr}

	proxyRepo := &recordingProxyRequestRepo{}
	e := &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      &recordingAttemptRepo{},
		modelMappingRepo: &stubModelMappingRepo{},
		settingsRepo:     &stubExecutorSettingsRepo{},
		converter:        converter.GetGlobalRegistry(),
	}

	const uri = "/v1/chat/completions"
	body := `{"model":"m"}`
	req := httptest.NewRequest(http.MethodPost, uri, strings.NewReader(body)).WithContext(context.Background())
	c := flow.NewCtx(httptest.NewRecorder(), req)

	proxyReq := &domain.ProxyRequest{
		ID:           304,
		TenantID:     domain.DefaultTenantID,
		ClientType:   domain.ClientTypeOpenAI,
		RequestModel: "m",
		Status:       "IN_PROGRESS",
		StartTime:    time.Now(),
	}
	state := &execState{
		ctx:                 context.Background(),
		proxyReq:            proxyReq,
		tenantID:            domain.DefaultTenantID,
		clientType:          domain.ClientTypeOpenAI,
		requestModel:        "m",
		requestBody:         []byte(body),
		originalRequestBody: []byte(body),
		requestHeaders:      http.Header{"Content-Type": []string{"application/json"}},
		requestURI:          uri,
		routes: []*router.MatchedRoute{
			{
				Route:           &domain.Route{ID: 1, TenantID: domain.DefaultTenantID, ProviderID: providerID, ClientType: domain.ClientTypeOpenAI},
				Provider:        &domain.Provider{ID: providerID, TenantID: domain.DefaultTenantID, Type: "custom", Name: "solo"},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch to fail")
	}
	if strings.Contains(c.Err.Error(), "first upstream") {
		t.Fatalf("single-provider error should not be augmented with a first-upstream note: %q", c.Err.Error())
	}
}
