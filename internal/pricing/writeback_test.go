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
	// review 提的契约:multiplier 也要回写到 attempt 上,
	// 不能只填返回值 — 后续审计 / MirrorCostToRequest 都依赖 attempt 自己的字段。
	if att.Multiplier != DefaultMultiplier {
		t.Errorf("attempt.Multiplier = %d, want %d (must be written back)", att.Multiplier, DefaultMultiplier)
	}
}

// TestFinalizeAttemptCost_CacheOnlyTokensTriggersBilling 是 review 提出的核心遗漏:
// 之前的 no-token guard 只看 InputTokenCount/OutputTokenCount,会让"只命中缓存"的请求
// (如缓存重放的流式 turn)被静默置零 Cost / ModelPriceID。
// Calculator 本身按 cache_read / cache_5m / cache_1h 单独算钱,guard 必须放行。
func TestFinalizeAttemptCost_CacheOnlyTokensTriggersBilling(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		// CacheReadPriceMicro = input * 0.1,这里 InputPriceMicro=3e6 → cache_read=3e5
		{ID: 77, ModelID: "test-model", InputPriceMicro: 3_000_000},
	})

	att := &domain.ProxyUpstreamAttempt{
		ResponseModel:  "test-model",
		CacheReadCount: 1_000_000, // 1M cache-read tokens, 没有 input/output
	}
	res := FinalizeAttemptCost(att, DefaultMultiplier)

	// 1M cache_read × $0.30/M (input × 0.1) = $0.30 = 3e8 nano
	if res.Cost == 0 {
		t.Fatal("Cost = 0; cache-only attempt was silently dropped — guard漏掉了 cache 字段")
	}
	if res.ModelPriceID != 77 {
		t.Errorf("ModelPriceID = %d, want 77 (cache-only attempt 也要绑定到当前价格行)", res.ModelPriceID)
	}
	if att.Cost == 0 || att.ModelPriceID != 77 {
		t.Errorf("attempt.Cost=%d ModelPriceID=%d; both must be written back", att.Cost, att.ModelPriceID)
	}
}

// TestFinalizeAttemptCost_NoTokensPreservesExistingFields 锁住 docstring 的承诺:
// 没有 token 时只回写 Multiplier,既有的 Cost / ModelPriceID 不能被覆盖。
// (旧 NoTokensKeepsMultiplierOnly 测试只验证零初始化的情况,review 指出契约其实更强。)
func TestFinalizeAttemptCost_NoTokensPreservesExistingFields(t *testing.T) {
	resetGlobalCalculator(t)
	att := &domain.ProxyUpstreamAttempt{
		ResponseModel: "test-model",
		Cost:          12_345, // 既有非零值
		ModelPriceID:  7,      // 既有非零值
		// 没有任何 token / cache 字段
	}
	_ = FinalizeAttemptCost(att, 11_000)
	if att.Cost != 12_345 {
		t.Errorf("attempt.Cost = %d, want 12345 (no-token 路径不应覆盖既有 Cost)", att.Cost)
	}
	if att.ModelPriceID != 7 {
		t.Errorf("attempt.ModelPriceID = %d, want 7 (no-token 路径不应覆盖既有 ID)", att.ModelPriceID)
	}
}

// TestFinalizeAttemptCost_UnknownModelDoesNotPanic 验证当 token 非零但模型未在价表里时,
// Calculator 返回零成本/零 ID,FinalizeAttemptCost 把这些原样写回(不抛错也不留旧值)。
// 这是 production 里有时会看到的"不在价表里的 unknown-model" log 噪音对应的实际行为。
func TestFinalizeAttemptCost_UnknownModelDoesNotPanic(t *testing.T) {
	resetGlobalCalculator(t)
	att := &domain.ProxyUpstreamAttempt{
		ResponseModel:   "no-such-model",
		InputTokenCount: 1_000_000,
		Cost:            999, // 假设上一次算过了
		ModelPriceID:    88,
	}
	res := FinalizeAttemptCost(att, DefaultMultiplier)
	if res.Cost != 0 || res.ModelPriceID != 0 {
		t.Errorf("unknown model res = %+v, want Cost=0 ModelPriceID=0", res)
	}
	// attempt 的旧 Cost/ModelPriceID 在 unknown-model 路径下被清零 — 这是当前 Calculator 行为,
	// 锁住它防止意外回归(若以后想保留旧值,需要在 Calculator 层显式处理)。
	if att.Cost != 0 || att.ModelPriceID != 0 {
		t.Errorf("attempt after unknown model = Cost %d, ID %d; want both 0", att.Cost, att.ModelPriceID)
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
