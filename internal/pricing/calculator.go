package pricing

import (
	"log"
	"strings"
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
)

// DefaultMultiplier 表示 1× 倍率。倍率单位为万分之一。
const DefaultMultiplier uint64 = 10000

// CostResult 成本计算结果
type CostResult struct {
	Cost         uint64 // 成本（纳美元）
	ModelPriceID uint64 // 使用的价格记录ID（0 表示使用内置默认价表）
	Multiplier   uint64 // 实际应用的倍率（10000=1倍）
}

// HistoricalLookup 通过 DB 反查任意 ModelPriceID(包括已软删的历史快照)。
// 由仓储层注入,用于按 attempt.ModelPriceID 还原当时的价格做"当时价"重算。
type HistoricalLookup func(id uint64) (*domain.ModelPrice, error)

// Calculator 维护 modelID → ModelPrice 映射。
// 启动时载入内置默认价表(ID=0),LoadFromDatabase 会用 DB 记录覆盖同名条目。
// 这样运行期只有一份价格源,不再有“内置 vs DB”分支。
//
// pricesByID 缓存"当前价"行(ID > 0)。需要查历史快照(已软删行)时,
// CalculateByPriceID 通过 historicalLookup 懒加载到 pricesByID,避免重算
// 大批 attempt 时打爆 DB。
type Calculator struct {
	mu               sync.RWMutex
	pricesByKey      map[string]*domain.ModelPrice
	pricesByID       map[uint64]*domain.ModelPrice
	historicalLookup HistoricalLookup
}

var (
	globalCalculator *Calculator
	calculatorOnce   sync.Once
)

// GlobalCalculator 返回全局计算器单例。
func GlobalCalculator() *Calculator {
	calculatorOnce.Do(func() {
		globalCalculator = NewCalculator()
	})
	return globalCalculator
}

// NewCalculator 构造仅含内置默认价的计算器。
func NewCalculator() *Calculator {
	c := &Calculator{
		pricesByKey: make(map[string]*domain.ModelPrice),
		pricesByID:  make(map[uint64]*domain.ModelPrice),
	}
	c.loadBuiltinsLocked()
	return c
}

// loadBuiltinsLocked 用内置默认价表填充 pricesByKey。调用方需独占 c 或在构造期间。
func (c *Calculator) loadBuiltinsLocked() {
	for _, mp := range ConvertToDBPrices(DefaultPriceTable()) {
		// 内置价表的 ID 恒为 0,只按 ModelID 索引。
		c.pricesByKey[mp.ModelID] = mp
	}
}

// LoadFromDatabase 用 DB 价格覆盖内置默认价。
// 同一 ModelID 的 DB 记录会取代内置默认;未在 DB 出现的内置默认仍然可用。
func (c *Calculator) LoadFromDatabase(prices []*domain.ModelPrice) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pricesByKey = make(map[string]*domain.ModelPrice)
	c.pricesByID = make(map[uint64]*domain.ModelPrice)
	c.loadBuiltinsLocked()
	for _, p := range prices {
		c.pricesByKey[p.ModelID] = p
		c.pricesByID[p.ID] = p
	}
	log.Printf("[Pricing] Loaded %d model prices from database", len(prices))
}

// GetModelPrice 按模型名取价格,支持前缀匹配。未命中返回 nil。
func (c *Calculator) GetModelPrice(model string) *domain.ModelPrice {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lookupLocked(model)
}

// GetModelPriceByID 按 DB 记录 ID 取价格(仅看内存缓存,不触发懒加载)。
func (c *Calculator) GetModelPriceByID(id uint64) *domain.ModelPrice {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pricesByID[id]
}

// SetHistoricalLookup 注入历史价反查函数。由仓储装配阶段调用一次,
// 之后 CalculateByPriceID 在缓存未命中时会经此回填历史快照行。
func (c *Calculator) SetHistoricalLookup(fn HistoricalLookup) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.historicalLookup = fn
}

// resolvePriceByID 优先查内存缓存;未命中且配置了 historicalLookup 时
// 回 DB 取并缓存。
//   - 返回 (p, nil)     表示命中(p != nil)或确认不存在(p == nil)
//   - 返回 (nil, err)   表示 DB 查询出错,调用方不得以此为由回退到当前价——
//     否则会用今天的价格悄悄覆盖历史 attempt 的成本
func (c *Calculator) resolvePriceByID(id uint64) (*domain.ModelPrice, error) {
	if id == 0 {
		return nil, nil
	}
	c.mu.RLock()
	if p, ok := c.pricesByID[id]; ok {
		c.mu.RUnlock()
		return p, nil
	}
	lookup := c.historicalLookup
	c.mu.RUnlock()

	if lookup == nil {
		return nil, nil
	}
	p, err := lookup(id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	c.mu.Lock()
	// 双检:并发情况下别人可能已经填好。
	if existing, ok := c.pricesByID[id]; ok {
		c.mu.Unlock()
		return existing, nil
	}
	c.pricesByID[id] = p
	c.mu.Unlock()
	return p, nil
}

// lookupLocked 在 pricesByKey 中查找,先精确再最长前缀。需持有 RLock 或 Lock。
func (c *Calculator) lookupLocked(model string) *domain.ModelPrice {
	if p, ok := c.pricesByKey[model]; ok {
		return p
	}
	var best *domain.ModelPrice
	var bestLen int
	for key, p := range c.pricesByKey {
		if len(key) > bestLen && strings.HasPrefix(model, key) {
			best = p
			bestLen = len(key)
		}
	}
	return best
}

// Calculate 计算成本。multiplier 单位为万分之一,0 视作 DefaultMultiplier(1×)。
// 模型未命中价表时返回零成本(Multiplier 仍带回入参,便于审计)。
func (c *Calculator) Calculate(model string, metrics *usage.Metrics, multiplier uint64) CostResult {
	if multiplier == 0 {
		multiplier = DefaultMultiplier
	}
	if metrics == nil {
		return CostResult{Multiplier: multiplier}
	}

	c.mu.RLock()
	price := c.lookupLocked(model)
	c.mu.RUnlock()

	if price == nil {
		log.Printf("[Pricing] Unknown model: %s, cost will be 0", model)
		return CostResult{Multiplier: multiplier}
	}

	return applyPrice(price, metrics, multiplier)
}

// CalculateByPriceID 按指定的 ModelPriceID 取价格快照计算成本——重算路径
// 用这条:attempt 保存了它写入时的 ModelPriceID,据此还原"当时价" × token
// × multiplier,而不是按今天的当前价回填。
//
// 返回值语义:
//   - (result, true,  nil): 找到价格快照并完成计算
//   - (empty,  false, nil): ID=0 或 DB 中确认不存在(p==nil),调用方可安全
//     回退到按模型名走 Calculate
//   - (empty,  false, err): historicalLookup 返回错误,调用方必须终止本次
//     重算并上报错误,不得回退到当前价——否则会用今天的价格覆盖历史成本
func (c *Calculator) CalculateByPriceID(priceID uint64, metrics *usage.Metrics, multiplier uint64) (CostResult, bool, error) {
	if multiplier == 0 {
		multiplier = DefaultMultiplier
	}
	if metrics == nil {
		return CostResult{Multiplier: multiplier}, false, nil
	}
	price, err := c.resolvePriceByID(priceID)
	if err != nil {
		return CostResult{Multiplier: multiplier}, false, err
	}
	if price == nil {
		return CostResult{Multiplier: multiplier}, false, nil
	}
	return applyPrice(price, metrics, multiplier), true, nil
}

// applyPrice 把 token 用量按指定价格 + 倍率算成 cost,统一两条计算入口。
func applyPrice(price *domain.ModelPrice, metrics *usage.Metrics, multiplier uint64) CostResult {
	cost := computeCost(price, metrics)
	if multiplier != DefaultMultiplier {
		cost = cost * multiplier / DefaultMultiplier
	}
	return CostResult{
		Cost:         cost,
		ModelPriceID: price.ID,
		Multiplier:   multiplier,
	}
}

// effectivePrice 把 domain.ModelPrice 上“0 表示用默认”的字段解析为实际值,
// 便于 computeCost 不必到处写 fallback 逻辑。
type effectivePrice struct {
	InputMicro         uint64
	OutputMicro        uint64
	CacheReadMicro     uint64
	Cache5mWriteMicro  uint64
	Cache1hWriteMicro  uint64
	Has1MContext       bool
	Context1MThreshold uint64
	InputPremNum       uint64
	InputPremDenom     uint64
	OutputPremNum      uint64
	OutputPremDenom    uint64
}

func resolveEffective(p *domain.ModelPrice) effectivePrice {
	e := effectivePrice{
		InputMicro:         p.InputPriceMicro,
		OutputMicro:        p.OutputPriceMicro,
		CacheReadMicro:     p.CacheReadPriceMicro,
		Cache5mWriteMicro:  p.Cache5mWritePriceMicro,
		Cache1hWriteMicro:  p.Cache1hWritePriceMicro,
		Has1MContext:       p.Has1MContext,
		Context1MThreshold: p.Context1MThreshold,
		InputPremNum:       p.InputPremiumNum,
		InputPremDenom:     p.InputPremiumDenom,
		OutputPremNum:      p.OutputPremiumNum,
		OutputPremDenom:    p.OutputPremiumDenom,
	}
	if e.CacheReadMicro == 0 {
		e.CacheReadMicro = e.InputMicro / 10
	}
	if e.Cache5mWriteMicro == 0 {
		e.Cache5mWriteMicro = e.InputMicro * 5 / 4
	}
	if e.Cache1hWriteMicro == 0 {
		e.Cache1hWriteMicro = e.InputMicro * 2
	}
	if e.Context1MThreshold == 0 {
		e.Context1MThreshold = 200_000
	}
	if e.InputPremNum == 0 {
		e.InputPremNum = 2
	}
	if e.InputPremDenom == 0 {
		e.InputPremDenom = 1
	}
	if e.OutputPremNum == 0 {
		e.OutputPremNum = 3
	}
	if e.OutputPremDenom == 0 {
		e.OutputPremDenom = 2
	}
	return e
}

func computeCost(p *domain.ModelPrice, m *usage.Metrics) uint64 {
	e := resolveEffective(p)
	var total uint64

	if m.InputTokens > 0 {
		if e.Has1MContext {
			total += CalculateTieredCost(m.InputTokens, e.InputMicro, e.InputPremNum, e.InputPremDenom, e.Context1MThreshold)
		} else {
			total += CalculateLinearCost(m.InputTokens, e.InputMicro)
		}
	}
	if m.OutputTokens > 0 {
		if e.Has1MContext {
			total += CalculateTieredCost(m.OutputTokens, e.OutputMicro, e.OutputPremNum, e.OutputPremDenom, e.Context1MThreshold)
		} else {
			total += CalculateLinearCost(m.OutputTokens, e.OutputMicro)
		}
	}
	if m.CacheReadCount > 0 {
		total += CalculateLinearCost(m.CacheReadCount, e.CacheReadMicro)
	}
	if m.Cache5mCreationCount > 0 {
		total += CalculateLinearCost(m.Cache5mCreationCount, e.Cache5mWriteMicro)
	}
	if m.Cache1hCreationCount > 0 {
		total += CalculateLinearCost(m.Cache1hCreationCount, e.Cache1hWriteMicro)
	}
	// 旧响应只给 cache_creation_input_tokens、没有拆 5m/1h:按 5m 价格计。
	if m.Cache5mCreationCount == 0 && m.Cache1hCreationCount == 0 && m.CacheCreationCount > 0 {
		total += CalculateLinearCost(m.CacheCreationCount, e.Cache5mWriteMicro)
	}

	return total
}
