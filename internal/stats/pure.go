// Package stats provides pure functions for usage statistics aggregation and rollup.
// These functions are separated from the repository layer to enable easier testing
// and to ensure the aggregation logic is correct and predictable.
package stats

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// AttemptRecord represents a single upstream attempt record for aggregation.
// This is a simplified representation of the data needed for minute-level aggregation.
type AttemptRecord struct {
	EndTime      time.Time
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
