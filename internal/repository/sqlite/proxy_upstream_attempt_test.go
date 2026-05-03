package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// seedAttemptForRequest 为指定 ProxyRequest 创建一个带详情的 attempt，并把 created_at 回拨
func seedAttemptForRequest(t *testing.T, repo *ProxyUpstreamAttemptRepository, db *DB, parentID uint64, status string, createdAt time.Time) *domain.ProxyUpstreamAttempt {
	t.Helper()
	a := &domain.ProxyUpstreamAttempt{
		TenantID:       1,
		StartTime:      createdAt,
		Status:         status,
		ProxyRequestID: parentID,
		RequestModel:   "model",
		RequestInfo:    &domain.RequestInfo{Method: "POST", URL: "u", Body: "b"},
		ResponseInfo:   &domain.ResponseInfo{Status: 200, Body: "r"},
	}
	if err := repo.Create(a); err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if err := db.gorm.Model(&ProxyUpstreamAttempt{}).Where("id = ?", a.ID).
		Update("created_at", createdAt.UnixMilli()).Error; err != nil {
		t.Fatalf("backdate attempt: %v", err)
	}
	return a
}

func attemptDetailCleared(t *testing.T, db *DB, id uint64) bool {
	t.Helper()
	var got ProxyUpstreamAttempt
	if err := db.gorm.First(&got, id).Error; err != nil {
		t.Fatalf("reload attempt %d: %v", id, err)
	}
	return string(got.RequestInfo) == "" && string(got.ResponseInfo) == ""
}

func TestProxyUpstreamAttemptClearDetailOlderThan(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	cutoff := now.Add(-1 * time.Hour)

	t.Run("filters by attempt status, not parent", func(t *testing.T) {
		// 回归 Codex r6 P1：重试场景下父请求 status=COMPLETED 但其下若干次
		// attempt status=FAILED；按父过滤会把失败 attempt 误判为 success 桶，
		// 短保留时间会把用户最想保留的失败 attempt 详情清掉。必须按 attempt
		// 自身的 status 过滤
		db, err := NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := NewProxyRequestRepository(db)
		attRepo := NewProxyUpstreamAttemptRepository(db)

		// 一个最终成功的请求，但中间有两次失败重试
		retriedParent := seedRequestWithDetail(t, reqRepo, "COMPLETED", false, old, 1)
		failedAttempt1 := seedAttemptForRequest(t, attRepo, db, retriedParent.ID, "FAILED", old)
		failedAttempt2 := seedAttemptForRequest(t, attRepo, db, retriedParent.ID, "FAILED", old)
		successAttempt := seedAttemptForRequest(t, attRepo, db, retriedParent.ID, "COMPLETED", old)

		// 按 success 桶清理：只能清成功的那次 attempt
		n, err := attRepo.ClearDetailOlderThan(cutoff, []string{"COMPLETED"})
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 attempt cleared, got %d", n)
		}
		if !attemptDetailCleared(t, db, successAttempt.ID) {
			t.Error("COMPLETED attempt must be cleared")
		}
		if attemptDetailCleared(t, db, failedAttempt1.ID) {
			t.Error("FAILED attempt under COMPLETED parent must be retained (this is the retry-debug case)")
		}
		if attemptDetailCleared(t, db, failedAttempt2.ID) {
			t.Error("FAILED attempt under COMPLETED parent must be retained (this is the retry-debug case)")
		}
	})

	t.Run("dev_mode parent shields attempt", func(t *testing.T) {
		db, err := NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := NewProxyRequestRepository(db)
		attRepo := NewProxyUpstreamAttemptRepository(db)

		devParent := seedRequestWithDetail(t, reqRepo, "COMPLETED", true, old, 1)
		nonDevParent := seedRequestWithDetail(t, reqRepo, "COMPLETED", false, old, 2)

		devAttempt := seedAttemptForRequest(t, attRepo, db, devParent.ID, "COMPLETED", old)
		nonDevAttempt := seedAttemptForRequest(t, attRepo, db, nonDevParent.ID, "COMPLETED", old)

		n, err := attRepo.ClearDetailOlderThan(cutoff, nil)
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 cleared, got %d", n)
		}
		if attemptDetailCleared(t, db, devAttempt.ID) {
			t.Error("attempt under dev_mode parent must be retained")
		}
		if !attemptDetailCleared(t, db, nonDevAttempt.ID) {
			t.Error("attempt under non-dev parent must be cleared")
		}
	})
}
