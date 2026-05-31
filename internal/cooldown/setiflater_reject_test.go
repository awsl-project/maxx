package cooldown

import (
	"context"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
)

// 当 SetIfLater 在 store 上被拒绝(已有更晚 cooldown)时,本实例的本地状态
// 不应该被本实例计算出的"更早 until"覆盖。否则本地的 IsInCooldown 会
// 短暂返回错误结果,直到下次 generation 同步。
//
// 实现策略:store reject 时,把 lastGenSync 清零,下一次 IsInCooldown
// 触发同步,从 store reload 拉到真值。
func TestSetIfLaterRejectKeepsLocalCoherent(t *testing.T) {
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetCoordinator(ctx, coord)

	// 模拟"另一实例"先写了一个 5 分钟后的 cooldown
	sp := m.store.Load()
	store := *sp
	key := CooldownKey{ProviderID: 1, ClientType: "claude", Model: "opus"}
	farUntil := time.Now().Add(5 * time.Minute)
	if err := store.Set(ctx, key, farUntil); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if _, err := store.BumpGeneration(ctx, 1); err != nil {
		t.Fatalf("bump: %v", err)
	}

	// 现在本实例 RecordFailure。policy 算出来的 cooldown 比 5 分钟近(默认 5s)。
	// SetIfLater 在 store 上会被拒绝。本地不应该用 5s 那个 until 覆盖。
	m.RecordFailure(1, "claude", "opus", ReasonRateLimit, domain.ScopeModel, nil)

	// IsInCooldown 应该返回 true(因为下一次调用触发 syncProviderGeneration,
	// 拉到 store 上的 5min cooldown)
	if !m.IsInCooldown(1, "claude", "opus") {
		t.Fatal("after store-rejected mutation, local view should reflect store truth (in cooldown)")
	}

	// GetCooldownUntil 应该是接近 5 分钟而不是 5 秒
	got := m.GetCooldownUntil(1, "claude", "opus")
	if got.Before(time.Now().Add(4 * time.Minute)) {
		t.Fatalf("local cooldown until = %v, want close to %v (was overwritten by rejected mutation)", got, farUntil)
	}
}

// applyRemoteEvent 在收到过期(generation 倒退)事件时应该跳过 reload,
// 避免不必要的 ListByProvider。
func TestApplyRemoteEventSkipsStaleGeneration(t *testing.T) {
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetCoordinator(ctx, coord)

	// 先用一次本地 mutation 把 generation 推到 1
	m.SetCooldownDuration(7, "openai", "gpt", 30*time.Second)

	m.mu.RLock()
	genBefore := m.providerGen[7]
	m.mu.RUnlock()
	if genBefore < 1 {
		t.Fatalf("genBefore should be >= 1, got %d", genBefore)
	}

	// 现在收到一条 generation=0 的"陈旧"事件
	m.applyRemoteEvent(providerEvent{ProviderID: 7, Generation: 0})

	m.mu.RLock()
	genAfter := m.providerGen[7]
	m.mu.RUnlock()

	if genAfter != genBefore {
		t.Fatalf("stale event should not rewind providerGen: before=%d after=%d", genBefore, genAfter)
	}
}
