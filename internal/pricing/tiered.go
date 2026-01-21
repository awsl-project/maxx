package pricing

import "math/big"

// 价格单位常量
const (
	// MicroUSDPerUSD 1美元 = 1,000,000 微美元 (用于价格表存储)
	MicroUSDPerUSD = 1_000_000
	// NanoUSDPerUSD 1美元 = 1,000,000,000 纳美元 (用于成本存储，提供更高精度)
	NanoUSDPerUSD = 1_000_000_000
	// TokensPerMillion 百万tokens
	TokensPerMillion = 1_000_000
	// MicroToNano 微美元转纳美元的倍数
	MicroToNano = 1000
)

var (
	bigTokensPerMillion = big.NewInt(TokensPerMillion)
	bigMicroToNano      = big.NewInt(MicroToNano)
)

// CalculateTieredCost 计算分层定价成本（使用 big.Int 防止溢出）
// tokens: token数量
// basePriceMicro: 基础价格 (microUSD/M tokens)
// premiumNum, premiumDenom: 超阈值倍率（分数表示，如 2.0 = 2/1, 1.5 = 3/2）
// threshold: 阈值 token 数
// 返回: 纳美元成本 (nanoUSD)
func CalculateTieredCost(tokens uint64, basePriceMicro uint64, premiumNum, premiumDenom, threshold uint64) uint64 {
	if tokens <= threshold {
		return calculateLinearCostBig(tokens, basePriceMicro)
	}

	baseCostNano := calculateLinearCostBig(threshold, basePriceMicro)
	premiumTokens := tokens - threshold

	// premiumCost = premiumTokens * basePriceMicro * MicroToNano / TokensPerMillion * premiumNum / premiumDenom
	t := big.NewInt(0).SetUint64(premiumTokens)
	p := big.NewInt(0).SetUint64(basePriceMicro)
	num := big.NewInt(0).SetUint64(premiumNum)
	denom := big.NewInt(0).SetUint64(premiumDenom)

	// t * p * MicroToNano * num / TokensPerMillion / denom
	t.Mul(t, p)
	t.Mul(t, bigMicroToNano)
	t.Mul(t, num)
	t.Div(t, bigTokensPerMillion)
	t.Div(t, denom)

	return baseCostNano + t.Uint64()
}

// CalculateLinearCost 计算线性定价成本（使用 big.Int 防止溢出）
// tokens: token数量
// priceMicro: 价格 (microUSD/M tokens)
// 返回: 纳美元成本 (nanoUSD)
func CalculateLinearCost(tokens, priceMicro uint64) uint64 {
	return calculateLinearCostBig(tokens, priceMicro)
}

// calculateLinearCostBig 使用 big.Int 计算线性成本
func calculateLinearCostBig(tokens, priceMicro uint64) uint64 {
	// cost = tokens * priceMicro * MicroToNano / TokensPerMillion
	t := big.NewInt(0).SetUint64(tokens)
	p := big.NewInt(0).SetUint64(priceMicro)

	t.Mul(t, p)
	t.Mul(t, bigMicroToNano)
	t.Div(t, bigTokensPerMillion)

	return t.Uint64()
}

// Deprecated: 使用 CalculateTieredCost 代替
func CalculateTieredCostMicro(tokens uint64, basePriceMicro uint64, premiumNum, premiumDenom, threshold uint64) uint64 {
	return CalculateTieredCost(tokens, basePriceMicro, premiumNum, premiumDenom, threshold) / MicroToNano
}

// Deprecated: 使用 CalculateLinearCost 代替
func CalculateLinearCostMicro(tokens, priceMicro uint64) uint64 {
	return CalculateLinearCost(tokens, priceMicro) / MicroToNano
}

// NanoToUSD 将纳美元转换为美元（用于显示）
func NanoToUSD(nanoUSD uint64) float64 {
	return float64(nanoUSD) / NanoUSDPerUSD
}

// MicroToUSD 将微美元转换为美元（用于显示）
func MicroToUSD(microUSD uint64) float64 {
	return float64(microUSD) / MicroUSDPerUSD
}
