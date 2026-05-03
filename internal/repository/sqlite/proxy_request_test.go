package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func buildTestProxyRequest(status string, index int) *domain.ProxyRequest {
	start := time.Unix(int64(index), 0).UTC()
	return &domain.ProxyRequest{
		TenantID:     1,
		InstanceID:   "test-instance",
		RequestID:    fmt.Sprintf("request-%d", index),
		SessionID:    fmt.Sprintf("session-%d", index),
		ClientType:   domain.ClientType("claude"),
		RequestModel: fmt.Sprintf("model-%d", index),
		StartTime:    start,
		Status:       status,
		StatusCode:   200,
	}
}

func collectRequestIDs(items []*domain.ProxyRequest) []uint64 {
	ids := make([]uint64, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func TestProxyRequestListCursorReturnsNewestIDsFirst(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewProxyRequestRepository(db)
	requests := []*domain.ProxyRequest{
		buildTestProxyRequest("COMPLETED", 1),
		buildTestProxyRequest("PENDING", 2),
		buildTestProxyRequest("FAILED", 3),
		buildTestProxyRequest("IN_PROGRESS", 4),
		buildTestProxyRequest("CANCELLED", 5),
		buildTestProxyRequest("PENDING", 6),
	}

	for _, request := range requests {
		if err := repo.Create(request); err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
	}

	items, err := repo.ListCursor(1, 10, 0, 0, nil)
	if err != nil {
		t.Fatalf("ListCursor failed: %v", err)
	}

	expected := []uint64{
		requests[5].ID,
		requests[4].ID,
		requests[3].ID,
		requests[2].ID,
		requests[1].ID,
		requests[0].ID,
	}
	if got := collectRequestIDs(items); fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Fatalf("expected descending id order %v, got %v", expected, got)
	}
}

func TestProxyRequestListCursorBeforeCursorDoesNotRepeatOrSkipRecords(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewProxyRequestRepository(db)
	requests := []*domain.ProxyRequest{
		buildTestProxyRequest("COMPLETED", 1),
		buildTestProxyRequest("PENDING", 2),
		buildTestProxyRequest("FAILED", 3),
		buildTestProxyRequest("IN_PROGRESS", 4),
		buildTestProxyRequest("CANCELLED", 5),
		buildTestProxyRequest("PENDING", 6),
	}

	for _, request := range requests {
		if err := repo.Create(request); err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
	}

	firstPage, err := repo.ListCursor(1, 3, 0, 0, nil)
	if err != nil {
		t.Fatalf("ListCursor failed: %v", err)
	}
	firstPageExpected := []uint64{requests[5].ID, requests[4].ID, requests[3].ID}
	if got := collectRequestIDs(firstPage); fmt.Sprint(got) != fmt.Sprint(firstPageExpected) {
		t.Fatalf("expected first page %v, got %v", firstPageExpected, got)
	}

	secondPage, err := repo.ListCursor(1, 3, firstPage[len(firstPage)-1].ID, 0, nil)
	if err != nil {
		t.Fatalf("ListCursor failed: %v", err)
	}

	secondPageExpected := []uint64{
		requests[2].ID,
		requests[1].ID,
		requests[0].ID,
	}
	if got := collectRequestIDs(secondPage); fmt.Sprint(got) != fmt.Sprint(secondPageExpected) {
		t.Fatalf("expected second page %v, got %v", secondPageExpected, got)
	}

	combined := append(collectRequestIDs(firstPage), collectRequestIDs(secondPage)...)
	expectedCombined := []uint64{
		requests[5].ID,
		requests[4].ID,
		requests[3].ID,
		requests[2].ID,
		requests[1].ID,
		requests[0].ID,
	}
	if fmt.Sprint(combined) != fmt.Sprint(expectedCombined) {
		t.Fatalf("expected combined pages %v, got %v", expectedCombined, combined)
	}
}

// seedRequestWithDetail 创建一条带有 request/response 详情的记录，并把 created_at 强制回拨到指定时间
// 直接绕过 Create 的 now-stamping 是为了在 ClearDetailOlderThan 测试中构造"老到该清理"的样本
func seedRequestWithDetail(t *testing.T, repo *ProxyRequestRepository, status string, devMode bool, createdAt time.Time, index int) *domain.ProxyRequest {
	t.Helper()
	req := buildTestProxyRequest(status, index)
	req.DevMode = devMode
	req.RequestInfo = &domain.RequestInfo{
		Method:  "POST",
		URL:     "https://example.com",
		Headers: map[string]string{"x": "y"},
		Body:    "body",
	}
	req.ResponseInfo = &domain.ResponseInfo{
		Status: 200,
		Body:   "resp",
	}
	if err := repo.Create(req); err != nil {
		t.Fatalf("create req: %v", err)
	}
	if err := repo.db.gorm.Model(&ProxyRequest{}).Where("id = ?", req.ID).
		Update("created_at", createdAt.UnixMilli()).Error; err != nil {
		t.Fatalf("backdate req: %v", err)
	}
	return req
}

func detailCleared(t *testing.T, repo *ProxyRequestRepository, id uint64) bool {
	t.Helper()
	var got ProxyRequest
	if err := repo.db.gorm.First(&got, id).Error; err != nil {
		t.Fatalf("reload req %d: %v", id, err)
	}
	return string(got.RequestInfo) == "" && string(got.ResponseInfo) == ""
}

func TestProxyRequestClearDetailOlderThan(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	cutoff := now.Add(-1 * time.Hour)

	t.Run("nil statuses clears all (split=false path)", func(t *testing.T) {
		db, err := NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		repo := NewProxyRequestRepository(db)

		completed := seedRequestWithDetail(t, repo, "COMPLETED", false, old, 1)
		failed := seedRequestWithDetail(t, repo, "FAILED", false, old, 2)
		pending := seedRequestWithDetail(t, repo, "PENDING", false, old, 3)

		n, err := repo.ClearDetailOlderThan(cutoff, nil)
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if n != 3 {
			t.Fatalf("expected 3 cleared, got %d", n)
		}
		for _, req := range []*domain.ProxyRequest{completed, failed, pending} {
			if !detailCleared(t, repo, req.ID) {
				t.Errorf("req %d (%s) not cleared", req.ID, req.Status)
			}
		}
	})

	t.Run("success-only filter spares failed", func(t *testing.T) {
		db, err := NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		repo := NewProxyRequestRepository(db)

		completed := seedRequestWithDetail(t, repo, "COMPLETED", false, old, 1)
		failed := seedRequestWithDetail(t, repo, "FAILED", false, old, 2)
		cancelled := seedRequestWithDetail(t, repo, "CANCELLED", false, old, 3)

		n, err := repo.ClearDetailOlderThan(cutoff, []string{"COMPLETED"})
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 cleared, got %d", n)
		}
		if !detailCleared(t, repo, completed.ID) {
			t.Error("COMPLETED should be cleared")
		}
		if detailCleared(t, repo, failed.ID) {
			t.Error("FAILED must be retained")
		}
		if detailCleared(t, repo, cancelled.ID) {
			t.Error("CANCELLED must be retained")
		}
	})

	t.Run("failed-set filter spares completed", func(t *testing.T) {
		db, err := NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		repo := NewProxyRequestRepository(db)

		completed := seedRequestWithDetail(t, repo, "COMPLETED", false, old, 1)
		failed := seedRequestWithDetail(t, repo, "FAILED", false, old, 2)
		cancelled := seedRequestWithDetail(t, repo, "CANCELLED", false, old, 3)
		rejected := seedRequestWithDetail(t, repo, "REJECTED", false, old, 4)

		n, err := repo.ClearDetailOlderThan(cutoff, []string{"FAILED", "CANCELLED", "REJECTED"})
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if n != 3 {
			t.Fatalf("expected 3 cleared, got %d", n)
		}
		if detailCleared(t, repo, completed.ID) {
			t.Error("COMPLETED must be retained")
		}
		for _, req := range []*domain.ProxyRequest{failed, cancelled, rejected} {
			if !detailCleared(t, repo, req.ID) {
				t.Errorf("%s should be cleared", req.Status)
			}
		}
	})

	t.Run("respects dev_mode and time cutoff", func(t *testing.T) {
		db, err := NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		repo := NewProxyRequestRepository(db)

		oldNonDev := seedRequestWithDetail(t, repo, "COMPLETED", false, old, 1)
		oldDev := seedRequestWithDetail(t, repo, "COMPLETED", true, old, 2)
		freshNonDev := seedRequestWithDetail(t, repo, "COMPLETED", false, now, 3)

		n, err := repo.ClearDetailOlderThan(cutoff, nil)
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 cleared, got %d", n)
		}
		if !detailCleared(t, repo, oldNonDev.ID) {
			t.Error("old non-dev must be cleared")
		}
		if detailCleared(t, repo, oldDev.ID) {
			t.Error("dev_mode record must be retained")
		}
		if detailCleared(t, repo, freshNonDev.ID) {
			t.Error("fresh record must be retained")
		}
	})
}
