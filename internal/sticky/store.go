// Package sticky stores per-session "remember which upstream provider this
// session is currently routed to" decisions. It is consumed by the router as
// an affinity layer on top of weighted_random: when enabled, a session's
// first successful provider becomes its preferred pick for a TTL window.
//
// Architecture mirrors internal/cooldown:
//   - Store interface with memory + Redis implementations
//   - sticky.Default() process-wide singleton, coordinator-aware
//   - Multi-instance correctness via shared Redis; standalone falls back to
//     in-process map (still functionally correct for a single instance)
package sticky

import (
	"context"
	"time"
)

// Key identifies a single sticky entry. The fingerprint of the routing policy
// (routes + weights + provider IDs) is embedded so any config change naturally
// invalidates all entries without explicit cleanup.
type Key struct {
	TenantID   uint64
	ClientType string
	ProjectID  uint64
	PolicyVer  string
	BaseKey    string
}

// Store is the persistence interface. Implementations must be safe for
// concurrent use.
type Store interface {
	Get(ctx context.Context, key Key) (providerID uint64, found bool, err error)
	Set(ctx context.Context, key Key, providerID uint64, ttl time.Duration) error
	Delete(ctx context.Context, key Key) error
}
