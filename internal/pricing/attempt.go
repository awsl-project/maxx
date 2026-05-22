package pricing

import (
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
)

// PricingModel 按优先级挑选用来查价表的模型名:
// response_model(上游实际计费的型号)→ mapped_model(我们发上游的型号)→ request_model(客户端原始请求)。
// 空字符串向下穿透到下一级。
func PricingModel(responseModel, mappedModel, requestModel string) string {
	if responseModel != "" {
		return responseModel
	}
	if mappedModel != "" {
		return mappedModel
	}
	return requestModel
}

// attemptMetrics 把 attempt 上的 *Count 字段映射到 usage.Metrics 命名。
// 仅 pricing 包内部使用,屏蔽两种 attempt 类型(ProxyUpstreamAttempt / AttemptCostData)的字段差异。
func attemptMetrics(in, out, cacheRead, cacheWrite, c5m, c1h uint64) *usage.Metrics {
	return &usage.Metrics{
		InputTokens:          in,
		OutputTokens:         out,
		CacheReadCount:       cacheRead,
		CacheCreationCount:   cacheWrite,
		Cache5mCreationCount: c5m,
		Cache1hCreationCount: c1h,
	}
}

// RecalcAttemptUpdate 用当前价表+attempt 历史倍率重算 cost。
// 第三个返回值 changed=true 当且仅当 Cost 或 ModelPriceID 跟当前值不一致;
// 这样即便金额没变,只要价格记录被换成等额新版本,也会刷新 model_price_id 保留审计链。
// 第一个返回值是新算出的 cost,即使无变化也填充(给上层做 totalCost 累加用)。
//
// 注意:multiplier 用 attempt 自己的历史值,而不是 Provider 当下的合约值。
// 否则 backfill 会把历史折扣率悄悄改写,违反"重算价格不改合约"的原则。
func RecalcAttemptUpdate(model string, metrics *usage.Metrics, multiplier, currentCost, currentModelPriceID uint64) (uint64, domain.AttemptCostUpdate, bool) {
	res := GlobalCalculator().Calculate(model, metrics, multiplier)
	if res.Cost == currentCost && res.ModelPriceID == currentModelPriceID {
		return res.Cost, domain.AttemptCostUpdate{}, false
	}
	return res.Cost, domain.AttemptCostUpdate{Cost: res.Cost, ModelPriceID: res.ModelPriceID}, true
}

// RecalcFromAttempt 重算一个完整 ProxyUpstreamAttempt 的 cost(单请求 backfill 路径)。
func RecalcFromAttempt(a *domain.ProxyUpstreamAttempt) (uint64, domain.AttemptCostUpdate, bool) {
	return RecalcAttemptUpdate(
		PricingModel(a.ResponseModel, a.MappedModel, a.RequestModel),
		attemptMetrics(a.InputTokenCount, a.OutputTokenCount, a.CacheReadCount, a.CacheWriteCount, a.Cache5mWriteCount, a.Cache1hWriteCount),
		a.Multiplier, a.Cost, a.ModelPriceID,
	)
}

// RecalcFromCostData 重算一个 AttemptCostData 的 cost(全量 backfill 流式路径)。
func RecalcFromCostData(a *domain.AttemptCostData) (uint64, domain.AttemptCostUpdate, bool) {
	return RecalcAttemptUpdate(
		PricingModel(a.ResponseModel, a.MappedModel, a.RequestModel),
		attemptMetrics(a.InputTokenCount, a.OutputTokenCount, a.CacheReadCount, a.CacheWriteCount, a.Cache5mWriteCount, a.Cache1hWriteCount),
		a.Multiplier, a.Cost, a.ModelPriceID,
	)
}
