package multiinstance

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// CodeRabbit Round 3 找出的 "DeleteOlderThan 后 peer 本地 cache stale" 风险。
// 真实跨实例场景:
//  1. peer (B) 之前命中并 cache 了一条 session
//  2. deleter (A) 调 DeleteOlderThan,删 DB + KV + 发 session:delete 事件
//  3. peer 收到事件,**必须**清掉它本地 cache 中的对应条目,否则
//     GetBySessionID 命中本地直接返回 stale。
func TestSessionDeleteOlderThanInvalidatesPeerLocalCache(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A") // deleter
	b := c.newInstance(t, "inst-B") // peer

	// peer (B) 创建一条 session 并写入本地 cache + KV。
	// 注意:sqlite SessionRepository.Create 会强制把 UpdatedAt 设为 now,所以
	// 我们 Create 后再用底层 DB raw UPDATE 把 updated_at 改成 10 天前,以模拟
	// 一条该被 DeleteOlderThan 清理的过期 session。
	stale := &domain.Session{
		TenantID:   domain.DefaultTenantID,
		SessionID:  "doomed",
		ClientType: domain.ClientTypeClaude,
	}
	if err := b.Comp.Session.Create(stale); err != nil {
		t.Fatalf("B Create: %v", err)
	}
	// raw UPDATE 直接绕开 GORM hooks(避免 BeforeUpdate 把 updated_at 又拉回 now)
	tenMs := time.Now().Add(-10 * 24 * time.Hour).UnixMilli()
	if err := b.DB.GormDB().Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", tenMs, stale.ID).Error; err != nil {
		t.Fatalf("backdate updated_at: %v", err)
	}

	// 验证 B 本地能命中(stale 是 KV/本地里有的)
	if _, err := b.Comp.Session.GetBySessionID(stale.TenantID, stale.SessionID); err != nil {
		t.Fatalf("B GetBySessionID before delete: %v", err)
	}

	// A 跑 DeleteOlderThan(删除 7 天之前的) — A 没本地 cache 这条,
	// 但 KV 和 DB 上有(B 写的)
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	if _, err := a.Comp.Session.DeleteOlderThan(cutoff); err != nil {
		t.Fatalf("A DeleteOlderThan: %v", err)
	}

	// 等事件传播到 B(Redis pub/sub 是异步的)。
	// 通过 B 再 GetBySessionID:如果本地 cache 已被事件清空,会去查 KV(已被 A 删)
	// 再查 DB(已被 A 删),最终返回 ErrNotFound。本测试覆盖跨实例 invalidation。
	if !waitFor(t, 2*time.Second, func() bool {
		_, err := b.Comp.Session.GetBySessionID(stale.TenantID, stale.SessionID)
		return err == domain.ErrNotFound
	}) {
		t.Fatal("B still returns the deleted session from local cache after A's DeleteOlderThan + event broadcast")
	}
}
