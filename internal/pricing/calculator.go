package pricing

import (
	"log"
	"sync"

	"github.com/awsl-project/maxx/internal/usage"
)

// Calculator 成本计算器
type Calculator struct {
	priceTable *PriceTable
	mu         sync.RWMutex
}

// 全局计算器实例
var (
	globalCalculator *Calculator
	calculatorOnce   sync.Once
)

// GlobalCalculator 返回全局计算器实例
func GlobalCalculator() *Calculator {
	calculatorOnce.Do(func() {
		globalCalculator = NewCalculator(DefaultPriceTable())
	})
	return globalCalculator
}

// NewCalculator 创建新的计算器
func NewCalculator(pt *PriceTable) *Calculator {
	return &Calculator{
		priceTable: pt,
	}
}

// Calculate 计算成本，返回纳美元 (1 USD = 1,000,000,000 nanoUSD)
// model: 模型名称
// metrics: token使用指标
// 如果模型未找到，返回0并记录警告日志
func (c *Calculator) Calculate(model string, metrics *usage.Metrics) uint64 {
	if metrics == nil {
		return 0
	}

	c.mu.RLock()
	pricing := c.priceTable.Get(model)
	c.mu.RUnlock()

	if pricing == nil {
		log.Printf("[Pricing] Unknown model: %s, cost will be 0", model)
		return 0
	}

	return c.CalculateWithPricing(pricing, metrics)
}

// CalculateWithPricing 使用指定价格计算成本（纯整数运算）
// 返回: 纳美元成本 (nanoUSD)
func (c *Calculator) CalculateWithPricing(pricing *ModelPricing, metrics *usage.Metrics) uint64 {
	if pricing == nil || metrics == nil {
		return 0
	}

	var totalCost uint64

	// 1. 输入成本
	if metrics.InputTokens > 0 {
		if pricing.Has1MContext {
			inputNum, inputDenom := pricing.GetInputPremiumFraction()
			totalCost += CalculateTieredCost(
				metrics.InputTokens,
				pricing.InputPriceMicro,
				inputNum, inputDenom,
				pricing.GetContext1MThreshold(),
			)
		} else {
			totalCost += CalculateLinearCost(metrics.InputTokens, pricing.InputPriceMicro)
		}
	}

	// 2. 输出成本
	if metrics.OutputTokens > 0 {
		if pricing.Has1MContext {
			outputNum, outputDenom := pricing.GetOutputPremiumFraction()
			totalCost += CalculateTieredCost(
				metrics.OutputTokens,
				pricing.OutputPriceMicro,
				outputNum, outputDenom,
				pricing.GetContext1MThreshold(),
			)
		} else {
			totalCost += CalculateLinearCost(metrics.OutputTokens, pricing.OutputPriceMicro)
		}
	}

	// 3. 缓存读取成本（使用 input 价格的 10%）
	if metrics.CacheReadCount > 0 {
		totalCost += CalculateLinearCost(
			metrics.CacheReadCount,
			pricing.GetEffectiveCacheReadPriceMicro(),
		)
	}

	// 4. 5分钟缓存写入成本（使用 input 价格的 125%）
	if metrics.Cache5mCreationCount > 0 {
		totalCost += CalculateLinearCost(
			metrics.Cache5mCreationCount,
			pricing.GetEffectiveCache5mWritePriceMicro(),
		)
	}

	// 5. 1小时缓存写入成本（使用 input 价格的 200%）
	if metrics.Cache1hCreationCount > 0 {
		totalCost += CalculateLinearCost(
			metrics.Cache1hCreationCount,
			pricing.GetEffectiveCache1hWritePriceMicro(),
		)
	}

	// 6. Fallback: 如果没有 5m/1h 细分但有总缓存写入数
	if metrics.Cache5mCreationCount == 0 && metrics.Cache1hCreationCount == 0 && metrics.CacheCreationCount > 0 {
		totalCost += CalculateLinearCost(
			metrics.CacheCreationCount,
			pricing.GetEffectiveCache5mWritePriceMicro(), // 使用 5m 价格作为默认
		)
	}

	return totalCost
}

// SetPriceTable 更新价格表
func (c *Calculator) SetPriceTable(pt *PriceTable) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.priceTable = pt
}

// GetPricing 获取模型价格
func (c *Calculator) GetPricing(model string) *ModelPricing {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.priceTable.Get(model)
}
