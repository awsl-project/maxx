package service

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

// TestRecalculateCosts_PreservesHistoricalMultiplier 是 PR1 修复的核心 bug 的端到端覆盖:
//
// 重算之前的实现把 multiplier 完全忽略,导致用户合约里 "Claude 客户端在 X provider
// 上算 1.2×" 这种历史成本被悄悄抹平成 1.0×。修复后,重算应:
//  1. 用当前价表算出新的基础 cost
//  2. 应用 attempt 上历史保存的 Multiplier
//  3. 把 model_price_id 同步更新到当前匹配的价格记录 ID
//
// 这个测试同时锁住这三条不变量;任一回退都会失败。
func TestRecalculateCosts_PreservesHistoricalMultiplier(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	attRepo := sqlite.NewProxyUpstreamAttemptRepository(db)
	reqRepo := sqlite.NewProxyRequestRepository(db)

	// 注入一个完全受控的价格记录:test-model, input=$3/M, 无 1M context, 无 cache 字段。
	// 这样 1M input tokens 的基础成本就是 3,000,000,000 nanoUSD,容易心算验证。
	pricing.GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{
			ID:              123,
			ModelID:         "test-model",
			InputPriceMicro: 3_000_000, // $3/M
		},
	})
	// 测试完后还原,避免污染后续测试中的 GlobalCalculator 单例。
	t.Cleanup(func() {
		pricing.GlobalCalculator().LoadFromDatabase(nil)
	})

	// 父请求(RecalculateCostsFromAttemptsWithProgress 需要 proxy_requests 行存在)。
	req := &domain.ProxyRequest{
		TenantID:  1,
		Status:    "COMPLETED",
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}
	if err := reqRepo.Create(req); err != nil {
		t.Fatalf("create request: %v", err)
	}

	// Attempt:历史 Multiplier=12000(1.2×),旧 Cost=999(故意写错以验证会被改正),
	// 旧 ModelPriceID=88(故意指向不存在的 ID,以验证会被更新)。
	att := &domain.ProxyUpstreamAttempt{
		TenantID:        1,
		Status:          "COMPLETED",
		ProxyRequestID:  req.ID,
		ResponseModel:   "test-model",
		InputTokenCount: 1_000_000,
		Multiplier:      12_000, // 1.2×
		ModelPriceID:    88,     // 历史旧记录
		Cost:            999,    // 故意写错
	}
	if err := attRepo.Create(att); err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	svc := &AdminService{
		attemptRepo:      attRepo,
		proxyRequestRepo: reqRepo,
	}

	result, err := svc.RecalculateCosts()
	if err != nil {
		t.Fatalf("RecalculateCosts: %v", err)
	}
	if result.UpdatedAttempts != 1 {
		t.Errorf("UpdatedAttempts = %d, want 1", result.UpdatedAttempts)
	}

	// Expected:1M tokens × $3/M = $3 = 3_000_000_000 nano,再乘 1.2 → 3_600_000_000。
	const expected = uint64(3_600_000_000)

	var attempts []sqlite.ProxyUpstreamAttempt
	if err := db.GormDB().Find(&attempts).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("got %d attempts, want 1", len(attempts))
	}
	got := attempts[0]
	if got.Cost != expected {
		t.Errorf("Cost = %d, want %d (multiplier应应用)", got.Cost, expected)
	}
	if got.ModelPriceID != 123 {
		t.Errorf("ModelPriceID = %d, want 123 (应同步到当前匹配 ID)", got.ModelPriceID)
	}

	// 父请求的 cost 应被刷新为 attempt cost 之和。
	updatedReq, err := reqRepo.GetByID(1, req.ID)
	if err != nil {
		t.Fatalf("reload request: %v", err)
	}
	if updatedReq.Cost != expected {
		t.Errorf("request.Cost = %d, want %d", updatedReq.Cost, expected)
	}
}

// TestRecalculateCosts_ModelPriceIDOnlyChange 验证 review 提的 audit-trail 风险:
// 当价格被替换成等额的新版本(cost 不变,但 model_price ID 换了)时,
// 重算必须把 model_price_id 也刷新到新行;否则审计上会看到指向已删除/旧版价格的孤儿引用。
//
// 修复前的 gate 只检查 res.Cost != attempt.Cost,会漏掉这种"金额等同但价格版本换了"的场景。
func TestRecalculateCosts_ModelPriceIDOnlyChange(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	attRepo := sqlite.NewProxyUpstreamAttemptRepository(db)
	reqRepo := sqlite.NewProxyRequestRepository(db)

	// 当前价表:test-model 指向 ID=200(假设旧 ID=100 已被替换成等额新版本)。
	pricing.GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 200, ModelID: "test-model", InputPriceMicro: 3_000_000},
	})
	t.Cleanup(func() {
		pricing.GlobalCalculator().LoadFromDatabase(nil)
	})

	req := &domain.ProxyRequest{
		TenantID:  1,
		Status:    "COMPLETED",
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}
	if err := reqRepo.Create(req); err != nil {
		t.Fatalf("create request: %v", err)
	}

	// Attempt:Cost 已经正确(3e9),但 ModelPriceID 还指向旧的 100。
	// 旧的 gate (`cost != cost`) 会判定无需更新,model_price_id 永远停在 100。
	const correctCost = uint64(3_000_000_000)
	att := &domain.ProxyUpstreamAttempt{
		TenantID:        1,
		Status:          "COMPLETED",
		ProxyRequestID:  req.ID,
		ResponseModel:   "test-model",
		InputTokenCount: 1_000_000,
		Multiplier:      10_000, // 1.0×
		ModelPriceID:    100,    // 旧版本 ID
		Cost:            correctCost,
	}
	if err := attRepo.Create(att); err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	svc := &AdminService{attemptRepo: attRepo, proxyRequestRepo: reqRepo}
	result, err := svc.RecalculateCosts()
	if err != nil {
		t.Fatalf("RecalculateCosts: %v", err)
	}
	if result.UpdatedAttempts != 1 {
		t.Errorf("UpdatedAttempts = %d, want 1 (model_price_id 不一致也应触发 update)", result.UpdatedAttempts)
	}

	var attempts []sqlite.ProxyUpstreamAttempt
	if err := db.GormDB().Find(&attempts).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if attempts[0].Cost != correctCost {
		t.Errorf("Cost = %d, want %d (本应不变)", attempts[0].Cost, correctCost)
	}
	if attempts[0].ModelPriceID != 200 {
		t.Errorf("ModelPriceID = %d, want 200 (必须刷新到当前匹配 ID,即使 cost 不变)", attempts[0].ModelPriceID)
	}
}
