package cooldown

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
)

// flakyStore wraps memory store with controllable Set/SetIfLater failures.
// 用来模拟 Redis 在某一刻挂掉的场景,验证此时 manager 不污染其他实例。
type flakyStore struct {
	CooldownStore
	failSet         atomic.Bool
	failSetIfLater  atomic.Bool
	bumpGenCalls    atomic.Int32
	publishExpected atomic.Int32
}

func (f *flakyStore) Set(ctx context.Context, key CooldownKey, until time.Time) error {
	if f.failSet.Load() {
		return errors.New("simulated redis down: Set")
	}
	return f.CooldownStore.Set(ctx, key, until)
}

func (f *flakyStore) SetIfLater(ctx context.Context, key CooldownKey, until time.Time) (bool, error) {
	if f.failSetIfLater.Load() {
		return false, errors.New("simulated redis down: SetIfLater")
	}
	return f.CooldownStore.SetIfLater(ctx, key, until)
}

func (f *flakyStore) BumpGeneration(ctx context.Context, providerID uint64) (int64, error) {
	f.bumpGenCalls.Add(1)
	return f.CooldownStore.BumpGeneration(ctx, providerID)
}

// BLOCKER regression(Codex Round 3 找出的问题):
// store.Set 失败时,manager **不应该** bump generation + publish,否则其他
// 实例 reload 会拿到 store 上的空状态,erase 它们本地刚记的 cooldown,造成
// 漏封禁。
//
// 这个测试构造场景:A 实例 store.Set 失败,验证 BumpGeneration 没有被调用。
func TestStoreSetFailureDoesNotBumpGeneration(t *testing.T) {
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetCoordinator(ctx, coord)

	// 用 flakyStore 替换实际 store
	flaky := &flakyStore{CooldownStore: NewMemoryCooldownStore()}
	var s CooldownStore = flaky
	m.store.Store(&s)

	flaky.failSet.Store(true)

	// SetCooldownUntil 走 Set 路径(useIfLater=false)
	m.SetCooldownUntil(42, "claude", "opus", time.Now().Add(5*time.Minute))

	if got := flaky.bumpGenCalls.Load(); got != 0 {
		t.Fatalf("BumpGeneration should NOT be called when Set fails; called %d times", got)
	}

	// 本地仍然有这条 cooldown(单机视图正确)
	if !m.IsInCooldown(42, "claude", "opus") {
		// 注意 IsInCooldown 会触发 syncProviderGeneration,可能查 store 看到空
		// 但 generation 没变也不会 reload,所以本地仍保留
		t.Fatal("local view should still show cooldown despite store failure")
	}
}

// 同样测试 SetIfLater 失败(RecordFailure 路径)
func TestStoreSetIfLaterErrorDoesNotBumpGeneration(t *testing.T) {
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetCoordinator(ctx, coord)

	flaky := &flakyStore{CooldownStore: NewMemoryCooldownStore()}
	var s CooldownStore = flaky
	m.store.Store(&s)

	flaky.failSetIfLater.Store(true)

	m.RecordFailure(1, "claude", "opus", ReasonRateLimit, domain.ScopeModel, nil)

	if got := flaky.bumpGenCalls.Load(); got != 0 {
		t.Fatalf("BumpGeneration should NOT be called when SetIfLater errors; called %d times", got)
	}
}

// 验证 Set 成功后 BumpGeneration 才被调用一次。
func TestStoreSetSuccessBumpsGenerationOnce(t *testing.T) {
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetCoordinator(ctx, coord)

	flaky := &flakyStore{CooldownStore: NewMemoryCooldownStore()}
	var s CooldownStore = flaky
	m.store.Store(&s)

	// store 不失败
	m.SetCooldownUntil(7, "openai", "gpt-4", time.Now().Add(time.Minute))

	if got := flaky.bumpGenCalls.Load(); got != 1 {
		t.Fatalf("BumpGeneration should be called exactly once on success; called %d times", got)
	}
}
