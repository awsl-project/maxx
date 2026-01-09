package pricing

// CalculateTieredCost 计算分层定价成本
// tokens: token数量
// basePrice: 基础价格 ($/M tokens)
// premium: 超阈值倍率
// threshold: 阈值 token 数
// 返回: 美元成本
func CalculateTieredCost(tokens uint64, basePrice, premium float64, threshold uint64) float64 {
	if tokens <= threshold {
		return float64(tokens) / 1_000_000 * basePrice
	}
	baseCost := float64(threshold) / 1_000_000 * basePrice
	premiumCost := float64(tokens-threshold) / 1_000_000 * basePrice * premium
	return baseCost + premiumCost
}

// CalculateLinearCost 计算线性定价成本
// tokens: token数量
// price: 价格 ($/M tokens)
// 返回: 美元成本
func CalculateLinearCost(tokens uint64, price float64) float64 {
	return float64(tokens) / 1_000_000 * price
}

// ToMicroUSD 将美元转换为微美元
// 1 USD = 1,000,000 microUSD
func ToMicroUSD(usd float64) uint64 {
	return uint64(usd * 1_000_000)
}

// FromMicroUSD 将微美元转换为美元
func FromMicroUSD(microUSD uint64) float64 {
	return float64(microUSD) / 1_000_000
}
