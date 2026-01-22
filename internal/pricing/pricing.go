// Package pricing 提供模型定价和成本计算功能
package pricing

import (
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// ModelPricing 单个模型的价格配置
// 价格单位：微美元/百万tokens (microUSD/M tokens)
// 例如 $3/M tokens = 3,000,000 microUSD/M tokens
type ModelPricing struct {
	ModelID string `json:"modelId"`

	// 基础价格 (microUSD/M tokens)
	InputPriceMicro  uint64 `json:"inputPriceMicro"`  // 输入价格
	OutputPriceMicro uint64 `json:"outputPriceMicro"` // 输出价格

	// 缓存价格 (microUSD/M tokens)，0 表示使用默认值
	CacheReadPriceMicro    uint64 `json:"cacheReadPriceMicro,omitempty"`    // 缓存读取（默认 input / 10）
	Cache5mWritePriceMicro uint64 `json:"cache5mWritePriceMicro,omitempty"` // 5分钟缓存（默认 input * 5/4）
	Cache1hWritePriceMicro uint64 `json:"cache1hWritePriceMicro,omitempty"` // 1小时缓存（默认 input * 2）

	// 1M Context Window 分层定价 (Claude Sonnet 4/4.5)
	Has1MContext       bool   `json:"has1mContext"`                 // 是否支持 1M context
	Context1MThreshold uint64 `json:"context1mThreshold,omitempty"` // 阈值（默认 200,000）
	// 倍率使用分数表示：premiumNum/premiumDenom
	// 例如 2.0 = 2/1, 1.5 = 3/2
	InputPremiumNum    uint64 `json:"inputPremiumNum,omitempty"`    // 超阈值 input 倍率分子（默认 2）
	InputPremiumDenom  uint64 `json:"inputPremiumDenom,omitempty"`  // 超阈值 input 倍率分母（默认 1）
	OutputPremiumNum   uint64 `json:"outputPremiumNum,omitempty"`   // 超阈值 output 倍率分子（默认 3）
	OutputPremiumDenom uint64 `json:"outputPremiumDenom,omitempty"` // 超阈值 output 倍率分母（默认 2）
}

// PriceTable 完整价格表
type PriceTable struct {
	Version string                   `json:"version"`
	Models  map[string]*ModelPricing `json:"models"` // key: modelID 或 modelID 前缀
}

// NewPriceTable 创建空价格表
func NewPriceTable(version string) *PriceTable {
	return &PriceTable{
		Version: version,
		Models:  make(map[string]*ModelPricing),
	}
}

// Get 获取模型价格，支持前缀匹配
// 例如 "claude-sonnet-4-20250514" 会匹配 "claude-sonnet-4"
func (pt *PriceTable) Get(modelID string) *ModelPricing {
	// 精确匹配
	if p, ok := pt.Models[modelID]; ok {
		return p
	}

	// 前缀匹配：找最长匹配
	var bestMatch *ModelPricing
	var bestLen int

	for key, pricing := range pt.Models {
		if strings.HasPrefix(modelID, key) && len(key) > bestLen {
			bestMatch = pricing
			bestLen = len(key)
		}
	}

	return bestMatch
}

// Set 设置模型价格
func (pt *PriceTable) Set(pricing *ModelPricing) {
	pt.Models[pricing.ModelID] = pricing
}

// All 返回所有模型价格
func (pt *PriceTable) All() []*ModelPricing {
	prices := make([]*ModelPricing, 0, len(pt.Models))
	for _, p := range pt.Models {
		prices = append(prices, p)
	}
	return prices
}

// GetEffectiveCacheReadPriceMicro 获取有效的缓存读取价格 (microUSD/M tokens)
// 如果未设置，返回 inputPriceMicro / 10
func (p *ModelPricing) GetEffectiveCacheReadPriceMicro() uint64 {
	if p.CacheReadPriceMicro > 0 {
		return p.CacheReadPriceMicro
	}
	return p.InputPriceMicro / 10
}

// GetEffectiveCache5mWritePriceMicro 获取有效的5分钟缓存写入价格 (microUSD/M tokens)
// 如果未设置，返回 inputPriceMicro * 5/4
func (p *ModelPricing) GetEffectiveCache5mWritePriceMicro() uint64 {
	if p.Cache5mWritePriceMicro > 0 {
		return p.Cache5mWritePriceMicro
	}
	return p.InputPriceMicro * 5 / 4
}

// GetEffectiveCache1hWritePriceMicro 获取有效的1小时缓存写入价格 (microUSD/M tokens)
// 如果未设置，返回 inputPriceMicro * 2
func (p *ModelPricing) GetEffectiveCache1hWritePriceMicro() uint64 {
	if p.Cache1hWritePriceMicro > 0 {
		return p.Cache1hWritePriceMicro
	}
	return p.InputPriceMicro * 2
}

// GetContext1MThreshold 获取1M上下文阈值
// 如果未设置，返回默认值 200000
func (p *ModelPricing) GetContext1MThreshold() uint64 {
	if p.Context1MThreshold > 0 {
		return p.Context1MThreshold
	}
	return 200000
}

// GetInputPremiumFraction 获取超阈值input倍率（分数）
// 默认 2/1 = 2.0
func (p *ModelPricing) GetInputPremiumFraction() (num, denom uint64) {
	num, denom = p.InputPremiumNum, p.InputPremiumDenom
	if num == 0 {
		num = 2
	}
	if denom == 0 {
		denom = 1
	}
	return
}

// GetInputPremiumNum 获取超阈值input倍率分子（默认2）
func (p *ModelPricing) GetInputPremiumNum() uint64 {
	if p.InputPremiumNum > 0 {
		return p.InputPremiumNum
	}
	return 2
}

// GetInputPremiumDenom 获取超阈值input倍率分母（默认1）
func (p *ModelPricing) GetInputPremiumDenom() uint64 {
	if p.InputPremiumDenom > 0 {
		return p.InputPremiumDenom
	}
	return 1
}

// GetOutputPremiumFraction 获取超阈值output倍率（分数）
// 默认 3/2 = 1.5
func (p *ModelPricing) GetOutputPremiumFraction() (num, denom uint64) {
	num, denom = p.OutputPremiumNum, p.OutputPremiumDenom
	if num == 0 {
		num = 3
	}
	if denom == 0 {
		denom = 2
	}
	return
}

// GetOutputPremiumNum 获取超阈值output倍率分子（默认3）
func (p *ModelPricing) GetOutputPremiumNum() uint64 {
	if p.OutputPremiumNum > 0 {
		return p.OutputPremiumNum
	}
	return 3
}

// GetOutputPremiumDenom 获取超阈值output倍率分母（默认2）
func (p *ModelPricing) GetOutputPremiumDenom() uint64 {
	if p.OutputPremiumDenom > 0 {
		return p.OutputPremiumDenom
	}
	return 2
}

// ConvertToDBPrices 将内置价格表转换为数据库价格记录
func ConvertToDBPrices(pt *PriceTable) []*domain.ModelPrice {
	prices := make([]*domain.ModelPrice, 0, len(pt.Models))

	for _, mp := range pt.Models {
		price := &domain.ModelPrice{
			ModelID:                mp.ModelID,
			InputPriceMicro:        mp.InputPriceMicro,
			OutputPriceMicro:       mp.OutputPriceMicro,
			CacheReadPriceMicro:    mp.CacheReadPriceMicro,
			Cache5mWritePriceMicro: mp.Cache5mWritePriceMicro,
			Cache1hWritePriceMicro: mp.Cache1hWritePriceMicro,
			Has1MContext:           mp.Has1MContext,
			Context1MThreshold:     mp.Context1MThreshold,
			InputPremiumNum:        mp.InputPremiumNum,
			InputPremiumDenom:      mp.InputPremiumDenom,
			OutputPremiumNum:       mp.OutputPremiumNum,
			OutputPremiumDenom:     mp.OutputPremiumDenom,
		}
		prices = append(prices, price)
	}

	return prices
}
