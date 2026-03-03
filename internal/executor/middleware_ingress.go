package executor

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func (e *Executor) ingress(c *flow.Ctx) {
	state, ok := getExecState(c)
	if !ok {
		err := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "executor state missing")
		c.Err = err
		c.Abort()
		return
	}

	ctx := state.ctx
	state.tenantID = maxxctx.GetTenantID(ctx)
	if state.tenantID == 0 {
		state.tenantID = domain.DefaultTenantID
	}
	if v, ok := c.Get(flow.KeyClientType); ok {
		if ct, ok := v.(domain.ClientType); ok {
			state.clientType = ct
		}
	}
	if v, ok := c.Get(flow.KeyProjectID); ok {
		if pid, ok := v.(uint64); ok {
			state.projectID = pid
		}
	}
	if v, ok := c.Get(flow.KeySessionID); ok {
		if sid, ok := v.(string); ok {
			state.sessionID = sid
		}
	}
	if v, ok := c.Get(flow.KeyRequestModel); ok {
		if model, ok := v.(string); ok {
			state.requestModel = model
		}
	}
	if v, ok := c.Get(flow.KeyIsStream); ok {
		if s, ok := v.(bool); ok {
			state.isStream = s
		}
	}
	if v, ok := c.Get(flow.KeyAPITokenID); ok {
		if id, ok := v.(uint64); ok {
			state.apiTokenID = id
		}
	}
	if v, ok := c.Get(flow.KeyAPITokenDevMode); ok {
		if devMode, ok := v.(bool); ok {
			state.apiTokenDevMode = devMode
		}
	}
	if v, ok := c.Get(flow.KeyRequestBody); ok {
		if body, ok := v.([]byte); ok {
			state.requestBody = body
		}
	}
	if v, ok := c.Get(flow.KeyOriginalRequestBody); ok {
		if body, ok := v.([]byte); ok {
			state.originalRequestBody = body
		}
	}
	if v, ok := c.Get(flow.KeyRequestHeaders); ok {
		if headers, ok := v.(map[string][]string); ok {
			state.requestHeaders = headers
		}
		if headers, ok := v.(http.Header); ok {
			state.requestHeaders = headers
		}
	}
	if v, ok := c.Get(flow.KeyRequestURI); ok {
		if uri, ok := v.(string); ok {
			state.requestURI = uri
		}
	}

	proxyReq := &domain.ProxyRequest{
		TenantID:     state.tenantID,
		InstanceID:   e.instanceID,
		RequestID:    generateRequestID(),
		SessionID:    state.sessionID,
		ClientType:   state.clientType,
		ProjectID:    state.projectID,
		RequestModel: state.requestModel,
		StartTime:    time.Now(),
		IsStream:     state.isStream,
		Status:       "PENDING",
		APITokenID:   state.apiTokenID,
		DevMode:     state.apiTokenDevMode,
	}

	clearDetail := e.shouldClearRequestDetailFor(state)
	if !clearDetail {
		requestURI := state.requestURI
		requestHeaders := state.requestHeaders
		requestBody := state.requestBody
		headers := flattenHeaders(requestHeaders)
		if c.Request != nil {
			if c.Request.Host != "" {
				if headers == nil {
					headers = make(map[string]string)
				}
				headers["Host"] = c.Request.Host
			}
			proxyReq.RequestInfo = &domain.RequestInfo{
				Method:  c.Request.Method,
				URL:     requestURI,
				Headers: headers,
				Body:    string(requestBody),
			}
		}
	}

	if err := e.proxyRequestRepo.Create(proxyReq); err != nil {
		log.Printf("[Executor] Failed to create proxy request: %v", err)
	}

	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
	}

	state.proxyReq = proxyReq
	state.ctx = ctx

	if state.projectID == 0 && e.projectWaiter != nil {
		session, _ := e.sessionRepo.GetBySessionID(state.tenantID, state.sessionID)
		if session == nil {
			session = &domain.Session{
				TenantID:   state.tenantID,
				SessionID:  state.sessionID,
				ClientType: state.clientType,
				ProjectID:  0,
			}
		}

		if err := e.projectWaiter.WaitForProject(ctx, session); err != nil {
			status := "REJECTED"
			errorMsg := "project binding timeout: " + err.Error()
			if errors.Is(err, context.Canceled) {
				status = "CANCELLED"
				errorMsg = "client cancelled: " + err.Error()
				if e.broadcaster != nil {
					e.broadcaster.BroadcastMessage("session_pending_cancelled", map[string]interface{}{
						"sessionID": state.sessionID,
					})
				}
			}

			proxyReq.Status = status
			proxyReq.Error = errorMsg
			proxyReq.EndTime = time.Now()
			proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
			_ = e.proxyRequestRepo.Update(proxyReq)

			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			err := domain.NewProxyErrorWithMessage(err, false, "project binding required: "+err.Error())
			state.lastErr = err
			c.Err = err
			c.Abort()
			return
		}

		state.projectID = session.ProjectID
		proxyReq.ProjectID = state.projectID
		state.ctx = ctx
	}

	c.Next()
}
