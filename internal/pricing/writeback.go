package pricing

import (
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
)

// FinalizeAttemptCost 根据 attempt 上已有的 token 字段算成本并写回。
// 调用前提:attempt 的 token/cache 字段已由 adapter 的 EventMetrics 填好。
// 没有 token 数据时不会触发查表(避免对未结束/未计量的 attempt 写假成本)。
//
// 返回值同时供调用方镜像到 proxyReq:线上请求路径上,attempt 是唯一事实源,
// proxyReq 上的成本/计价字段都从这里复制过去。
func FinalizeAttemptCost(attempt *domain.ProxyUpstreamAttempt, multiplier uint64) CostResult {
	if attempt == nil {
		return CostResult{Multiplier: defaultIfZero(multiplier)}
	}
	if attempt.InputTokenCount == 0 && attempt.OutputTokenCount == 0 {
		// 没有任何 token 数据:不查价表,不覆盖 attempt 上既有字段,只把入参 multiplier 回填。
		// 这样空 attempt 也带上正确的合约倍率,供后续审计。
		res := CostResult{Multiplier: defaultIfZero(multiplier)}
		attempt.Multiplier = res.Multiplier
		return res
	}

	pricingModel := attempt.ResponseModel
	if pricingModel == "" {
		pricingModel = attempt.MappedModel
	}

	res := GlobalCalculator().Calculate(pricingModel, &usage.Metrics{
		InputTokens:          attempt.InputTokenCount,
		OutputTokens:         attempt.OutputTokenCount,
		CacheReadCount:       attempt.CacheReadCount,
		CacheCreationCount:   attempt.CacheWriteCount,
		Cache5mCreationCount: attempt.Cache5mWriteCount,
		Cache1hCreationCount: attempt.Cache1hWriteCount,
	}, multiplier)

	attempt.Cost = res.Cost
	attempt.ModelPriceID = res.ModelPriceID
	attempt.Multiplier = res.Multiplier
	return res
}

// MirrorCostToRequest 把已结算的 attempt 的计费/token 字段复制到父 proxyReq。
//
// 之前 middleware 用 usage.ExtractFromResponse(body) 在 proxyReq 上独立再解析一遍
// 同样的 token 数据 —— 但所有 adapter 都会通过 EventMetrics 把 token 写到 attempt 上,
// 重新解析既浪费,又会和 attempt 漂移(EventMetrics 经过 AdjustForClientType,而 body
// 解析没有)。统一从 attempt 镜像可以让两端永远一致。
func MirrorCostToRequest(req *domain.ProxyRequest, attempt *domain.ProxyUpstreamAttempt) {
	if req == nil || attempt == nil {
		return
	}
	req.Cost = attempt.Cost
	req.ModelPriceID = attempt.ModelPriceID
	req.Multiplier = attempt.Multiplier
	req.InputTokenCount = attempt.InputTokenCount
	req.OutputTokenCount = attempt.OutputTokenCount
	req.CacheReadCount = attempt.CacheReadCount
	req.CacheWriteCount = attempt.CacheWriteCount
	req.Cache5mWriteCount = attempt.Cache5mWriteCount
	req.Cache1hWriteCount = attempt.Cache1hWriteCount
}

func defaultIfZero(m uint64) uint64 {
	if m == 0 {
		return DefaultMultiplier
	}
	return m
}
