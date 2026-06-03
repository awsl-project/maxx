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

// TestProxyRequestUpdatePreservesRequestInfo 锁定 OOM 优化后的写入不变量:
//   - Update 永不重写 request_info(Create 后不变),状态类 Update 不能把它清空;
//   - Update 仅在 ResponseInfo 非 nil 时写 response_info;ResponseInfo 为 nil 时
//     不能覆盖库中已有的 response_info。
func TestProxyRequestUpdatePreservesRequestInfo(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewProxyRequestRepository(db)

	req := buildTestProxyRequest("PENDING", 1)
	req.RequestInfo = &domain.RequestInfo{Method: "POST", URL: "u", Body: "the-request-body"}
	if err := repo.Create(req); err != nil {
		t.Fatalf("create: %v", err)
	}

	reload := func() *ProxyRequest {
		var m ProxyRequest
		if err := repo.db.gorm.First(&m, req.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		return &m
	}

	// 1) 状态类 Update(ResponseInfo 仍为 nil)：request_info 必须保留，
	//    response_info 不得被写入响应体(Create 写的 nil 占位 "null" 可保留)。
	req.Status = "IN_PROGRESS"
	req.RequestInfo = nil // 模拟调用方在 Update 时并不携带 RequestInfo
	if err := repo.Update(req); err != nil {
		t.Fatalf("update status: %v", err)
	}
	if got := reload(); !strings.Contains(string(got.RequestInfo), "the-request-body") {
		t.Fatalf("request_info wiped by status-only Update: %q", got.RequestInfo)
	} else if strings.Contains(string(got.ResponseInfo), "the-response-body") {
		t.Fatalf("response_info unexpectedly written: %q", got.ResponseInfo)
	}

	// 2) 终态 Update 带 ResponseInfo：response_info 必须落库，request_info 仍保留。
	req.Status = "COMPLETED"
	req.ResponseInfo = &domain.ResponseInfo{Status: 200, Body: "the-response-body"}
	if err := repo.Update(req); err != nil {
		t.Fatalf("update completed: %v", err)
	}
	got := reload()
	if !strings.Contains(string(got.ResponseInfo), "the-response-body") {
		t.Fatalf("response_info not persisted: %q", got.ResponseInfo)
	}
	if !strings.Contains(string(got.RequestInfo), "the-request-body") {
		t.Fatalf("request_info lost after completion Update: %q", got.RequestInfo)
	}
	if got.Status != "COMPLETED" {
		t.Fatalf("status not updated: %q", got.Status)
	}

	// 3) 关键不变量(CodeRabbit 指出):response_info 已落真实值后,再来一次
	//    ResponseInfo==nil 的状态类 Update,必须 Omit 掉 response_info、不能把已有值清空。
	req.Status = "FAILED"
	req.ResponseInfo = nil
	if err := repo.Update(req); err != nil {
		t.Fatalf("status update after response set: %v", err)
	}
	if got := reload(); !strings.Contains(string(got.ResponseInfo), "the-response-body") {
		t.Fatalf("existing response_info wiped by nil-ResponseInfo Update: %q", got.ResponseInfo)
	} else if got.Status != "FAILED" {
		t.Fatalf("status not updated on nil-ResponseInfo Update: %q", got.Status)
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

// TestClearDetailOlderThan_LegacyFallback 守护 sentinel 列缺失时的退化路径。
//
// 场景:v15 在大表上 threshold-skip 了 ADD COLUMN,运维没补建。运行期
// SetDetailClearedColumnMissing(true) 让 ClearDetailOlderThan 走 legacy
// IS NOT NULL 谓词,不引用不存在的 detail_cleared 列。功能正常但慢。
//
// 实现验证:用一个真实的 SQLite DB(列已建),手动 set flag,运行清理,
// 确认 legacy 谓词路径不报错且能清出数据。
func TestClearDetailOlderThan_LegacyFallback(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewProxyRequestRepository(db)

	// 模拟列缺失场景。注意 SQLite 本地其实有列,我们只是逼 ClearDetailOlderThan 走
	// legacy 路径——legacy 路径不引用列,不报错;清理结果与 sentinel 路径一致。
	SetDetailClearedColumnMissing(true)
	defer SetDetailClearedColumnMissing(false)

	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 3; i++ {
		r := buildTestProxyRequest("COMPLETED", i)
		if err := repo.Create(r); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := db.gorm.Table("proxy_requests").Where("id = ?", r.ID).
			Update("created_at", old.UnixMilli()+int64(i)).Error; err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	cleared, err := repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("legacy-path clear: %v", err)
	}
	if cleared != 3 {
		t.Fatalf("legacy-path cleared = %d, want 3", cleared)
	}
}

// TestClearDetailOlderThan_StatusBucketIsolation 守护"状态后变的行不会被永久跳过"。
//
// 历史 bug:之前的实现在 repo 上持久化 cursor。PENDING 行先被 cursor 越过(status 过滤
// 不命中),之后转 COMPLETED 时已经在 cursor 后面,永远清不到。Codex 在 PR #568 round 1
// 抓到。修复:去掉持久化 cursor,只保留 within-call 局部游标 + sentinel 索引。
//
// 这个测试模拟:先以 success bucket 清一遍,造一个 PENDING 老行;然后把它转 COMPLETED,
// 再次清,应该被命中清掉。如果回归到持久化 cursor,这个测试会捕获。
func TestClearDetailOlderThan_StatusBucketIsolation(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewProxyRequestRepository(db)

	old := time.Now().Add(-2 * time.Hour)
	// 三行同时间窗口:一个 COMPLETED, 一个 PENDING (会被 success bucket 跳过), 一个 COMPLETED
	completed1 := buildTestProxyRequest("COMPLETED", 1)
	pending := buildTestProxyRequest("PENDING", 2)
	completed2 := buildTestProxyRequest("COMPLETED", 3)
	for _, r := range []*domain.ProxyRequest{completed1, pending, completed2} {
		if err := repo.Create(r); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := db.gorm.Table("proxy_requests").Where("id = ?", r.ID).
			Update("created_at", old.UnixMilli()+int64(r.ID)).Error; err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	// 第一次以 success bucket 清。COMPLETED 两条被清,PENDING 跳过(留在 detail_cleared=0)。
	cleared, err := repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), []string{"COMPLETED"})
	if err != nil || cleared != 2 {
		t.Fatalf("first clear: %d, %v (want 2, nil)", cleared, err)
	}

	// PENDING 行转 FAILED。
	if err := db.gorm.Table("proxy_requests").Where("id = ?", pending.ID).
		Update("status", "FAILED").Error; err != nil {
		t.Fatalf("transition: %v", err)
	}

	// failed bucket 清。如果有持久化 cursor 把 PENDING 越过了,这里会清不到。
	cleared, err = repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), []string{"FAILED", "CANCELLED", "REJECTED"})
	if err != nil || cleared != 1 {
		t.Fatalf("second clear: %d, %v (want 1, nil) — status transition got skipped by stale cursor?", cleared, err)
	}
}

// TestClearDetailOlderThan_RespectsBatchCap 真·cap 测试:把封顶临时调小,验证调用
// 在 cap × batchSize 行后返回,后续调用接力清完剩余。
func TestClearDetailOlderThan_RespectsBatchCap(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewProxyRequestRepository(db)

	// SQLite batch=200,cap=50 → 单次 10000 行。造 10500 行验证 cap 起效太重,改用
	// 测试钩子把 cap 临时调到 2 实现等价验证。batchSize 也是 200(SQLite default)。
	origCap := maxCleanupBatchesPerCall
	maxCleanupBatchesPerCall = 2
	defer func() { maxCleanupBatchesPerCall = origCap }()

	const seed = 600 // 3 batches with batchSize=200; cap=2 → 第一次清 400, 第二次清 200
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

	// 第一次:cap=2 × batchSize=200 = 400 行
	cleared, err := repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("first clear: %v", err)
	}
	if cleared != 400 {
		t.Fatalf("first call cleared = %d, want 400 (cap × batch)", cleared)
	}

	// 第二次:剩余 200 行,一个 batch 不满就退出
	cleared, err = repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("second clear: %v", err)
	}
	if cleared != 200 {
		t.Fatalf("second call cleared = %d, want 200", cleared)
	}

	// 第三次:全清完,应该 0
	cleared, err = repo.ClearDetailOlderThan(time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("third clear: %v", err)
	}
	if cleared != 0 {
		t.Fatalf("third call cleared = %d, want 0", cleared)
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
