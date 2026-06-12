package multiinstance

import (
	"sync"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
)

// 实例 A 设了 cooldown,实例 B 在 Pub/Sub 事件到达后看到同样的状态。
// 这是 RFC 的核心承诺:Redis 是真值,事件只是加速同步信号。
func TestCooldownSharedAcrossInstances(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	// A 触发一次失败,设置 cooldown
	a.Mgr.RecordFailure(42, "claude", "opus", cooldown.ReasonRateLimit, domain.ScopeModel, nil)

	// B 等事件到达 (miniredis pub/sub 是同步的,但订阅 goroutine 调度需要时间)
	if !waitFor(t, 500*time.Millisecond, func() bool {
		return b.Mgr.IsInCooldown(42, "claude", "opus")
	}) {
		t.Fatal("instance B should see the cooldown set by A")
	}
}

// RecordSuccess 在 A 上 → cooldown 清除事件传到 B → B 的 IsInCooldown 返回 false。
func TestRecordSuccessClearsAcrossInstances(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	a.Mgr.RecordFailure(99, "openai", "gpt", cooldown.ReasonRateLimit, domain.ScopeModel, nil)
	waitFor(t, 500*time.Millisecond, func() bool { return b.Mgr.IsInCooldown(99, "openai", "gpt") })

	a.Mgr.RecordSuccess(99, "openai", "gpt")
	if !waitFor(t, 500*time.Millisecond, func() bool {
		return !b.Mgr.IsInCooldown(99, "openai", "gpt")
	}) {
		t.Fatal("instance B should see the cooldown cleared after A's RecordSuccess")
	}
}

// ClearCooldown(provider, "", "") = ClearAll,应该清掉该 provider 所有层级的
// cooldown,跨实例可见。
func TestClearAllAcrossInstances(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	// A 给同一 provider 设三种 scope 的 cooldown
	a.Mgr.RecordFailure(7, "claude", "opus", cooldown.ReasonRateLimit, domain.ScopeModel, nil)
	a.Mgr.RecordFailure(7, "claude", "", cooldown.ReasonRateLimit, domain.ScopeKey, nil)
	a.Mgr.RecordFailure(7, "", "", cooldown.ReasonRateLimit, domain.ScopeProvider, nil)

	waitFor(t, 500*time.Millisecond, func() bool { return b.Mgr.IsInCooldown(7, "claude", "opus") })

	a.Mgr.ClearCooldown(7, "", "")

	if !waitFor(t, 500*time.Millisecond, func() bool {
		return !b.Mgr.IsInCooldown(7, "claude", "opus") &&
			!b.Mgr.IsInCooldown(7, "claude", "") &&
			!b.Mgr.IsInCooldown(7, "", "")
	}) {
		t.Fatal("instance B should see all three cooldowns cleared")
	}
}

// 并发场景:两实例同时对同一 provider 调用 RecordFailure。
// SetIfLater 保证 store 上的 until 是较晚那个,两实例最终都看到同一个 until。
func TestConcurrentRecordFailureDoesNotCorruptStore(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	const provider = 123
	const clientType = "claude"
	const model = "sonnet"

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			a.Mgr.RecordFailure(provider, clientType, model, cooldown.ReasonRateLimit, domain.ScopeModel, nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			b.Mgr.RecordFailure(provider, clientType, model, cooldown.ReasonRateLimit, domain.ScopeModel, nil)
		}
	}()
	wg.Wait()

	// 给事件最后收敛一点时间
	time.Sleep(300 * time.Millisecond)

	untilA := a.Mgr.GetCooldownUntil(provider, clientType, model)
	untilB := b.Mgr.GetCooldownUntil(provider, clientType, model)

	if untilA.IsZero() || untilB.IsZero() {
		t.Fatalf("both instances should see a cooldown; A=%v B=%v", untilA, untilB)
	}
	// 允许微小漂移 — 事件可能在 GetCooldownUntil 调用瞬间还在传输。
	// 关键不变量:两实例分别查到的 until 差距不超过事件传播窗口(~100ms 在 miniredis 下)。
	diff := untilA.Sub(untilB)
	if diff < 0 {
		diff = -diff
	}
	if diff > 200*time.Millisecond {
		t.Fatalf("instance views diverged: A=%v B=%v diff=%v", untilA, untilB, diff)
	}
}

// waitFor polls until cond() returns true or timeout elapses.
// 返回 cond 是否在窗口内变 true。
func waitFor(t testing.TB, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}
