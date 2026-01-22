package pricing

import (
	"testing"

	"github.com/awsl-project/maxx/internal/usage"
)

func TestCalculateTieredCostMicro(t *testing.T) {
	// 测试: $3/M tokens, 阈值 200K, 超阈值倍率 2/1
	basePriceMicro := uint64(3_000_000) // $3/M

	tests := []struct {
		name     string
		tokens   uint64
		expected uint64
	}{
		{
			name:     "below threshold 100K",
			tokens:   100_000,
			expected: 300_000, // 100K × $3/M = $0.30 = 300,000 microUSD
		},
		{
			name:     "at threshold 200K",
			tokens:   200_000,
			expected: 600_000, // 200K × $3/M = $0.60 = 600,000 microUSD
		},
		{
			name:     "above threshold 300K",
			tokens:   300_000,
			expected: 1_200_000, // 200K × $3/M + 100K × $3/M × 2 = $0.60 + $0.60 = 1,200,000 microUSD
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateTieredCostMicro(tt.tokens, basePriceMicro, 2, 1, 200_000)
			if got != tt.expected {
				t.Errorf("CalculateTieredCostMicro() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCalculateLinearCostMicro(t *testing.T) {
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
			expected:   3_000_000, // $3
		},
		{
			name:       "100K tokens at $15/M",
			tokens:     100_000,
			priceMicro: 15_000_000,
			expected:   1_500_000, // $1.50
		},
		{
			name:       "50K tokens at $0.30/M (cache read)",
			tokens:     50_000,
			priceMicro: 300_000, // $0.30/M
			expected:   15_000,  // $0.015
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateLinearCostMicro(tt.tokens, tt.priceMicro)
			if got != tt.expected {
				t.Errorf("CalculateLinearCostMicro() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCalculator_Calculate(t *testing.T) {
	calc := GlobalCalculator()

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
			wantZero: false,
		},
		{
			name:  "gpt-5.1 basic",
			model: "gpt-5.1",
			metrics: &usage.Metrics{
				InputTokens:  50_000,
				OutputTokens: 5_000,
			},
			wantZero: false,
		},
		{
			name:  "gemini-2.5-pro basic",
			model: "gemini-2.5-pro",
			metrics: &usage.Metrics{
				InputTokens:  50_000,
				OutputTokens: 5_000,
			},
			wantZero: false,
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
			got := calc.Calculate(tt.model, tt.metrics)
			if tt.wantZero && got != 0 {
				t.Errorf("Calculate() = %d, want 0", got)
			}
			if !tt.wantZero && got == 0 {
				t.Errorf("Calculate() = 0, want non-zero")
			}
		})
	}
}

func TestCalculator_Calculate_WithCache(t *testing.T) {
	calc := GlobalCalculator()

	// Claude Sonnet 4.5: input=$3/M, output=$15/M
	// Cache read: $0.30/M (显式配置)
	// Cache 5m/1h write: $3.75/M (显式配置)
	metrics := &usage.Metrics{
		InputTokens:          100_000, // 100K × $3/M = $0.30 = 300,000,000 nanoUSD
		OutputTokens:         10_000,  // 10K × $15/M = $0.15 = 150,000,000 nanoUSD
		CacheReadCount:       50_000,  // 50K × $0.30/M = $0.015 = 15,000,000 nanoUSD
		Cache5mCreationCount: 20_000,  // 20K × $3.75/M = $0.075 = 75,000,000 nanoUSD
		Cache1hCreationCount: 10_000,  // 10K × $3.75/M = $0.0375 = 37,500,000 nanoUSD
	}

	cost := calc.Calculate("claude-sonnet-4-5", metrics)
	if cost == 0 {
		t.Fatal("Calculate() = 0, want non-zero")
	}

	// Expected: 300,000,000 + 150,000,000 + 15,000,000 + 75,000,000 + 37,500,000 = 577,500,000 nanoUSD
	expectedNanoUSD := uint64(577_500_000)
	if cost != expectedNanoUSD {
		t.Errorf("Calculate() = %d nanoUSD, want %d nanoUSD", cost, expectedNanoUSD)
	}
}

func TestCalculator_Calculate_1MContext(t *testing.T) {
	calc := GlobalCalculator()

	// Claude Sonnet 4.5 with 1M context: 超过 200K 时 input×2, output×1.5
	// input: $3/M, output: $15/M
	metrics := &usage.Metrics{
		InputTokens:  300_000, // 200K×$3 + 100K×$3×2 = $0.6 + $0.6 = $1.2 = 1,200,000,000 nanoUSD
		OutputTokens: 50_000,  // 全部低于 200K: 50K×$15/M = $0.75 = 750,000,000 nanoUSD
	}

	cost := calc.Calculate("claude-sonnet-4-5", metrics)
	expectedNanoUSD := uint64(1_200_000_000 + 750_000_000)
	if cost != expectedNanoUSD {
		t.Errorf("Calculate() = %d nanoUSD, want %d nanoUSD", cost, expectedNanoUSD)
	}
}

func TestPriceTable_Get_PrefixMatch(t *testing.T) {
	pt := DefaultPriceTable()

	tests := []struct {
		modelID   string
		wantFound bool
	}{
		{"claude-sonnet-4-5", true},
		{"claude-sonnet-4-5-20250514", true}, // prefix match
		{"claude-opus-4-5", true},
		{"claude-opus-4-5-20251001", true}, // prefix match
		{"claude-haiku-4-5", true},
		{"claude-haiku-4-5-20251001", true}, // prefix match
		{"gpt-5.1", true},
		{"gpt-5.1-codex", true},
		{"gpt-5.2", true},
		{"gemini-2.5-pro", true},
		{"gemini-2.5-flash", true},
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

	// 验证 Claude Sonnet 4.5 的显式缓存价格
	pricing := pt.Get("claude-sonnet-4-5")
	if pricing == nil {
		t.Fatal("claude-sonnet-4-5 not found")
	}

	// cache read: $0.30/M = 300,000 microUSD/M
	if got := pricing.GetEffectiveCacheReadPriceMicro(); got != 300_000 {
		t.Errorf("GetEffectiveCacheReadPriceMicro() = %d, want 300000", got)
	}

	// cache 5m write: $3.75/M = 3,750,000 microUSD/M
	if got := pricing.GetEffectiveCache5mWritePriceMicro(); got != 3_750_000 {
		t.Errorf("GetEffectiveCache5mWritePriceMicro() = %d, want 3750000", got)
	}

	// cache 1h write: $3.75/M = 3,750,000 microUSD/M
	if got := pricing.GetEffectiveCache1hWritePriceMicro(); got != 3_750_000 {
		t.Errorf("GetEffectiveCache1hWritePriceMicro() = %d, want 3750000", got)
	}
}

func TestDefaultCachePrices(t *testing.T) {
	// 验证没有显式配置缓存价格时的默认计算
	pricing := &ModelPricing{
		InputPriceMicro:  1_000_000, // $1/M
		OutputPriceMicro: 5_000_000, // $5/M
	}

	// cache read: input / 10 = $0.10/M = 100,000 microUSD/M
	if got := pricing.GetEffectiveCacheReadPriceMicro(); got != 100_000 {
		t.Errorf("GetEffectiveCacheReadPriceMicro() = %d, want 100000", got)
	}

	// cache 5m write: input * 5/4 = $1.25/M = 1,250,000 microUSD/M
	if got := pricing.GetEffectiveCache5mWritePriceMicro(); got != 1_250_000 {
		t.Errorf("GetEffectiveCache5mWritePriceMicro() = %d, want 1250000", got)
	}

	// cache 1h write: input * 2 = $2/M = 2,000,000 microUSD/M
	if got := pricing.GetEffectiveCache1hWritePriceMicro(); got != 2_000_000 {
		t.Errorf("GetEffectiveCache1hWritePriceMicro() = %d, want 2000000", got)
	}
}
