package executor

import (
	"context"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

type execState struct {
	ctx            context.Context
	proxyReq       *domain.ProxyRequest
	routes         []*router.MatchedRoute
	currentAttempt *domain.ProxyUpstreamAttempt
	lastErr        error

	tenantID            uint64
	clientType          domain.ClientType
	projectID           uint64
	sessionID           string
	requestModel        string
	isStream            bool
	apiTokenID          uint64
	apiTokenDevMode     bool
	requestBody         []byte
	originalRequestBody []byte
	requestHeaders      http.Header
	requestURI          string
}

func getExecState(c *flow.Ctx) (*execState, bool) {
	v, ok := c.Get(flow.KeyExecutorState)
	if !ok {
		return nil, false
	}
	st, ok := v.(*execState)
	return st, ok
}
