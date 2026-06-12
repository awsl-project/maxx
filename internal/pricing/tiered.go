package pricing

import "math/big"

// 价格单位常量
//
//	storage:  microUSD/M tokens   (1 USD = 1,000,000 microUSD)
//	output:   nanoUSD            (1 USD = 1,000,000,000 nanoUSD)
const (
	// TokensPerMillion 价格单位分母:百万 tokens。
	TokensPerMillion = 1_000_000
	// MicroToNano 微美元到纳美元的倍数(成本输出乘以这个值)。
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

