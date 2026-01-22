package pricing

import (
	"log"
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
)

// CostResult 成本计算结果
type CostResult struct {
	Cost         uint64 // 成本（纳美元）
	ModelPriceID uint64 // 使用的价格记录ID（0 表示使用内置价格表）
	Multiplier   uint64 // 倍率（10000=1倍）
}

// Calculator 成本计算器
type Calculator struct {
	priceTable *PriceTable

	// 数据库价格缓存
	modelPriceCache map[string]*domain.ModelPrice // key: modelID
	modelPriceByID  map[uint64]*domain.ModelPrice // key: price ID
	useDBPrices     bool                          // 是否使用数据库价格

	mu sync.RWMutex
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
		priceTable:      pt,
		modelPriceCache: make(map[string]*domain.ModelPrice),
		modelPriceByID:  make(map[uint64]*domain.ModelPrice),
		useDBPrices:     false,
	}
}

// LoadFromDatabase 从数据库加载当前价格
func (c *Calculator) LoadFromDatabase(prices []*domain.ModelPrice) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.modelPriceCache = make(map[string]*domain.ModelPrice, len(prices))
	c.modelPriceByID = make(map[uint64]*domain.ModelPrice, len(prices))

	for _, p := range prices {
		c.modelPriceCache[p.ModelID] = p
		c.modelPriceByID[p.ID] = p
	}
	c.useDBPrices = len(prices) > 0
	log.Printf("[Pricing] Loaded %d model prices from database", len(prices))
}

// GetModelPrice 获取模型价格（支持前缀匹配），返回价格记录
func (c *Calculator) GetModelPrice(model string) *domain.ModelPrice {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.useDBPrices {
		return nil
	}

	// 精确匹配
	if p, ok := c.modelPriceCache[model]; ok {
		return p
	}

	// 前缀匹配：找最长匹配
	var bestMatch *domain.ModelPrice
	var bestLen int

	for key, price := range c.modelPriceCache {
		if len(key) > 0 && len(model) >= len(key) && model[:len(key)] == key {
			if len(key) > bestLen {
				bestMatch = price
				bestLen = len(key)
			}
		}
	}

	return bestMatch
}

// GetModelPriceByID 根据ID获取价格记录
func (c *Calculator) GetModelPriceByID(id uint64) *domain.ModelPrice {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.modelPriceByID[id]
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

// CalculateWithResult 计算成本，返回完整结果（包含 model_price_id 和 multiplier）
// model: 模型名称
// metrics: token使用指标
// multiplier: 倍率（10000=1倍），0 表示使用默认值 10000
func (c *Calculator) CalculateWithResult(model string, metrics *usage.Metrics, multiplier uint64) CostResult {
	if metrics == nil {
		return CostResult{Cost: 0, ModelPriceID: 0, Multiplier: 10000}
	}

	if multiplier == 0 {
		multiplier = 10000
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// 优先使用数据库价格
	if c.useDBPrices {
		mp := c.getModelPriceLocked(model)
		if mp != nil {
			cost := c.calculateWithModelPrice(mp, metrics)
			// 应用倍率: cost * multiplier / 10000
			if multiplier != 10000 {
				cost = cost * multiplier / 10000
			}
			return CostResult{
				Cost:         cost,
				ModelPriceID: mp.ID,
				Multiplier:   multiplier,
			}
		}
	}

	// 回退到内置价格表
	pricing := c.priceTable.Get(model)
	if pricing == nil {
		log.Printf("[Pricing] Unknown model: %s, cost will be 0", model)
		return CostResult{Cost: 0, ModelPriceID: 0, Multiplier: multiplier}
	}

	cost := c.CalculateWithPricing(pricing, metrics)
	// 应用倍率
	if multiplier != 10000 {
		cost = cost * multiplier / 10000
	}
	return CostResult{
		Cost:         cost,
		ModelPriceID: 0, // 使用内置价格表
		Multiplier:   multiplier,
	}
}

// getModelPriceLocked 获取模型价格（需要持有读锁）
func (c *Calculator) getModelPriceLocked(model string) *domain.ModelPrice {
	// 精确匹配
	if p, ok := c.modelPriceCache[model]; ok {
		return p
	}

	// 前缀匹配：找最长匹配
	var bestMatch *domain.ModelPrice
	var bestLen int

	for key, price := range c.modelPriceCache {
		if len(key) > 0 && len(model) >= len(key) && model[:len(key)] == key {
			if len(key) > bestLen {
				bestMatch = price
				bestLen = len(key)
			}
		}
	}

	return bestMatch
}

// calculateWithModelPrice 使用数据库价格计算成本
func (c *Calculator) calculateWithModelPrice(mp *domain.ModelPrice, metrics *usage.Metrics) uint64 {
	if mp == nil || metrics == nil {
		return 0
	}

	var totalCost uint64

	// 获取有效的缓存价格
	cacheReadPrice := mp.CacheReadPriceMicro
	if cacheReadPrice == 0 {
		cacheReadPrice = mp.InputPriceMicro / 10
	}
	cache5mWritePrice := mp.Cache5mWritePriceMicro
	if cache5mWritePrice == 0 {
		cache5mWritePrice = mp.InputPriceMicro * 5 / 4
	}
	cache1hWritePrice := mp.Cache1hWritePriceMicro
	if cache1hWritePrice == 0 {
		cache1hWritePrice = mp.InputPriceMicro * 2
	}

	// 获取 1M context 参数
	threshold := mp.Context1MThreshold
	if threshold == 0 {
		threshold = 200000
	}
	inputNum := mp.InputPremiumNum
	if inputNum == 0 {
		inputNum = 2
	}
	inputDenom := mp.InputPremiumDenom
	if inputDenom == 0 {
		inputDenom = 1
	}
	outputNum := mp.OutputPremiumNum
	if outputNum == 0 {
		outputNum = 3
	}
	outputDenom := mp.OutputPremiumDenom
	if outputDenom == 0 {
		outputDenom = 2
	}

	// 1. 输入成本
	if metrics.InputTokens > 0 {
		if mp.Has1MContext {
			totalCost += CalculateTieredCost(
				metrics.InputTokens,
				mp.InputPriceMicro,
				inputNum, inputDenom,
				threshold,
			)
		} else {
			totalCost += CalculateLinearCost(metrics.InputTokens, mp.InputPriceMicro)
		}
	}

	// 2. 输出成本
	if metrics.OutputTokens > 0 {
		if mp.Has1MContext {
			totalCost += CalculateTieredCost(
				metrics.OutputTokens,
				mp.OutputPriceMicro,
				outputNum, outputDenom,
				threshold,
			)
		} else {
			totalCost += CalculateLinearCost(metrics.OutputTokens, mp.OutputPriceMicro)
		}
	}

	// 3. 缓存读取成本
	if metrics.CacheReadCount > 0 {
		totalCost += CalculateLinearCost(metrics.CacheReadCount, cacheReadPrice)
	}

	// 4. 5分钟缓存写入成本
	if metrics.Cache5mCreationCount > 0 {
		totalCost += CalculateLinearCost(metrics.Cache5mCreationCount, cache5mWritePrice)
	}

	// 5. 1小时缓存写入成本
	if metrics.Cache1hCreationCount > 0 {
		totalCost += CalculateLinearCost(metrics.Cache1hCreationCount, cache1hWritePrice)
	}

	// 6. Fallback: 如果没有 5m/1h 细分但有总缓存写入数
	if metrics.Cache5mCreationCount == 0 && metrics.Cache1hCreationCount == 0 && metrics.CacheCreationCount > 0 {
		totalCost += CalculateLinearCost(metrics.CacheCreationCount, cache5mWritePrice)
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

// IsUsingDBPrices 返回是否使用数据库价格
func (c *Calculator) IsUsingDBPrices() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.useDBPrices
}
