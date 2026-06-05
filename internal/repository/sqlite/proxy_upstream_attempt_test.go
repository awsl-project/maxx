package sqlite

import (
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// TestProxyUpstreamAttemptUpdatePreservesRequestInfo 锁定 OOM 优化对 attempt 的同等不变量:
// request_info 在 Create 时写入后,状态类 Update 不能把它清空(Update 走 toModelMeta+Omit,
// 不重新 marshal 完整请求体——profile 里 1.19 GB cum 的来源);response_info 仅在非 nil 时落库。
func TestProxyUpstreamAttemptUpdatePreservesRequestInfo(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	attRepo := NewProxyUpstreamAttemptRepository(db)

	att := &domain.ProxyUpstreamAttempt{
		TenantID:       1,
		Status:         "IN_PROGRESS",
		ProxyRequestID: 100,
		RequestModel:   "claude-sonnet-4-5",
		RequestInfo:    &domain.RequestInfo{Method: "POST", URL: "u", Body: "the-request-body"},
	}
	if err := attRepo.Create(att); err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	reload := func() *ProxyUpstreamAttempt {
		var m ProxyUpstreamAttempt
		if err := attRepo.db.gorm.First(&m, att.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		return &m
	}

	// 状态类 Update(不带 RequestInfo/ResponseInfo):request_info 必须保留。
	att.Status = "COMPLETED"
	att.RequestInfo = nil
	if err := attRepo.Update(att); err != nil {
		t.Fatalf("status update: %v", err)
	}
	if got := reload(); !strings.Contains(string(got.RequestInfo), "the-request-body") {
		t.Fatalf("request_info wiped by status-only attempt Update: %q", got.RequestInfo)
	}

	// 终态填了 response_info 后,再 nil Update 不得清空已有 response_info。
	att.ResponseInfo = &domain.ResponseInfo{Status: 200, Body: "the-response-body"}
	if err := attRepo.Update(att); err != nil {
		t.Fatalf("set response: %v", err)
	}
	att.ResponseInfo = nil
	att.Status = "FAILED"
	if err := attRepo.Update(att); err != nil {
		t.Fatalf("nil-response update: %v", err)
	}
	got := reload()
	if !strings.Contains(string(got.ResponseInfo), "the-response-body") {
		t.Fatalf("existing response_info wiped by nil-ResponseInfo attempt Update: %q", got.ResponseInfo)
	}
	if !strings.Contains(string(got.RequestInfo), "the-request-body") {
		t.Fatalf("request_info lost on attempt Update: %q", got.RequestInfo)
	}
}

// TestStreamForCostCalc_IncludesMultiplier 确保 backfill 流式读取路径会把
// 历史 Multiplier 带出来。这是 PR1 修复 backfill 倍率丢失 bug 的关键前提:
// 如果 SELECT 不查 multiplier,calculator 在重算时拿不到历史值,只能默认 1.0×。
func TestStreamForCostCalc_IncludesMultiplier(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	attRepo := NewProxyUpstreamAttemptRepository(db)

	att := &domain.ProxyUpstreamAttempt{
		TenantID:        1,
		Status:          "COMPLETED",
		ProxyRequestID:  100,
		RequestModel:    "claude-sonnet-4-5",
		ResponseModel:   "claude-sonnet-4-5",
		InputTokenCount: 1000,
		Multiplier:      12_500, // 1.25×
		ModelPriceID:    7,
		Cost:            1234,
	}
	if err := attRepo.Create(att); err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	var got *domain.AttemptCostData
	if err := attRepo.StreamForCostCalc(10, func(batch []*domain.AttemptCostData) error {
		if len(batch) != 1 {
			t.Fatalf("batch size = %d, want 1", len(batch))
		}
		got = batch[0]
		return nil
	}); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if got.Multiplier != 12_500 {
		t.Errorf("Multiplier = %d, want 12500", got.Multiplier)
	}
	if got.Cost != 1234 {
		t.Errorf("Cost = %d, want 1234", got.Cost)
	}
	if got.ResponseModel != "claude-sonnet-4-5" {
		t.Errorf("ResponseModel = %q, want claude-sonnet-4-5", got.ResponseModel)
	}
}

// TestBatchUpdateCosts_UpdatesCostAndModelPriceID 验证 cost 和 model_price_id
// 在同一条 UPDATE 中写入。这是 PR1 修复"重算后 cost 变了但 model_price_id
// 停留在旧值"的不一致问题的根本检查。
func TestBatchUpdateCosts_UpdatesCostAndModelPriceID(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	attRepo := NewProxyUpstreamAttemptRepository(db)

	a1 := &domain.ProxyUpstreamAttempt{
		TenantID: 1, Status: "COMPLETED", ProxyRequestID: 1,
		Cost: 100, ModelPriceID: 1,
	}
	a2 := &domain.ProxyUpstreamAttempt{
		TenantID: 1, Status: "COMPLETED", ProxyRequestID: 1,
		Cost: 200, ModelPriceID: 2,
	}
	if err := attRepo.Create(a1); err != nil {
		t.Fatalf("create a1: %v", err)
	}
	if err := attRepo.Create(a2); err != nil {
		t.Fatalf("create a2: %v", err)
	}

	updates := map[uint64]domain.AttemptCostUpdate{
		a1.ID: {Cost: 999, ModelPriceID: 42},
		a2.ID: {Cost: 888, ModelPriceID: 43},
	}
	if err := attRepo.BatchUpdateCosts(updates); err != nil {
		t.Fatalf("BatchUpdateCosts: %v", err)
	}

	// 直接从 DB 重新读出来,确认 cost 与 model_price_id 都被更新到新值且互相对应。
	var rows []ProxyUpstreamAttempt
	if err := db.gorm.Find(&rows).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := map[uint64]ProxyUpstreamAttempt{}
	for _, r := range rows {
		got[r.ID] = r
	}
	if got[a1.ID].Cost != 999 || got[a1.ID].ModelPriceID != 42 {
		t.Errorf("a1 = (cost=%d, priceID=%d), want (999, 42)", got[a1.ID].Cost, got[a1.ID].ModelPriceID)
	}
	if got[a2.ID].Cost != 888 || got[a2.ID].ModelPriceID != 43 {
		t.Errorf("a2 = (cost=%d, priceID=%d), want (888, 43)", got[a2.ID].Cost, got[a2.ID].ModelPriceID)
	}
}

// TestBatchUpdateCosts_Empty 验证空入参是 no-op,不抛错(被 RecalculateCosts 在没有
// 需要更新的 batch 上调用)。
func TestBatchUpdateCosts_Empty(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	attRepo := NewProxyUpstreamAttemptRepository(db)
	if err := attRepo.BatchUpdateCosts(nil); err != nil {
		t.Errorf("empty BatchUpdateCosts returned error: %v", err)
	}
	if err := attRepo.BatchUpdateCosts(map[uint64]domain.AttemptCostUpdate{}); err != nil {
		t.Errorf("empty map BatchUpdateCosts returned error: %v", err)
	}
}

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
