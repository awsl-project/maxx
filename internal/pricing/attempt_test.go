package pricing

import (
	"fmt"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
)

func TestPricingModel(t *testing.T) {
	tests := []struct {
		name                              string
		response, mapped, request, expect string
	}{
		{"response wins", "claude-3-opus", "claude-3-sonnet", "claude-2", "claude-3-opus"},
		{"mapped fallback", "", "claude-3-sonnet", "claude-2", "claude-3-sonnet"},
		{"request fallback", "", "", "claude-2", "claude-2"},
		{"all empty", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PricingModel(tt.response, tt.mapped, tt.request)
			if got != tt.expect {
				t.Errorf("PricingModel(%q,%q,%q) = %q, want %q",
					tt.response, tt.mapped, tt.request, got, tt.expect)
			}
		})
	}
}

func TestRecalcAttemptUpdate_NoChange(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 7, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	metrics := &usage.Metrics{InputTokens: 1_000_000, OutputTokens: 0}
	// 1M input × $3 × 1.0 = $3 = 3e9 nano
	const expectedCost = uint64(3_000_000_000)

	// currentCost = expectedCost AND currentModelPriceID = 7 (matches the loaded ID) → no change.
	newCost, update, changed := RecalcAttemptUpdate("claude-3-opus", metrics, DefaultMultiplier, expectedCost, 7)
	if changed {
		t.Errorf("changed = true, want false (cost AND model_price_id unchanged → no update)")
	}
	if newCost != expectedCost {
		t.Errorf("newCost = %d, want %d", newCost, expectedCost)
	}
	if update != (domain.AttemptCostUpdate{}) {
		t.Errorf("update = %+v, want zero value when unchanged", update)
	}
}

func TestRecalcAttemptUpdate_CostChanged(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 9, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	metrics := &usage.Metrics{InputTokens: 1_000_000}
	const correctCost = uint64(3_000_000_000)
	const wrongCost = uint64(999)

	// currentCost diverges → changed regardless of model_price_id arg.
	newCost, update, changed := RecalcAttemptUpdate("claude-3-opus", metrics, DefaultMultiplier, wrongCost, 9)
	if !changed {
		t.Fatalf("changed = false, want true (cost diff %d→%d)", wrongCost, correctCost)
	}
	if newCost != correctCost {
		t.Errorf("newCost = %d, want %d", newCost, correctCost)
	}
	if update.Cost != correctCost {
		t.Errorf("update.Cost = %d, want %d", update.Cost, correctCost)
	}
	if update.ModelPriceID != 9 {
		t.Errorf("update.ModelPriceID = %d, want 9 (must sync to current price record)", update.ModelPriceID)
	}
}

func TestRecalcAttemptUpdate_PreservesHistoricalMultiplier(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 3, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	// 1M input × $3 × 1.2 = 3.6e9 nano. 历史倍率 12000 (1.2×) 必须被尊重,
	// 否则 backfill 会偷偷把折扣率回退到 1.0×。
	metrics := &usage.Metrics{InputTokens: 1_000_000}
	const expectedCost = uint64(3_600_000_000)

	newCost, _, _ := RecalcAttemptUpdate("claude-3-opus", metrics, 12000, 0, 0)
	if newCost != expectedCost {
		t.Errorf("multiplier=12000: newCost = %d, want %d (historical 1.2× must apply)",
			newCost, expectedCost)
	}
}

// TestRecalcAttemptUpdate_ModelPriceIDOnlyChange 验证 review 反馈:
// 当 Cost 没变但价格记录被换成等额新版本(ModelPriceID 不同)时,
// 必须仍然触发 update,以刷新 attempt 上的 model_price_id 审计链。
func TestRecalcAttemptUpdate_ModelPriceIDOnlyChange(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 200, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	metrics := &usage.Metrics{InputTokens: 1_000_000}
	const correctCost = uint64(3_000_000_000)

	// currentCost 已正确,但 currentModelPriceID=100(旧版本 ID 已被替换成 200)。
	newCost, update, changed := RecalcAttemptUpdate("claude-3-opus", metrics, DefaultMultiplier, correctCost, 100)
	if !changed {
		t.Fatal("changed = false; 价格 ID 不匹配也必须触发 update 以刷新审计链")
	}
	if newCost != correctCost {
		t.Errorf("newCost = %d, want %d (cost 本应不变)", newCost, correctCost)
	}
	if update.Cost != correctCost {
		t.Errorf("update.Cost = %d, want %d", update.Cost, correctCost)
	}
	if update.ModelPriceID != 200 {
		t.Errorf("update.ModelPriceID = %d, want 200 (必须刷到当前匹配 ID)", update.ModelPriceID)
	}
}

func TestRecalcFromAttempt(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 11, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	attempt := &domain.ProxyUpstreamAttempt{
		ID:               42,
		ResponseModel:    "claude-3-opus",
		MappedModel:      "claude-3-sonnet", // 应被 ResponseModel 覆盖
		RequestModel:     "claude-2",        // 应被 ResponseModel 覆盖
		InputTokenCount:  1_000_000,
		OutputTokenCount: 0,
		Multiplier:       DefaultMultiplier,
		Cost:             0, // 实际 cost = 3e9 与之不符 → 应触发 update
	}

	newCost, update, changed := RecalcFromAttempt(attempt)
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if newCost != 3_000_000_000 {
		t.Errorf("newCost = %d, want 3e9 (claude-3-opus, not sonnet/claude-2)", newCost)
	}
	if update.ModelPriceID != 11 {
		t.Errorf("update.ModelPriceID = %d, want 11", update.ModelPriceID)
	}
}

func TestRecalcFromAttempt_FallsBackToMappedThenRequest(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 21, ModelID: "claude-3-sonnet", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	// 只填 MappedModel → 应该匹配到 sonnet 价格
	attempt := &domain.ProxyUpstreamAttempt{
		MappedModel:     "claude-3-sonnet",
		RequestModel:    "claude-2",
		InputTokenCount: 1_000_000,
		Multiplier:      DefaultMultiplier,
	}
	newCost, _, _ := RecalcFromAttempt(attempt)
	if newCost != 3_000_000_000 {
		t.Errorf("mapped-only: newCost = %d, want 3e9", newCost)
	}
}

func TestRecalcFromCostData(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 33, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	data := &domain.AttemptCostData{
		ID:               99,
		ResponseModel:    "claude-3-opus",
		InputTokenCount:  500_000,
		OutputTokenCount: 100_000,
		Multiplier:       DefaultMultiplier,
		Cost:             0,
	}
	// 0.5M × $3 + 0.1M × $15 = $1.5 + $1.5 = $3 = 3e9
	const expected = uint64(3_000_000_000)

	newCost, update, changed := RecalcFromCostData(data)
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if newCost != expected {
		t.Errorf("newCost = %d, want %d", newCost, expected)
	}
	if update.Cost != expected {
		t.Errorf("update.Cost = %d, want %d", update.Cost, expected)
	}
	if update.ModelPriceID != 33 {
		t.Errorf("update.ModelPriceID = %d, want 33", update.ModelPriceID)
	}
}

func TestRecalcFromCostData_PreservesHistoricalMultiplier(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 5, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	data := &domain.AttemptCostData{
		ResponseModel:   "claude-3-opus",
		InputTokenCount: 1_000_000,
		Multiplier:      15000, // 1.5×
	}
	newCost, _, _ := RecalcFromCostData(data)
	// 1M × $3 × 1.5 = $4.5 = 4.5e9
	if newCost != 4_500_000_000 {
		t.Errorf("multiplier=15000: newCost = %d, want 4.5e9 (historical 1.5× must apply)", newCost)
	}
}

func TestRecalcFromCostData_ZeroMultiplierDefaultsToOne(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 6, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000, OutputPriceMicro: 15_000_000},
	})

	data := &domain.AttemptCostData{
		ResponseModel:   "claude-3-opus",
		InputTokenCount: 1_000_000,
		Multiplier:      0, // 历史空字段 → 视作 10000 (1.0×)
	}
	newCost, _, _ := RecalcFromCostData(data)
	if newCost != 3_000_000_000 {
		t.Errorf("multiplier=0: newCost = %d, want 3e9 (zero must default to 1.0×)", newCost)
	}
}

// TestRecalcFromAttempt_UsesHistoricalSnapshotByID 锁住"当时价"重算契约:
// 当 attempt.ModelPriceID 指向已被新价覆盖的历史快照(版本化机制保留),
// 重算应该用旧快照价 × 最新 token,而不是按今天的当前价(模型名查表)。
//
// 这是这次改动的核心:之前 RecalcAttemptUpdate 用模型名走 Calculate,
// 等于把"重算"语义变成"按今天的价回填",违背了 token 后补/缺失指标
// backfill 的真正意图(按当时合约价 × 已知 token 数补足成本)。
func TestRecalcFromAttempt_UsesHistoricalSnapshotByID(t *testing.T) {
	resetGlobalCalculator(t)
	old := &domain.ModelPrice{ID: 10, ModelID: "claude-3-opus", InputPriceMicro: 3_000_000}
	current := &domain.ModelPrice{ID: 20, ModelID: "claude-3-opus", InputPriceMicro: 9_000_000}
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{current})
	GlobalCalculator().SetHistoricalLookup(func(id uint64) (*domain.ModelPrice, error) {
		if id == 10 {
			return old, nil
		}
		return nil, nil
	})

	// attempt 当时按 $3 价记入 ModelPriceID=10。流式结尾 token 后补,
	// Cost 字段是 0(或老值),触发重算。
	attempt := &domain.ProxyUpstreamAttempt{
		ResponseModel:   "claude-3-opus",
		InputTokenCount: 1_000_000,
		Multiplier:      DefaultMultiplier,
		Cost:            0,
		ModelPriceID:    10, // 历史 ID
	}

	newCost, update, changed := RecalcFromAttempt(attempt)
	if !changed {
		t.Fatal("changed = false, want true (cost 从 0 改到 3e9)")
	}
	// 当时价 $3 → 1M × $3 = 3e9 nanoUSD,不是按今天 $9 的 9e9
	if newCost != 3_000_000_000 {
		t.Errorf("newCost = %d, want 3e9 (当时价 $3, 不是 today's $9)", newCost)
	}
	if update.ModelPriceID != 10 {
		t.Errorf("update.ModelPriceID = %d, want 10 (维持历史 ID,不被刷成 20)", update.ModelPriceID)
	}
}

// TestRecalcFromAttempt_FallsBackToByModelWhenIDMissing 验证 ModelPriceID=0
// (极老数据,从未记录过 ID)走按模型名查当前价的兜底路径。
func TestRecalcFromAttempt_FallsBackToByModelWhenIDMissing(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 20, ModelID: "claude-3-opus", InputPriceMicro: 9_000_000},
	})
	// 未设置 historicalLookup;ModelPriceID=0 也根本不该触发它。

	attempt := &domain.ProxyUpstreamAttempt{
		ResponseModel:   "claude-3-opus",
		InputTokenCount: 1_000_000,
		Multiplier:      DefaultMultiplier,
		ModelPriceID:    0, // 没记录历史 ID
	}
	newCost, update, _ := RecalcFromAttempt(attempt)
	if newCost != 9_000_000_000 {
		t.Errorf("newCost = %d, want 9e9 (fallback 按模型名取当前价)", newCost)
	}
	if update.ModelPriceID != 20 {
		t.Errorf("update.ModelPriceID = %d, want 20 (写入当前 ID,补齐历史空缺)", update.ModelPriceID)
	}
}

// TestRecalcFromAttempt_DoesNotFallBackOnLookupError 锁住核心安全契约:
// historicalLookup 返回非 nil 错误时,不得回退到按模型名走当前价——那会用
// 今天的价格悄悄覆盖历史 attempt 的成本,违反"重算当时价"的语义保证。
// 正确行为:放弃本次重算,保留原 cost 和 model_price_id,并返回 changed=false。
func TestRecalcFromAttempt_DoesNotFallBackOnLookupError(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 20, ModelID: "claude-3-opus", InputPriceMicro: 9_000_000},
	})
	GlobalCalculator().SetHistoricalLookup(func(id uint64) (*domain.ModelPrice, error) {
		return nil, fmt.Errorf("db timeout") // 瞬态错误
	})

	const originalCost = uint64(3_000_000_000)
	attempt := &domain.ProxyUpstreamAttempt{
		ResponseModel:   "claude-3-opus",
		InputTokenCount: 1_000_000,
		Multiplier:      DefaultMultiplier,
		Cost:            originalCost, // 当时价 $3 算出的历史值
		ModelPriceID:    10,           // 历史 ID,反查会失败
	}

	newCost, update, changed := RecalcFromAttempt(attempt)

	// 必须 changed=false:不得因为 lookup 错误就写入一个当前价算出的新值
	if changed {
		t.Error("changed = true on lookup error; must be false to protect historical cost")
	}
	// 返回的 newCost 必须是原值,不是按今天的当前价($9)算出的 9e9
	if newCost != originalCost {
		t.Errorf("newCost = %d on lookup error, want %d (original cost must be preserved)", newCost, originalCost)
	}
	// update 必须是零值,不得更新 model_price_id
	if update != (domain.AttemptCostUpdate{}) {
		t.Errorf("update = %+v on lookup error; want zero value (no DB write should happen)", update)
	}
}

// TestRecalcFromAttempt_FallsBackWhenHistoricalLookupReturnsNil 验证:
// ModelPriceID 非 0,但反查不到(例如硬删了历史表 / 测试环境未注入 lookup),
// 不应该崩,而是降级走模型名取当前价。
func TestRecalcFromAttempt_FallsBackWhenHistoricalLookupReturnsNil(t *testing.T) {
	resetGlobalCalculator(t)
	GlobalCalculator().LoadFromDatabase([]*domain.ModelPrice{
		{ID: 20, ModelID: "claude-3-opus", InputPriceMicro: 9_000_000},
	})
	GlobalCalculator().SetHistoricalLookup(func(id uint64) (*domain.ModelPrice, error) {
		return nil, nil // 反查不到
	})

	attempt := &domain.ProxyUpstreamAttempt{
		ResponseModel:   "claude-3-opus",
		InputTokenCount: 1_000_000,
		Multiplier:      DefaultMultiplier,
		ModelPriceID:    99, // 不存在的 ID
	}
	newCost, _, _ := RecalcFromAttempt(attempt)
	if newCost != 9_000_000_000 {
		t.Errorf("newCost = %d, want 9e9 (fallback 必须工作)", newCost)
	}
}
