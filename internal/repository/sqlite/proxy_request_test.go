package sqlite

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestDetailCleanupBatchParams(t *testing.T) {
	// 子测试间共享全局 detailCleanupIndexMissing,确保不串扰。
	defer detailCleanupIndexMissing.Store(0)

	t.Run("dialect defaults (index present)", func(t *testing.T) {
		detailCleanupIndexMissing.Store(0)
		tests := []struct {
			dialector string
			wantBatch int
			wantSleep time.Duration
		}{
			{"sqlite", 200, 50 * time.Millisecond},
			{"mysql", 1000, 20 * time.Millisecond},
			{"postgres", 200, 50 * time.Millisecond}, // unknown → conservative defaults
			{"", 200, 50 * time.Millisecond},
		}
		for _, tt := range tests {
			gotBatch, gotSleep := detailCleanupBatchParams(tt.dialector)
			if gotBatch != tt.wantBatch || gotSleep != tt.wantSleep {
				t.Errorf("detailCleanupBatchParams(%q) = (%d, %v), want (%d, %v)",
					tt.dialector, gotBatch, gotSleep, tt.wantBatch, tt.wantSleep)
			}
		}
	})

	t.Run("MySQL falls back to conservative when index missing", func(t *testing.T) {
		SetDetailCleanupIndexMissing(true)
		defer SetDetailCleanupIndexMissing(false)
		gotBatch, gotSleep := detailCleanupBatchParams("mysql")
		if gotBatch != 200 || gotSleep != 50*time.Millisecond {
			t.Errorf("MySQL with missing index = (%d, %v), want (200, 50ms)", gotBatch, gotSleep)
		}
	})

	t.Run("SQLite unaffected by MySQL index-missing flag", func(t *testing.T) {
		SetDetailCleanupIndexMissing(true)
		defer SetDetailCleanupIndexMissing(false)
		gotBatch, gotSleep := detailCleanupBatchParams("sqlite")
		if gotBatch != 200 || gotSleep != 50*time.Millisecond {
			t.Errorf("SQLite default = (%d, %v), want (200, 50ms)", gotBatch, gotSleep)
		}
	})

	// 验证 flag 可恢复:Codex 反馈 sticky flag 会污染后续启动/测试。
	t.Run("SetDetailCleanupIndexMissing(false) restores fast-path", func(t *testing.T) {
		SetDetailCleanupIndexMissing(true)
		SetDetailCleanupIndexMissing(false)
		gotBatch, gotSleep := detailCleanupBatchParams("mysql")
		if gotBatch != 1000 || gotSleep != 20*time.Millisecond {
			t.Errorf("after reset, MySQL = (%d, %v), want (1000, 20ms)", gotBatch, gotSleep)
		}
	})
}

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

	t.Run("clears across more than one batch", func(t *testing.T) {
		// 验证分批循环：seed > batchSize(500) 条，保证至少触发两次迭代且终止条件正确
		db, err := NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		repo := NewProxyRequestRepository(db)

		const seedCount = 1200
		ids := make([]uint64, 0, seedCount)
		for i := 0; i < seedCount; i++ {
			req := seedRequestWithDetail(t, repo, "COMPLETED", false, old, i)
			ids = append(ids, req.ID)
		}

		n, err := repo.ClearDetailOlderThan(cutoff, nil)
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if n != seedCount {
			t.Fatalf("expected %d cleared, got %d", seedCount, n)
		}
		for _, id := range ids {
			if !detailCleared(t, repo, id) {
				t.Fatalf("req %d not cleared after multi-batch run", id)
				break
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

// TestProxyRequestClearDetailOlderThan_UsesPartialIndex 锁定 cleanup 查询确实
// 走 v13 partial index。回归守护：若有人改回 `id > ?` 游标 + `ORDER BY id`，
// SQLite planner 会回退到 PK 扫，partial index 形同虚设，该测试会捕获。
func TestProxyRequestClearDetailOlderThan_UsesPartialIndex(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	const sql = `SELECT id, created_at FROM proxy_requests ` +
		`WHERE created_at < ? AND (request_info IS NOT NULL OR response_info IS NOT NULL) AND dev_mode = 0 ` +
		`AND (created_at > ? OR (created_at = ? AND id > ?)) ` +
		`ORDER BY created_at, id LIMIT 200`

	rows, err := db.gorm.Raw("EXPLAIN QUERY PLAN "+sql, 0, 0, 0, 0).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var planLines []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		planLines = append(planLines, detail)
	}
	plan := strings.Join(planLines, "\n")
	if !strings.Contains(plan, "idx_proxy_requests_detail_cleanup") {
		t.Fatalf("expected plan to use idx_proxy_requests_detail_cleanup, got:\n%s", plan)
	}
	// 显式拒绝 TEMP B-TREE 排序：partial index 的键 (created_at, id) 已经匹配 ORDER BY，
	// 出现 TEMP B-TREE 意味着 cursor 或 ORDER BY 形状变了，planner 退化到扫+排。
	if strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("plan should not require TEMP B-TREE sort, got:\n%s", plan)
	}
}

// TestProxyUpstreamAttemptClearDetailOlderThan_UsesPartialIndex 同上，针对
// attempt 表的 cleanup 查询。
func TestProxyUpstreamAttemptClearDetailOlderThan_UsesPartialIndex(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	const sql = `SELECT id, created_at FROM proxy_upstream_attempts ` +
		`WHERE created_at < ? AND (request_info IS NOT NULL OR response_info IS NOT NULL) ` +
		`AND (created_at > ? OR (created_at = ? AND id > ?)) ` +
		`AND EXISTS (SELECT 1 FROM proxy_requests WHERE proxy_requests.id = proxy_upstream_attempts.proxy_request_id AND proxy_requests.dev_mode = 0) ` +
		`ORDER BY created_at, id LIMIT 200`

	rows, err := db.gorm.Raw("EXPLAIN QUERY PLAN "+sql, 0, 0, 0, 0).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var planLines []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		planLines = append(planLines, detail)
	}
	plan := strings.Join(planLines, "\n")
	if !strings.Contains(plan, "idx_proxy_upstream_attempts_detail_cleanup") {
		t.Fatalf("expected plan to use idx_proxy_upstream_attempts_detail_cleanup, got:\n%s", plan)
	}
	// 显式拒绝 TEMP B-TREE 排序：EXISTS 改写的全部意义就是避免 planner 从父表驱动后
	// 再做一次临时排序。若再次出现，说明有人把 EXISTS 改回了 IN 子查询。
	if strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("plan should not require TEMP B-TREE sort, got:\n%s", plan)
	}
}
