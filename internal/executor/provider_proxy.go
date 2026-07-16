package executor

import (
	"context"
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

	ctx := c.Request.Context()
	if v, ok := c.Get(flow.KeyProxyContext); ok {
		if stored, ok := v.(context.Context); ok && stored != nil {
			ctx = stored
		}
	}

	retryConfig := (*domain.RetryConfig)(nil)
	if route.RetryConfigID != 0 && e.retryConfigRepo != nil {
		retryConfig, _ = e.retryConfigRepo.GetByID(proxyReq.TenantID, route.RetryConfigID)
	}
	if retryConfig == nil {
		retryConfig = e.getRetryConfig(proxyReq.TenantID, nil)
	}

	state := &execState{
		ctx:      ctx,
		proxyReq: proxyReq,
		routes: []*router.MatchedRoute{{
			Route:           route,
			Provider:        provider,
			ProviderAdapter: adapter,
			RetryConfig:     retryConfig,
		}},

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
