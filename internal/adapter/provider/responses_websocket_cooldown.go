package provider

import (
	"sync"
	"time"
)

const responsesWebSocketTransportCooldownTTL = 30 * time.Second

type responsesWebSocketTransportCooldownStore struct {
	mu      sync.Mutex
	entries map[uint64]time.Time
}

var responsesWebSocketTransportCooldowns = &responsesWebSocketTransportCooldownStore{
	entries: make(map[uint64]time.Time),
}

// MarkResponsesWebSocketTransportUnavailable temporarily removes a provider
// from Responses WebSocket routing without affecting its HTTP/SSE traffic.
func MarkResponsesWebSocketTransportUnavailable(providerID uint64) {
	if providerID == 0 {
		return
	}
	responsesWebSocketTransportCooldowns.mu.Lock()
	responsesWebSocketTransportCooldowns.entries[providerID] = time.Now().Add(responsesWebSocketTransportCooldownTTL)
	responsesWebSocketTransportCooldowns.mu.Unlock()
}

// ResponsesWebSocketTransportAvailable reports whether a provider may accept
// a new upstream Responses WebSocket session.
func ResponsesWebSocketTransportAvailable(providerID uint64) bool {
	if providerID == 0 {
		return false
	}
	now := time.Now()
	responsesWebSocketTransportCooldowns.mu.Lock()
	defer responsesWebSocketTransportCooldowns.mu.Unlock()
	expiresAt, ok := responsesWebSocketTransportCooldowns.entries[providerID]
	if !ok {
		return true
	}
	if !now.Before(expiresAt) {
		delete(responsesWebSocketTransportCooldowns.entries, providerID)
		return true
	}
	return false
}

// ClearResponsesWebSocketTransportCooldown allows a refreshed provider
// configuration to probe its WebSocket endpoint immediately.
func ClearResponsesWebSocketTransportCooldown(providerID uint64) {
	responsesWebSocketTransportCooldowns.mu.Lock()
	delete(responsesWebSocketTransportCooldowns.entries, providerID)
	responsesWebSocketTransportCooldowns.mu.Unlock()
}
