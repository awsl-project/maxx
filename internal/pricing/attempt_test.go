package pricing

import (
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
