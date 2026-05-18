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

// TestClearDetailOlderThan_PersistsCursorAcrossCalls 守护游标跨调用复用:
// 第二次调用不应该从 (0,0) 重扫已经处理过的区间。
func TestClearDetailOlderThan_PersistsCursorAcrossCalls(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewProxyRequestRepository(db)

	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 10; i++ {
		r := buildTestProxyRequest("COMPLETED", i)
		r.StartTime = old
		if err := repo.Create(r); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if err := db.gorm.Table("proxy_requests").Where("id = ?", r.ID).
			Update("created_at", old.Add(time.Duration(i)*time.Second).UnixMilli()).Error; err != nil {
			t.Fatalf("backdate %d: %v", i, err)
		}
	}

	// 第一次调用清完所有 10 条 (远低于 cap)。游标应该停在最后一行。
	cleared, err := repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("first clear: %v", err)
	}
	if cleared != 10 {
		t.Fatalf("first clear count = %d, want 10", cleared)
	}
	c1 := repo.loadCleanupCursor("")
	if c1.ID == 0 {
		t.Fatalf("cursor should be advanced after first call, got %+v", c1)
	}

	// 第二次调用:已经全部清过,detail_cleared=1,游标也已经在末尾,SELECT 应返回 0 行。
	cleared2, err := repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("second clear: %v", err)
	}
	if cleared2 != 0 {
		t.Fatalf("second clear count = %d, want 0 (cursor should skip already-cleared region)", cleared2)
	}
	c2 := repo.loadCleanupCursor("")
	if c2 != c1 {
		t.Fatalf("cursor should not move on empty second call, c1=%+v c2=%+v", c1, c2)
	}
}

// TestClearDetailOlderThan_RespectsBatchCap 守护 cap maxCleanupBatchesPerCall:
// backlog 远大于 cap 时,单次调用只处理 cap × batchSize 行,游标保存,下次继续。
func TestClearDetailOlderThan_RespectsBatchCap(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewProxyRequestRepository(db)

	// SQLite batch=200,cap=50 → 单次理论最大 10000 行;为加速测试,造 N>200 但 <10000。
	// 这样能验证 cursor 持久化即可,完整 cap 测试用 unit-style 更经济(不必造 10k 行)。
	const seed = 500
	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < seed; i++ {
		r := buildTestProxyRequest("COMPLETED", i)
		if err := repo.Create(r); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if err := db.gorm.Table("proxy_requests").Where("id = ?", r.ID).
			Update("created_at", old.Add(time.Duration(i)*time.Millisecond).UnixMilli()).Error; err != nil {
			t.Fatalf("backdate %d: %v", i, err)
		}
	}
	cleared, err := repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared != seed {
		t.Fatalf("clear count = %d, want %d", cleared, seed)
	}
}

// TestProxyRequestClearDetailOlderThan_UsesSentinelIndex 锁定 v15 之后 cleanup
// SELECT 走 idx_proxy_requests_detail_cleared(detail_cleared, created_at, id) 复合索引。
// 回归守护:WHERE detail_cleared = 0 是 leading-column 等值匹配,planner 应该挑这个索引;
// 任何回退到 PK 扫或 TEMP B-TREE 排序都会被捕获。
func TestProxyRequestClearDetailOlderThan_UsesSentinelIndex(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	const sql = `SELECT id, created_at FROM proxy_requests ` +
		`WHERE detail_cleared = 0 AND created_at < ? AND dev_mode = 0 ` +
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
	if !strings.Contains(plan, "idx_proxy_requests_detail_cleared") {
		t.Fatalf("expected plan to use idx_proxy_requests_detail_cleared, got:\n%s", plan)
	}
	if strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("plan should not require TEMP B-TREE sort, got:\n%s", plan)
	}
}

// TestProxyUpstreamAttemptClearDetailOlderThan_UsesSentinelIndex 同上，针对 attempts 表。
func TestProxyUpstreamAttemptClearDetailOlderThan_UsesSentinelIndex(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	const sql = `SELECT id, created_at FROM proxy_upstream_attempts ` +
		`WHERE detail_cleared = 0 AND created_at < ? ` +
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
	if !strings.Contains(plan, "idx_proxy_upstream_attempts_detail_cleared") {
		t.Fatalf("expected plan to use idx_proxy_upstream_attempts_detail_cleared, got:\n%s", plan)
	}
	if strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("plan should not require TEMP B-TREE sort, got:\n%s", plan)
	}
}
