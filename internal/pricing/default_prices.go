package pricing

import "sync"

var (
	defaultTable *PriceTable
	defaultOnce  sync.Once
)

// DefaultPriceTable 返回默认价格表（单例）
func DefaultPriceTable() *PriceTable {
	defaultOnce.Do(func() {
		defaultTable = initDefaultPrices()
	})
	return defaultTable
}

// initDefaultPrices 初始化默认价格
func initDefaultPrices() *PriceTable {
	pt := NewPriceTable("2025.01")

	// ========== Claude 4.5 系列 ==========
	// Claude Sonnet 4.5: input=$3, output=$15, cache_creation=$3.75, cache_read=$0.30
	pt.Set(&ModelPricing{
		ModelID:                "claude-sonnet-4-5",
		InputPriceMicro:        3_000_000,  // $3.00/M
		OutputPriceMicro:       15_000_000, // $15.00/M
		Cache5mWritePriceMicro: 3_750_000,  // $3.75/M
		Cache1hWritePriceMicro: 3_750_000,  // $3.75/M
		CacheReadPriceMicro:    300_000,    // $0.30/M
		Has1MContext:           true,
	})

	// Claude Opus 4.5: input=$5, output=$25, cache_creation=$6.25, cache_read=$0.50
	pt.Set(&ModelPricing{
		ModelID:                "claude-opus-4-5",
		InputPriceMicro:        5_000_000,  // $5.00/M
		OutputPriceMicro:       25_000_000, // $25.00/M
		Cache5mWritePriceMicro: 6_250_000,  // $6.25/M
		Cache1hWritePriceMicro: 6_250_000,  // $6.25/M
		CacheReadPriceMicro:    500_000,    // $0.50/M
	})

	// Claude Haiku 4.5: input=$1, output=$5, cache_creation=$1.25, cache_read=$0.10
	pt.Set(&ModelPricing{
		ModelID:                "claude-haiku-4-5",
		InputPriceMicro:        1_000_000, // $1.00/M
		OutputPriceMicro:       5_000_000, // $5.00/M
		Cache5mWritePriceMicro: 1_250_000, // $1.25/M
		Cache1hWritePriceMicro: 1_250_000, // $1.25/M
		CacheReadPriceMicro:    100_000,   // $0.10/M
	})

	// Claude 4.5 系列 - 带版本号别名
	pt.Set(&ModelPricing{
		ModelID:                "claude-sonnet-4-5-20250514",
		InputPriceMicro:        3_000_000,
		OutputPriceMicro:       15_000_000,
		Cache5mWritePriceMicro: 3_750_000,
		Cache1hWritePriceMicro: 3_750_000,
		CacheReadPriceMicro:    300_000,
		Has1MContext:           true,
	})
	pt.Set(&ModelPricing{
		ModelID:                "claude-sonnet-4-5-20250929",
		InputPriceMicro:        3_000_000,
		OutputPriceMicro:       15_000_000,
		Cache5mWritePriceMicro: 3_750_000,
		Cache1hWritePriceMicro: 3_750_000,
		CacheReadPriceMicro:    300_000,
		Has1MContext:           true,
	})
	pt.Set(&ModelPricing{
		ModelID:                "claude-opus-4-5-20251101",
		InputPriceMicro:        5_000_000,
		OutputPriceMicro:       25_000_000,
		Cache5mWritePriceMicro: 6_250_000,
		Cache1hWritePriceMicro: 6_250_000,
		CacheReadPriceMicro:    500_000,
	})

	// ========== Claude 4 系列 ==========
	// Claude Sonnet 4: input=$3, output=$15, cache_creation=$3.75, cache_read=$0.30
	pt.Set(&ModelPricing{
		ModelID:                "claude-sonnet-4",
		InputPriceMicro:        3_000_000,  // $3.00/M
		OutputPriceMicro:       15_000_000, // $15.00/M
		Cache5mWritePriceMicro: 3_750_000,  // $3.75/M
		Cache1hWritePriceMicro: 3_750_000,  // $3.75/M
		CacheReadPriceMicro:    300_000,    // $0.30/M
	})

	// Claude Opus 4: input=$15, output=$75, cache_creation=$18.75, cache_read=$1.50
	pt.Set(&ModelPricing{
		ModelID:                "claude-opus-4",
		InputPriceMicro:        15_000_000, // $15.00/M
		OutputPriceMicro:       75_000_000, // $75.00/M
		Cache5mWritePriceMicro: 18_750_000, // $18.75/M
		Cache1hWritePriceMicro: 18_750_000, // $18.75/M
		CacheReadPriceMicro:    1_500_000,  // $1.50/M
	})

	// Claude 4 系列 - 带版本号别名
	pt.Set(&ModelPricing{
		ModelID:                "claude-sonnet-4-20250514",
		InputPriceMicro:        3_000_000,
		OutputPriceMicro:       15_000_000,
		Cache5mWritePriceMicro: 3_750_000,
		Cache1hWritePriceMicro: 3_750_000,
		CacheReadPriceMicro:    300_000,
	})

	// ========== Claude 3.7 系列 ==========
	// Claude 3.7 Sonnet: input=$3, output=$15
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-7-sonnet",
		InputPriceMicro:        3_000_000,  // $3.00/M
		OutputPriceMicro:       15_000_000, // $15.00/M
		Cache5mWritePriceMicro: 3_750_000,  // $3.75/M
		Cache1hWritePriceMicro: 3_750_000,  // $3.75/M
		CacheReadPriceMicro:    300_000,    // $0.30/M
	})

	// ========== Claude 3.5 系列 ==========
	// Claude 3.5 Sonnet: input=$3, output=$15
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-5-sonnet",
		InputPriceMicro:        3_000_000,  // $3.00/M
		OutputPriceMicro:       15_000_000, // $15.00/M
		Cache5mWritePriceMicro: 3_750_000,  // $3.75/M
		Cache1hWritePriceMicro: 3_750_000,  // $3.75/M
		CacheReadPriceMicro:    300_000,    // $0.30/M
	})

	// Claude 3.5 Haiku: input=$0.80, output=$4
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-5-haiku",
		InputPriceMicro:        800_000,   // $0.80/M
		OutputPriceMicro:       4_000_000, // $4.00/M
		Cache5mWritePriceMicro: 1_000_000, // $1.00/M
		Cache1hWritePriceMicro: 1_000_000, // $1.00/M
		CacheReadPriceMicro:    80_000,    // $0.08/M
	})

	// Claude 3.5 系列 - 带版本号别名
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-5-sonnet-20241022",
		InputPriceMicro:        3_000_000,
		OutputPriceMicro:       15_000_000,
		Cache5mWritePriceMicro: 3_750_000,
		Cache1hWritePriceMicro: 3_750_000,
		CacheReadPriceMicro:    300_000,
	})
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-5-sonnet-20240620",
		InputPriceMicro:        3_000_000,
		OutputPriceMicro:       15_000_000,
		Cache5mWritePriceMicro: 3_750_000,
		Cache1hWritePriceMicro: 3_750_000,
		CacheReadPriceMicro:    300_000,
	})
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-5-haiku-20241022",
		InputPriceMicro:        800_000,
		OutputPriceMicro:       4_000_000,
		Cache5mWritePriceMicro: 1_000_000,
		Cache1hWritePriceMicro: 1_000_000,
		CacheReadPriceMicro:    80_000,
	})

	// ========== Claude 3 系列 ==========
	// Claude 3 Opus: input=$15, output=$75
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-opus",
		InputPriceMicro:        15_000_000, // $15.00/M
		OutputPriceMicro:       75_000_000, // $75.00/M
		Cache5mWritePriceMicro: 18_750_000, // $18.75/M
		Cache1hWritePriceMicro: 18_750_000, // $18.75/M
		CacheReadPriceMicro:    1_500_000,  // $1.50/M
	})

	// Claude 3 Sonnet: input=$3, output=$15
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-sonnet",
		InputPriceMicro:        3_000_000,  // $3.00/M
		OutputPriceMicro:       15_000_000, // $15.00/M
		Cache5mWritePriceMicro: 3_750_000,  // $3.75/M
		Cache1hWritePriceMicro: 3_750_000,  // $3.75/M
		CacheReadPriceMicro:    300_000,    // $0.30/M
	})

	// Claude 3 Haiku: input=$0.25, output=$1.25
	pt.Set(&ModelPricing{
		ModelID:                "claude-3-haiku",
		InputPriceMicro:        250_000,   // $0.25/M
		OutputPriceMicro:       1_250_000, // $1.25/M
		Cache5mWritePriceMicro: 312_500,   // $0.3125/M
		Cache1hWritePriceMicro: 312_500,   // $0.3125/M
		CacheReadPriceMicro:    30_000,    // $0.03/M
	})

	// ========== GPT 5.x 系列 ==========
	// gpt-5.1: input=$1.25, cache_read=$0.125, output=$10
	pt.Set(&ModelPricing{
		ModelID:             "gpt-5.1",
		InputPriceMicro:     1_250_000,  // $1.25/M
		OutputPriceMicro:    10_000_000, // $10.00/M
		CacheReadPriceMicro: 125_000,    // $0.125/M
	})

	// gpt-5.1-codex: input=$1.25, cache_read=$0.125, output=$10
	pt.Set(&ModelPricing{
		ModelID:             "gpt-5.1-codex",
		InputPriceMicro:     1_250_000,  // $1.25/M
		OutputPriceMicro:    10_000_000, // $10.00/M
		CacheReadPriceMicro: 125_000,    // $0.125/M
	})

	// gpt-5.1-codex-max: input=$1.25, cache_read=$0.125, output=$10
	pt.Set(&ModelPricing{
		ModelID:             "gpt-5.1-codex-max",
		InputPriceMicro:     1_250_000,  // $1.25/M
		OutputPriceMicro:    10_000_000, // $10.00/M
		CacheReadPriceMicro: 125_000,    // $0.125/M
	})

	// gpt-5.2: input=$1.75, cache_read=$0.175, output=$14
	pt.Set(&ModelPricing{
		ModelID:             "gpt-5.2",
		InputPriceMicro:     1_750_000,  // $1.75/M
		OutputPriceMicro:    14_000_000, // $14.00/M
		CacheReadPriceMicro: 175_000,    // $0.175/M
	})

	// gpt-5.2-codex: input=$1.75, cache_read=$0.175, output=$14
	pt.Set(&ModelPricing{
		ModelID:             "gpt-5.2-codex",
		InputPriceMicro:     1_750_000,  // $1.75/M
		OutputPriceMicro:    14_000_000, // $14.00/M
		CacheReadPriceMicro: 175_000,    // $0.175/M
	})

	// ========== GPT-4o 系列 ==========
	// gpt-4o: input=$2.50, output=$10, cache_read=$1.25
	pt.Set(&ModelPricing{
		ModelID:             "gpt-4o",
		InputPriceMicro:     2_500_000,  // $2.50/M
		OutputPriceMicro:    10_000_000, // $10.00/M
		CacheReadPriceMicro: 1_250_000,  // $1.25/M
	})

	// gpt-4o-mini: input=$0.15, output=$0.60, cache_read=$0.075
	pt.Set(&ModelPricing{
		ModelID:             "gpt-4o-mini",
		InputPriceMicro:     150_000, // $0.15/M
		OutputPriceMicro:    600_000, // $0.60/M
		CacheReadPriceMicro: 75_000,  // $0.075/M
	})

	// ========== GPT-4.1 系列 ==========
	// gpt-4.1: input=$2, output=$8, cache_read=$0.50
	pt.Set(&ModelPricing{
		ModelID:             "gpt-4.1",
		InputPriceMicro:     2_000_000, // $2.00/M
		OutputPriceMicro:    8_000_000, // $8.00/M
		CacheReadPriceMicro: 500_000,   // $0.50/M
	})

	// gpt-4.1-mini: input=$0.40, output=$1.60, cache_read=$0.10
	pt.Set(&ModelPricing{
		ModelID:             "gpt-4.1-mini",
		InputPriceMicro:     400_000,   // $0.40/M
		OutputPriceMicro:    1_600_000, // $1.60/M
		CacheReadPriceMicro: 100_000,   // $0.10/M
	})

	// gpt-4.1-nano: input=$0.10, output=$0.40, cache_read=$0.025
	pt.Set(&ModelPricing{
		ModelID:             "gpt-4.1-nano",
		InputPriceMicro:     100_000, // $0.10/M
		OutputPriceMicro:    400_000, // $0.40/M
		CacheReadPriceMicro: 25_000,  // $0.025/M
	})

	// ========== OpenAI o 系列 ==========
	// o1: input=$15, output=$60, cache_read=$7.50
	pt.Set(&ModelPricing{
		ModelID:             "o1",
		InputPriceMicro:     15_000_000, // $15.00/M
		OutputPriceMicro:    60_000_000, // $60.00/M
		CacheReadPriceMicro: 7_500_000,  // $7.50/M
	})

	// o1-mini: input=$1.10, output=$4.40, cache_read=$0.55
	pt.Set(&ModelPricing{
		ModelID:             "o1-mini",
		InputPriceMicro:     1_100_000, // $1.10/M
		OutputPriceMicro:    4_400_000, // $4.40/M
		CacheReadPriceMicro: 550_000,   // $0.55/M
	})

	// o1-pro: input=$150, output=$600, cache_read=$75
	pt.Set(&ModelPricing{
		ModelID:             "o1-pro",
		InputPriceMicro:     150_000_000, // $150.00/M
		OutputPriceMicro:    600_000_000, // $600.00/M
		CacheReadPriceMicro: 75_000_000,  // $75.00/M
	})

	// o3: input=$2, output=$8, cache_read=$1
	pt.Set(&ModelPricing{
		ModelID:             "o3",
		InputPriceMicro:     2_000_000, // $2.00/M
		OutputPriceMicro:    8_000_000, // $8.00/M
		CacheReadPriceMicro: 1_000_000, // $1.00/M
	})

	// o3-mini: input=$1.10, output=$4.40, cache_read=$0.55
	pt.Set(&ModelPricing{
		ModelID:             "o3-mini",
		InputPriceMicro:     1_100_000, // $1.10/M
		OutputPriceMicro:    4_400_000, // $4.40/M
		CacheReadPriceMicro: 550_000,   // $0.55/M
	})

	// o4-mini: input=$1.10, output=$4.40, cache_read=$0.55
	pt.Set(&ModelPricing{
		ModelID:             "o4-mini",
		InputPriceMicro:     1_100_000, // $1.10/M
		OutputPriceMicro:    4_400_000, // $4.40/M
		CacheReadPriceMicro: 550_000,   // $0.55/M
	})

	// ========== Gemini 3.x 系列 ==========
	// gemini-3-pro-preview: input=$2, cache_read=$0.20, output=$12
	pt.Set(&ModelPricing{
		ModelID:             "gemini-3-pro-preview",
		InputPriceMicro:     2_000_000,  // $2.00/M
		OutputPriceMicro:    12_000_000, // $12.00/M
		CacheReadPriceMicro: 200_000,    // $0.20/M
	})

	// gemini-3-flash-preview: input=$0.50, cache_read=$0.05, output=$3
	pt.Set(&ModelPricing{
		ModelID:             "gemini-3-flash-preview",
		InputPriceMicro:     500_000,   // $0.50/M
		OutputPriceMicro:    3_000_000, // $3.00/M
		CacheReadPriceMicro: 50_000,    // $0.05/M
	})

	// ========== Gemini 2.5 系列 ==========
	// gemini-2.5-pro: input=$1.25, cache_read=$0.3125, output=$10
	pt.Set(&ModelPricing{
		ModelID:             "gemini-2.5-pro",
		InputPriceMicro:     1_250_000,  // $1.25/M
		OutputPriceMicro:    10_000_000, // $10.00/M
		CacheReadPriceMicro: 312_500,    // $0.3125/M
	})

	// gemini-2.5-flash: input=$0.15, cache_read=$0.0375, output=$0.60
	pt.Set(&ModelPricing{
		ModelID:             "gemini-2.5-flash",
		InputPriceMicro:     150_000, // $0.15/M
		OutputPriceMicro:    600_000, // $0.60/M
		CacheReadPriceMicro: 37_500,  // $0.0375/M
	})

	// gemini-2.5-flash-lite: input=$0.10, cache_read=$0.025, output=$0.40
	pt.Set(&ModelPricing{
		ModelID:             "gemini-2.5-flash-lite",
		InputPriceMicro:     100_000, // $0.10/M
		OutputPriceMicro:    400_000, // $0.40/M
		CacheReadPriceMicro: 25_000,  // $0.025/M
	})

	// ========== Gemini 2.0 系列 ==========
	// gemini-2.0-flash: input=$0.10, cache_read=$0.025, output=$0.40
	pt.Set(&ModelPricing{
		ModelID:             "gemini-2.0-flash",
		InputPriceMicro:     100_000, // $0.10/M
		OutputPriceMicro:    400_000, // $0.40/M
		CacheReadPriceMicro: 25_000,  // $0.025/M
	})

	// gemini-2.0-flash-lite: input=$0.075, cache_read=$0.01875, output=$0.30
	pt.Set(&ModelPricing{
		ModelID:             "gemini-2.0-flash-lite",
		InputPriceMicro:     75_000,  // $0.075/M
		OutputPriceMicro:    300_000, // $0.30/M
		CacheReadPriceMicro: 18_750,  // $0.01875/M
	})

	// ========== Gemini 1.5 系列 ==========
	// gemini-1.5-pro: input=$1.25, cache_read=$0.3125, output=$5
	pt.Set(&ModelPricing{
		ModelID:             "gemini-1.5-pro",
		InputPriceMicro:     1_250_000, // $1.25/M
		OutputPriceMicro:    5_000_000, // $5.00/M
		CacheReadPriceMicro: 312_500,   // $0.3125/M
	})

	// gemini-1.5-flash: input=$0.075, cache_read=$0.01875, output=$0.30
	pt.Set(&ModelPricing{
		ModelID:             "gemini-1.5-flash",
		InputPriceMicro:     75_000,  // $0.075/M
		OutputPriceMicro:    300_000, // $0.30/M
		CacheReadPriceMicro: 18_750,  // $0.01875/M
	})

	// gemini-1.5-flash-8b: input=$0.0375, cache_read=$0.01, output=$0.15
	pt.Set(&ModelPricing{
		ModelID:             "gemini-1.5-flash-8b",
		InputPriceMicro:     37_500,  // $0.0375/M
		OutputPriceMicro:    150_000, // $0.15/M
		CacheReadPriceMicro: 10_000,  // $0.01/M
	})

	// ========== DeepSeek 系列 ==========
	// deepseek-chat (V3): input=$0.27, cache_read=$0.07, output=$1.10
	pt.Set(&ModelPricing{
		ModelID:             "deepseek-chat",
		InputPriceMicro:     270_000,   // $0.27/M
		OutputPriceMicro:    1_100_000, // $1.10/M
		CacheReadPriceMicro: 70_000,    // $0.07/M
	})

	// deepseek-reasoner (R1): input=$0.55, cache_read=$0.14, output=$2.19
	pt.Set(&ModelPricing{
		ModelID:             "deepseek-reasoner",
		InputPriceMicro:     550_000,   // $0.55/M
		OutputPriceMicro:    2_190_000, // $2.19/M
		CacheReadPriceMicro: 140_000,   // $0.14/M
	})

	return pt
}
