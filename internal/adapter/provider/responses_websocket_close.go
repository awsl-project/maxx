package provider

import (
	"errors"
	"strings"

	"github.com/gorilla/websocket"
)

// ClassifyUpstreamResponsesWebSocketClose extracts structured close metadata
// and applies transport-only cooldown policy for restart/retry signals.
func ClassifyUpstreamResponsesWebSocketClose(err error, providerID uint64) (code int, reason string, ok bool) {
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		return 0, "", false
	}
	code = closeErr.Code
	reason = strings.TrimSpace(closeErr.Text)
	if code == websocket.CloseServiceRestart || code == websocket.CloseTryAgainLater {
		MarkResponsesWebSocketTransportUnavailable(providerID)
	}
	return code, reason, true
}
