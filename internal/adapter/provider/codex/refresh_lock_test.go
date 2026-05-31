package codex

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRefreshLockKey(t *testing.T) {
	if got := RefreshLockKey("acc-123", "rt"); got != "acct:acc-123" {
		t.Fatalf("with account ID: got %q, want acct:acc-123", got)
	}
	if got := RefreshLockKey("  acc-123  ", "rt"); got != "acct:acc-123" {
		t.Fatalf("account ID is trimmed: got %q, want acct:acc-123", got)
	}
	if got := RefreshLockKey("", "  secret  "); got != "rt:secret" {
		t.Fatalf("no account ID, with token: got %q, want rt:secret", got)
	}
	if got := RefreshLockKey("", "   "); got != "acct:" {
		t.Fatalf("both empty: got %q, want acct:", got)
	}
}

func TestAcquireRefreshLockMutualExclusion(t *testing.T) {
	const key = "id:1"
	var active int32
	var maxActive int32
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := AcquireRefreshLock(key)
			n := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&maxActive)
				if n <= old || atomic.CompareAndSwapInt32(&maxActive, old, n) {
					break
				}
			}
			atomic.AddInt32(&active, -1)
			unlock()
		}()
	}
	wg.Wait()

	if maxActive != 1 {
		t.Fatalf("lock allowed %d concurrent holders, want 1", maxActive)
	}
	// Entry must be evicted once all holders released.
	refreshLocksMu.Lock()
	_, exists := refreshLocks[key]
	refreshLocksMu.Unlock()
	if exists {
		t.Fatalf("refresh lock entry for %q not evicted after release", key)
	}
}

func TestAcquireRefreshLockDistinctKeysDoNotBlock(t *testing.T) {
	// Different keys must not serialize against each other: acquire both,
	// hold simultaneously, release. If they shared a mutex this would deadlock.
	u1 := AcquireRefreshLock("id:1")
	u2 := AcquireRefreshLock("id:2")
	u2()
	u1()
}

func TestAcquireRefreshLockUnlockIdempotent(t *testing.T) {
	unlock := AcquireRefreshLock("id:99")
	unlock()
	unlock() // second call must be a no-op, not panic or double-unlock
}
