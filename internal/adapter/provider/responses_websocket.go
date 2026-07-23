package provider

import (
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// ResponsesWebSocketAdapter is an optional native Responses WebSocket
// transport capability. HTTP-only adapters must not implement it.
type ResponsesWebSocketAdapter interface {
	ExecuteResponsesWebSocket(
		c *flow.Ctx,
		provider *domain.Provider,
		exchange *domain.ResponsesWebSocketExchange,
	) (*domain.ResponsesWebSocketResult, error)
}

// ResponsesWebSocketSessionCleaner releases upstream sessions owned by one
// downstream WebSocket connection.
type ResponsesWebSocketSessionCleaner interface {
	CloseResponsesWebSocketConnection(connectionID string)
}
