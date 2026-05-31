package cached

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
)

// 回归测试(@awsl233777 review):DeleteOlderThan 删 DB 后,coordinator KV 上
// 残留的同名 session 必须同步删除,否则其他实例可能从 KV 读到 stale session
// 并写回本地 cache,造成"DB 中已 hard-delete 但跨实例仍可见"。
func TestDeleteOlderThanRemovesCoordKVEntry(t *testing.T) {
	t.Parallel()

	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	base := &sessionTestRepo{}
	repo := NewSessionRepository(base)
	repo.SetCoordinator(coord, time.Hour)

	// 写入一个 session(自动写 KV)
	old := &domain.Session{
		TenantID:   1,
		SessionID:  "to-be-expired",
		ClientType: domain.ClientTypeCodex,
		UpdatedAt:  time.Now().Add(-8 * 24 * time.Hour), // 8 天前
	}
	if err := repo.Create(old); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 验证 KV 有这条
	ctx := context.Background()
	if _, err := coord.Get(ctx, sessionCoordKey(1, "to-be-expired")); err != nil {
		t.Fatalf("expected KV to be populated, got %v", err)
	}

	// DeleteOlderThan 删 7 天之前的(覆盖上面 8 天前那条)
	// 因为 sessionTestRepo.DeleteOlderThan 不实际删,我们手动让 ListExpiredKeys 返回
	// 这条 key + 让 DeleteOlderThan 报告 deleted=1。
	base.mu.Lock()
	base.session.UpdatedAt = old.UpdatedAt
	base.mu.Unlock()

	// 触发 DeleteOlderThan;sessionTestRepo.DeleteOlderThan 返回 0(没真删),
	// 但 ListExpiredKeys 返回这条 → cached 层应当照样清 KV。
	// 我们用一个 wrapper 让 DeleteOlderThan 返回 1。
	wrapper := &deleteReportingRepo{sessionTestRepo: base, deletedCount: 1}
	repo2 := NewSessionRepository(wrapper)
	repo2.SetCoordinator(coord, time.Hour)

	if _, err := repo2.DeleteOlderThan(time.Now().Add(-7 * 24 * time.Hour)); err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}

	// 验证 KV 上这条已经被删
	if _, err := coord.Get(ctx, sessionCoordKey(1, "to-be-expired")); !errors.Is(err, coordinator.ErrNotFound) {
		t.Fatalf("expected KV entry to be deleted, got err=%v", err)
	}
}

// deleteReportingRepo 让 DeleteOlderThan 报告 deletedCount,模拟 DB 实际删除发生。
type deleteReportingRepo struct {
	*sessionTestRepo
	deletedCount int64
}

func (r *deleteReportingRepo) DeleteOlderThan(before time.Time) (int64, error) {
	return r.deletedCount, nil
}

// Cross-instance invalidation 由 tests/multiinstance/session_delete_test.go
// 覆盖(需要真 Redis pub/sub 跨 instance ID 传播事件,memory coordinator 是
// 进程内的不适合模拟跨实例 sender)。
