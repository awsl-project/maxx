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
	stickyWrite    *router.StickyWrite
	currentAttempt *domain.ProxyUpstreamAttempt
	lastErr        error

	// attemptTrail records a one-line summary of every failed upstream attempt
	// (provider id → status/message) across all routes for this request. When a
	// request ends in failure after failing over between providers, the final
	// error alone can be misleading — e.g. a meaningful "provider N: 403 balance"
	// gets masked by a catch-all provider's unrelated "404 no model". The trail
	// lets us surface the real first cause both in the returned error and in a
	// single WARN log line. It never contains auth headers or keys — only
	// provider ids and upstream status/message text.
	attemptTrail []attemptFailure

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
	wsExchange          *domain.ResponsesWebSocketExchange
}

func getExecState(c *flow.Ctx) (*execState, bool) {
	v, ok := c.Get(flow.KeyExecutorState)
	if !ok {
		return nil, false
	}
	st, ok := v.(*execState)
	return st, ok
}
