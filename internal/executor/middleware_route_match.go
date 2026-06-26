package executor

import (
	"fmt"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

func (e *Executor) routeMatch(c *flow.Ctx) {
	state, ok := getExecState(c)
	if !ok {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "executor state missing")
		proxyErr.Scope = domain.ScopeRequest
		c.Err = proxyErr
		c.Abort()
		return
	}

	result, err := e.router.Match(&router.MatchContext{
		Ctx:          state.ctx,
		TenantID:     state.tenantID,
		ClientType:   state.clientType,
		ProjectID:    state.projectID,
		RequestModel: state.requestModel,
		APITokenID:   state.apiTokenID,
		SessionID:    state.sessionID,
	})
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrNoRoutes, false, fmt.Sprintf("route match failed: %v", err))
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
		state.lastErr = proxyErr
		c.Err = proxyErr
		c.Abort()
		return
	}

	if len(result.Routes) == 0 {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrNoRoutes, false, "no routes configured")
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
		state.lastErr = proxyErr
		c.Err = proxyErr
		c.Abort()
		return
	}

	proxyReq := e.newProxyRequest(c, state, "IN_PROGRESS")
	e.createProxyRequest(proxyReq)
	state.proxyReq = proxyReq
	state.routes = result.Routes
	state.stickyWrite = result.Sticky

	c.Next()
}
