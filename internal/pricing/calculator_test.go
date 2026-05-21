package pricing

import (
	"testing"

	"github.com/awsl-project/maxx/internal/usage"
)

func TestCalculateTieredCost(t *testing.T) {
	// $3/M tokens, 阈值 200K, 超阈值倍率 2/1。期望值为纳美元。
	basePriceMicro := uint64(3_000_000)

	tests := []struct {
		name     string
		tokens   uint64
		expected uint64
	}{
		{
			name:     "below threshold 100K",
			tokens:   100_000,
			expected: 300_000_000, // 100K × $3/M = $0.30 = 300,000,000 nanoUSD
		},
		{
			name:     "at threshold 200K",
			tokens:   200_000,
			expected: 600_000_000, // 200K × $3/M = $0.60
		},
		{
			name:     "above threshold 300K",
			tokens:   300_000,
			expected: 1_200_000_000, // 200K × $3 + 100K × $3 × 2 = $0.60 + $0.60
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateTieredCost(tt.tokens, basePriceMicro, 2, 1, 200_000)
			if got != tt.expected {
				t.Errorf("CalculateTieredCost() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCalculateLinearCost(t *testing.T) {
	tests := []struct {
		name       string
		tokens     uint64
		priceMicro uint64
		expected   uint64
	}{
		{
			name:       "1M tokens at $3/M",
			tokens:     1_000_000,
			priceMicro: 3_000_000,
			expected:   3_000_000_000, // $3 = 3,000,000,000 nanoUSD
		},
		{
			name:       "100K tokens at $15/M",
			tokens:     100_000,
			priceMicro: 15_000_000,
			expected:   1_500_000_000, // $1.50
		},
		{
			name:       "50K tokens at $0.30/M (cache read)",
			tokens:     50_000,
			priceMicro: 300_000,
			expected:   15_000_000, // $0.015
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateLinearCost(tt.tokens, tt.priceMicro)
			if got != tt.expected {
				t.Errorf("CalculateLinearCost() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCalculator_Calculate(t *testing.T) {
	calc := NewCalculator()

	tests := []struct {
		name     string
		model    string
		metrics  *usage.Metrics
		wantZero bool
	}{
		{
			name:  "claude-sonnet-4-5 basic",
			model: "claude-sonnet-4-5-20250514",
			metrics: &usage.Metrics{
				InputTokens:  100_000,
				OutputTokens: 10_000,
			},
		},
		{
			name:  "gpt-5.1 basic",
			model: "gpt-5.1",
			metrics: &usage.Metrics{
				InputTokens:  50_000,
				OutputTokens: 5_000,
			},
		},
		{
			name:  "gpt-5.4-mini basic",
			model: "gpt-5.4-mini",
			metrics: &usage.Metrics{
				InputTokens:  50_000,
				OutputTokens: 5_000,
			},
		},
		{
			name:  "gemini-2.5-pro basic",
			model: "gemini-2.5-pro",
			metrics: &usage.Metrics{
				InputTokens:  50_000,
				OutputTokens: 5_000,
			},
		},
		{
			name:  "unknown model",
			model: "unknown-model-xyz",
			metrics: &usage.Metrics{
				InputTokens:  100_000,
				OutputTokens: 10_000,
			},
			wantZero: true,
		},
		{
			name:     "nil metrics",
			model:    "claude-sonnet-4-5",
			metrics:  nil,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.Calculate(tt.model, tt.metrics, 0)
			if tt.wantZero && got.Cost != 0 {
				t.Errorf("Calculate() Cost = %d, want 0", got.Cost)
			}
			if !tt.wantZero && got.Cost == 0 {
				t.Errorf("Calculate() Cost = 0, want non-zero")
			}
			if got.Multiplier != DefaultMultiplier {
				t.Errorf("Calculate() Multiplier = %d, want %d", got.Multiplier, DefaultMultiplier)
			}
		})
	}
}

func TestCalculator_Calculate_WithCache(t *testing.T) {
	calc := NewCalculator()

	// Claude Sonnet 4.5: input=$3/M, output=$15/M
	// Cache read: $0.30/M(显式), 5m/1h write: $3.75/M(显式)
	metrics := &usage.Metrics{
		InputTokens:          100_000, // 100K × $3/M = 300,000,000 nanoUSD
		OutputTokens:         10_000,  // 10K × $15/M = 150,000,000 nanoUSD
		CacheReadCount:       50_000,  // 50K × $0.30/M = 15,000,000 nanoUSD
		Cache5mCreationCount: 20_000,  // 20K × $3.75/M = 75,000,000 nanoUSD
		Cache1hCreationCount: 10_000,  // 10K × $3.75/M = 37,500,000 nanoUSD
	}

	got := calc.Calculate("claude-sonnet-4-5", metrics, 0)
	if got.Cost == 0 {
		t.Fatal("Calculate() Cost = 0, want non-zero")
	}

	expected := uint64(577_500_000)
	if got.Cost != expected {
		t.Errorf("Calculate() Cost = %d nanoUSD, want %d nanoUSD", got.Cost, expected)
	}
}

func TestCalculator_Calculate_1MContext(t *testing.T) {
	calc := NewCalculator()

	// Claude Sonnet 4.5 1M context: 超 200K 时 input×2, output×1.5
	// input: $3/M, output: $15/M
	metrics := &usage.Metrics{
		InputTokens:  300_000, // 200K×$3 + 100K×$3×2 = $0.6 + $0.6 = $1.2
		OutputTokens: 50_000,  // <200K: 50K×$15/M = $0.75
	}

	got := calc.Calculate("claude-sonnet-4-5", metrics, 0)
	expected := uint64(1_200_000_000 + 750_000_000)
	if got.Cost != expected {
		t.Errorf("Calculate() Cost = %d nanoUSD, want %d nanoUSD", got.Cost, expected)
	}
}

func TestCalculator_Calculate_AppliesMultiplier(t *testing.T) {
	calc := NewCalculator()

	metrics := &usage.Metrics{InputTokens: 1_000_000} // $3 = 3,000,000,000 nanoUSD
	base := calc.Calculate("claude-sonnet-4-5", metrics, DefaultMultiplier)
	scaled := calc.Calculate("claude-sonnet-4-5", metrics, 12_000) // 1.2×

	if scaled.Cost != base.Cost*12000/10000 {
		t.Errorf("multiplier not applied: base=%d scaled=%d", base.Cost, scaled.Cost)
	}
	if scaled.Multiplier != 12_000 {
		t.Errorf("returned Multiplier = %d, want 12000", scaled.Multiplier)
	}
}

func TestPriceTable_Get_PrefixMatch(t *testing.T) {
	pt := DefaultPriceTable()

	tests := []struct {
		modelID   string
		wantFound bool
	}{
		{"claude-sonnet-4-5", true},
		{"claude-sonnet-4-5-20250514", true},
		{"claude-opus-4-5", true},
		{"claude-opus-4-5-20251001", true},
		{"claude-opus-4-6", true},
		{"claude-opus-4-6-20260205", true},
		{"claude-haiku-4-5", true},
		{"claude-haiku-4-5-20251001", true},
		{"gpt-5.1", true},
		{"gpt-5.1-codex", true},
		{"gpt-5.4", true},
		{"gpt-5.4-mini", true},
		{"gpt-5.5", true},
		{"gpt-5.5-pro", true},
		{"gemini-2.5-pro", true},
		{"gemini-3-pro-preview", true},
		{"unknown-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			pricing := pt.Get(tt.modelID)
			if tt.wantFound && pricing == nil {
				t.Errorf("Get(%s) = nil, want non-nil", tt.modelID)
			}
			if !tt.wantFound && pricing != nil {
				t.Errorf("Get(%s) = %v, want nil", tt.modelID, pricing)
			}
		})
	}
}

func TestExplicitCachePrices(t *testing.T) {
	pt := DefaultPriceTable()

	pricing := pt.Get("claude-sonnet-4-5")
	if pricing == nil {
		t.Fatal("claude-sonnet-4-5 not found")
	}

	if got := pricing.GetEffectiveCacheReadPriceMicro(); got != 300_000 {
		t.Errorf("GetEffectiveCacheReadPriceMicro() = %d, want 300000", got)
	}
	if got := pricing.GetEffectiveCache5mWritePriceMicro(); got != 3_750_000 {
		t.Errorf("GetEffectiveCache5mWritePriceMicro() = %d, want 3750000", got)
	}
	if got := pricing.GetEffectiveCache1hWritePriceMicro(); got != 3_750_000 {
		t.Errorf("GetEffectiveCache1hWritePriceMicro() = %d, want 3750000", got)
	}
}

func TestDefaultCachePrices(t *testing.T) {
	pricing := &ModelPricing{
		InputPriceMicro:  1_000_000,
		OutputPriceMicro: 5_000_000,
	}

	if got := pricing.GetEffectiveCacheReadPriceMicro(); got != 100_000 {
		t.Errorf("GetEffectiveCacheReadPriceMicro() = %d, want 100000", got)
	}
	if got := pricing.GetEffectiveCache5mWritePriceMicro(); got != 1_250_000 {
		t.Errorf("GetEffectiveCache5mWritePriceMicro() = %d, want 1250000", got)
	}
	if got := pricing.GetEffectiveCache1hWritePriceMicro(); got != 2_000_000 {
		t.Errorf("GetEffectiveCache1hWritePriceMicro() = %d, want 2000000", got)
	}
}
