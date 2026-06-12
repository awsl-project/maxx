package pricing

import (
	"fmt"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
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

func TestCalculator_Calculate_BuiltinHasZeroPriceID(t *testing.T) {
	// 内置默认价表的条目都没有 DB ID,Calculate 返回的 ModelPriceID 应为 0。
	// 这保证 attempt 表里"用内置价表算出"的记录可以和"用 DB 价算出"的区分开。
	calc := NewCalculator()
	got := calc.Calculate("claude-sonnet-4-5", &usage.Metrics{InputTokens: 1_000_000}, 0)
	if got.ModelPriceID != 0 {
		t.Errorf("builtin price ModelPriceID = %d, want 0", got.ModelPriceID)
	}
	if got.Cost == 0 {
		t.Fatal("expected non-zero cost from builtin price")
	}
}

func TestCalculator_LoadFromDatabase_OverlaysBuiltin(t *testing.T) {
	// DB 中 claude-sonnet-4-5 的 input 改为 $6/M(默认是 $3/M)。
	// LoadFromDatabase 后,Calculate 应当走 DB 价(cost 翻倍),并把 DB 记录 ID 写回结果。
	// 用 100K tokens 避开 1M-context 阈值,让两条路径都走线性公式,比较精确。
	calc := NewCalculator()
	metrics := &usage.Metrics{InputTokens: 100_000}
	baseline := calc.Calculate("claude-sonnet-4-5", metrics, 0)

	calc.LoadFromDatabase([]*domain.ModelPrice{
		{
			ID:              42,
			ModelID:         "claude-sonnet-4-5",
			InputPriceMicro: 6_000_000,
		},
	})

	got := calc.Calculate("claude-sonnet-4-5", metrics, 0)
	if got.ModelPriceID != 42 {
		t.Errorf("after LoadFromDatabase, ModelPriceID = %d, want 42", got.ModelPriceID)
	}
	if got.Cost != baseline.Cost*2 {
		t.Errorf("DB price not applied: db=%d builtin=%d (want db = 2× builtin)", got.Cost, baseline.Cost)
	}
}

func TestCalculator_LoadFromDatabase_KeepsBuiltinForUnoverridden(t *testing.T) {
	// LoadFromDatabase 只覆盖 DB 中存在的 ModelID;未覆盖的模型仍应能用内置价计算。
	// 这是为了让用户只配置自己关心的模型,其余继续走内置。
	calc := NewCalculator()
	calc.LoadFromDatabase([]*domain.ModelPrice{
		{ID: 7, ModelID: "claude-sonnet-4-5", InputPriceMicro: 6_000_000},
	})

	got := calc.Calculate("gpt-4o", &usage.Metrics{InputTokens: 1_000_000}, 0)
	if got.Cost == 0 {
		t.Fatal("gpt-4o cost should still be computed from builtin defaults")
	}
	if got.ModelPriceID != 0 {
		t.Errorf("gpt-4o ModelPriceID = %d, want 0 (builtin)", got.ModelPriceID)
	}
}

func TestCalculator_GptImage2_BilledFromBuiltinDefaults(t *testing.T) {
	// gpt-image-2 按 token 类型分档计费(对齐 LiteLLM):
	//   文本输入 $5/M, 图像输入 $8/M, 文本输出 $10/M, 图像输出 $30/M。
	// 响应 usage 把 input/output token 拆成 text/image,Image* 是 Input/Output 的子集。
	// 即便部署的 DB 早已 seed 过、不含 gpt-image-2 行,内置默认仍兜底(ModelPriceID=0)。
	calc := NewCalculator()

	// 用真实 edits 那一笔的 token 数: input 28 = 文本12 + 图像16, output 196 = 图像196。
	got := calc.Calculate("gpt-image-2", &usage.Metrics{
		InputTokens:       28,
		InputImageTokens:  16,
		OutputTokens:      196,
		OutputImageTokens: 196,
	}, 0)

	// 文本输入 12×$5/M=60_000; 图像输入 16×$8/M=128_000; 文本输出 0;
	// 图像输出 196×$30/M=5_880_000 → 合计 6_068_000 nanoUSD。
	const want = 60_000 + 128_000 + 5_880_000
	if got.Cost != want {
		t.Errorf("gpt-image-2 cost = %d, want %d", got.Cost, want)
	}
	if got.ModelPriceID != 0 {
		t.Errorf("gpt-image-2 ModelPriceID = %d, want 0 (builtin default)", got.ModelPriceID)
	}

	// 纯图像 1M in / 1M out → 图像输入 $8 + 图像输出 $30。
	pure := calc.Calculate("gpt-image-2", &usage.Metrics{
		InputTokens: 1_000_000, InputImageTokens: 1_000_000,
		OutputTokens: 1_000_000, OutputImageTokens: 1_000_000,
	}, 0)
	const wantPure = 8_000_000_000 + 30_000_000_000
	if pure.Cost != wantPure {
		t.Errorf("gpt-image-2 pure-image cost = %d, want %d", pure.Cost, wantPure)
	}
}

// TestCalculator_ImageTokenSplit_BackwardCompatible 确认没有图像 token 的普通文本
// 模型计费完全不受新拆分逻辑影响(image 字段恒为 0 → 全额走文本价)。
func TestCalculator_ImageTokenSplit_BackwardCompatible(t *testing.T) {
	calc := NewCalculator()
	// gpt-4o: input $2.50/M, 无 1M 分层。1M input、无图像 token → 纯线性文本价。
	got := calc.Calculate("gpt-4o", &usage.Metrics{InputTokens: 1_000_000}, 0)
	if got.Cost != 2_500_000_000 {
		t.Errorf("text-model cost = %d, want 2_500_000_000 (image split must not change text billing)", got.Cost)
	}
}

func TestCalculator_GetModelPriceByID(t *testing.T) {
	calc := NewCalculator()
	calc.LoadFromDatabase([]*domain.ModelPrice{
		{ID: 99, ModelID: "claude-sonnet-4-5", InputPriceMicro: 1_000_000},
	})

	got := calc.GetModelPriceByID(99)
	if got == nil || got.ModelID != "claude-sonnet-4-5" {
		t.Errorf("GetModelPriceByID(99) = %+v, want claude-sonnet-4-5", got)
	}
	if calc.GetModelPriceByID(0) != nil {
		t.Error("GetModelPriceByID(0) should be nil; builtins are not indexed by ID")
	}
	if calc.GetModelPriceByID(123) != nil {
		t.Error("GetModelPriceByID for unknown ID should be nil")
	}
}

func TestCalculator_Calculate_ZeroMultiplierDefaultsToOne(t *testing.T) {
	// 倍率传 0 时,实际应用 DefaultMultiplier(10000),且结果里也回填 10000。
	// 这是 backfill 旧数据(没有 multiplier 列)的兜底语义。
	calc := NewCalculator()
	metrics := &usage.Metrics{InputTokens: 1_000_000}
	base := calc.Calculate("claude-sonnet-4-5", metrics, DefaultMultiplier)
	zero := calc.Calculate("claude-sonnet-4-5", metrics, 0)
	if zero.Cost != base.Cost {
		t.Errorf("Cost(mul=0) = %d, want = Cost(mul=10000) = %d", zero.Cost, base.Cost)
	}
	if zero.Multiplier != DefaultMultiplier {
		t.Errorf("Multiplier(mul=0) = %d, want %d", zero.Multiplier, DefaultMultiplier)
	}
}

func TestCalculator_Calculate_UnknownModelKeepsMultiplier(t *testing.T) {
	// 模型未命中价表时返回零成本,但 Multiplier 仍按入参回填(便于审计与日志)。
	calc := NewCalculator()
	got := calc.Calculate("nope-not-in-table", &usage.Metrics{InputTokens: 1000}, 15000)
	if got.Cost != 0 {
		t.Errorf("unknown model Cost = %d, want 0", got.Cost)
	}
	if got.ModelPriceID != 0 {
		t.Errorf("unknown model ModelPriceID = %d, want 0", got.ModelPriceID)
	}
	if got.Multiplier != 15000 {
		t.Errorf("unknown model Multiplier = %d, want 15000 (echo back input)", got.Multiplier)
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

// TestCalculator_CalculateByPriceID_UsesCachedSnapshot 验证按 ID 计算时,
// 即使同名模型在 pricesByKey 已被新价覆盖,仍然从 pricesByID 取当时的快照。
// 这就是"当时价"重算的核心契约。
func TestCalculator_CalculateByPriceID_UsesCachedSnapshot(t *testing.T) {
	calc := NewCalculator()
	old := &domain.ModelPrice{ID: 10, ModelID: "claude-sonnet-4-5", InputPriceMicro: 3_000_000}
	current := &domain.ModelPrice{ID: 20, ModelID: "claude-sonnet-4-5", InputPriceMicro: 6_000_000}

	calc.LoadFromDatabase([]*domain.ModelPrice{current})
	// 注入历史反查:让 ID=10 能被解析出来。
	calc.SetHistoricalLookup(func(id uint64) (*domain.ModelPrice, error) {
		if id == 10 {
			return old, nil
		}
		return nil, nil
	})

	metrics := &usage.Metrics{InputTokens: 1_000_000}
	res, ok, err := calc.CalculateByPriceID(10, metrics, DefaultMultiplier)
	if err != nil {
		t.Fatalf("CalculateByPriceID(10) unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("CalculateByPriceID(10) should succeed via historical lookup")
	}
	if res.ModelPriceID != 10 {
		t.Errorf("res.ModelPriceID = %d, want 10 (历史快照,不被新价 ID 覆盖)", res.ModelPriceID)
	}
	// 按模型名走当前价
	byName := calc.Calculate("claude-sonnet-4-5", metrics, DefaultMultiplier)
	if res.Cost == byName.Cost {
		t.Errorf("by-ID cost (snapshot $3) == by-name cost (current $6) = %d; 二者应分歧", res.Cost)
	}
	if res.Cost*2 != byName.Cost {
		t.Errorf("by-ID cost should be half of by-name (snapshot=$3, current=$6); got %d vs %d", res.Cost, byName.Cost)
	}
}

// TestCalculator_CalculateByPriceID_LazyLoadCaches 验证未命中缓存时通过
// historicalLookup 加载,且只回 DB 一次(后续命中走内存缓存)。
func TestCalculator_CalculateByPriceID_LazyLoadCaches(t *testing.T) {
	calc := NewCalculator()
	calls := 0
	calc.SetHistoricalLookup(func(id uint64) (*domain.ModelPrice, error) {
		calls++
		return &domain.ModelPrice{ID: id, ModelID: "claude-sonnet-4-5", InputPriceMicro: 3_000_000}, nil
	})

	metrics := &usage.Metrics{InputTokens: 1_000_000}
	for i := 0; i < 5; i++ {
		_, ok, err := calc.CalculateByPriceID(42, metrics, DefaultMultiplier)
		if err != nil {
			t.Fatalf("CalculateByPriceID(42) unexpected error on iter=%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("CalculateByPriceID(42) should return ok=true on iter=%d", i)
		}
	}
	if calls != 1 {
		t.Errorf("historicalLookup called %d times, want 1 (后续应命中缓存)", calls)
	}
}

// TestCalculator_CalculateByPriceID_NoLookupOrUnknownID 验证 ok=false 路径:
// 没注入 lookup、lookup 返回 nil 或返回 error,都应返回 false。
// 其中 error 情形还要求调用方拿到非 nil 的 err,不得盲目 fallback。
func TestCalculator_CalculateByPriceID_NoLookupOrUnknownID(t *testing.T) {
	metrics := &usage.Metrics{InputTokens: 1_000_000}

	t.Run("ID==0 unconditionally false", func(t *testing.T) {
		calc := NewCalculator()
		_, ok, err := calc.CalculateByPriceID(0, metrics, DefaultMultiplier)
		if err != nil {
			t.Fatalf("CalculateByPriceID(0) unexpected error: %v", err)
		}
		if ok {
			t.Error("CalculateByPriceID(0) should return ok=false")
		}
	})

	t.Run("no lookup configured", func(t *testing.T) {
		calc := NewCalculator()
		_, ok, err := calc.CalculateByPriceID(99, metrics, DefaultMultiplier)
		if err != nil {
			t.Fatalf("CalculateByPriceID without lookup unexpected error: %v", err)
		}
		if ok {
			t.Error("CalculateByPriceID without lookup should return ok=false")
		}
	})

	t.Run("lookup returns nil", func(t *testing.T) {
		calc := NewCalculator()
		calc.SetHistoricalLookup(func(id uint64) (*domain.ModelPrice, error) {
			return nil, nil
		})
		_, ok, err := calc.CalculateByPriceID(99, metrics, DefaultMultiplier)
		if err != nil {
			t.Fatalf("CalculateByPriceID with nil-returning lookup unexpected error: %v", err)
		}
		if ok {
			t.Error("CalculateByPriceID with nil-returning lookup should return ok=false")
		}
	})

	t.Run("lookup returns error", func(t *testing.T) {
		calc := NewCalculator()
		lookupErr := fmt.Errorf("db connection timeout")
		calc.SetHistoricalLookup(func(id uint64) (*domain.ModelPrice, error) {
			return nil, lookupErr
		})
		_, ok, err := calc.CalculateByPriceID(99, metrics, DefaultMultiplier)
		if ok {
			t.Error("CalculateByPriceID with erroring lookup should return ok=false")
		}
		if err == nil {
			t.Error("CalculateByPriceID with erroring lookup should return non-nil error")
		}
	})
}
