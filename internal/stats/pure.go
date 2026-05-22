// Package stats provides pure functions for usage statistics aggregation and rollup.
// These functions are separated from the repository layer to enable easier testing
// and to ensure the aggregation logic is correct and predictable.
package stats

import (
	"sort"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// AttemptRecord represents a single upstream attempt record for aggregation.
// This is a simplified representation of the data needed for minute-level aggregation.
type AttemptRecord struct {
	EndTime      time.Time
	TenantID     uint64
	RouteID      uint64
	ProviderID   uint64
	ProjectID    uint64
	APITokenID   uint64
	ClientType   string
	Model        string // response_model
	IsSuccessful bool
	IsFailed     bool
	DurationMs   uint64
	TTFTMs       uint64 // Time To First Token (milliseconds)
	InputTokens  uint64
	OutputTokens uint64
	CacheRead    uint64
	CacheWrite   uint64
	Cost         uint64
}

// TruncateToGranularity truncates a time to the start of its time bucket
// based on granularity using the specified timezone.
// The loc parameter is required and must not be nil.
func TruncateToGranularity(t time.Time, g domain.Granularity, loc *time.Location) time.Time {
	t = t.In(loc)
	switch g {
	case domain.GranularityMinute:
		return t.Truncate(time.Minute)
	case domain.GranularityHour:
		return t.Truncate(time.Hour)
	case domain.GranularityDay:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	case domain.GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	default:
		return t.Truncate(time.Hour)
	}
}

// AggregateAttempts aggregates a list of attempt records into UsageStats by minute.
// This is a pure function that takes raw attempt data and returns aggregated stats.
// The loc parameter specifies the timezone for time bucket calculation.
func AggregateAttempts(records []AttemptRecord, loc *time.Location) []*domain.UsageStats {
	if len(records) == 0 {
		return nil
	}

	type aggKey struct {
		minuteBucket int64
		tenantID     uint64
		routeID      uint64
		providerID   uint64
		projectID    uint64
		apiTokenID   uint64
		clientType   string
		model        string
	}
	statsMap := make(map[aggKey]*domain.UsageStats)

	for _, r := range records {
		minuteBucket := TruncateToGranularity(r.EndTime, domain.GranularityMinute, loc).UnixMilli()

		key := aggKey{
			minuteBucket: minuteBucket,
			tenantID:     r.TenantID,
			routeID:      r.RouteID,
			providerID:   r.ProviderID,
			projectID:    r.ProjectID,
			apiTokenID:   r.APITokenID,
			clientType:   r.ClientType,
			model:        r.Model,
		}

		var successful, failed uint64
		if r.IsSuccessful {
			successful = 1
		}
		if r.IsFailed {
			failed = 1
		}

		if s, ok := statsMap[key]; ok {
			s.TotalRequests++
			s.SuccessfulRequests += successful
			s.FailedRequests += failed
			s.TotalDurationMs += r.DurationMs
			s.TotalTTFTMs += r.TTFTMs
			s.InputTokens += r.InputTokens
			s.OutputTokens += r.OutputTokens
			s.CacheRead += r.CacheRead
			s.CacheWrite += r.CacheWrite
			s.Cost += r.Cost
		} else {
			statsMap[key] = &domain.UsageStats{
				Granularity:        domain.GranularityMinute,
				TimeBucket:         time.UnixMilli(minuteBucket),
				TenantID:           r.TenantID,
				RouteID:            r.RouteID,
				ProviderID:         r.ProviderID,
				ProjectID:          r.ProjectID,
				APITokenID:         r.APITokenID,
				ClientType:         r.ClientType,
				Model:              r.Model,
				TotalRequests:      1,
				SuccessfulRequests: successful,
				FailedRequests:     failed,
				TotalDurationMs:    r.DurationMs,
				TotalTTFTMs:        r.TTFTMs,
				InputTokens:        r.InputTokens,
				OutputTokens:       r.OutputTokens,
				CacheRead:          r.CacheRead,
				CacheWrite:         r.CacheWrite,
				Cost:               r.Cost,
			}
		}
	}

	result := make([]*domain.UsageStats, 0, len(statsMap))
	for _, s := range statsMap {
		result = append(result, s)
	}
	return result
}

// RollUp aggregates stats from a finer granularity to a coarser granularity.
// It takes a list of source stats and returns aggregated stats at the target granularity.
// The loc parameter specifies the timezone for time bucket calculation.
func RollUp(stats []*domain.UsageStats, to domain.Granularity, loc *time.Location) []*domain.UsageStats {
	if len(stats) == 0 {
		return nil
	}

	type rollupKey struct {
		targetBucket int64
		tenantID     uint64
		routeID      uint64
		providerID   uint64
		projectID    uint64
		apiTokenID   uint64
		clientType   string
		model        string
	}
	statsMap := make(map[rollupKey]*domain.UsageStats)

	for _, s := range stats {
		targetBucket := TruncateToGranularity(s.TimeBucket, to, loc)

		key := rollupKey{
			targetBucket: targetBucket.UnixMilli(),
			tenantID:     s.TenantID,
			routeID:      s.RouteID,
			providerID:   s.ProviderID,
			projectID:    s.ProjectID,
			apiTokenID:   s.APITokenID,
			clientType:   s.ClientType,
			model:        s.Model,
		}

		if existing, ok := statsMap[key]; ok {
			existing.TotalRequests += s.TotalRequests
			existing.SuccessfulRequests += s.SuccessfulRequests
			existing.FailedRequests += s.FailedRequests
			existing.TotalDurationMs += s.TotalDurationMs
			existing.TotalTTFTMs += s.TotalTTFTMs
			existing.InputTokens += s.InputTokens
			existing.OutputTokens += s.OutputTokens
			existing.CacheRead += s.CacheRead
			existing.CacheWrite += s.CacheWrite
			existing.Cost += s.Cost
		} else {
			statsMap[key] = &domain.UsageStats{
				Granularity:        to,
				TimeBucket:         targetBucket,
				TenantID:           s.TenantID,
				RouteID:            s.RouteID,
				ProviderID:         s.ProviderID,
				ProjectID:          s.ProjectID,
				APITokenID:         s.APITokenID,
				ClientType:         s.ClientType,
				Model:              s.Model,
				TotalRequests:      s.TotalRequests,
				SuccessfulRequests: s.SuccessfulRequests,
				FailedRequests:     s.FailedRequests,
				TotalDurationMs:    s.TotalDurationMs,
				TotalTTFTMs:        s.TotalTTFTMs,
				InputTokens:        s.InputTokens,
				OutputTokens:       s.OutputTokens,
				CacheRead:          s.CacheRead,
				CacheWrite:         s.CacheWrite,
				Cost:               s.Cost,
			}
		}
	}

	result := make([]*domain.UsageStats, 0, len(statsMap))
	for _, s := range statsMap {
		result = append(result, s)
	}
	return result
}

// MergeStats merges multiple UsageStats slices into one, combining stats with matching keys.
// This is useful for combining pre-aggregated data with real-time data.
func MergeStats(statsList ...[]*domain.UsageStats) []*domain.UsageStats {
	type mergeKey struct {
		granularity domain.Granularity
		timeBucket  int64
		tenantID    uint64
		routeID     uint64
		providerID  uint64
		projectID   uint64
		apiTokenID  uint64
		clientType  string
		model       string
	}
	merged := make(map[mergeKey]*domain.UsageStats)

	for _, stats := range statsList {
		for _, s := range stats {
			key := mergeKey{
				granularity: s.Granularity,
				timeBucket:  s.TimeBucket.UnixMilli(),
				tenantID:    s.TenantID,
				routeID:     s.RouteID,
				providerID:  s.ProviderID,
				projectID:   s.ProjectID,
				apiTokenID:  s.APITokenID,
				clientType:  s.ClientType,
				model:       s.Model,
			}

			if existing, ok := merged[key]; ok {
				existing.TotalRequests += s.TotalRequests
				existing.SuccessfulRequests += s.SuccessfulRequests
				existing.FailedRequests += s.FailedRequests
				existing.TotalDurationMs += s.TotalDurationMs
				existing.TotalTTFTMs += s.TotalTTFTMs
				existing.InputTokens += s.InputTokens
				existing.OutputTokens += s.OutputTokens
				existing.CacheRead += s.CacheRead
				existing.CacheWrite += s.CacheWrite
				existing.Cost += s.Cost
			} else {
				// Make a copy to avoid modifying the original
				copied := *s
				merged[key] = &copied
			}
		}
	}

	result := make([]*domain.UsageStats, 0, len(merged))
	for _, s := range merged {
		result = append(result, s)
	}
	return result
}

// SumStats calculates the summary of a list of UsageStats.
// Returns total requests, successful requests, failed requests, input tokens, output tokens,
// cache read, cache write, and cost.
func SumStats(stats []*domain.UsageStats) (totalReq, successReq, failedReq, inputTokens, outputTokens, cacheRead, cacheWrite, cost uint64) {
	for _, s := range stats {
		totalReq += s.TotalRequests
		successReq += s.SuccessfulRequests
		failedReq += s.FailedRequests
		inputTokens += s.InputTokens
		outputTokens += s.OutputTokens
		cacheRead += s.CacheRead
		cacheWrite += s.CacheWrite
		cost += s.Cost
	}
	return
}

// Summarize 把一组 UsageStats 聚合为一个 UsageStatsSummary(含成功率)。
// 内部复用 SumStats 做字段累加,避免维护两套聚合逻辑;后续 UsageStats 加字段时只改 SumStats。
func Summarize(stats []*domain.UsageStats) *domain.UsageStatsSummary {
	totalReq, successReq, failedReq, inputTokens, outputTokens, cacheRead, cacheWrite, cost := SumStats(stats)
	s := &domain.UsageStatsSummary{
		TotalRequests:      totalReq,
		SuccessfulRequests: successReq,
		FailedRequests:     failedReq,
		TotalInputTokens:   inputTokens,
		TotalOutputTokens:  outputTokens,
		TotalCacheRead:     cacheRead,
		TotalCacheWrite:    cacheWrite,
		TotalCost:          cost,
	}
	if s.TotalRequests > 0 {
		s.SuccessRate = float64(s.SuccessfulRequests) / float64(s.TotalRequests) * 100
	}
	return s
}

// GroupByProvider groups stats by provider ID and sums them.
// Returns a map of provider ID to aggregated totals.
func GroupByProvider(stats []*domain.UsageStats) map[uint64]*domain.ProviderStats {
	result := make(map[uint64]*domain.ProviderStats)

	for _, s := range stats {
		if s.ProviderID == 0 {
			continue
		}

		if existing, ok := result[s.ProviderID]; ok {
			existing.TotalRequests += s.TotalRequests
			existing.SuccessfulRequests += s.SuccessfulRequests
			existing.FailedRequests += s.FailedRequests
			existing.TotalInputTokens += s.InputTokens
			existing.TotalOutputTokens += s.OutputTokens
			existing.TotalCacheRead += s.CacheRead
			existing.TotalCacheWrite += s.CacheWrite
			existing.TotalCost += s.Cost
		} else {
			result[s.ProviderID] = &domain.ProviderStats{
				ProviderID:         s.ProviderID,
				TotalRequests:      s.TotalRequests,
				SuccessfulRequests: s.SuccessfulRequests,
				FailedRequests:     s.FailedRequests,
				TotalInputTokens:   s.InputTokens,
				TotalOutputTokens:  s.OutputTokens,
				TotalCacheRead:     s.CacheRead,
				TotalCacheWrite:    s.CacheWrite,
				TotalCost:          s.Cost,
			}
		}
	}

	// Calculate success rate
	for _, ps := range result {
		if ps.TotalRequests > 0 {
			ps.SuccessRate = float64(ps.SuccessfulRequests) / float64(ps.TotalRequests) * 100
		}
	}

	return result
}

// TopModelsByRequests 按请求数降序返回 Top N 模型(用于 Dashboard 等场景)。
// 跨多个 UsageStats 行按 Model 字段聚合,空 Model 行被忽略。
// 当 limit <= 0 时返回空切片;不足 limit 时返回所有非空模型。
// 平手时按 model 字典序(保证确定性)。
func TopModelsByRequests(stats []*domain.UsageStats, limit int) []domain.DashboardModelStats {
	if limit <= 0 {
		return []domain.DashboardModelStats{}
	}
	type agg struct {
		requests uint64
		tokens   uint64
	}
	byModel := make(map[string]*agg)
	for _, s := range stats {
		if s.Model == "" {
			continue
		}
		a, ok := byModel[s.Model]
		if !ok {
			a = &agg{}
			byModel[s.Model] = a
		}
		a.requests += s.TotalRequests
		a.tokens += s.InputTokens + s.OutputTokens + s.CacheRead + s.CacheWrite
	}

	models := make([]domain.DashboardModelStats, 0, len(byModel))
	for name, a := range byModel {
		models = append(models, domain.DashboardModelStats{
			Model:    name,
			Requests: a.requests,
			Tokens:   a.tokens,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Requests != models[j].Requests {
			return models[i].Requests > models[j].Requests
		}
		return models[i].Model < models[j].Model
	})
	if len(models) > limit {
		models = models[:limit]
	}
	return models
}

// DailyCounts 按日期(YYYY-MM-DD,在指定时区下渲染)聚合 stats 的 TotalRequests,
// 只返回非零日,日期升序。用于 dashboard 热力图。
// stats 可以是任意粒度,日期键基于 TimeBucket 在 loc 下的渲染。
// loc=nil 时默认 UTC(避免 t.In(nil) panic;让调用方误用也能拿到合理结果)。
func DailyCounts(in []*domain.UsageStats, loc *time.Location) []domain.DashboardHeatmapPoint {
	if loc == nil {
		loc = time.UTC
	}
	byDate := make(map[string]uint64)
	for _, s := range in {
		if s.TotalRequests == 0 {
			continue
		}
		key := s.TimeBucket.In(loc).Format("2006-01-02")
		byDate[key] += s.TotalRequests
	}
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		// byDate 里的 entries 都来自 TotalRequests > 0 的行,
		// 无需二次 if count > 0 过滤(原代码冗余,被 review 指出)。
		dates = append(dates, d)
	}
	sort.Strings(dates)
	out := make([]domain.DashboardHeatmapPoint, 0, len(dates))
	for _, d := range dates {
		out = append(out, domain.DashboardHeatmapPoint{Date: d, Count: byDate[d]})
	}
	return out
}

// HourlyTrend 构建从 start 起 24 小时滚动请求量趋势,与 stats 的 hour-bucket 网格对齐,在指定时区下渲染。
// 没有数据的小时填 0 以保证 x 轴连续。窗口外的 stats 被跳过。返回切片严格 24 个元素。
//
// **设计要点:bucket key 用绝对 UnixMilli,而不是 wall-clock-hour key。**
//
// 第一版用 "HH:MM" 字符串作 key,跨日同 wall-clock 时间会折叠 — review 已修。
// 第二版用 `truncateHourInLoc(bucket[i])` 作 key,但 DST fall-back 时 wall-clock 整点会重复
// (NY Nov 3: 01:00 EDT 和 01:00 EST 都被 time.Date 解析成 EDT 那一个 instant),
// 两个真实相隔一小时的桶被折叠成一个 — Codex/Claude R3 codex 用 repro 跑出来确认了。
//
// 第三版(本次):一次性把 start 截到 hour-bucket 网格,之后只做"绝对加 1 小时"。
// 这样:
//   1. 不同日的同 wall-clock 时间 → 不同 UnixMilli ✓
//   2. DST fall-back 的 01:00 EDT / 01:00 EST → 真实相差 1 小时的两个 UnixMilli ✓
//      (chart 上会显示两个都贴 "01:00" 标签,这是 wall-clock 渲染的固有歧义,不是 bucket 错乱)
//   3. 跟 stats 的 TimeBucket 网格对齐 — 后者由 TruncateToGranularity(t, Hour, loc) 写入,
//      所以这里也走同一个函数 snap start,key 自然对得上。
func HourlyTrend(in []*domain.UsageStats, start time.Time, loc *time.Location) []domain.DashboardTrendPoint {
	if loc == nil {
		loc = time.UTC
	}
	// 把 start 截到 stats 存储用的 hour 网格上;之后只做绝对加法,DST 安全。
	snappedStart := TruncateToGranularity(start, domain.GranularityHour, loc)
	bucketStarts := make([]time.Time, 24)
	bucketKeys := make([]int64, 24)
	for i := 0; i < 24; i++ {
		bucketStarts[i] = snappedStart.Add(time.Duration(i) * time.Hour)
		bucketKeys[i] = bucketStarts[i].UnixMilli()
	}
	counts := make(map[int64]uint64, 24)
	for _, k := range bucketKeys {
		counts[k] = 0
	}
	for _, s := range in {
		// 直接拿 stat 绝对 UnixMilli。stats 已经被 TruncateToGranularity hour 对齐,
		// 跟 bucketKeys 同一网格,UnixMilli 应当精确匹配。
		k := s.TimeBucket.UnixMilli()
		if _, ok := counts[k]; ok {
			counts[k] += s.TotalRequests
		}
	}
	trend := make([]domain.DashboardTrendPoint, 24)
	for i, k := range bucketKeys {
		trend[i] = domain.DashboardTrendPoint{
			Hour:     bucketStarts[i].In(loc).Format("15:04"),
			Requests: counts[k],
		}
	}
	return trend
}

// FilterByGranularity filters stats to only include the specified granularity.
func FilterByGranularity(stats []*domain.UsageStats, g domain.Granularity) []*domain.UsageStats {
	result := make([]*domain.UsageStats, 0)
	for _, s := range stats {
		if s.Granularity == g {
			result = append(result, s)
		}
	}
	return result
}

// FilterByTimeRange filters stats to only include those within the specified time range.
// start is inclusive, end is exclusive.
func FilterByTimeRange(stats []*domain.UsageStats, start, end time.Time) []*domain.UsageStats {
	result := make([]*domain.UsageStats, 0)
	for _, s := range stats {
		if !s.TimeBucket.Before(start) && s.TimeBucket.Before(end) {
			result = append(result, s)
		}
	}
	return result
}
