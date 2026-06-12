package multiinstance

import (
	"context"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
)

// TestSeamlessRollingUpdate 端到端复刻一次完整的滚动更新:
//
//	t0  实例 A 在跑,有一条进行中的长流式请求 + 一条 cooldown + 一条 provider 配置
//	t1  实例 B (canary) 启动,RegisterInstance,做启动 sweep
//	    断言:A 的活的请求没被误杀;B 看到 A 设的 cooldown;B 缓存里有 A 创建的 provider
//	t2  A 完成它的请求(标 COMPLETED),然后清掉它的 cooldown,然后 UnregisterInstance
//	    断言:B 看到 cooldown 已清;B 的 alive 列表不含 A
//	t3  B 做一次周期 sweep
//	    断言:A 留下的卡死请求(超 grace)被回收;A 已完成的请求不动
//
// 这条测试是"多实例数据不混乱 + 无缝滚动更新"的回归测试。
func TestSeamlessRollingUpdate(t *testing.T) {
	c := newCluster(t)

	// ─── t0: A 已在线 ──────────────────────────────────────────────
	a := c.newInstance(t, "inst-A")

	// A 写一条 5 分钟前开始的长流式请求(仍 in-progress)
	liveReq := a.seedInProgressRequest(t, "live-stream", time.Now().Add(-5*time.Minute))

	// A 在某个 provider 上设了 cooldown
	a.Mgr.RecordFailure(101, "claude", "sonnet", cooldown.ReasonRateLimit, domain.ScopeModel, nil)

	// A 创建一个 provider 配置
	prov := &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     "rolling-update-claude",
		Type:     "claude",
	}
	if err := a.Comp.Provider.Create(prov); err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	// ─── t1: B canary 上线 ─────────────────────────────────────────
	b := c.newInstance(t, "inst-B")

	alive, err := b.Coord.ListAliveInstances(context.Background())
	if err != nil {
		t.Fatalf("list alive: %v", err)
	}
	if !containsString(alive, "inst-A") || !containsString(alive, "inst-B") {
		t.Fatalf("expected both A and B alive at t1, got %v", alive)
	}

	// B 做启动 sweep
	if _, err := b.Comp.ProxyRequest.MarkStaleAsFailed(alive); err != nil {
		t.Fatalf("startup sweep: %v", err)
	}

	// 断言 1:A 的长流式请求没被误杀
	if status := b.requestStatus(t, liveReq.ID); status != "IN_PROGRESS" {
		t.Fatalf("rolling-update killed A's live request; status=%s", status)
	}

	// 断言 2:B 看到 A 设的 cooldown(走 generation sync)
	if !waitFor(t, 2*time.Second, func() bool {
		return b.Mgr.IsInCooldown(101, "claude", "sonnet")
	}) {
		t.Fatal("B did not learn A's cooldown")
	}

	// 断言 3:B 缓存里有 A 创建的 provider
	if !waitFor(t, time.Second, func() bool {
		list, _ := b.Comp.Provider.List(domain.DefaultTenantID)
		for _, p := range list {
			if p.Name == "rolling-update-claude" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("B did not see A's provider in cache")
	}

	// ─── t2: A 收尾后下线 ──────────────────────────────────────────
	liveReq.Status = "COMPLETED"
	liveReq.EndTime = time.Now()
	liveReq.Duration = liveReq.EndTime.Sub(liveReq.StartTime)
	if err := a.Comp.ProxyRequest.Update(liveReq); err != nil {
		t.Fatalf("complete request: %v", err)
	}

	// A 清掉它的 cooldown
	a.Mgr.RecordSuccess(101, "claude", "sonnet")

	// 在写入新的卡死请求来模拟"A 关停时还有一个真正卡死的"
	wedged := a.seedInProgressRequest(t, "wedged-on-shutdown", time.Now().Add(-2*time.Minute))

	// A 优雅退出
	a.shutdown()

	// 断言:B 看到 cooldown 清除
	if !waitFor(t, 2*time.Second, func() bool {
		return !b.Mgr.IsInCooldown(101, "claude", "sonnet")
	}) {
		t.Fatal("B did not see cooldown clear after A's RecordSuccess")
	}

	// 断言:alive 列表里 A 已下线
	if !waitFor(t, time.Second, func() bool {
		alive, _ := b.Coord.ListAliveInstances(context.Background())
		return !containsString(alive, "inst-A")
	}) {
		t.Fatal("A still showing as alive after shutdown")
	}

	// ─── t3: B 周期 sweep,回收 A 留下的卡死请求 ──────────────────
	alive, _ = b.Coord.ListAliveInstances(context.Background())
	if _, err := b.Comp.ProxyRequest.MarkStaleAsFailed(alive); err != nil {
		t.Fatalf("post-shutdown sweep: %v", err)
	}

	// 已完成的请求保持 COMPLETED
	if status := b.requestStatus(t, liveReq.ID); status != "COMPLETED" {
		t.Fatalf("completed request was tampered with; status=%s", status)
	}

	// 卡死的请求被回收成 FAILED
	if status := b.requestStatus(t, wedged.ID); status != "FAILED" {
		t.Fatalf("wedged orphan should be reaped after A shutdown; status=%s", status)
	}
}
