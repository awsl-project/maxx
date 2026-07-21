package codex

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const codexWebSocketUnsupportedTTL = 5 * time.Minute

type codexWebSocketUnsupportedEntry struct {
	reason    string
	expiresAt time.Time
}

type codexWebSocketUnsupportedCache struct {
	mu      sync.Mutex
	entries map[string]codexWebSocketUnsupportedEntry
}

var globalCodexWebSocketUnsupported = &codexWebSocketUnsupportedCache{
	entries: make(map[string]codexWebSocketUnsupportedEntry),
}

func codexWebSocketUnsupportedKey(providerID uint64, target string) string {
	return fmt.Sprintf("%d:%s", providerID, target)
}

func markCodexWebSocketUnsupported(providerID uint64, target, reason string) {
	globalCodexWebSocketUnsupported.mu.Lock()
	globalCodexWebSocketUnsupported.entries[codexWebSocketUnsupportedKey(providerID, target)] = codexWebSocketUnsupportedEntry{
		reason:    reason,
		expiresAt: time.Now().Add(codexWebSocketUnsupportedTTL),
	}
	globalCodexWebSocketUnsupported.mu.Unlock()
}

func isCodexWebSocketUnsupported(providerID uint64, target string) bool {
	key := codexWebSocketUnsupportedKey(providerID, target)
	globalCodexWebSocketUnsupported.mu.Lock()
	defer globalCodexWebSocketUnsupported.mu.Unlock()
	entry, ok := globalCodexWebSocketUnsupported.entries[key]
	if !ok {
		return false
	}
	if time.Now().After(entry.expiresAt) {
		delete(globalCodexWebSocketUnsupported.entries, key)
		return false
	}
	return true
}

func clearCodexWebSocketUnsupportedForTests() {
	globalCodexWebSocketUnsupported.mu.Lock()
	globalCodexWebSocketUnsupported.entries = make(map[string]codexWebSocketUnsupportedEntry)
	globalCodexWebSocketUnsupported.mu.Unlock()
}

// clearCodexWebSocketUnsupported drops unsupported-cache entries for a
// provider so configuration/URL updates can re-probe the endpoint.
func clearCodexWebSocketUnsupported(providerID uint64) {
	prefix := fmt.Sprintf("%d:", providerID)
	globalCodexWebSocketUnsupported.mu.Lock()
	for key := range globalCodexWebSocketUnsupported.entries {
		if strings.HasPrefix(key, prefix) {
			delete(globalCodexWebSocketUnsupported.entries, key)
		}
	}
	globalCodexWebSocketUnsupported.mu.Unlock()
}
