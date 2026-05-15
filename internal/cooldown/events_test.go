package cooldown

import (
	"context"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
)

// 远程事件触发本地 reload:模拟"另一实例"修改了 store,然后发送事件;
// 本实例收到后应当从 store 重新拉取该 provider 的条目。
func TestRemoteEventTriggersReload(t *testing.T) {
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetCoordinator(ctx, coord)

	// 模拟另一实例对 store 直接写入(绕过 Manager,因此 Manager 本地没有这条数据)
	sp := m.store.Load()
	if sp == nil {
		t.Fatal("store not set after SetCoordinator")
	}
	store := *sp

	key := CooldownKey{ProviderID: 42, ClientType: "claude", Model: "opus"}
	until := time.Now().Add(time.Minute)
	if err := store.Set(ctx, key, until); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	newGen, err := store.BumpGeneration(ctx, 42)
	if err != nil {
		t.Fatalf("BumpGeneration: %v", err)
	}

	// 本实例此刻应该还看不到这条 cooldown(本地 map 是空的)
	if m.IsInCooldown(42, "claude", "opus") {
		// 注意:syncProviderGeneration 节流允许首次同步;实际可能立刻看到。
		// 这里不报错,但是下面手动注入事件验证 reload 路径。
	}

	// 直接调用 applyRemoteEvent 模拟事件被接收(避免依赖 pub/sub 时序)
	m.applyRemoteEvent(providerEvent{ProviderID: 42, Generation: newGen})

	if !m.IsInCooldown(42, "claude", "opus") {
		t.Fatal("expected provider 42 in cooldown after applyRemoteEvent + reload")
	}
}

// 本地 mutation 调用 store.Set + 广播事件
func TestLocalMutationWritesStore(t *testing.T) {
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetCoordinator(ctx, coord)

	m.SetCooldownDuration(7, "openai", "gpt-4", 30*time.Second)

	sp := m.store.Load()
	if sp == nil {
		t.Fatal("store not set")
	}
	got, ok, err := (*sp).Get(ctx, CooldownKey{ProviderID: 7, ClientType: "openai", Model: "gpt-4"})
	if err != nil || !ok {
		t.Fatalf("expected store hit, ok=%v err=%v", ok, err)
	}
	if got.IsZero() {
		t.Fatal("until should not be zero")
	}

	// generation 也应已 bump
	gen, _ := (*sp).GetGeneration(ctx, 7)
	if gen == 0 {
		t.Fatal("generation should have been bumped")
	}
}

// SetIfLater 在 RecordFailure 路径下生效:已经存在更晚 cooldown 时,
// 再来一次更短的 RecordFailure 不应缩短 store 的 cooldown。
func TestRecordFailureRespectsSetIfLater(t *testing.T) {
	// 这是 manager 行为测试,不直接验证 SetIfLater 返回值。
	// 通过两次 RecordFailure 时间差,验证 store 上的 until 是较晚那个。
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetCoordinator(ctx, coord)

	// 先手动给 store 写一个 5 分钟 cooldown(模拟先有的更晚状态)
	sp := m.store.Load()
	store := *sp
	key := CooldownKey{ProviderID: 1, ClientType: "claude", Model: "opus"}
	farUntil := time.Now().Add(5 * time.Minute)
	store.Set(ctx, key, farUntil)

	// 现在 RecordFailure 计算出的可能只是 5 秒
	// (具体 duration 取决于 policy 与 failureCount;这里 ReasonUnknown 默认 5s)
	m.RecordFailure(1, "claude", "opus", ReasonUnknown, "model", nil)

	got, ok, _ := store.Get(ctx, key)
	if !ok {
		t.Fatal("store should still have the entry")
	}
	if got.Before(farUntil) {
		t.Fatalf("SetIfLater should not shrink cooldown: got %v, want >= %v", got, farUntil)
	}
}
