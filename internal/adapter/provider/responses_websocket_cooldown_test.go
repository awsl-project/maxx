package provider

import (
	"testing"
	"time"
)

func TestResponsesWebSocketTransportCooldown(t *testing.T) {
	const providerID = uint64(99101)
	ClearResponsesWebSocketTransportCooldown(providerID)
	t.Cleanup(func() { ClearResponsesWebSocketTransportCooldown(providerID) })

	if !ResponsesWebSocketTransportAvailable(providerID) {
		t.Fatal("provider unexpectedly unavailable before cooldown")
	}
	MarkResponsesWebSocketTransportUnavailable(providerID)
	if ResponsesWebSocketTransportAvailable(providerID) {
		t.Fatal("provider remained available during websocket transport cooldown")
	}
	ClearResponsesWebSocketTransportCooldown(providerID)
	if !ResponsesWebSocketTransportAvailable(providerID) {
		t.Fatal("provider remained unavailable after cooldown clear")
	}
}

func TestResponsesWebSocketTransportCooldownExpires(t *testing.T) {
	const providerID = uint64(99102)
	ClearResponsesWebSocketTransportCooldown(providerID)
	t.Cleanup(func() { ClearResponsesWebSocketTransportCooldown(providerID) })

	responsesWebSocketTransportCooldowns.mu.Lock()
	responsesWebSocketTransportCooldowns.entries[providerID] = time.Now().Add(-time.Second)
	responsesWebSocketTransportCooldowns.mu.Unlock()

	if !ResponsesWebSocketTransportAvailable(providerID) {
		t.Fatal("expired websocket transport cooldown was not removed")
	}
}
