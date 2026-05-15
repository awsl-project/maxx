package multiinstance

import (
	"context"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// 模拟 in-progress 请求的辅助。
func (inst *instance) seedInProgressRequest(t *testing.T, requestID string, startTime time.Time) *domain.ProxyRequest {
	t.Helper()
	req := &domain.ProxyRequest{
		TenantID:   domain.DefaultTenantID,
		InstanceID: inst.ID,
		RequestID:  requestID,
		SessionID:  "s-" + requestID,
		ClientType: domain.ClientTypeClaude,
		Status:     "IN_PROGRESS",
		StartTime:  startTime,
	}
	if err := inst.Comp.ProxyRequest.Create(req); err != nil {
		t.Fatalf("seed proxy_request: %v", err)
	}
	return req
}

func (inst *instance) requestStatus(t *testing.T, id uint64) string {
	t.Helper()
	got, err := inst.Comp.ProxyRequest.GetByID(domain.DefaultTenantID, id)
	if err != nil {
		t.Fatalf("read proxy_request %d: %v", id, err)
	}
	return got.Status
}

// 滚动更新核心场景:
// 1. 实例 A 已经在跑,有一条 IN_PROGRESS 请求(几分钟前开始,典型长流式)
// 2. 实例 B 启动并立刻 sweep。
// 3. 因为 A 仍然在活实例集合内,B 的 sweep 必须**不能**把 A 的请求标记 FAILED。
//
// 这是 user 之前明确点出的"多实例后不能再直接基于 instance_id 强停"问题
// 在集成层的回归测试。
func TestRollingUpdateDoesNotKillRunningInstanceRequests(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")

	// A 写一条 5 分钟前开始的 in-progress 请求。
	// 5 分钟 > 60s grace,但因 A 在 alive 列表里,sweep 不应该动它。
	req := a.seedInProgressRequest(t, "long-stream", time.Now().Add(-5*time.Minute))

	// B 启动(setup 里会 RegisterInstance 自身,A 之前也已经注册)
	b := c.newInstance(t, "inst-B")

	alive, err := b.Coord.ListAliveInstances(context.Background())
	if err != nil {
		t.Fatalf("list alive: %v", err)
	}
	if !containsString(alive, "inst-A") || !containsString(alive, "inst-B") {
		t.Fatalf("alive list should contain both instances, got %v", alive)
	}

	// B 模拟"启动时 sweep"
	if _, err := b.Comp.ProxyRequest.MarkStaleAsFailed(alive); err != nil {
		t.Fatalf("MarkStaleAsFailed: %v", err)
	}

	// A 的 in-progress 必须仍是 IN_PROGRESS
	if status := a.requestStatus(t, req.ID); status != "IN_PROGRESS" {
		t.Fatalf("A's long-stream request was reaped by B's sweep, status=%s", status)
	}
}

// A 优雅退出后,A 留下的 in-progress 请求超过 60s grace 才会被 B 的 sweep 清掉。
// 测试两个时间点:
//   - A unregister 时立即 sweep:start_time = 30s ago → 仍然 IN_PROGRESS(grace 未过)
//   - 同样的请求 start_time = 90s ago → 被标记 FAILED(grace 已过)
func TestDeadInstanceOrphansClearedOnlyAfterGrace(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	freshReq := a.seedInProgressRequest(t, "fresh", time.Now().Add(-30*time.Second))
	staleReq := a.seedInProgressRequest(t, "stale", time.Now().Add(-90*time.Second))

	// A 模拟优雅退出 (UnregisterInstance)。从此 alive 列表里不含 A。
	a.shutdown()

	alive, _ := b.Coord.ListAliveInstances(context.Background())
	if containsString(alive, "inst-A") {
		t.Fatalf("A should be gone from alive list after shutdown, got %v", alive)
	}

	if _, err := b.Comp.ProxyRequest.MarkStaleAsFailed(alive); err != nil {
		t.Fatalf("MarkStaleAsFailed: %v", err)
	}

	if s := b.requestStatus(t, freshReq.ID); s != "IN_PROGRESS" {
		t.Fatalf("fresh request within grace should not be reaped, status=%s", s)
	}
	if s := b.requestStatus(t, staleReq.ID); s != "FAILED" {
		t.Fatalf("stale request past grace should be reaped, status=%s", s)
	}
}

// MarkStaleAsFailed 的安全门:当 coordinator 异常导致 alive list 为空时,
// 不应该把 in-progress 都误杀。
func TestEmptyAliveListSkipsSweep(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")

	req := a.seedInProgressRequest(t, "fragile", time.Now().Add(-2*time.Hour))

	// 空 alive list 模拟"coord 异常"
	count, err := a.Comp.ProxyRequest.MarkStaleAsFailed(nil)
	if err != nil {
		t.Fatalf("MarkStaleAsFailed: %v", err)
	}
	if count != 0 {
		t.Fatalf("empty alive list should sweep 0 rows, got %d", count)
	}

	if s := a.requestStatus(t, req.ID); s != "IN_PROGRESS" {
		t.Fatalf("request must not be reaped when alive list is empty, got status=%s", s)
	}
}

// 超过 hard timeout (30min) 的请求,即使所属实例还活着,也要被回收
// (说明该实例事实上卡死了)。这是 RFC 中 hard stuck timeout 的语义。
func TestHardTimeoutReapsRegardlessOfInstanceAlive(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")

	stuck := a.seedInProgressRequest(t, "wedged", time.Now().Add(-45*time.Minute))

	alive, _ := a.Coord.ListAliveInstances(context.Background())
	if !containsString(alive, "inst-A") {
		t.Fatalf("A should be alive, got %v", alive)
	}

	if _, err := a.Comp.ProxyRequest.MarkStaleAsFailed(alive); err != nil {
		t.Fatalf("MarkStaleAsFailed: %v", err)
	}
	if s := a.requestStatus(t, stuck.ID); s != "FAILED" {
		t.Fatalf("wedged request older than hard timeout should be reaped even for alive instance, got %s", s)
	}
}

func containsString(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}
