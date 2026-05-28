package sticky

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
)

// DefaultTTL is the fallback TTL when none is configured. 30 minutes balances
// affinity stickiness with bounded staleness in case a provider silently
// degrades — the next request after expiry re-rolls via weighted_random.
const DefaultTTL = 30 * time.Minute

// Manager wraps a Store with type-safe helpers used by the router/dispatcher.
//
// Errors are intentionally swallowed (return ok=false) on the read path: the
// caller can always fall back to a fresh routing decision. On the write path
// errors are reported so callers can choose to log them.
type Manager struct {
	store atomic.Pointer[Store]
}

// NewManager creates a Manager backed by the in-process memory store. Use
// SetStore to swap to a Redis-backed implementation after the coordinator is
// available.
func NewManager() *Manager {
	m := &Manager{}
	s := NewMemoryStore()
	m.store.Store(&s)
	return m
}

// Default global manager.
var defaultManager = NewManager()

// Default returns the default global sticky manager. Wire SetStore via
// SetCoordinator at startup.
func Default() *Manager { return defaultManager }

// SetCoordinator picks an appropriate Store based on the coordinator's
// underlying implementation (Redis vs in-memory). Safe to call multiple times.
func (m *Manager) SetCoordinator(_ context.Context, c coordinator.Coordinator) {
	store := StoreFor(c)
	m.store.Store(&store)
}

// SetStore lets tests or alternative wiring inject an explicit Store.
func (m *Manager) SetStore(s Store) {
	m.store.Store(&s)
}

func (m *Manager) currentStore() Store {
	p := m.store.Load()
	if p == nil {
		return nil
	}
	return *p
}

// Get returns the sticky provider for the key, or (0,false) if none.
// Any error is treated as a miss; callers fall through to fresh selection.
func (m *Manager) Get(ctx context.Context, key Key) (uint64, bool) {
	s := m.currentStore()
	if s == nil {
		return 0, false
	}
	id, ok, err := s.Get(ctx, key)
	if err != nil || !ok {
		return 0, false
	}
	return id, true
}

// Set records the sticky decision with the given TTL (clamped to DefaultTTL
// when non-positive).
func (m *Manager) Set(ctx context.Context, key Key, providerID uint64, ttl time.Duration) error {
	s := m.currentStore()
	if s == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return s.Set(ctx, key, providerID, ttl)
}

// Delete drops the entry. Errors are non-fatal — the entry will expire on its
// own and the next selection cannot rely on stale data anyway.
func (m *Manager) Delete(ctx context.Context, key Key) error {
	s := m.currentStore()
	if s == nil {
		return nil
	}
	return s.Delete(ctx, key)
}

// BaseKey builds the per-session anchor. With scope=token the same api token
// always lands on the same provider (best prompt-cache locality at the cost
// of coarser affinity). With scope=conversation each session id forks.
//
// The api token is an internal uint64, not the user-visible API key string,
// so no obfuscation is required for it. Session IDs (which may carry
// user-supplied data via the X-Session-Id header) are short-hashed before
// joining to keep Redis keys bounded and to avoid embedding raw user input
// in the schema.
func BaseKey(scope domain.RoutingStickyScope, apiTokenID uint64, sessionID string) string {
	switch scope {
	case domain.RoutingStickyScopeConversation:
		return "c/" + strconv.FormatUint(apiTokenID, 10) + "/" + shortHash(sessionID)
	default: // token or unset
		return "t/" + strconv.FormatUint(apiTokenID, 10)
	}
}

// TTLFromConfig returns the configured TTL or DefaultTTL when unset.
func TTLFromConfig(seconds int) time.Duration {
	if seconds <= 0 {
		return DefaultTTL
	}
	return time.Duration(seconds) * time.Second
}

func shortHash(s string) string {
	if s == "" {
		return "0"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
