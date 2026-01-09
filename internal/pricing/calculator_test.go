package pricing

import (
	"math"
	"testing"

	"github.com/Bowl42/maxx-next/internal/usage"
)

// almostEqual compares two float64 values with a tolerance
func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestCalculateTieredCost(t *testing.T) {
	tests := []struct {
		name      string
		tokens    uint64
		basePrice float64
		premium   float64
		threshold uint64
		expected  float64
	}{
		{
			name:      "below threshold",
			tokens:    100000,
			basePrice: 3.0,
			premium:   2.0,
			threshold: 200000,
			expected:  0.3, // 100K × $3/M = $0.30
		},
		{
			name:      "at threshold",
			tokens:    200000,
			basePrice: 3.0,
			premium:   2.0,
			threshold: 200000,
			expected:  0.6, // 200K × $3/M = $0.60
		},
		{
			name:      "above threshold",
			tokens:    300000,
			basePrice: 3.0,
			premium:   2.0,
			threshold: 200000,
			expected:  1.2, // 200K × $3/M + 100K × $3/M × 2.0 = $0.60 + $0.60 = $1.20
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateTieredCost(tt.tokens, tt.basePrice, tt.premium, tt.threshold)
			if !almostEqual(got, tt.expected, 0.0001) {
				t.Errorf("CalculateTieredCost() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCalculateLinearCost(t *testing.T) {
	tests := []struct {
		name     string
		tokens   uint64
		price    float64
		expected float64
	}{
		{
			name:     "1M tokens",
			tokens:   1000000,
			price:    3.0,
			expected: 3.0,
		},
		{
			name:     "100K tokens",
			tokens:   100000,
			price:    15.0,
			expected: 1.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateLinearCost(tt.tokens, tt.price)
			if got != tt.expected {
				t.Errorf("CalculateLinearCost() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestToMicroUSD(t *testing.T) {
	tests := []struct {
		usd      float64
		expected uint64
	}{
		{1.0, 1000000},
		{0.001, 1000},
		{0.000001, 1},
	}

	for _, tt := range tests {
		got := ToMicroUSD(tt.usd)
		if got != tt.expected {
			t.Errorf("ToMicroUSD(%v) = %v, want %v", tt.usd, got, tt.expected)
		}
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
			name:  "claude-sonnet-4 basic",
			model: "claude-sonnet-4-20250514",
			metrics: &usage.Metrics{
				InputTokens:  100000,
				OutputTokens: 10000,
			},
			wantZero: false,
		},
		{
			name:  "gpt-4o basic",
			model: "gpt-4o-2024-05-13",
			metrics: &usage.Metrics{
				InputTokens:  50000,
				OutputTokens: 5000,
			},
			wantZero: false,
		},
		{
			name:  "unknown model",
			model: "unknown-model-xyz",
			metrics: &usage.Metrics{
				InputTokens:  100000,
				OutputTokens: 10000,
			},
			wantZero: true,
		},
		{
			name:    "nil metrics",
			model:   "claude-sonnet-4",
			metrics: nil,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.Calculate(tt.model, tt.metrics)
			if tt.wantZero && got != 0 {
				t.Errorf("Calculate() = %v, want 0", got)
			}
			if !tt.wantZero && got == 0 {
				t.Errorf("Calculate() = 0, want non-zero")
			}
		})
	}
}

func TestCalculator_Calculate_WithCache(t *testing.T) {
	calc := GlobalCalculator()

	// Claude Sonnet 4: input=$3/M, output=$15/M
	// Cache read: $3 × 0.1 = $0.30/M
	// Cache 5m write: $3 × 1.25 = $3.75/M
	// Cache 1h write: $3 × 2.0 = $6/M
	metrics := &usage.Metrics{
		InputTokens:          100000,  // $0.30
		OutputTokens:         10000,   // $0.15
		CacheReadCount:       50000,   // $0.015
		Cache5mCreationCount: 20000,   // $0.075
		Cache1hCreationCount: 10000,   // $0.06
	}

	cost := calc.Calculate("claude-sonnet-4", metrics)
	if cost == 0 {
		t.Error("Calculate() = 0, want non-zero")
	}

	// Expected: $0.30 + $0.15 + $0.015 + $0.075 + $0.06 = $0.60
	// In microUSD: 600000
	expectedMicroUSD := uint64(600000)
	if cost != expectedMicroUSD {
		t.Errorf("Calculate() = %v microUSD, want %v microUSD", cost, expectedMicroUSD)
	}
}

func TestPriceTable_Get_PrefixMatch(t *testing.T) {
	pt := DefaultPriceTable()

	tests := []struct {
		modelID   string
		wantFound bool
	}{
		{"claude-sonnet-4", true},
		{"claude-sonnet-4-20250514", true},           // prefix match
		{"claude-sonnet-4-5-20250514", true},         // prefix match
		{"gpt-4o", true},
		{"gpt-4o-2024-05-13", true},                  // prefix match
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
