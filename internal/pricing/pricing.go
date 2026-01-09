// Package pricing 提供模型定价和成本计算功能
package pricing

import "strings"

// ModelPricing 单个模型的价格配置
// 价格单位：美元/百万tokens ($/M tokens)
type ModelPricing struct {
	ModelID           string  `json:"modelId"`
	InputPrice        float64 `json:"inputPrice"`        // 输入价格 $/M
	OutputPrice       float64 `json:"outputPrice"`       // 输出价格 $/M
	CacheReadPrice    float64 `json:"cacheReadPrice"`    // 缓存读取（默认 input × 0.1）
	Cache5mWritePrice float64 `json:"cache5mWritePrice"` // 5分钟缓存（默认 input × 1.25）
	Cache1hWritePrice float64 `json:"cache1hWritePrice"` // 1小时缓存（默认 input × 2.0）

	// 1M Context Window 分层定价 (Claude Sonnet 4/4.5)
	Has1MContext       bool    `json:"has1mContext"`       // 是否支持 1M context
	Context1MThreshold uint64  `json:"context1mThreshold"` // 阈值（默认 200,000）
	InputPremium       float64 `json:"inputPremium"`       // 超阈值 input 倍率（默认 2.0）
	OutputPremium      float64 `json:"outputPremium"`      // 超阈值 output 倍率（默认 1.5）
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

// GetEffectiveCacheReadPrice 获取有效的缓存读取价格
// 如果未设置，返回 inputPrice × 0.1
func (p *ModelPricing) GetEffectiveCacheReadPrice() float64 {
	if p.CacheReadPrice > 0 {
		return p.CacheReadPrice
	}
	return p.InputPrice * 0.1
}

// GetEffectiveCache5mWritePrice 获取有效的5分钟缓存写入价格
// 如果未设置，返回 inputPrice × 1.25
func (p *ModelPricing) GetEffectiveCache5mWritePrice() float64 {
	if p.Cache5mWritePrice > 0 {
		return p.Cache5mWritePrice
	}
	return p.InputPrice * 1.25
}

// GetEffectiveCache1hWritePrice 获取有效的1小时缓存写入价格
// 如果未设置，返回 inputPrice × 2.0
func (p *ModelPricing) GetEffectiveCache1hWritePrice() float64 {
	if p.Cache1hWritePrice > 0 {
		return p.Cache1hWritePrice
	}
	return p.InputPrice * 2.0
}

// GetContext1MThreshold 获取1M上下文阈值
// 如果未设置，返回默认值 200000
func (p *ModelPricing) GetContext1MThreshold() uint64 {
	if p.Context1MThreshold > 0 {
		return p.Context1MThreshold
	}
	return 200000
}

// GetInputPremium 获取超阈值input倍率
// 如果未设置，返回默认值 2.0
func (p *ModelPricing) GetInputPremium() float64 {
	if p.InputPremium > 0 {
		return p.InputPremium
	}
	return 2.0
}

// GetOutputPremium 获取超阈值output倍率
// 如果未设置，返回默认值 1.5
func (p *ModelPricing) GetOutputPremium() float64 {
	if p.OutputPremium > 0 {
		return p.OutputPremium
	}
	return 1.5
}
