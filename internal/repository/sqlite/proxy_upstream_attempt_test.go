package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// seedAttemptForRequest 为指定 ProxyRequest 创建一个带详情的 attempt，并把 created_at 回拨
func seedAttemptForRequest(t *testing.T, repo *ProxyUpstreamAttemptRepository, db *DB, parentID uint64, createdAt time.Time) *domain.ProxyUpstreamAttempt {
	t.Helper()
	a := &domain.ProxyUpstreamAttempt{
		TenantID:       1,
		StartTime:      createdAt,
		Status:         "COMPLETED",
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

	t.Run("filters by parent ProxyRequest status", func(t *testing.T) {
		db, err := NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := NewProxyRequestRepository(db)
		attRepo := NewProxyUpstreamAttemptRepository(db)

		// 父请求：成功 + 失败 各一
		successParent := seedRequestWithDetail(t, reqRepo, "COMPLETED", false, old, 1)
		failedParent := seedRequestWithDetail(t, reqRepo, "FAILED", false, old, 2)

		successAttempt := seedAttemptForRequest(t, attRepo, db, successParent.ID, old)
		failedAttempt := seedAttemptForRequest(t, attRepo, db, failedParent.ID, old)

		// 仅清理成功父请求下的 attempt
		n, err := attRepo.ClearDetailOlderThan(cutoff, []string{"COMPLETED"})
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 attempt cleared, got %d", n)
		}
		if !attemptDetailCleared(t, db, successAttempt.ID) {
			t.Error("attempt under COMPLETED parent must be cleared")
		}
		if attemptDetailCleared(t, db, failedAttempt.ID) {
			t.Error("attempt under FAILED parent must be retained")
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

		devAttempt := seedAttemptForRequest(t, attRepo, db, devParent.ID, old)
		nonDevAttempt := seedAttemptForRequest(t, attRepo, db, nonDevParent.ID, old)

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
