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

func TestProxyRequestListCursorPrioritizesActiveRequests(t *testing.T) {
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
		requests[3].ID,
		requests[1].ID,
		requests[4].ID,
		requests[2].ID,
		requests[0].ID,
	}
	if got := collectRequestIDs(items); fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Fatalf("expected active-first order %v, got %v", expected, got)
	}
}

func TestProxyRequestListCursorBeforeCursorKeepsActiveRequestsAheadOfTerminalOnes(t *testing.T) {
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

	items, err := repo.ListCursor(1, 10, requests[4].ID, 0, nil)
	if err != nil {
		t.Fatalf("ListCursor failed: %v", err)
	}

	expected := []uint64{
		requests[3].ID,
		requests[1].ID,
		requests[2].ID,
		requests[0].ID,
	}
	if got := collectRequestIDs(items); fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Fatalf("expected active-first order before cursor %v, got %v", expected, got)
	}
}

func TestProxyRequestListCursorIncludesNullStatusesInTerminalSegment(t *testing.T) {
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
	}

	for _, request := range requests {
		if err := repo.Create(request); err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
	}

	// 直接写入 NULL，覆盖 status NOT IN 的 SQL 三值逻辑边界。
	if err := db.gorm.Exec("UPDATE proxy_requests SET status = NULL WHERE id = ?", requests[2].ID).Error; err != nil {
		t.Fatalf("Failed to null out status: %v", err)
	}

	items, err := repo.ListCursor(1, 10, 0, 0, nil)
	if err != nil {
		t.Fatalf("ListCursor failed: %v", err)
	}

	expected := []uint64{
		requests[1].ID,
		requests[2].ID,
		requests[0].ID,
	}
	if got := collectRequestIDs(items); fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Fatalf("expected NULL-status rows to remain in terminal segment %v, got %v", expected, got)
	}
}
