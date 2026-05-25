package pricing

import (
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
)

// FinalizeAttemptCost 根据 attempt 上已有的 token 字段算成本并写回。
// 调用前提:attempt 的 token/cache 字段已由 adapter 的 EventMetrics 填好。
// 没有任何可计费 token 时不会触发查表(避免对未结束/未计量的 attempt 写假成本)。
//
// 调用方拿到的返回值随后会被持久化(attemptRepo.Update)+ 广播
// (BroadcastProxyUpstreamAttempt)+ 镜像到 proxyReq(MirrorCostToRequest)。
// 三个下游消费者都视 attempt 为事实源。
//
// 重要前提(best-effort EventMetrics): adapter 通过 AdapterEventChan.SendMetrics
// 把 token 投到 attempt 上,该 channel 缓冲满时会 `default:` drop
// (见 internal/domain/adapter_event.go)。极端压力下事件可能丢失,attempt token
// 字段保持 0,这条 attempt 会走 no-token 分支按 0 cost 写回 + 镜像到 proxyReq。
// 这与上一版"middleware 在 proxyReq 上独立 body-parse"是等价取舍(那条路径在同样压力下
// 也可能没读完 body),不是本 PR 引入的回归 —— 但写在这里以免后续误把 attempt 当成
// "绝对一致"的事实源。
func FinalizeAttemptCost(attempt *domain.ProxyUpstreamAttempt, multiplier uint64) CostResult {
	// 统一在入口归一化,有 token 分支和无 token 分支用同一份默认值,
	// 不再依赖 Calculator.Calculate 内部的 fallback。
	mult := defaultIfZero(multiplier)
	if attempt == nil {
		return CostResult{Multiplier: mult}
	}
	// 计费信号要覆盖所有计费 token:Calculator 也会按 cache_read / cache_5m / cache_1h
	// 单独算钱,只查 input/output 会漏掉 cache-only 响应(如缓存命中型流式请求)。
	if !hasBillableTokens(attempt) {
		// 没有任何可计费 token:不查价表,不覆盖 attempt 上既有 Cost/ModelPriceID,
		// 只把入参 multiplier 回填(空 attempt 也带上正确的合约倍率,供后续审计)。
		res := CostResult{Multiplier: mult}
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
		InputImageTokens:     attempt.InputImageTokenCount,
		OutputImageTokens:    attempt.OutputImageTokenCount,
		CacheReadCount:       attempt.CacheReadCount,
		CacheCreationCount:   attempt.CacheWriteCount,
		Cache5mCreationCount: attempt.Cache5mWriteCount,
		Cache1hCreationCount: attempt.Cache1hWriteCount,
	}, mult)

	attempt.Cost = res.Cost
	attempt.ModelPriceID = res.ModelPriceID
	attempt.Multiplier = res.Multiplier
	return res
}

// hasBillableTokens 判断 attempt 是否有任何会被 Calculator 收费的 token 字段。
// input/output 不算时,cache_read / cache_5m / cache_1h 单独都能产生 cost,
// CacheWriteCount 是兼容兜底字段(老响应只给总量,没拆 5m/1h)。
func hasBillableTokens(a *domain.ProxyUpstreamAttempt) bool {
	return a.InputTokenCount > 0 ||
		a.OutputTokenCount > 0 ||
		a.CacheReadCount > 0 ||
		a.CacheWriteCount > 0 ||
		a.Cache5mWriteCount > 0 ||
		a.Cache1hWriteCount > 0
}

// MirrorCostToRequest 把已结算的 attempt 的计费/token 字段复制到父 proxyReq。
//
// 之前 middleware 用 usage.ExtractFromResponse(body) 在 proxyReq 上独立再解析一遍
// 同样的 token 数据 —— 但所有 adapter 都会通过 EventMetrics 把 token 写到 attempt 上,
// 重新解析既浪费,又会和 attempt 漂移(EventMetrics 经过 AdjustForClientType,而 body
// 解析没有)。统一从 attempt 镜像可以让两端永远一致。
//
// best-effort 投递的 caveat 见 FinalizeAttemptCost 的 docstring;此函数只是把 attempt 上
// 的字段复制到 req,如果 attempt 字段因为 EventMetrics drop 而是 0,req 上同样会是 0。
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
