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

	// Claude 4 系列
	pt.Set(&ModelPricing{
		ModelID:      "claude-sonnet-4-5",
		InputPrice:   3.0,
		OutputPrice:  15.0,
		Has1MContext: true,
	})
	pt.Set(&ModelPricing{
		ModelID:      "claude-sonnet-4",
		InputPrice:   3.0,
		OutputPrice:  15.0,
		Has1MContext: true,
	})
	pt.Set(&ModelPricing{
		ModelID:     "claude-opus-4",
		InputPrice:  15.0,
		OutputPrice: 75.0,
	})

	// Claude 3.5 系列
	pt.Set(&ModelPricing{
		ModelID:     "claude-3-5-sonnet",
		InputPrice:  3.0,
		OutputPrice: 15.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "claude-3-5-haiku",
		InputPrice:  0.80,
		OutputPrice: 4.0,
	})

	// Claude 3 系列
	pt.Set(&ModelPricing{
		ModelID:     "claude-3-opus",
		InputPrice:  15.0,
		OutputPrice: 75.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "claude-3-sonnet",
		InputPrice:  3.0,
		OutputPrice: 15.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "claude-3-haiku",
		InputPrice:  0.25,
		OutputPrice: 1.25,
	})

	// Gemini 系列
	pt.Set(&ModelPricing{
		ModelID:     "gemini-2.5-pro",
		InputPrice:  1.25,
		OutputPrice: 10.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "gemini-2.5-flash",
		InputPrice:  0.15,
		OutputPrice: 0.60,
	})
	pt.Set(&ModelPricing{
		ModelID:     "gemini-2.0-flash",
		InputPrice:  0.10,
		OutputPrice: 0.40,
	})
	pt.Set(&ModelPricing{
		ModelID:     "gemini-1.5-pro",
		InputPrice:  1.25,
		OutputPrice: 5.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "gemini-1.5-flash",
		InputPrice:  0.075,
		OutputPrice: 0.30,
	})

	// OpenAI GPT 系列
	pt.Set(&ModelPricing{
		ModelID:     "gpt-4o",
		InputPrice:  2.50,
		OutputPrice: 10.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "gpt-4o-mini",
		InputPrice:  0.15,
		OutputPrice: 0.60,
	})
	pt.Set(&ModelPricing{
		ModelID:     "gpt-4-turbo",
		InputPrice:  10.0,
		OutputPrice: 30.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "gpt-4",
		InputPrice:  30.0,
		OutputPrice: 60.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "gpt-3.5-turbo",
		InputPrice:  0.50,
		OutputPrice: 1.50,
	})

	// OpenAI o 系列
	pt.Set(&ModelPricing{
		ModelID:     "o1",
		InputPrice:  15.0,
		OutputPrice: 60.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "o1-mini",
		InputPrice:  3.0,
		OutputPrice: 12.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "o1-pro",
		InputPrice:  150.0,
		OutputPrice: 600.0,
	})
	pt.Set(&ModelPricing{
		ModelID:     "o3-mini",
		InputPrice:  1.10,
		OutputPrice: 4.40,
	})

	return pt
}
