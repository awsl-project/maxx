# Multi-Instance Coordination — RFC

> Status: draft, in implementation
> Branch: `feat/coordinator-redis`
> Last updated: 2026-05-15

This document captures the design for running multiple `maxx` instances behind a
shared database and a shared Redis. It distills two rounds of design discussion
into a concrete, minimal-diff implementation plan.

## Goals

1. Allow more than one `maxx` instance to share a database (MySQL/Postgres) and
   coordinate via Redis without misbehaving.
2. Preserve the single-instance / desktop experience unchanged when Redis is
   not configured.
3. Avoid changing public method signatures on widely-used internal APIs
   (notably `cooldown.Manager`) so the surface area of the PR stays reviewable.

## Non-goals (v1)

- Switching `cooldown.Manager` public method signatures to take `ctx` / return
  `error`. The blast radius across `executor`, `middleware`, `handler`, and
  `service` packages is too large for this PR.
- Replacing the existing `cache:invalidate:<entity>` channel naming.
- Cross-provider global generation counters. Generation is strictly per-provider.
- Single-flight / pipeline / scoped-lock optimizations.
- Live coordinator swap. `degraded` mode only logs reconnect success; it does
  not hot-swap implementations. Operators restart the process to upgrade.
- Loading the on-disk cooldown table back into Redis at startup. Local memory
  may hydrate from DB, but Redis remains the *runtime* source of truth for
  distributed deployments.

## Modes

A single environment variable governs distributed behavior:

```
MAXX_COORDINATOR_MODE = standalone | fail-fast | degraded   # default: standalone
```

| Mode        | Redis URL absent           | Redis URL present, connect OK | Redis URL present, connect fails        |
| ----------- | -------------------------- | ----------------------------- | --------------------------------------- |
| standalone  | memory coordinator         | memory coordinator (URL ignored, warned) | memory coordinator (URL ignored, warned) |
| fail-fast   | startup error              | redis coordinator             | startup error                           |
| degraded    | startup error              | redis coordinator             | memory; background reconnect logs WARN on recovery (no hot-swap) |

`degraded` recovery in v1 is limited to a startup-time grace window: the
process keeps trying to reach Redis in the background (every
`MAXX_COORDINATOR_RECONNECT_INTERVAL`) and logs at WARN level when the
connection is restored — *but does not switch the running coordinator
implementation*. Operators see a log line that says "Redis became
reachable, restart the process to enable distributed coordination" and
restart at their convenience. The motivation is conservatism: live-swapping
a coordinator means re-subscribing every cached-repo and cooldown channel
and re-bumping every instance heartbeat, with subtle ordering hazards
across goroutines. v1.x can add the live-swap later if the operator-restart
workflow turns out to be too painful.

In all three modes, runtime Redis errors after a successful start are
surfaced through normal error paths (logged, metric-counted) but do *not*
trigger a memory fallback. The contract is "if you asked for Redis at
boot, you keep getting Redis errors until Redis recovers."

### Configuration

| Env                                         | Default       | Purpose                                           |
| ------------------------------------------- | ------------- | ------------------------------------------------- |
| `MAXX_COORDINATOR_MODE`                     | `standalone`  | mode selector                                     |
| `MAXX_COORDINATOR_REDIS_URL`                | `""`          | Redis URL; required when mode != standalone      |
| `MAXX_COORDINATOR_INSTANCE_TTL`             | `60s`         | instance heartbeat TTL                           |
| `MAXX_COORDINATOR_HEARTBEAT_INTERVAL`       | `20s`         | heartbeat refresh interval                        |
| `MAXX_PROXY_REQUEST_SWEEP_INTERVAL`         | `45s`         | stale-request sweep period                        |
| `MAXX_COOLDOWN_GEN_SYNC_INTERVAL`           | `2s`          | per-provider generation re-check throttle         |
| `MAXX_COORDINATOR_RECONNECT_INTERVAL`       | `5s`          | degraded-mode reconnect cadence                   |
| `MAXX_ROUTING_SEED_SALT`                    | `""`          | shared HMAC salt for weighted_random first-pick   |

Legacy `MAXX_REDIS_URL` is retained as an alias for
`MAXX_COORDINATOR_REDIS_URL` for backward compatibility with the original
draft of this work.

`MAXX_ROUTING_SEED_SALT` is **optional but recommended** in multi-instance
deployments using the `weighted_random` routing strategy. Each instance
falls back to a per-process random 32-byte salt when the variable is
unset. Anti-grinding (a client cannot brute-force `X-Session-Id` to
steer themselves onto a specific upstream) still holds in either mode,
because the salt is never exposed. Redis sticky bindings remain
consistent across instances as soon as the first dispatch succeeds —
sticky reads the provider ID directly from Redis without re-shuffling.
The visible difference between "shared salt" and "per-process salt" is
only the **pre-sticky first pick** for the same `(token, session)`:
without a shared salt, instance A and instance B may pick a different
provider on the first request; whichever instance's pick succeeds first
writes the sticky binding, and both instances converge for the rest of
the session's lifetime.

Set the same `MAXX_ROUTING_SEED_SALT` value on every instance if you
want first-pick consistency before sticky writes land — useful for
debugging hot-spot reports or making cold-start behavior reproducible
across the fleet.

## Stale request reclamation

`MarkStaleAsFailed(aliveInstanceIDs []string)` reclaims orphaned
`PENDING/IN_PROGRESS` rows from `proxy_requests`:

1. If `aliveInstanceIDs` is empty (coordinator unhealthy), do nothing.
2. Sweep where `instance_id ∉ aliveInstanceIDs` AND `start_time < now − 60s`
   (orphan grace).
3. *Or* sweep where `start_time < now − 30min` (hard timeout) regardless of
   instance ownership.

The 60s orphan grace covers GC pauses, brief Redis hiccups, and short network
partitions; the 30min hard timeout reaps requests stuck in flight on any
instance. Both thresholds will become tunable in a follow-up.

The startup sweep is paired with a periodic sweep every 45s
(`MAXX_PROXY_REQUEST_SWEEP_INTERVAL`) so live instances continuously reclaim
dead peers' orphans without waiting for restarts.

## Cooldown

Cooldown is the most subtle component because every upstream request hits
`IsInCooldown` on the hot path. The design constraints are:

- `IsInCooldown(providerID, clientType, model) bool` — **no `ctx`, no
  `error`**. Adding either would cascade across executor / middleware /
  handler call sites.
- Hot reads must stay in the local map. Each request can fan out to four
  hierarchical lookups; we cannot afford a Redis round-trip per lookup.
- Redis must be the source of truth across instances. A lost pub/sub event
  cannot leave instances permanently disagreeing.

### Architecture

```
                ┌───────────────────────┐
                │   cooldown.Manager    │
                │  (per-process state)  │
                └──────────┬────────────┘
                           │
        ┌──────────────────┴───────────────────┐
        ▼                                       ▼
 local cache (mu, map)                  CooldownStore
 - hot-path reads                         (sub-interface)
 - bound by min(until, now+ttl)         - Get / ListByProvider
 - keyed on CooldownKey                  - Set / SetIfLater / Delete
                                         - GetGeneration / BumpGeneration
                                         - backed by Redis in distributed
                                           mode, no-op in standalone
```

A separate pub/sub channel `maxx:v1:event:cooldown` carries lightweight
notifications. Payload is intentionally minimal:

```json
{"provider_id": 42, "generation": 17}
```

Subscribers compare the incoming `generation` to their cached
`providerGen[providerID]`. On mismatch they call `reloadProviderFromStore`,
which clears that provider's local entries and rehydrates them from
`store.ListByProvider`. Lost events therefore degrade to "the next call to
`IsInCooldown` may use a slightly stale local entry until the throttled
`syncProviderGeneration` re-checks Redis" — not "instances disagree
forever".

### Manager internals (additive)

New fields:

```go
type Manager struct {
    // ...existing fields preserved...
    store        atomic.Pointer[CooldownStore]   // distributed truth, nil in standalone
    providerGen  map[uint64]int64                // last-known generation per provider
    lastGenCheck map[uint64]time.Time            // throttle for syncProviderGeneration
    genSyncEvery time.Duration                   // MAXX_COOLDOWN_GEN_SYNC_INTERVAL
}
```

New non-exported helpers (no signature changes to existing public methods):

```go
func (m *Manager) reloadProviderFromStore(ctx context.Context, providerID uint64) error
func (m *Manager) syncProviderGeneration(ctx context.Context, providerID uint64)
func (m *Manager) publishCooldownChange(providerID uint64)
func (m *Manager) applyLocalCooldown(key CooldownKey, until time.Time)
func (m *Manager) clearLocalProvider(providerID uint64)
```

`applyLocalCooldown` enforces `localUntil = min(until, now+staleWindow)` so
the local cache never outlives Redis. With `staleWindow = 2 ×
genSyncEvery`, a missed event is corrected within a few seconds.

Public methods are reimplemented internally to:
1. Mutate Redis truth first (`Set` / `SetIfLater` / `Delete` /
   `BumpGeneration`).
2. Update local memory.
3. Publish the lightweight notification.

`LoadFromDatabase` continues to seed the *local* map from SQL at startup. It
deliberately does *not* push those rows to Redis — that would let stale
on-disk state from a long-stopped instance overwrite a more recent state
written by a live peer.

### CooldownStore interface

```go
type CooldownStore interface {
    Get(ctx context.Context, key CooldownKey) (until time.Time, ttl time.Duration, found bool, err error)
    ListByProvider(ctx context.Context, providerID uint64) ([]CooldownStoreEntry, error)

    Set(ctx context.Context, key CooldownKey, until time.Time) error
    SetIfLater(ctx context.Context, key CooldownKey, until time.Time) (bool, error)
    Delete(ctx context.Context, key CooldownKey) error

    GetGeneration(ctx context.Context, providerID uint64) (int64, error)
    BumpGeneration(ctx context.Context, providerID uint64) (int64, error)
}
```

Two implementations:
- `memoryCooldownStore` — used in standalone mode, also covers tests.
- `redisCooldownStore` — wraps a `*redis.Client`. `SetIfLater` and
  `BumpGeneration` are implemented as Redis Lua to keep them atomic.

The store is owned by the coordinator implementation; `cooldown.Manager`
sees it through the interface only.

### Redis key schema

All keys carry a `maxx:v1:` prefix to keep the namespace explicit and to
leave room for future schema migrations.

```
maxx:v1:cooldown:p:{providerID}:c:{*|clientType}:m:{*|model}
  type: string
  value: until.UnixMilli() as decimal
  TTL: until - now

maxx:v1:cooldown:gen:p:{providerID}
  type: counter
  value: monotonic int64
  TTL: none

maxx:v1:instance:{instanceID}
  type: string
  TTL: MAXX_COORDINATOR_INSTANCE_TTL

# pub/sub channels
maxx:v1:event:cooldown
cache:invalidate:<entity>             # preserved for compatibility
```

`*` is the canonical token for wildcards: a provider-wide cooldown is
`maxx:v1:cooldown:p:42:c:*:m:*`. This avoids accidentally colliding with
client-type or model values that happen to be empty strings.

## Cache invalidation

`cached/*` repos already publish on `cache:invalidate:<entity>`. v1 expands
the payload from an empty marker to a structured envelope:

```json
{
  "entity": "provider",
  "op": "update",
  "id": "42",
  "updated_at": "2026-05-15T12:34:56Z",
  "instance_id": "i-abc"
}
```

Subscribers in v1 still react by reloading the whole entity (`Load()`);
the extra fields exist so a future PR can opt individual repos into
fine-grained invalidation without changing the wire protocol.

The channel name and per-entity routing are unchanged.

## Session

Sessions retain the two-tier behavior from the current branch: DB is source
of truth, Redis KV provides a cross-instance hot cache, local map provides
the in-process cache. KV entries get a one-hour TTL by default; cold
sessions fall through to DB on miss. No further changes in v1.

## Startup sequence

```
1.  Open DB.
2.  Construct SQL repositories.
3.  Generate instanceID.
4.  Resolve coordinator mode + config.
5.  Build coordinator:
      standalone  -> memory
      fail-fast   -> NewRedis or os.Exit(1)
      degraded    -> NewRedis, or memory with reconnect goroutine
6.  RegisterInstance + StartHeartbeat.
7.  Construct cooldown.Manager:
      - SetRepository / SetFailureCountRepository (unchanged)
      - LoadFromDatabase (local seed; does not write Redis)
      - SetCoordinator: builds CooldownStore from coord, subscribes to
        maxx:v1:event:cooldown, starts gen-sync throttle.
8.  Initial MarkStaleAsFailed(ListAliveInstances()).
9.  Construct cached repositories, wire SetCoordinator + AttachInvalidation.
10. Build router / executor / handlers.
11. Start periodic sweep (MAXX_PROXY_REQUEST_SWEEP_INTERVAL).
12. Graceful shutdown: UnregisterInstance → cancel ctx → Close coordinator.
```

The same sequence runs from both `cmd/maxx/main.go` and the desktop
launcher via a new `core.SetupCoordinator(ctx, instanceID, opts) (Coordinator, Cleanup)`
helper. Desktop deployments default to `standalone` mode regardless of env
vars set by accident.

## What we are explicitly not changing

This list is repeated so future contributors don't infer "v2 = full
refactor":

- `cooldown.Manager` public method signatures.
- `IsInCooldown` and `GetCooldownUntil` return types.
- The `cache:invalidate:<entity>` channel naming.
- DB schema for cooldowns or proxy_requests.
- Any caller-visible behavior in standalone mode.
- Metrics surface — this RFC sketches a metric list but actual emission
  ships in a follow-up PR.

## Observability (sketch, not part of this PR)

```
cooldown_local_cache_hit_total
cooldown_local_cache_miss_total
cooldown_redis_lookup_total
cooldown_redis_lookup_error_total
cooldown_event_received_total{op}
cooldown_event_lag_seconds
cooldown_generation_mismatch_total
coordinator_degraded_total
redis_disconnect_total
cache_reload_rows{entity}
cache_reload_duration_seconds{entity}
cache_reload_errors_total{entity}
```

## Open questions

- Should the 30min `hard stuck timeout` in `MarkStaleAsFailed` be exposed as
  an env? Long thinking streams from Claude/Codex can legitimately run
  10–20 min. Leaning yes; deferred to a follow-up.
- Should the cooldown event include `op` (set/clear/clear_all)? Strictly we
  only need `{provider_id, generation}` because reload is idempotent, but
  the extra field aids debugging. Leaning no for v1 (smaller payload, fewer
  fields to keep in sync between writer and reader).
