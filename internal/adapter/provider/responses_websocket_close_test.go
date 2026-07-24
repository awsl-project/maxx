package provider

import (
	"errors"
	"testing"

	"github.com/gorilla/websocket"
)

func TestClassifyUpstreamResponsesWebSocketClose(t *testing.T) {
	const providerID = uint64(99103)
	ClearResponsesWebSocketTransportCooldown(providerID)
	t.Cleanup(func() { ClearResponsesWebSocketTransportCooldown(providerID) })

	code, reason, ok := ClassifyUpstreamResponsesWebSocketClose(
		&websocket.CloseError{Code: websocket.CloseServiceRestart, Text: " replay over HTTP "},
		providerID,
	)
	if !ok || code != websocket.CloseServiceRestart || reason != "replay over HTTP" {
		t.Fatalf("classified close = (%d, %q, %v)", code, reason, ok)
	}
	if ResponsesWebSocketTransportAvailable(providerID) {
		t.Fatal("1012 did not trigger websocket transport cooldown")
	}
}

func TestClassifyUpstreamResponsesWebSocketCloseIgnoresOrdinaryErrors(t *testing.T) {
	if code, reason, ok := ClassifyUpstreamResponsesWebSocketClose(errors.New("read failed"), 99104); ok || code != 0 || reason != "" {
		t.Fatalf("ordinary error classified as close = (%d, %q, %v)", code, reason, ok)
	}
}
