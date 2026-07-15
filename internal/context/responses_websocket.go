package context

import (
	"bytes"
	stdcontext "context"
)

type responsesWebSocketRequestKey struct{}

// ResponsesWebSocketRequest carries the original downstream WebSocket event
// alongside the HTTP/SSE fallback request built by the handler.
type ResponsesWebSocketRequest struct {
	SessionID string
	Payload   []byte
}

// WithResponsesWebSocketRequest attaches WebSocket request metadata to ctx.
func WithResponsesWebSocketRequest(ctx stdcontext.Context, sessionID string, payload []byte) stdcontext.Context {
	return stdcontext.WithValue(ctx, responsesWebSocketRequestKey{}, ResponsesWebSocketRequest{
		SessionID: sessionID,
		Payload:   bytes.Clone(payload),
	})
}

// GetResponsesWebSocketRequest returns a copy of the WebSocket request metadata.
func GetResponsesWebSocketRequest(ctx stdcontext.Context) (ResponsesWebSocketRequest, bool) {
	if ctx == nil {
		return ResponsesWebSocketRequest{}, false
	}
	request, ok := ctx.Value(responsesWebSocketRequestKey{}).(ResponsesWebSocketRequest)
	if !ok || request.SessionID == "" || len(request.Payload) == 0 {
		return ResponsesWebSocketRequest{}, false
	}
	request.Payload = bytes.Clone(request.Payload)
	return request, true
}
