package sticky

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestRedisStoreCrossInstance simulates a multi-instance deployment by
// creating two separate Store instances backed by the same Redis. This is the
// real distributed correctness contract: when instance A writes a sticky
// binding, instance B (which never ran the dispatcher) must be able to read
// it on the next request. miniredis is used so we don't need a real Redis
// running in CI.
func TestRedisStoreCrossInstance(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	newRdb := func() *redis.Client {
		return redis.NewClient(&redis.Options{Addr: mr.Addr()})
	}
	storeA := NewRedisStore(newRdb())
	storeB := NewRedisStore(newRdb())

	ctx := context.Background()
	key := Key{
		TenantID:   1,
		ClientType: "openai",
		ProjectID:  0,
		PolicyVer:  "abc123",
		BaseKey:    "t/42",
	}

	// Cross-instance read after write
	if err := storeA.Set(ctx, key, 7, 10*time.Second); err != nil {
		t.Fatalf("storeA.Set: %v", err)
	}
	got, found, err := storeB.Get(ctx, key)
	if err != nil {
		t.Fatalf("storeB.Get: %v", err)
	}
	if !found || got != 7 {
		t.Errorf("cross-instance Get: got=%d found=%v, want providerID=7 found=true", got, found)
	}
	t.Logf("verify> Probe C — write on store A, read on store B → providerID=%d found=%v ✓", got, found)

	// Overwrite from B; A must see it
	if err := storeB.Set(ctx, key, 11, 10*time.Second); err != nil {
		t.Fatalf("storeB.Set: %v", err)
	}
	got, found, err = storeA.Get(ctx, key)
	if err != nil {
		t.Fatalf("storeA.Get after overwrite: %v", err)
	}
	if !found || got != 11 {
		t.Errorf("overwrite from B then read from A: got=%d found=%v, want 11/true", got, found)
	}
	t.Logf("verify> Probe C — overwrite on B, read on A → providerID=%d found=%v ✓", got, found)

	// TTL expiry across instances
	shortKey := Key{TenantID: 1, ClientType: "openai", PolicyVer: "abc123", BaseKey: "t/expiry"}
	if err := storeA.Set(ctx, shortKey, 9, 100*time.Millisecond); err != nil {
		t.Fatalf("set short ttl: %v", err)
	}
	mr.FastForward(500 * time.Millisecond)
	if _, found, err := storeB.Get(ctx, shortKey); err != nil {
		t.Fatalf("storeB.Get after TTL: %v", err)
	} else if found {
		t.Errorf("TTL: expected expiry on B, still found")
	}
	t.Logf("verify> Probe C — TTL expiry (set on A, observe on B after fast-forward) → expired ✓")

	// Special characters in BaseKey and PolicyVer should not break the schema
	weirdKey := Key{
		TenantID:   2,
		ClientType: "claude:turbo",
		PolicyVer:  "ver/with:colon",
		BaseKey:    "c/42/abc:def/with spaces",
	}
	if err := storeA.Set(ctx, weirdKey, 13, time.Minute); err != nil {
		t.Fatalf("set weird key: %v", err)
	}
	got, found, err = storeB.Get(ctx, weirdKey)
	if err != nil {
		t.Fatalf("storeB.Get weirdKey: %v", err)
	}
	if !found || got != 13 {
		t.Errorf("escape: got=%d found=%v, want 13/true", got, found)
	}
	t.Logf("verify> Probe C — key escaping (colons + spaces) → providerID=%d found=%v ✓", got, found)

	// Concurrent writers: the last write must win and Get must not return
	// garbage or error out. Run under -race to catch any racy bookkeeping.
	concKey := Key{TenantID: 3, ClientType: "openai", PolicyVer: "abc", BaseKey: "t/conc"}
	const writers = 50
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(n int) {
			defer wg.Done()
			_ = storeA.Set(ctx, concKey, uint64(n+1), time.Minute)
		}(i)
	}
	wg.Wait()
	got, found, err = storeB.Get(ctx, concKey)
	if err != nil {
		t.Fatalf("concurrent Get: %v", err)
	}
	if !found || got == 0 || got > writers {
		t.Errorf("concurrent: got=%d found=%v, want one of 1..%d", got, found, writers)
	}
	t.Logf("verify> Probe C — %d concurrent writers, surviving value providerID=%d (must be 1..%d) ✓",
		writers, got, writers)

	// Delete on one instance, miss on the other
	if err := storeA.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, found, err := storeB.Get(ctx, key); err != nil {
		t.Fatalf("storeB.Get after Delete: %v", err)
	} else if found {
		t.Errorf("delete: expected miss after A.Delete, B.Get still found")
	}
	t.Logf("verify> Probe C — Delete on A, Get on B → miss ✓")

	// Quick sanity: serialized form remains a decimal uint64 in the bucket
	raw, err := newRdb().Get(ctx, keyName(concKey)).Result()
	if err != nil {
		t.Fatalf("raw read: %v", err)
	}
	if _, err := strconv.ParseUint(raw, 10, 64); err != nil {
		t.Errorf("raw value not uint64: %q", raw)
	}
}
