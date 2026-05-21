package pricing

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

// resetGlobalCalculator 在测试间还原 GlobalCalculator,避免上一个测试写入的 DB 价格
// 污染后续测试中的查表结果。
func resetGlobalCalculator(t *testing.T) {
	t.Helper()
	GlobalCalculator().LoadFromDatabase(nil)
	t.Cleanup(func() {
		GlobalCalculator().LoadFromDatabase(nil)
	})
}

func TestFinalizeAttemptCost_WritesAllBillingFields(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 11, ModelID: "test-model", InputPriceMicro: 3_000_000},
	})

	att := &domain.ProxyUpstreamAttempt{
		ResponseModel:   "test-model",
		InputTokenCount: 1_000_000,
	}

	res := FinalizeAttemptCost(att, 12_000) // 1.2×

	// $3/M × 1M × 1.2 = $3.6 = 3_600_000_000 nanoUSD
	const expected = uint64(3_600_000_000)
	if res.Cost != expected || att.Cost != expected {
		t.Errorf("cost = res=%d attempt=%d, want %d", res.Cost, att.Cost, expected)
	}
	if att.ModelPriceID != 11 {
		t.Errorf("attempt.ModelPriceID = %d, want 11", att.ModelPriceID)
	}
	if att.Multiplier != 12_000 {
		t.Errorf("attempt.Multiplier = %d, want 12000", att.Multiplier)
	}
}

func TestFinalizeAttemptCost_NoTokensKeepsMultiplierOnly(t *testing.T) {
	// 失败 attempt 经常没有 token,这种情况下 FinalizeAttemptCost 不该去查表写 cost
	// (会被 unknown-model 日志污染,也容易在 attempt 上塞个 0 把已有合理字段覆盖)。
	// 只把合约 Multiplier 回填到 attempt,Cost/ModelPriceID 保持 0/未触动。
	resetGlobalCalculator(t)

	att := &domain.ProxyUpstreamAttempt{
		ResponseModel: "test-model",
		// 没有 InputTokenCount / OutputTokenCount
	}
	res := FinalizeAttemptCost(att, 11_000)

	if res.Cost != 0 {
		t.Errorf("Cost = %d, want 0 (no tokens means no billing)", res.Cost)
	}
	if att.Multiplier != 11_000 {
		t.Errorf("attempt.Multiplier = %d, want 11000", att.Multiplier)
	}
	if att.Cost != 0 {
		t.Errorf("attempt.Cost = %d, want 0 (no tokens → no calc)", att.Cost)
	}
}

func TestFinalizeAttemptCost_ZeroMultiplierDefaultsToOne(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 11, ModelID: "test-model", InputPriceMicro: 3_000_000},
	})

	att := &domain.ProxyUpstreamAttempt{
		ResponseModel:   "test-model",
		InputTokenCount: 100_000,
	}
	res := FinalizeAttemptCost(att, 0)
	if res.Multiplier != DefaultMultiplier {
		t.Errorf("Multiplier = %d, want %d (0 should default)", res.Multiplier, DefaultMultiplier)
	}
}

func TestFinalizeAttemptCost_NilAttemptDoesNotPanic(t *testing.T) {
	resetGlobalCalculator(t)
	res := FinalizeAttemptCost(nil, 10_000)
	if res.Multiplier != 10_000 {
		t.Errorf("Multiplier on nil attempt = %d, want 10000", res.Multiplier)
	}
}

func TestFinalizeAttemptCost_FallsBackToMappedModelWhenResponseEmpty(t *testing.T) {
	// 没拿到 ResponseModel(上游不规范、或失败前没收到响应):用 MappedModel 查价。
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 22, ModelID: "mapped-model", InputPriceMicro: 1_000_000},
	})

	att := &domain.ProxyUpstreamAttempt{
		MappedModel:     "mapped-model",
		InputTokenCount: 100_000,
	}
	res := FinalizeAttemptCost(att, 0)
	if res.ModelPriceID != 22 {
		t.Errorf("ModelPriceID = %d, want 22 (should fall back to MappedModel)", res.ModelPriceID)
	}
}

func TestMirrorCostToRequest_CopiesAllBillingAndTokenFields(t *testing.T) {
	att := &domain.ProxyUpstreamAttempt{
		Cost:              999,
		ModelPriceID:      42,
		Multiplier:        13_000,
		InputTokenCount:   100,
		OutputTokenCount:  200,
		CacheReadCount:    300,
		CacheWriteCount:   400,
		Cache5mWriteCount: 500,
		Cache1hWriteCount: 600,
	}
	req := &domain.ProxyRequest{
		// 故意填一些“脏”值,验证镜像确实会覆盖
		Cost:            1,
		InputTokenCount: 1,
	}

	MirrorCostToRequest(req, att)

	if req.Cost != 999 || req.ModelPriceID != 42 || req.Multiplier != 13_000 {
		t.Errorf("billing fields: cost=%d priceID=%d mul=%d", req.Cost, req.ModelPriceID, req.Multiplier)
	}
	if req.InputTokenCount != 100 || req.OutputTokenCount != 200 {
		t.Errorf("token in/out: in=%d out=%d", req.InputTokenCount, req.OutputTokenCount)
	}
	if req.CacheReadCount != 300 || req.CacheWriteCount != 400 ||
		req.Cache5mWriteCount != 500 || req.Cache1hWriteCount != 600 {
		t.Errorf("cache: r=%d w=%d 5m=%d 1h=%d",
			req.CacheReadCount, req.CacheWriteCount, req.Cache5mWriteCount, req.Cache1hWriteCount)
	}
}

func TestMirrorCostToRequest_NilArgsNoPanic(t *testing.T) {
	MirrorCostToRequest(nil, &domain.ProxyUpstreamAttempt{})
	MirrorCostToRequest(&domain.ProxyRequest{}, nil)
}
