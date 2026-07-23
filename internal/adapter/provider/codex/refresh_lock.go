package codex

import (
	"strings"
	"sync"
)

// refreshLockEntry is a per-account refresh mutex with a waiter refcount so the
// registry can evict it once nobody holds or waits on it (avoids unbounded
// growth of the lock map in long-lived processes with provider/token churn).
type refreshLockEntry struct {
	mu       sync.Mutex
	refcount int
}

var (
	refreshLocksMu sync.Mutex
	refreshLocks   = map[string]*refreshLockEntry{}
)

// AcquireRefreshLock locks refreshing for the given account key and returns the
// unlock function. Callers should `defer` the returned function (or call it on
// every branch). The underlying mutex is shared across all callers with the same
// key and is removed from the registry once the last holder releases it.
//
// OpenAI rotates the refresh_token on every refresh and rejects reuse of an old
// one (refresh_token_reused / invalid_grant), so all code paths that may refresh
// the same account concurrently — the request adapter, the quota background
// task, and the batch-quota handler — must take the same lock.
//
// The refcount is incremented under refreshLocksMu BEFORE blocking on entry.mu,
// so a waiter pins its entry: the holder's unlock sees refcount != 0 and will
// not evict-and-recreate the entry out from under the waiter (which would split
// the lock). refreshLocksMu and entry.mu are never held simultaneously, so there
// is no lock-ordering inversion.
func AcquireRefreshLock(key string) func() {
	refreshLocksMu.Lock()
	entry := refreshLocks[key]
	if entry == nil {
		entry = &refreshLockEntry{}
		refreshLocks[key] = entry
	}
	entry.refcount++
	refreshLocksMu.Unlock()

	entry.mu.Lock()

	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mu.Unlock()
			refreshLocksMu.Lock()
			entry.refcount--
			if entry.refcount == 0 {
				delete(refreshLocks, key)
			}
			refreshLocksMu.Unlock()
		})
	}
}

// RefreshLockKey returns a lock key scoped to the OpenAI account so that two
// provider rows backed by the same account serialize against each other (a
// provider-ID key would not). It prefers the ChatGPT account ID, which is stable
// across refresh_token rotation.
//
// Before a provider's first successful refresh its account ID may be empty; in
// that case the key falls back to the current refresh_token. Concurrent callers
// that start in this window share the same (not-yet-rotated) token and therefore
// the same key, so they still serialize: the first to refresh persists the new
// token + account ID, and the others — already holding that same key — re-read
// under the lock and adopt the fresh token instead of refreshing again. (The
// only unserialized case is a brand-new caller arriving after rotation while an
// old-key holder is still blocked, which is both rare and self-correcting once
// the account ID is set.) Both empty degrades to a single global key rather than
// vanishing.
func RefreshLockKey(accountID, refreshToken string) string {
	if id := strings.TrimSpace(accountID); id != "" {
		return "acct:" + id
	}
	if rt := strings.TrimSpace(refreshToken); rt != "" {
		return "rt:" + rt
	}
	return "acct:"
}
