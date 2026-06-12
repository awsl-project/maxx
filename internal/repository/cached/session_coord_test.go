package cached

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
)

// 一个 SessionRepository,DB 路径始终返回 ErrNotFound:验证读路径回退到
// coordinator KV,而 KV 由"另一个实例"(同 coord)先写入。
type alwaysMissDBRepo struct{ sessionTestRepo }

func (r *alwaysMissDBRepo) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	return nil, domain.ErrNotFound
}

func TestSessionRepository_KVHitFallback(t *testing.T) {
	t.Parallel()

	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	// 模拟"另一个实例"写入 KV
	other := NewSessionRepository(&sessionTestRepo{})
	other.SetCoordinator(coord, time.Hour)
	written := &domain.Session{
		TenantID:   42,
		SessionID:  "cross-instance",
		ClientType: domain.ClientTypeCodex,
		ProjectID:  3,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := other.Create(written); err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	// 本实例:DB 中没有这条数据,本地缓存为空,期望从 KV 命中
	repo := NewSessionRepository(&alwaysMissDBRepo{})
	repo.SetCoordinator(coord, time.Hour)

	got, err := repo.GetBySessionID(42, "cross-instance")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if got.ProjectID != 3 {
		t.Fatalf("ProjectID = %d, want 3", got.ProjectID)
	}
}

func TestSessionRepository_NilCoordinatorNoop(t *testing.T) {
	t.Parallel()
	repo := NewSessionRepository(&sessionTestRepo{})
	// nil coord 不应改变行为
	repo.SetCoordinator(nil, 0)

	s := &domain.Session{TenantID: 1, SessionID: "x", ClientType: domain.ClientTypeCodex}
	if err := repo.Create(s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetBySessionID(1, "x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SessionID != "x" {
		t.Fatalf("unexpected sessionID %q", got.SessionID)
	}
}

func TestSessionRepository_KVMissFallsBackToDB(t *testing.T) {
	t.Parallel()
	base := &sessionTestRepo{}
	repo := NewSessionRepository(base)
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()
	repo.SetCoordinator(coord, time.Hour)

	// 直接通过底层 repo 注入一条 session(绕过 cached 层,模拟"DB 已有但 KV 没"的场景)
	seeded := &domain.Session{
		TenantID: 1, SessionID: "db-only", ClientType: domain.ClientTypeCodex,
	}
	if err := base.Create(seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := repo.GetBySessionID(1, "db-only")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SessionID != "db-only" {
		t.Fatalf("unexpected sessionID %q", got.SessionID)
	}

	// 现在 KV 应该被回填了
	ctx := context.Background()
	if _, err := coord.Get(ctx, sessionCoordKey(1, "db-only")); errors.Is(err, coordinator.ErrNotFound) {
		t.Fatal("expected KV to be backfilled after DB hit")
	}
}
