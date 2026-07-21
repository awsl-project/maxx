package context

import (
	stdcontext "context"
)

type responsesWebSocketRequestKey struct{}

// ResponsesWebSocketRequest carries the immutable response.create frame and
// downstream WebSocket connection ID used by the control-plane request.
type ResponsesWebSocketRequest struct {
	SessionID string
	Payload   []byte
}

// WithResponsesWebSocketRequest attaches immutable WebSocket request metadata to
// ctx. The payload is valid for the synchronous lifetime of the proxied request;
// consumers must treat it as read-only and copy before modifying it.
func WithResponsesWebSocketRequest(ctx stdcontext.Context, sessionID string, payload []byte) stdcontext.Context {
	return stdcontext.WithValue(ctx, responsesWebSocketRequestKey{}, ResponsesWebSocketRequest{
		SessionID: sessionID,
		Payload:   payload,
	})
}

// GetResponsesWebSocketRequest returns immutable WebSocket request metadata.
func GetResponsesWebSocketRequest(ctx stdcontext.Context) (ResponsesWebSocketRequest, bool) {
	if ctx == nil {
		return ResponsesWebSocketRequest{}, false
	}
	request, ok := ctx.Value(responsesWebSocketRequestKey{}).(ResponsesWebSocketRequest)
	if !ok || request.SessionID == "" || len(request.Payload) == 0 {
		return ResponsesWebSocketRequest{}, false
	}
	return request, true
}
