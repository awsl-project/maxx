package executor

import (
	"fmt"
	"log"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

func (e *Executor) routeMatch(c *flow.Ctx) {
	state, ok := getExecState(c)
	if !ok {
		err := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "executor state missing")
		c.Err = err
		c.Abort()
		return
	}

	proxyReq := state.proxyReq
	routes, err := e.router.Match(&router.MatchContext{
		TenantID:     state.tenantID,
		ClientType:   state.clientType,
		ProjectID:    state.projectID,
		RequestModel: state.requestModel,
		APITokenID:   state.apiTokenID,
	})
	if err != nil {
		proxyReq.Status = "FAILED"
		proxyReq.Error = "no routes available"
		proxyReq.EndTime = time.Now()
		proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
		if err := e.proxyRequestRepo.Update(proxyReq); err != nil {
			log.Printf("[Executor] failed to update proxy request: %v", err)
		}
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(proxyReq)
		}
		err = domain.NewProxyErrorWithMessage(domain.ErrNoRoutes, false, fmt.Sprintf("route match failed: %v", err))
		state.lastErr = err
		c.Err = err
		c.Abort()
		return
	}

	if len(routes) == 0 {
		proxyReq.Status = "FAILED"
		proxyReq.Error = "no routes configured"
		proxyReq.EndTime = time.Now()
		proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
		if err := e.proxyRequestRepo.Update(proxyReq); err != nil {
			log.Printf("[Executor] failed to update proxy request: %v", err)
		}
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(proxyReq)
		}
		err = domain.NewProxyErrorWithMessage(domain.ErrNoRoutes, false, "no routes configured")
		state.lastErr = err
		c.Err = err
		c.Abort()
		return
	}

	proxyReq.Status = "IN_PROGRESS"
	if err := e.proxyRequestRepo.Update(proxyReq); err != nil {
		log.Printf("[Executor] failed to update proxy request: %v", err)
	}
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
	}
	state.routes = routes

	c.Next()
}
