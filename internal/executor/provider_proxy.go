package executor

import (
	"context"
	"errors"
	"net/http"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

// MapModelForProviderProxy applies the same model-mapping contract used by the
// generic dispatch path to direct /provider/<id>/... requests.
func (e *Executor) MapModelForProviderProxy(tenantID uint64, requestModel string, route *domain.Route, provider *domain.Provider, clientType domain.ClientType, projectID uint64, apiTokenID uint64) string {
	if e == nil || e.modelMappingRepo == nil || route == nil || provider == nil {
		return requestModel
	}
	return e.mapModel(tenantID, requestModel, route, provider, clientType, projectID, apiTokenID)
}

// MatchProviderProxyRoute applies the generic route matcher to direct
// /provider/<id>/... requests, then constrains the result to the requested
// provider. This preserves project/session/token route selection, cooldown,
// model support checks and retry policy resolution without allowing provider
// proxy requests to fail over to a different provider.
func (e *Executor) MatchProviderProxyRoute(ctx context.Context, tenantID uint64, providerID uint64, clientType domain.ClientType, projectID uint64, requestModel string, apiTokenID uint64, sessionID string) (*router.MatchedRoute, error) {
	if e == nil || e.router == nil || providerID == 0 || clientType == "" {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "provider proxy route match input missing")
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.HTTPStatusCode = http.StatusInternalServerError
		return nil, proxyErr
	}

	result, err := e.router.Match(&router.MatchContext{
		Ctx:          ctx,
		TenantID:     tenantID,
		ClientType:   clientType,
		ProjectID:    projectID,
		RequestModel: requestModel,
		ModelCandidates: func(route *domain.Route, provider *domain.Provider, clientType domain.ClientType, requestModel string) []string {
			return e.mapModelCandidates(tenantID, requestModel, route, provider, clientType, projectID, apiTokenID)
		},
		APITokenID: apiTokenID,
		SessionID:  sessionID,
	})
	if err != nil {
		return nil, err
	}
	for _, matched := range result.Routes {
		if matched != nil && matched.Provider != nil && matched.Provider.ID == providerID {
			return matched, nil
		}
	}
	return nil, domain.ErrNoAvailableProviders
}

// ExecuteProviderProxy dispatches a direct /provider/<id>/... request through
// the normal executor retry/attempt/billing path, constrained to the requested
// provider. It intentionally does not route/fail over to other providers.
func (e *Executor) ExecuteProviderProxy(c *flow.Ctx, proxyReq *domain.ProxyRequest, route *domain.Route, provider *domain.Provider, adapter provideradapter.ProviderAdapter) error {
	if e == nil || c == nil || proxyReq == nil || route == nil || provider == nil || adapter == nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "provider proxy executor input missing")
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.HTTPStatusCode = http.StatusInternalServerError
		return proxyErr
	}

	retryConfig := (*domain.RetryConfig)(nil)
	if route.RetryConfigID != 0 && e.retryConfigRepo != nil {
		retryConfig, _ = e.retryConfigRepo.GetByID(proxyReq.TenantID, route.RetryConfigID)
	}
	if retryConfig == nil {
		retryConfig = e.getRetryConfig(proxyReq.TenantID, nil)
	}

	return e.ExecuteProviderProxyMatched(c, proxyReq, &router.MatchedRoute{
		Route:           route,
		Provider:        provider,
		ProviderAdapter: adapter,
		RetryConfig:     retryConfig,
	})
}

// ExecuteProviderProxyMatched runs a direct provider proxy request through the
// normal dispatch loop using an already matched, provider-constrained route.
func (e *Executor) ExecuteProviderProxyMatched(c *flow.Ctx, proxyReq *domain.ProxyRequest, matched *router.MatchedRoute) error {
	if e == nil || c == nil || proxyReq == nil || matched == nil || matched.Route == nil || matched.Provider == nil || matched.ProviderAdapter == nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "provider proxy matched executor input missing")
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.HTTPStatusCode = http.StatusInternalServerError
		return proxyErr
	}
	if matched.RetryConfig == nil {
		matched.RetryConfig = e.getRetryConfig(proxyReq.TenantID, nil)
	}

	ctx := c.Request.Context()
	if v, ok := c.Get(flow.KeyProxyContext); ok {
		if stored, ok := v.(context.Context); ok && stored != nil {
			ctx = stored
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ctx.Err()
	}

	state := &execState{
		ctx:      ctx,
		proxyReq: proxyReq,
		routes:   []*router.MatchedRoute{matched},

		tenantID:            proxyReq.TenantID,
		clientType:          proxyReq.ClientType,
		projectID:           proxyReq.ProjectID,
		sessionID:           proxyReq.SessionID,
		requestModel:        proxyReq.RequestModel,
		isStream:            proxyReq.IsStream,
		apiTokenID:          proxyReq.APITokenID,
		apiTokenDevMode:     proxyReq.DevMode,
		requestBody:         flow.GetRequestBody(c),
		originalRequestBody: flow.GetOriginalRequestBody(c),
		requestHeaders:      flow.GetRequestHeaders(c),
		requestURI:          flow.GetRequestURI(c),
	}
	if len(state.originalRequestBody) == 0 {
		state.originalRequestBody = state.requestBody
	}
	if state.requestHeaders == nil {
		state.requestHeaders = http.Header{}
	}

	c.Set(flow.KeyExecutorState, state)
	e.dispatch(c)
	return state.lastErr
}
