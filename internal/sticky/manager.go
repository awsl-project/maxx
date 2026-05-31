package sticky

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
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
	store          atomic.Pointer[Store]
	lastGetErrorAt atomic.Int64 // unix nano of last logged Get error
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
// underlying implementation (Redis vs in-memory).
//
// Lifecycle expectation: this is meant to be called once at process start,
// before any traffic is served. The atomic.Pointer swap is safe under
// concurrent reads, but any in-flight Get/Set against the previous Store
// continues against the *old* backend — if the old store owns connection
// resources that need closing, the caller is responsible for ordering the
// new store's installation before tearing the old one down. Today both the
// memory and Redis stores share the coordinator's pool, so there is no
// teardown to coordinate.
//
// The ctx is currently unused but kept in the signature for symmetry with
// cooldown.Manager.SetCoordinator and in case a future Store needs to do
// async initialization here.
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
// Persistent errors (e.g. a chronically unreachable Redis) are surfaced as
// rate-limited warnings so the operator notices the affinity layer is down
// rather than silently observing degraded prompt-cache hit rates.
//
// Context errors (Canceled / DeadlineExceeded) are *not* logged: the
// router wraps every Get in a 100ms timeout derived from the request ctx,
// so a client disconnect or our own short cap shows up here as a context
// error and would otherwise drown the genuine-Redis-down signal.
func (m *Manager) Get(ctx context.Context, key Key) (uint64, bool) {
	s := m.currentStore()
	if s == nil {
		return 0, false
	}
	id, ok, err := s.Get(ctx, key)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			m.maybeLogGetError(err)
		}
		return 0, false
	}
	if !ok {
		return 0, false
	}
	return id, true
}

// maybeLogGetError prints at most one Get-error log per
// stickyErrorLogInterval — enough to make operational outages visible
// without spamming the log when Redis is down for hours.
func (m *Manager) maybeLogGetError(err error) {
	now := time.Now().UnixNano()
	last := m.lastGetErrorAt.Load()
	if last != 0 && time.Duration(now-last) < stickyErrorLogInterval {
		return
	}
	if m.lastGetErrorAt.CompareAndSwap(last, now) {
		log.Printf("[Sticky] Get failed (best-effort, falling through to weighted_random): %v", err)
	}
}

const stickyErrorLogInterval = time.Minute

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
func TTLFromConfig(seconds int64) time.Duration {
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
