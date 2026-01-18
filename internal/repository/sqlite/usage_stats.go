package sqlite

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm/clause"
)

type UsageStatsRepository struct {
	db *DB
}

func NewUsageStatsRepository(db *DB) *UsageStatsRepository {
	return &UsageStatsRepository{db: db}
}

// getConfiguredTimezone 获取配置的时区，默认 Asia/Shanghai
func (r *UsageStatsRepository) getConfiguredTimezone() *time.Location {
	var value string
	err := r.db.gorm.Table("system_settings").
		Where("key = ?", domain.SettingKeyTimezone).
		Pluck("value", &value).Error
	if err != nil || value == "" {
		value = "Asia/Shanghai" // 默认时区
	}

	loc, err := time.LoadLocation(value)
	if err != nil {
		log.Printf("[UsageStats] Invalid timezone %q, falling back to UTC+8: %v", value, err)
		// 手动创建 UTC+8 时区作为 fallback（避免 Docker 容器无 tzdata 导致 panic）
		loc = time.FixedZone("UTC+8", 8*60*60)
	}
	return loc
}

// TruncateToGranularity 将时间截断到指定粒度的时间桶（使用 UTC）
func TruncateToGranularity(t time.Time, g domain.Granularity) time.Time {
	t = t.UTC()
	switch g {
	case domain.GranularityMinute:
		return t.Truncate(time.Minute)
	case domain.GranularityHour:
		return t.Truncate(time.Hour)
	case domain.GranularityDay:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case domain.GranularityWeek:
		// 截断到周一
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(t.Year(), t.Month(), t.Day()-(weekday-1), 0, 0, 0, 0, time.UTC)
	case domain.GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return t.Truncate(time.Hour)
	}
}

// TruncateToGranularityInTimezone 将时间截断到指定粒度的时间桶（使用指定时区）
func TruncateToGranularityInTimezone(t time.Time, g domain.Granularity, loc *time.Location) time.Time {
	t = t.In(loc)
	switch g {
	case domain.GranularityMinute:
		return t.Truncate(time.Minute)
	case domain.GranularityHour:
		return t.Truncate(time.Hour)
	case domain.GranularityDay:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	case domain.GranularityWeek:
		// 截断到周一
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(t.Year(), t.Month(), t.Day()-(weekday-1), 0, 0, 0, 0, loc)
	case domain.GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	default:
		return t.Truncate(time.Hour)
	}
}

// Upsert 更新或插入统计记录
func (r *UsageStatsRepository) Upsert(stats *domain.UsageStats) error {
	now := time.Now()
	stats.CreatedAt = now

	model := r.toModel(stats)
	return r.db.gorm.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "granularity"},
			{Name: "time_bucket"},
			{Name: "route_id"},
			{Name: "provider_id"},
			{Name: "project_id"},
			{Name: "api_token_id"},
			{Name: "client_type"},
			{Name: "model"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"total_requests":      stats.TotalRequests,
			"successful_requests": stats.SuccessfulRequests,
			"failed_requests":     stats.FailedRequests,
			"total_duration_ms":   stats.TotalDurationMs,
			"input_tokens":        stats.InputTokens,
			"output_tokens":       stats.OutputTokens,
			"cache_read":          stats.CacheRead,
			"cache_write":         stats.CacheWrite,
			"cost":                stats.Cost,
		}),
	}).Create(model).Error
}

// BatchUpsert 批量更新或插入统计记录
func (r *UsageStatsRepository) BatchUpsert(stats []*domain.UsageStats) error {
	now := time.Now()
	for _, s := range stats {
		s.CreatedAt = now
		if err := r.Upsert(s); err != nil {
			return err
		}
	}
	return nil
}

// Query 查询统计数据
func (r *UsageStatsRepository) Query(filter repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "granularity = ?")
	args = append(args, filter.Granularity)

	if filter.StartTime != nil {
		conditions = append(conditions, "time_bucket >= ?")
		args = append(args, toTimestamp(*filter.StartTime))
	}
	if filter.EndTime != nil {
		conditions = append(conditions, "time_bucket <= ?")
		args = append(args, toTimestamp(*filter.EndTime))
	}
	if filter.RouteID != nil {
		conditions = append(conditions, "route_id = ?")
		args = append(args, *filter.RouteID)
	}
	if filter.ProviderID != nil {
		conditions = append(conditions, "provider_id = ?")
		args = append(args, *filter.ProviderID)
	}
	if filter.ProjectID != nil {
		conditions = append(conditions, "project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.ClientType != nil {
		conditions = append(conditions, "client_type = ?")
		args = append(args, *filter.ClientType)
	}
	if filter.APITokenID != nil {
		conditions = append(conditions, "api_token_id = ?")
		args = append(args, *filter.APITokenID)
	}
	if filter.Model != nil {
		conditions = append(conditions, "model = ?")
		args = append(args, *filter.Model)
	}

	var models []UsageStats
	err := r.db.gorm.Where(strings.Join(conditions, " AND "), args...).
		Order("time_bucket DESC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	return r.toDomainList(models), nil
}

// QueryWithRealtime 查询统计数据并补全当前时间桶的数据
// 策略（分层查询，每层用最粗粒度的预聚合数据）：
//   - 历史时间桶：使用目标粒度的预聚合数据
//   - 当前时间桶：week → day → hour → minute → 最近 2 分钟实时
//
// 示例（查询 month 粒度，当前是 1月17日 10:30）：
//   - 1月1日-1月5日（第1周）: usage_stats (granularity='week')
//   - 1月6日-1月12日（第2周）: usage_stats (granularity='week')
//   - 1月13日-1月16日: usage_stats (granularity='day')
//   - 1月17日 00:00-09:00: usage_stats (granularity='hour')
//   - 1月17日 10:00-10:28: usage_stats (granularity='minute')
//   - 1月17日 10:29-10:30: proxy_upstream_attempts (实时)
func (r *UsageStatsRepository) QueryWithRealtime(filter repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	now := time.Now().UTC()
	currentBucket := TruncateToGranularity(now, filter.Granularity)
	currentWeek := TruncateToGranularity(now, domain.GranularityWeek)
	currentDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	currentHour := now.Truncate(time.Hour)
	currentMinute := now.Truncate(time.Minute)
	twoMinutesAgo := currentMinute.Add(-time.Minute)

	// 判断是否需要补全当前时间桶
	needCurrentBucket := filter.EndTime == nil || !filter.EndTime.Before(currentBucket)

	// 1. 查询历史数据（使用目标粒度的预聚合数据）
	// 如果需要补全当前时间桶，则排除当前时间桶（避免查出会被替换的数据）
	historyFilter := filter
	if needCurrentBucket {
		endTime := currentBucket.Add(-time.Millisecond) // 排除当前时间桶
		historyFilter.EndTime = &endTime
	}
	results, err := r.Query(historyFilter)
	if err != nil {
		return nil, err
	}

	if !needCurrentBucket {
		return results, nil
	}

	// 2. 对于当前时间桶，并发分层查询（每层用最粗粒度的预聚合数据）：
	//    - 已完成的周: usage_stats (granularity='week') [仅 month 粒度]
	//    - 已完成的天: usage_stats (granularity='day') [week/month 粒度]
	//    - 已完成的小时: usage_stats (granularity='hour')
	//    - 已完成的分钟: usage_stats (granularity='minute')
	//    - 最近 2 分钟: proxy_upstream_attempts (实时)

	var (
		mu       sync.Mutex
		allStats []*domain.UsageStats
		g        errgroup.Group
	)

	// 2a. 查询当前时间桶内已完成的周数据 (仅 month 粒度需要)
	if filter.Granularity == domain.GranularityMonth && currentWeek.After(currentBucket) {
		g.Go(func() error {
			weekStats, err := r.queryStatsInRange(domain.GranularityWeek, currentBucket, currentWeek, filter)
			if err != nil {
				return err
			}
			mu.Lock()
			allStats = append(allStats, weekStats...)
			mu.Unlock()
			return nil
		})
	}

	// 2b. 查询当前周（或当前时间桶）内已完成的天数据 (week/month 粒度需要)
	if filter.Granularity == domain.GranularityWeek || filter.Granularity == domain.GranularityMonth {
		dayStart := currentWeek
		if currentBucket.After(currentWeek) {
			dayStart = currentBucket
		}
		if currentDay.After(dayStart) {
			g.Go(func() error {
				dayStats, err := r.queryStatsInRange(domain.GranularityDay, dayStart, currentDay, filter)
				if err != nil {
					return err
				}
				mu.Lock()
				allStats = append(allStats, dayStats...)
				mu.Unlock()
				return nil
			})
		}
	}

	// 2c. 查询今天（或当前时间桶）内已完成的小时数据
	hourStart := currentDay
	if currentBucket.After(currentDay) {
		hourStart = currentBucket
	}
	if currentHour.After(hourStart) {
		g.Go(func() error {
			hourStats, err := r.queryStatsInRange(domain.GranularityHour, hourStart, currentHour, filter)
			if err != nil {
				return err
			}
			mu.Lock()
			allStats = append(allStats, hourStats...)
			mu.Unlock()
			return nil
		})
	}

	// 2d. 查询当前小时内已完成的分钟数据（不包括最近 2 分钟）
	minuteStart := currentHour
	if currentBucket.After(currentHour) {
		minuteStart = currentBucket
	}
	if twoMinutesAgo.After(minuteStart) {
		g.Go(func() error {
			minuteStats, err := r.queryStatsInRange(domain.GranularityMinute, minuteStart, twoMinutesAgo, filter)
			if err != nil {
				return err
			}
			mu.Lock()
			allStats = append(allStats, minuteStats...)
			mu.Unlock()
			return nil
		})
	}

	// 2e. 查询最近 2 分钟的实时数据
	g.Go(func() error {
		realtimeStats, err := r.queryRecentMinutesStats(twoMinutesAgo, filter)
		if err != nil {
			return err
		}
		mu.Lock()
		allStats = append(allStats, realtimeStats...)
		mu.Unlock()
		return nil
	})

	// 等待所有查询完成
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// 3. 将所有数据聚合为当前时间桶
	currentBucketStats := r.aggregateToTargetBucket(allStats, currentBucket, filter.Granularity)

	// 4. 将当前时间桶数据合并到结果中（替换预聚合数据）
	results = r.mergeCurrentBucketStats(results, currentBucketStats, currentBucket, filter.Granularity)

	return results, nil
}

// queryStatsInRange 查询指定粒度和时间范围内的统计数据
func (r *UsageStatsRepository) queryStatsInRange(granularity domain.Granularity, start, end time.Time, filter repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "granularity = ?")
	args = append(args, granularity)

	conditions = append(conditions, "time_bucket >= ?")
	args = append(args, toTimestamp(start))

	conditions = append(conditions, "time_bucket < ?")
	args = append(args, toTimestamp(end))

	if filter.RouteID != nil {
		conditions = append(conditions, "route_id = ?")
		args = append(args, *filter.RouteID)
	}
	if filter.ProviderID != nil {
		conditions = append(conditions, "provider_id = ?")
		args = append(args, *filter.ProviderID)
	}
	if filter.ProjectID != nil {
		conditions = append(conditions, "project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.ClientType != nil {
		conditions = append(conditions, "client_type = ?")
		args = append(args, *filter.ClientType)
	}
	if filter.APITokenID != nil {
		conditions = append(conditions, "api_token_id = ?")
		args = append(args, *filter.APITokenID)
	}
	if filter.Model != nil {
		conditions = append(conditions, "model = ?")
		args = append(args, *filter.Model)
	}

	var models []UsageStats
	err := r.db.gorm.Where(strings.Join(conditions, " AND "), args...).Find(&models).Error
	if err != nil {
		return nil, err
	}

	return r.toDomainList(models), nil
}

// aggregateToTargetBucket 将多个粒度的数据聚合为目标时间桶
func (r *UsageStatsRepository) aggregateToTargetBucket(
	stats []*domain.UsageStats,
	targetBucket time.Time,
	granularity domain.Granularity,
) []*domain.UsageStats {
	type dimKey struct {
		routeID    uint64
		providerID uint64
		projectID  uint64
		apiTokenID uint64
		clientType string
		model      string
	}

	aggregated := make(map[dimKey]*domain.UsageStats)

	for _, s := range stats {
		key := dimKey{s.RouteID, s.ProviderID, s.ProjectID, s.APITokenID, s.ClientType, s.Model}
		if existing, ok := aggregated[key]; ok {
			existing.TotalRequests += s.TotalRequests
			existing.SuccessfulRequests += s.SuccessfulRequests
			existing.FailedRequests += s.FailedRequests
			existing.TotalDurationMs += s.TotalDurationMs
			existing.InputTokens += s.InputTokens
			existing.OutputTokens += s.OutputTokens
			existing.CacheRead += s.CacheRead
			existing.CacheWrite += s.CacheWrite
			existing.Cost += s.Cost
		} else {
			aggregated[key] = &domain.UsageStats{
				TimeBucket:         targetBucket,
				Granularity:        granularity,
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
				InputTokens:        s.InputTokens,
				OutputTokens:       s.OutputTokens,
				CacheRead:          s.CacheRead,
				CacheWrite:         s.CacheWrite,
				Cost:               s.Cost,
			}
		}
	}

	result := make([]*domain.UsageStats, 0, len(aggregated))
	for _, s := range aggregated {
		result = append(result, s)
	}
	return result
}

// mergeCurrentBucketStats 将当前时间桶的聚合数据合并到结果中（替换预聚合数据）
func (r *UsageStatsRepository) mergeCurrentBucketStats(
	results []*domain.UsageStats,
	currentBucketStats []*domain.UsageStats,
	targetBucket time.Time,
	granularity domain.Granularity,
) []*domain.UsageStats {
	// 移除结果中已有的当前时间桶数据（预聚合的可能不完整）
	filtered := make([]*domain.UsageStats, 0, len(results))
	for _, s := range results {
		if !(s.TimeBucket.Equal(targetBucket) && s.Granularity == granularity) {
			filtered = append(filtered, s)
		}
	}

	// 将当前时间桶数据添加到最前面
	return append(currentBucketStats, filtered...)
}

// queryRecentMinutesStats 查询最近 2 分钟的实时统计数据
// 只查询已完成的请求，使用 end_time 作为时间条件
func (r *UsageStatsRepository) queryRecentMinutesStats(startMinute time.Time, filter repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	var conditions []string
	var args []interface{}

	// 从 startMinute 到当前时间（最近 2 分钟），只查询已完成的请求
	conditions = append(conditions, "a.end_time >= ?")
	args = append(args, toTimestamp(startMinute))
	conditions = append(conditions, "a.status IN ('COMPLETED', 'FAILED', 'CANCELLED')")

	if filter.RouteID != nil {
		conditions = append(conditions, "r.route_id = ?")
		args = append(args, *filter.RouteID)
	}
	if filter.ProviderID != nil {
		conditions = append(conditions, "a.provider_id = ?")
		args = append(args, *filter.ProviderID)
	}
	if filter.ProjectID != nil {
		conditions = append(conditions, "r.project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.ClientType != nil {
		conditions = append(conditions, "r.client_type = ?")
		args = append(args, *filter.ClientType)
	}
	if filter.APITokenID != nil {
		conditions = append(conditions, "r.api_token_id = ?")
		args = append(args, *filter.APITokenID)
	}
	if filter.Model != nil {
		conditions = append(conditions, "a.response_model = ?")
		args = append(args, *filter.Model)
	}

	query := `
		SELECT
			COALESCE(r.route_id, 0), COALESCE(a.provider_id, 0),
			COALESCE(r.project_id, 0), COALESCE(r.api_token_id, 0), COALESCE(r.client_type, ''),
			COALESCE(a.response_model, ''),
			COUNT(*),
			SUM(CASE WHEN a.status = 'COMPLETED' THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.status IN ('FAILED', 'CANCELLED') THEN 1 ELSE 0 END),
			COALESCE(SUM(a.duration_ms), 0),
			COALESCE(SUM(a.input_token_count), 0),
			COALESCE(SUM(a.output_token_count), 0),
			COALESCE(SUM(a.cache_read_count), 0),
			COALESCE(SUM(a.cache_write_count), 0),
			COALESCE(SUM(a.cost), 0)
		FROM proxy_upstream_attempts a
		LEFT JOIN proxy_requests r ON a.proxy_request_id = r.id
		WHERE ` + strings.Join(conditions, " AND ") + `
		GROUP BY r.route_id, a.provider_id, r.project_id, r.api_token_id, r.client_type, a.response_model
	`

	rows, err := r.db.gorm.Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.UsageStats
	for rows.Next() {
		s := &domain.UsageStats{
			TimeBucket:  startMinute, // 会在合并时被替换为目标时间桶
			Granularity: domain.GranularityMinute,
		}
		err := rows.Scan(
			&s.RouteID, &s.ProviderID, &s.ProjectID, &s.APITokenID, &s.ClientType,
			&s.Model,
			&s.TotalRequests, &s.SuccessfulRequests, &s.FailedRequests, &s.TotalDurationMs,
			&s.InputTokens, &s.OutputTokens, &s.CacheRead, &s.CacheWrite, &s.Cost,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetSummary 获取汇总统计数据（总计）
func (r *UsageStatsRepository) GetSummary(filter repository.UsageStatsFilter) (*domain.UsageStatsSummary, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "granularity = ?")
	args = append(args, filter.Granularity)

	if filter.StartTime != nil {
		conditions = append(conditions, "time_bucket >= ?")
		args = append(args, toTimestamp(*filter.StartTime))
	}
	if filter.EndTime != nil {
		conditions = append(conditions, "time_bucket <= ?")
		args = append(args, toTimestamp(*filter.EndTime))
	}
	if filter.RouteID != nil {
		conditions = append(conditions, "route_id = ?")
		args = append(args, *filter.RouteID)
	}
	if filter.ProviderID != nil {
		conditions = append(conditions, "provider_id = ?")
		args = append(args, *filter.ProviderID)
	}
	if filter.ProjectID != nil {
		conditions = append(conditions, "project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.ClientType != nil {
		conditions = append(conditions, "client_type = ?")
		args = append(args, *filter.ClientType)
	}
	if filter.APITokenID != nil {
		conditions = append(conditions, "api_token_id = ?")
		args = append(args, *filter.APITokenID)
	}
	if filter.Model != nil {
		conditions = append(conditions, "model = ?")
		args = append(args, *filter.Model)
	}

	query := `
		SELECT
			COALESCE(SUM(total_requests), 0),
			COALESCE(SUM(successful_requests), 0),
			COALESCE(SUM(failed_requests), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read), 0),
			COALESCE(SUM(cache_write), 0),
			COALESCE(SUM(cost), 0)
		FROM usage_stats
		WHERE ` + strings.Join(conditions, " AND ")

	var s domain.UsageStatsSummary
	err := r.db.gorm.Raw(query, args...).Row().Scan(
		&s.TotalRequests, &s.SuccessfulRequests, &s.FailedRequests,
		&s.TotalInputTokens, &s.TotalOutputTokens,
		&s.TotalCacheRead, &s.TotalCacheWrite, &s.TotalCost,
	)
	if err != nil {
		return nil, err
	}
	if s.TotalRequests > 0 {
		s.SuccessRate = float64(s.SuccessfulRequests) / float64(s.TotalRequests) * 100
	}
	return &s, nil
}

// GetSummaryByProvider 按 Provider 维度获取汇总统计
func (r *UsageStatsRepository) GetSummaryByProvider(filter repository.UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error) {
	return r.getSummaryByDimension(filter, "provider_id")
}

// GetSummaryByRoute 按 Route 维度获取汇总统计
func (r *UsageStatsRepository) GetSummaryByRoute(filter repository.UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error) {
	return r.getSummaryByDimension(filter, "route_id")
}

// GetSummaryByProject 按 Project 维度获取汇总统计
func (r *UsageStatsRepository) GetSummaryByProject(filter repository.UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error) {
	return r.getSummaryByDimension(filter, "project_id")
}

// GetSummaryByAPIToken 按 APIToken 维度获取汇总统计
func (r *UsageStatsRepository) GetSummaryByAPIToken(filter repository.UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error) {
	return r.getSummaryByDimension(filter, "api_token_id")
}

// getSummaryByDimension 通用的按维度聚合方法
func (r *UsageStatsRepository) getSummaryByDimension(filter repository.UsageStatsFilter, dimension string) (map[uint64]*domain.UsageStatsSummary, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "granularity = ?")
	args = append(args, filter.Granularity)

	if filter.StartTime != nil {
		conditions = append(conditions, "time_bucket >= ?")
		args = append(args, toTimestamp(*filter.StartTime))
	}
	if filter.EndTime != nil {
		conditions = append(conditions, "time_bucket <= ?")
		args = append(args, toTimestamp(*filter.EndTime))
	}
	if filter.RouteID != nil {
		conditions = append(conditions, "route_id = ?")
		args = append(args, *filter.RouteID)
	}
	if filter.ProviderID != nil {
		conditions = append(conditions, "provider_id = ?")
		args = append(args, *filter.ProviderID)
	}
	if filter.ProjectID != nil {
		conditions = append(conditions, "project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.ClientType != nil {
		conditions = append(conditions, "client_type = ?")
		args = append(args, *filter.ClientType)
	}
	if filter.APITokenID != nil {
		conditions = append(conditions, "api_token_id = ?")
		args = append(args, *filter.APITokenID)
	}
	if filter.Model != nil {
		conditions = append(conditions, "model = ?")
		args = append(args, *filter.Model)
	}

	query := fmt.Sprintf(`
		SELECT
			%s,
			COALESCE(SUM(total_requests), 0),
			COALESCE(SUM(successful_requests), 0),
			COALESCE(SUM(failed_requests), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read), 0),
			COALESCE(SUM(cache_write), 0),
			COALESCE(SUM(cost), 0)
		FROM usage_stats
		WHERE %s
		GROUP BY %s
	`, dimension, strings.Join(conditions, " AND "), dimension)

	rows, err := r.db.gorm.Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[uint64]*domain.UsageStatsSummary)
	for rows.Next() {
		var dimID uint64
		var s domain.UsageStatsSummary
		err := rows.Scan(
			&dimID,
			&s.TotalRequests, &s.SuccessfulRequests, &s.FailedRequests,
			&s.TotalInputTokens, &s.TotalOutputTokens,
			&s.TotalCacheRead, &s.TotalCacheWrite, &s.TotalCost,
		)
		if err != nil {
			return nil, err
		}
		if s.TotalRequests > 0 {
			s.SuccessRate = float64(s.SuccessfulRequests) / float64(s.TotalRequests) * 100
		}
		results[dimID] = &s
	}
	return results, rows.Err()
}

// GetSummaryByClientType 按 ClientType 维度获取汇总统计
func (r *UsageStatsRepository) GetSummaryByClientType(filter repository.UsageStatsFilter) (map[string]*domain.UsageStatsSummary, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "granularity = ?")
	args = append(args, filter.Granularity)

	if filter.StartTime != nil {
		conditions = append(conditions, "time_bucket >= ?")
		args = append(args, toTimestamp(*filter.StartTime))
	}
	if filter.EndTime != nil {
		conditions = append(conditions, "time_bucket <= ?")
		args = append(args, toTimestamp(*filter.EndTime))
	}
	if filter.RouteID != nil {
		conditions = append(conditions, "route_id = ?")
		args = append(args, *filter.RouteID)
	}
	if filter.ProviderID != nil {
		conditions = append(conditions, "provider_id = ?")
		args = append(args, *filter.ProviderID)
	}
	if filter.ProjectID != nil {
		conditions = append(conditions, "project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.ClientType != nil {
		conditions = append(conditions, "client_type = ?")
		args = append(args, *filter.ClientType)
	}
	if filter.APITokenID != nil {
		conditions = append(conditions, "api_token_id = ?")
		args = append(args, *filter.APITokenID)
	}
	if filter.Model != nil {
		conditions = append(conditions, "model = ?")
		args = append(args, *filter.Model)
	}

	query := `
		SELECT
			client_type,
			COALESCE(SUM(total_requests), 0),
			COALESCE(SUM(successful_requests), 0),
			COALESCE(SUM(failed_requests), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read), 0),
			COALESCE(SUM(cache_write), 0),
			COALESCE(SUM(cost), 0)
		FROM usage_stats
		WHERE ` + strings.Join(conditions, " AND ") + `
		GROUP BY client_type
	`

	rows, err := r.db.gorm.Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]*domain.UsageStatsSummary)
	for rows.Next() {
		var clientType string
		var s domain.UsageStatsSummary
		err := rows.Scan(
			&clientType,
			&s.TotalRequests, &s.SuccessfulRequests, &s.FailedRequests,
			&s.TotalInputTokens, &s.TotalOutputTokens,
			&s.TotalCacheRead, &s.TotalCacheWrite, &s.TotalCost,
		)
		if err != nil {
			return nil, err
		}
		if s.TotalRequests > 0 {
			s.SuccessRate = float64(s.SuccessfulRequests) / float64(s.TotalRequests) * 100
		}
		results[clientType] = &s
	}
	return results, rows.Err()
}

// DeleteOlderThan 删除指定粒度下指定时间之前的统计记录
func (r *UsageStatsRepository) DeleteOlderThan(granularity domain.Granularity, before time.Time) (int64, error) {
	result := r.db.gorm.Where("granularity = ? AND time_bucket < ?", granularity, toTimestamp(before)).Delete(&UsageStats{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// GetLatestTimeBucket 获取指定粒度的最新时间桶
func (r *UsageStatsRepository) GetLatestTimeBucket(granularity domain.Granularity) (*time.Time, error) {
	var bucket *int64
	err := r.db.gorm.Model(&UsageStats{}).
		Select("MAX(time_bucket)").
		Where("granularity = ?", granularity).
		Scan(&bucket).Error
	if err != nil || bucket == nil || *bucket == 0 {
		return nil, err
	}

	t := fromTimestamp(*bucket)
	return &t, nil
}

// GetProviderStats 获取 Provider 统计数据
func (r *UsageStatsRepository) GetProviderStats(clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error) {
	stats := make(map[uint64]*domain.ProviderStats)

	conditions := []string{"provider_id > 0"}
	var args []any

	if clientType != "" {
		conditions = append(conditions, "client_type = ?")
		args = append(args, clientType)
	}
	if projectID > 0 {
		conditions = append(conditions, "project_id = ?")
		args = append(args, projectID)
	}

	query := `
		SELECT
			provider_id,
			COALESCE(SUM(total_requests), 0),
			COALESCE(SUM(successful_requests), 0),
			COALESCE(SUM(failed_requests), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read), 0),
			COALESCE(SUM(cache_write), 0),
			COALESCE(SUM(cost), 0)
		FROM usage_stats
		WHERE ` + strings.Join(conditions, " AND ") + `
		GROUP BY provider_id
	`

	rows, err := r.db.gorm.Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s domain.ProviderStats
		err := rows.Scan(
			&s.ProviderID,
			&s.TotalRequests,
			&s.SuccessfulRequests,
			&s.FailedRequests,
			&s.TotalInputTokens,
			&s.TotalOutputTokens,
			&s.TotalCacheRead,
			&s.TotalCacheWrite,
			&s.TotalCost,
		)
		if err != nil {
			return nil, err
		}
		if s.TotalRequests > 0 {
			s.SuccessRate = float64(s.SuccessfulRequests) / float64(s.TotalRequests) * 100
		}
		stats[s.ProviderID] = &s
	}

	return stats, rows.Err()
}

// AggregateMinute 从原始数据聚合到分钟级别
// 只聚合已完成的请求（COMPLETED/FAILED/CANCELLED），使用 end_time 作为时间桶
func (r *UsageStatsRepository) AggregateMinute() (int, error) {
	now := time.Now().UTC()
	currentMinute := now.Truncate(time.Minute)

	// 获取最新的聚合分钟
	latestMinute, err := r.GetLatestTimeBucket(domain.GranularityMinute)
	var startTime time.Time
	if err != nil || latestMinute == nil {
		// 如果没有历史数据，从 2 小时前开始
		startTime = now.Add(-2 * time.Hour).Truncate(time.Minute)
	} else {
		// 从最新记录前 2 分钟开始，确保补齐延迟数据
		startTime = latestMinute.Add(-2 * time.Minute)
	}

	// 查询在时间范围内已完成的 proxy_upstream_attempts
	// 使用 end_time 作为时间桶，确保请求在完成后才被计入
	query := `
		SELECT
			a.end_time,
			COALESCE(r.route_id, 0), COALESCE(a.provider_id, 0),
			COALESCE(r.project_id, 0), COALESCE(r.api_token_id, 0), COALESCE(r.client_type, ''),
			COALESCE(a.response_model, ''),
			CASE WHEN a.status = 'COMPLETED' THEN 1 ELSE 0 END,
			CASE WHEN a.status IN ('FAILED', 'CANCELLED') THEN 1 ELSE 0 END,
			COALESCE(a.duration_ms, 0),
			COALESCE(a.input_token_count, 0),
			COALESCE(a.output_token_count, 0),
			COALESCE(a.cache_read_count, 0),
			COALESCE(a.cache_write_count, 0),
			COALESCE(a.cost, 0)
		FROM proxy_upstream_attempts a
		LEFT JOIN proxy_requests r ON a.proxy_request_id = r.id
		WHERE a.end_time >= ? AND a.end_time < ?
		AND a.status IN ('COMPLETED', 'FAILED', 'CANCELLED')
	`

	rows, err := r.db.gorm.Raw(query, toTimestamp(startTime), toTimestamp(currentMinute)).Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	// 使用 map 聚合数据
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
	responseModels := make(map[string]bool)

	for rows.Next() {
		var endTime int64
		var routeID, providerID, projectID, apiTokenID uint64
		var clientType, model string
		var successful, failed int
		var durationMs, inputTokens, outputTokens, cacheRead, cacheWrite, cost uint64

		err := rows.Scan(
			&endTime, &routeID, &providerID, &projectID, &apiTokenID, &clientType,
			&model,
			&successful, &failed, &durationMs,
			&inputTokens, &outputTokens, &cacheRead, &cacheWrite, &cost,
		)
		if err != nil {
			continue
		}

		// 记录 response model
		if model != "" {
			responseModels[model] = true
		}

		// 截断到分钟（使用 end_time）
		minuteBucket := fromTimestamp(endTime).Truncate(time.Minute).UnixMilli()

		key := aggKey{
			minuteBucket: minuteBucket,
			routeID:      routeID,
			providerID:   providerID,
			projectID:    projectID,
			apiTokenID:   apiTokenID,
			clientType:   clientType,
			model:        model,
		}

		if s, ok := statsMap[key]; ok {
			s.TotalRequests++
			s.SuccessfulRequests += uint64(successful)
			s.FailedRequests += uint64(failed)
			s.TotalDurationMs += durationMs
			s.InputTokens += inputTokens
			s.OutputTokens += outputTokens
			s.CacheRead += cacheRead
			s.CacheWrite += cacheWrite
			s.Cost += cost
		} else {
			statsMap[key] = &domain.UsageStats{
				Granularity:        domain.GranularityMinute,
				TimeBucket:         time.UnixMilli(minuteBucket),
				RouteID:            routeID,
				ProviderID:         providerID,
				ProjectID:          projectID,
				APITokenID:         apiTokenID,
				ClientType:         clientType,
				Model:              model,
				TotalRequests:      1,
				SuccessfulRequests: uint64(successful),
				FailedRequests:     uint64(failed),
				TotalDurationMs:    durationMs,
				InputTokens:        inputTokens,
				OutputTokens:       outputTokens,
				CacheRead:          cacheRead,
				CacheWrite:         cacheWrite,
				Cost:               cost,
			}
		}
	}

	// 记录 response models 到独立表
	if len(responseModels) > 0 {
		models := make([]string, 0, len(responseModels))
		for m := range responseModels {
			models = append(models, m)
		}
		responseModelRepo := NewResponseModelRepository(r.db)
		_ = responseModelRepo.BatchUpsert(models)
	}

	if len(statsMap) == 0 {
		return 0, nil
	}

	statsList := make([]*domain.UsageStats, 0, len(statsMap))
	for _, s := range statsMap {
		statsList = append(statsList, s)
	}

	return len(statsList), r.BatchUpsert(statsList)
}

// RollUp 从细粒度上卷到粗粒度
// 对于 day/week/month 粒度，使用配置的时区来划分边界
func (r *UsageStatsRepository) RollUp(from, to domain.Granularity) (int, error) {
	now := time.Now().UTC()

	// 对于 day 及以上粒度，使用配置的时区
	var loc *time.Location
	if to == domain.GranularityDay || to == domain.GranularityWeek || to == domain.GranularityMonth {
		loc = r.getConfiguredTimezone()
	}

	// 计算当前时间桶
	var currentBucket time.Time
	if loc != nil {
		currentBucket = TruncateToGranularityInTimezone(now, to, loc)
	} else {
		currentBucket = TruncateToGranularity(now, to)
	}

	// 获取目标粒度的最新时间桶
	latestBucket, _ := r.GetLatestTimeBucket(to)
	var startTime time.Time
	if latestBucket == nil {
		// 如果没有历史数据，根据源粒度的保留时间决定
		switch from {
		case domain.GranularityMinute:
			startTime = now.Add(-2 * time.Hour)
		case domain.GranularityHour:
			startTime = now.Add(-7 * 24 * time.Hour)
		case domain.GranularityDay:
			startTime = now.Add(-90 * 24 * time.Hour)
		default:
			startTime = now.Add(-30 * 24 * time.Hour)
		}
	} else {
		startTime = *latestBucket
	}

	// 查询源粒度数据
	var models []UsageStats
	err := r.db.gorm.Where("granularity = ? AND time_bucket >= ? AND time_bucket < ?",
		from, toTimestamp(startTime), toTimestamp(currentBucket)).
		Find(&models).Error
	if err != nil {
		return 0, err
	}

	// 使用 map 聚合数据
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

	for _, m := range models {
		// 截断到目标粒度（使用配置的时区）
		t := fromTimestamp(m.TimeBucket)
		var targetBucket int64
		if loc != nil {
			targetBucket = TruncateToGranularityInTimezone(t, to, loc).UnixMilli()
		} else {
			targetBucket = TruncateToGranularity(t, to).UnixMilli()
		}

		key := rollupKey{
			targetBucket: targetBucket,
			routeID:      m.RouteID,
			providerID:   m.ProviderID,
			projectID:    m.ProjectID,
			apiTokenID:   m.APITokenID,
			clientType:   m.ClientType,
			model:        m.Model,
		}

		if s, ok := statsMap[key]; ok {
			s.TotalRequests += m.TotalRequests
			s.SuccessfulRequests += m.SuccessfulRequests
			s.FailedRequests += m.FailedRequests
			s.TotalDurationMs += m.TotalDurationMs
			s.InputTokens += m.InputTokens
			s.OutputTokens += m.OutputTokens
			s.CacheRead += m.CacheRead
			s.CacheWrite += m.CacheWrite
			s.Cost += m.Cost
		} else {
			statsMap[key] = &domain.UsageStats{
				Granularity:        to,
				TimeBucket:         time.UnixMilli(targetBucket),
				RouteID:            m.RouteID,
				ProviderID:         m.ProviderID,
				ProjectID:          m.ProjectID,
				APITokenID:         m.APITokenID,
				ClientType:         m.ClientType,
				Model:              m.Model,
				TotalRequests:      m.TotalRequests,
				SuccessfulRequests: m.SuccessfulRequests,
				FailedRequests:     m.FailedRequests,
				TotalDurationMs:    m.TotalDurationMs,
				InputTokens:        m.InputTokens,
				OutputTokens:       m.OutputTokens,
				CacheRead:          m.CacheRead,
				CacheWrite:         m.CacheWrite,
				Cost:               m.Cost,
			}
		}
	}

	if len(statsMap) == 0 {
		return 0, nil
	}

	statsList := make([]*domain.UsageStats, 0, len(statsMap))
	for _, s := range statsMap {
		statsList = append(statsList, s)
	}

	return len(statsList), r.BatchUpsert(statsList)
}

// RollUpAll 从细粒度上卷到粗粒度（处理所有历史数据，用于重新计算）
// 对于 day/week/month 粒度，使用配置的时区来划分边界
func (r *UsageStatsRepository) RollUpAll(from, to domain.Granularity) (int, error) {
	now := time.Now().UTC()

	// 对于 day 及以上粒度，使用配置的时区
	var loc *time.Location
	if to == domain.GranularityDay || to == domain.GranularityWeek || to == domain.GranularityMonth {
		loc = r.getConfiguredTimezone()
	}

	// 计算当前时间桶
	var currentBucket time.Time
	if loc != nil {
		currentBucket = TruncateToGranularityInTimezone(now, to, loc)
	} else {
		currentBucket = TruncateToGranularity(now, to)
	}

	// 查询所有源粒度数据
	var models []UsageStats
	err := r.db.gorm.Where("granularity = ? AND time_bucket < ?", from, toTimestamp(currentBucket)).
		Find(&models).Error
	if err != nil {
		return 0, err
	}

	// 使用 map 聚合数据
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

	for _, m := range models {
		// 截断到目标粒度（使用配置的时区）
		t := fromTimestamp(m.TimeBucket)
		var targetBucket int64
		if loc != nil {
			targetBucket = TruncateToGranularityInTimezone(t, to, loc).UnixMilli()
		} else {
			targetBucket = TruncateToGranularity(t, to).UnixMilli()
		}

		key := rollupKey{
			targetBucket: targetBucket,
			routeID:      m.RouteID,
			providerID:   m.ProviderID,
			projectID:    m.ProjectID,
			apiTokenID:   m.APITokenID,
			clientType:   m.ClientType,
			model:        m.Model,
		}

		if s, ok := statsMap[key]; ok {
			s.TotalRequests += m.TotalRequests
			s.SuccessfulRequests += m.SuccessfulRequests
			s.FailedRequests += m.FailedRequests
			s.TotalDurationMs += m.TotalDurationMs
			s.InputTokens += m.InputTokens
			s.OutputTokens += m.OutputTokens
			s.CacheRead += m.CacheRead
			s.CacheWrite += m.CacheWrite
			s.Cost += m.Cost
		} else {
			statsMap[key] = &domain.UsageStats{
				Granularity:        to,
				TimeBucket:         time.UnixMilli(targetBucket),
				RouteID:            m.RouteID,
				ProviderID:         m.ProviderID,
				ProjectID:          m.ProjectID,
				APITokenID:         m.APITokenID,
				ClientType:         m.ClientType,
				Model:              m.Model,
				TotalRequests:      m.TotalRequests,
				SuccessfulRequests: m.SuccessfulRequests,
				FailedRequests:     m.FailedRequests,
				TotalDurationMs:    m.TotalDurationMs,
				InputTokens:        m.InputTokens,
				OutputTokens:       m.OutputTokens,
				CacheRead:          m.CacheRead,
				CacheWrite:         m.CacheWrite,
				Cost:               m.Cost,
			}
		}
	}

	if len(statsMap) == 0 {
		return 0, nil
	}

	statsList := make([]*domain.UsageStats, 0, len(statsMap))
	for _, s := range statsMap {
		statsList = append(statsList, s)
	}

	return len(statsList), r.BatchUpsert(statsList)
}

// ClearAndRecalculate 清空统计数据并重新从原始数据计算
func (r *UsageStatsRepository) ClearAndRecalculate() error {
	// 1. 清空所有统计数据
	if err := r.db.gorm.Exec(`DELETE FROM usage_stats`).Error; err != nil {
		return fmt.Errorf("failed to clear usage_stats: %w", err)
	}

	// 2. 重新聚合分钟级数据（从所有历史数据）
	_, err := r.aggregateAllMinutes()
	if err != nil {
		return fmt.Errorf("failed to aggregate minutes: %w", err)
	}

	// 3. Roll-up 到各个粒度（使用完整时间范围）
	_, _ = r.RollUpAll(domain.GranularityMinute, domain.GranularityHour)
	_, _ = r.RollUpAll(domain.GranularityHour, domain.GranularityDay)
	_, _ = r.RollUpAll(domain.GranularityDay, domain.GranularityWeek)
	_, _ = r.RollUpAll(domain.GranularityDay, domain.GranularityMonth)

	return nil
}

// aggregateAllMinutes 从所有历史数据聚合分钟级统计
// 只聚合已完成的请求，使用 end_time 作为时间桶
func (r *UsageStatsRepository) aggregateAllMinutes() (int, error) {
	now := time.Now().UTC()
	currentMinute := now.Truncate(time.Minute)

	query := `
		SELECT
			a.end_time,
			COALESCE(r.route_id, 0), COALESCE(a.provider_id, 0),
			COALESCE(r.project_id, 0), COALESCE(r.api_token_id, 0), COALESCE(r.client_type, ''),
			COALESCE(a.response_model, ''),
			CASE WHEN a.status = 'COMPLETED' THEN 1 ELSE 0 END,
			CASE WHEN a.status IN ('FAILED', 'CANCELLED') THEN 1 ELSE 0 END,
			COALESCE(a.duration_ms, 0),
			COALESCE(a.input_token_count, 0),
			COALESCE(a.output_token_count, 0),
			COALESCE(a.cache_read_count, 0),
			COALESCE(a.cache_write_count, 0),
			COALESCE(a.cost, 0)
		FROM proxy_upstream_attempts a
		LEFT JOIN proxy_requests r ON a.proxy_request_id = r.id
		WHERE a.end_time < ? AND a.status IN ('COMPLETED', 'FAILED', 'CANCELLED')
	`

	rows, err := r.db.gorm.Raw(query, toTimestamp(currentMinute)).Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	// 使用 map 聚合数据
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
	responseModels := make(map[string]bool)

	for rows.Next() {
		var endTime int64
		var routeID, providerID, projectID, apiTokenID uint64
		var clientType, model string
		var successful, failed int
		var durationMs, inputTokens, outputTokens, cacheRead, cacheWrite, cost uint64

		err := rows.Scan(
			&endTime, &routeID, &providerID, &projectID, &apiTokenID, &clientType,
			&model,
			&successful, &failed, &durationMs,
			&inputTokens, &outputTokens, &cacheRead, &cacheWrite, &cost,
		)
		if err != nil {
			log.Printf("[aggregateAllMinutes] Scan error: %v", err)
			continue
		}

		// 记录 response model
		if model != "" {
			responseModels[model] = true
		}

		// 截断到分钟（使用 end_time）
		minuteBucket := fromTimestamp(endTime).Truncate(time.Minute).UnixMilli()

		key := aggKey{
			minuteBucket: minuteBucket,
			routeID:      routeID,
			providerID:   providerID,
			projectID:    projectID,
			apiTokenID:   apiTokenID,
			clientType:   clientType,
			model:        model,
		}

		if s, ok := statsMap[key]; ok {
			s.TotalRequests++
			s.SuccessfulRequests += uint64(successful)
			s.FailedRequests += uint64(failed)
			s.TotalDurationMs += durationMs
			s.InputTokens += inputTokens
			s.OutputTokens += outputTokens
			s.CacheRead += cacheRead
			s.CacheWrite += cacheWrite
			s.Cost += cost
		} else {
			statsMap[key] = &domain.UsageStats{
				Granularity:        domain.GranularityMinute,
				TimeBucket:         time.UnixMilli(minuteBucket),
				RouteID:            routeID,
				ProviderID:         providerID,
				ProjectID:          projectID,
				APITokenID:         apiTokenID,
				ClientType:         clientType,
				Model:              model,
				TotalRequests:      1,
				SuccessfulRequests: uint64(successful),
				FailedRequests:     uint64(failed),
				TotalDurationMs:    durationMs,
				InputTokens:        inputTokens,
				OutputTokens:       outputTokens,
				CacheRead:          cacheRead,
				CacheWrite:         cacheWrite,
				Cost:               cost,
			}
		}
	}

	// 记录 response models 到独立表
	if len(responseModels) > 0 {
		models := make([]string, 0, len(responseModels))
		for m := range responseModels {
			models = append(models, m)
		}
		responseModelRepo := NewResponseModelRepository(r.db)
		if err := responseModelRepo.BatchUpsert(models); err != nil {
			log.Printf("[aggregateAllMinutes] Failed to upsert response models: %v", err)
		}
	}

	if len(statsMap) == 0 {
		return 0, nil
	}

	statsList := make([]*domain.UsageStats, 0, len(statsMap))
	for _, s := range statsMap {
		statsList = append(statsList, s)
	}

	return len(statsList), r.BatchUpsert(statsList)
}

func (r *UsageStatsRepository) toModel(s *domain.UsageStats) *UsageStats {
	return &UsageStats{
		ID:                 s.ID,
		CreatedAt:          toTimestamp(s.CreatedAt),
		TimeBucket:         toTimestamp(s.TimeBucket),
		Granularity:        string(s.Granularity),
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
		InputTokens:        s.InputTokens,
		OutputTokens:       s.OutputTokens,
		CacheRead:          s.CacheRead,
		CacheWrite:         s.CacheWrite,
		Cost:               s.Cost,
	}
}

func (r *UsageStatsRepository) toDomain(m *UsageStats) *domain.UsageStats {
	return &domain.UsageStats{
		ID:                 m.ID,
		CreatedAt:          fromTimestamp(m.CreatedAt),
		TimeBucket:         fromTimestamp(m.TimeBucket),
		Granularity:        domain.Granularity(m.Granularity),
		RouteID:            m.RouteID,
		ProviderID:         m.ProviderID,
		ProjectID:          m.ProjectID,
		APITokenID:         m.APITokenID,
		ClientType:         m.ClientType,
		Model:              m.Model,
		TotalRequests:      m.TotalRequests,
		SuccessfulRequests: m.SuccessfulRequests,
		FailedRequests:     m.FailedRequests,
		TotalDurationMs:    m.TotalDurationMs,
		InputTokens:        m.InputTokens,
		OutputTokens:       m.OutputTokens,
		CacheRead:          m.CacheRead,
		CacheWrite:         m.CacheWrite,
		Cost:               m.Cost,
	}
}

func (r *UsageStatsRepository) toDomainList(models []UsageStats) []*domain.UsageStats {
	results := make([]*domain.UsageStats, len(models))
	for i, m := range models {
		results[i] = r.toDomain(&m)
	}
	return results
}

// QueryDashboardData 查询 Dashboard 所需的所有数据（单次请求）
// 优化：只执行 3 次主查询
//   1. 历史 day 粒度数据 (371天) → 热力图、昨日、Provider统计(30天)
//   2. 今日实时 hour 粒度 (QueryWithRealtime) → 今日统计、24h趋势、今日热力图
//   3. 全量 month 粒度 (QueryWithRealtime) → 全量统计、Top模型(全量)
func (r *UsageStatsRepository) QueryDashboardData() (*domain.DashboardData, error) {
	// 获取配置的时区
	loc := r.getConfiguredTimezone()
	now := time.Now().In(loc)

	// 使用配置的时区计算今日、昨日等
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.Add(-24 * time.Hour)
	days30Ago := todayStart.Add(-30 * 24 * time.Hour)
	days371Ago := todayStart.Add(-371 * 24 * time.Hour) // 53周

	hours24Ago := now.Add(-24 * time.Hour)

	var (
		mu     sync.Mutex
		result = &domain.DashboardData{
			ProviderStats: make(map[uint64]domain.DashboardProviderStats),
			Timezone:      loc.String(),
		}
		g errgroup.Group
	)

	// 查询1: 历史 day 粒度数据 (371天，不含今天)
	// 用于：热力图历史、昨日统计、Provider统计(30天)
	g.Go(func() error {
		query := `
			SELECT time_bucket, provider_id, model,
				SUM(total_requests), SUM(successful_requests),
				SUM(input_tokens + output_tokens + cache_read + cache_write), SUM(cost)
			FROM usage_stats
			WHERE granularity = 'day'
			AND time_bucket >= ? AND time_bucket < ?
			GROUP BY time_bucket, provider_id, model
		`
		rows, err := r.db.gorm.Raw(query, toTimestamp(days371Ago), toTimestamp(todayStart)).Rows()
		if err != nil {
			return err
		}
		defer rows.Close()

		// 初始化热力图（使用配置的时区格式化日期）
		days := int(now.Sub(days371Ago).Hours()/24) + 1
		heatmapData := make(map[string]uint64, days)
		for i := 0; i < days; i++ {
			date := days371Ago.Add(time.Duration(i) * 24 * time.Hour).In(loc)
			heatmapData[date.Format("2006-01-02")] = 0
		}

		var yesterdaySummary domain.DashboardDaySummary
		providerData := make(map[uint64]*struct {
			requests   uint64
			successful uint64
		})

		for rows.Next() {
			var bucket int64
			var providerID uint64
			var model string
			var requests, successful, tokens, cost uint64
			if err := rows.Scan(&bucket, &providerID, &model, &requests, &successful, &tokens, &cost); err != nil {
				continue
			}

			bucketTime := fromTimestamp(bucket).In(loc)
			dateStr := bucketTime.Format("2006-01-02")

			// 热力图
			heatmapData[dateStr] += requests

			// 昨日统计
			if !bucketTime.Before(yesterdayStart) && bucketTime.Before(todayStart) {
				yesterdaySummary.Requests += requests
				yesterdaySummary.Tokens += tokens
				yesterdaySummary.Cost += cost
			}

			// Provider统计 (30天)
			if !bucketTime.Before(days30Ago) && providerID > 0 {
				if _, ok := providerData[providerID]; !ok {
					providerData[providerID] = &struct {
						requests   uint64
						successful uint64
					}{}
				}
				providerData[providerID].requests += requests
				providerData[providerID].successful += successful
			}
		}

		mu.Lock()
		// 设置昨日
		result.Yesterday = yesterdaySummary

		// 设置 Provider 统计 (30天)
		for providerID, data := range providerData {
			var successRate float64
			if data.requests > 0 {
				successRate = float64(data.successful) / float64(data.requests) * 100
			}
			result.ProviderStats[providerID] = domain.DashboardProviderStats{
				Requests:    data.requests,
				SuccessRate: successRate,
			}
		}

		// 暂存热力图数据（后面会补充今天的）
		result.Heatmap = make([]domain.DashboardHeatmapPoint, 0, days)
		for i := 0; i < days; i++ {
			date := days371Ago.Add(time.Duration(i) * 24 * time.Hour).In(loc)
			dateStr := date.Format("2006-01-02")
			result.Heatmap = append(result.Heatmap, domain.DashboardHeatmapPoint{
				Date:  dateStr,
				Count: heatmapData[dateStr],
			})
		}
		mu.Unlock()
		return nil
	})

	// 查询2: 今日实时 hour 粒度 (QueryWithRealtime)
	// 用于：今日统计、24h趋势、今日热力图、Provider今日RPM/TPM
	g.Go(func() error {
		filter := repository.UsageStatsFilter{
			Granularity: domain.GranularityHour,
			StartTime:   &hours24Ago,
		}
		stats, err := r.QueryWithRealtime(filter)
		if err != nil {
			return err
		}

		// 初始化 24 小时趋势（使用配置的时区）
		hourMap := make(map[string]uint64, 24)
		for i := 0; i < 24; i++ {
			hour := hours24Ago.Add(time.Duration(i) * time.Hour).In(loc).Truncate(time.Hour)
			hourMap[hour.Format("15:04")] = 0
		}

		var todaySummary domain.DashboardDaySummary
		var todaySuccessful uint64
		var todayRequests uint64
		var todayDurationMs uint64

		// Provider 今日统计（用于计算 RPM/TPM）
		providerTodayData := make(map[uint64]*struct {
			requests   uint64
			tokens     uint64
			durationMs uint64
		})

		for _, s := range stats {
			// 24h趋势（使用配置的时区）
			hourStr := s.TimeBucket.In(loc).Format("15:04")
			hourMap[hourStr] += s.TotalRequests

			// 今日统计（只统计今天的数据）
			if !s.TimeBucket.Before(todayStart) {
				todaySummary.Requests += s.TotalRequests
				todaySuccessful += s.SuccessfulRequests
				todaySummary.Tokens += s.InputTokens + s.OutputTokens + s.CacheRead + s.CacheWrite
				todaySummary.Cost += s.Cost
				todayRequests += s.TotalRequests
				todayDurationMs += s.TotalDurationMs

				// Provider 今日数据
				if s.ProviderID > 0 {
					if _, ok := providerTodayData[s.ProviderID]; !ok {
						providerTodayData[s.ProviderID] = &struct {
							requests   uint64
							tokens     uint64
							durationMs uint64
						}{}
					}
					providerTodayData[s.ProviderID].requests += s.TotalRequests
					providerTodayData[s.ProviderID].tokens += s.InputTokens + s.OutputTokens + s.CacheRead + s.CacheWrite
					providerTodayData[s.ProviderID].durationMs += s.TotalDurationMs
				}
			}
		}

		if todaySummary.Requests > 0 {
			todaySummary.SuccessRate = float64(todaySuccessful) / float64(todaySummary.Requests) * 100
		}

		// 计算 RPM 和 TPM（基于请求处理总时间）
		// RPM = (totalRequests / totalDurationMs) * 60000
		// TPM = (totalTokens / totalDurationMs) * 60000
		if todayDurationMs > 0 {
			todaySummary.RPM = (float64(todaySummary.Requests) / float64(todayDurationMs)) * 60000
			todaySummary.TPM = (float64(todaySummary.Tokens) / float64(todayDurationMs)) * 60000
		}

		// 构建24h趋势数组（使用配置的时区）
		trend := make([]domain.DashboardTrendPoint, 0, 24)
		for i := 0; i < 24; i++ {
			hour := hours24Ago.Add(time.Duration(i) * time.Hour).In(loc).Truncate(time.Hour)
			hourStr := hour.Format("15:04")
			trend = append(trend, domain.DashboardTrendPoint{
				Hour:     hourStr,
				Requests: hourMap[hourStr],
			})
		}

		mu.Lock()
		result.Today = todaySummary
		result.Trend24h = trend

		// 补充今日热力图
		if len(result.Heatmap) > 0 {
			todayDateStr := todayStart.Format("2006-01-02")
			for i := range result.Heatmap {
				if result.Heatmap[i].Date == todayDateStr {
					result.Heatmap[i].Count = todayRequests
					break
				}
			}
		}

		// 补充 Provider 今日 RPM/TPM
		for providerID, data := range providerTodayData {
			if data.durationMs > 0 {
				rpm := (float64(data.requests) / float64(data.durationMs)) * 60000
				tpm := (float64(data.tokens) / float64(data.durationMs)) * 60000
				if existing, ok := result.ProviderStats[providerID]; ok {
					existing.RPM = rpm
					existing.TPM = tpm
					result.ProviderStats[providerID] = existing
				} else {
					// 如果 Provider 只有今天的数据（30天统计中没有）
					result.ProviderStats[providerID] = domain.DashboardProviderStats{
						Requests: data.requests,
						RPM:      rpm,
						TPM:      tpm,
					}
				}
			}
		}
		mu.Unlock()
		return nil
	})

	// 查询3: 全量 month 粒度 (QueryWithRealtime)
	// 用于：全量统计、Top模型(全量)
	g.Go(func() error {
		filter := repository.UsageStatsFilter{
			Granularity: domain.GranularityMonth,
		}
		stats, err := r.QueryWithRealtime(filter)
		if err != nil {
			return err
		}

		var allTimeSummary domain.DashboardAllTimeSummary
		modelData := make(map[string]*struct {
			requests uint64
			tokens   uint64
		})

		for _, s := range stats {
			allTimeSummary.Requests += s.TotalRequests
			allTimeSummary.Tokens += s.InputTokens + s.OutputTokens + s.CacheRead + s.CacheWrite
			allTimeSummary.Cost += s.Cost

			// Top模型（全量）
			if s.Model != "" {
				tokens := s.InputTokens + s.OutputTokens + s.CacheRead + s.CacheWrite
				if _, ok := modelData[s.Model]; !ok {
					modelData[s.Model] = &struct {
						requests uint64
						tokens   uint64
					}{}
				}
				modelData[s.Model].requests += s.TotalRequests
				modelData[s.Model].tokens += tokens
			}
		}

		// 从 proxy_requests 表获取真正的首次使用时间
		var firstRequestTime *int64
		err = r.db.gorm.Raw("SELECT MIN(created_at) FROM proxy_requests").Scan(&firstRequestTime).Error
		if err == nil && firstRequestTime != nil && *firstRequestTime > 0 {
			firstUse := fromTimestamp(*firstRequestTime)
			allTimeSummary.FirstUseDate = &firstUse
			allTimeSummary.DaysSinceFirstUse = int(now.Sub(firstUse).Hours() / 24)
		}

		mu.Lock()
		result.AllTime = allTimeSummary
		result.TopModels = r.getTopModels(modelData, 3)
		mu.Unlock()
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

// getTopModels 从 model->stats map 中提取 Top N 模型
func (r *UsageStatsRepository) getTopModels(modelData map[string]*struct {
	requests uint64
	tokens   uint64
}, limit int) []domain.DashboardModelStats {
	// 转换为切片并排序
	type modelReq struct {
		model    string
		requests uint64
		tokens   uint64
	}
	models := make([]modelReq, 0, len(modelData))
	for model, data := range modelData {
		models = append(models, modelReq{model, data.requests, data.tokens})
	}

	// 按请求数降序排序
	for i := 0; i < len(models)-1; i++ {
		for j := i + 1; j < len(models); j++ {
			if models[j].requests > models[i].requests {
				models[i], models[j] = models[j], models[i]
			}
		}
	}

	// 取前 N 个
	result := make([]domain.DashboardModelStats, 0, limit)
	for i := 0; i < len(models) && i < limit; i++ {
		result = append(result, domain.DashboardModelStats{
			Model:    models[i].model,
			Requests: models[i].requests,
			Tokens:   models[i].tokens,
		})
	}
	return result
}

// aggregateToSummary 将 UsageStats 列表聚合为 DashboardDaySummary
func (r *UsageStatsRepository) aggregateToSummary(stats []*domain.UsageStats) domain.DashboardDaySummary {
	var result domain.DashboardDaySummary
	var successfulRequests uint64

	for _, s := range stats {
		result.Requests += s.TotalRequests
		successfulRequests += s.SuccessfulRequests
		result.Tokens += s.InputTokens + s.OutputTokens + s.CacheRead + s.CacheWrite
		result.Cost += s.Cost
	}

	if result.Requests > 0 {
		result.SuccessRate = float64(successfulRequests) / float64(result.Requests) * 100
	}

	return result
}

// statsToHeatmap 将 UsageStats 列表转换为热力图数据
func (r *UsageStatsRepository) statsToHeatmap(stats []*domain.UsageStats, start, end time.Time) []domain.DashboardHeatmapPoint {
	// 初始化所有日期
	days := int(end.Sub(start).Hours() / 24)
	dateMap := make(map[string]uint64, days)
	for i := 0; i < days; i++ {
		date := start.Add(time.Duration(i) * 24 * time.Hour)
		dateStr := date.Format("2006-01-02")
		dateMap[dateStr] = 0
	}

	// 按天聚合
	for _, s := range stats {
		dateStr := s.TimeBucket.Format("2006-01-02")
		dateMap[dateStr] += s.TotalRequests
	}

	// 转换为有序数组
	result := make([]domain.DashboardHeatmapPoint, 0, days)
	for i := 0; i < days; i++ {
		date := start.Add(time.Duration(i) * 24 * time.Hour)
		dateStr := date.Format("2006-01-02")
		result = append(result, domain.DashboardHeatmapPoint{
			Date:  dateStr,
			Count: dateMap[dateStr],
		})
	}

	return result
}

// statsToTrend24h 将 UsageStats 列表转换为 24 小时趋势数据
func (r *UsageStatsRepository) statsToTrend24h(stats []*domain.UsageStats, start, end time.Time) []domain.DashboardTrendPoint {
	// 初始化 24 小时
	hourMap := make(map[string]uint64, 24)
	for i := 0; i < 24; i++ {
		hour := start.Add(time.Duration(i) * time.Hour).Truncate(time.Hour)
		hourStr := hour.Format("15:04")
		hourMap[hourStr] = 0
	}

	// 按小时聚合
	for _, s := range stats {
		hourStr := s.TimeBucket.Format("15:04")
		hourMap[hourStr] += s.TotalRequests
	}

	// 转换为有序数组
	result := make([]domain.DashboardTrendPoint, 0, 24)
	for i := 0; i < 24; i++ {
		hour := start.Add(time.Duration(i) * time.Hour).Truncate(time.Hour)
		hourStr := hour.Format("15:04")
		result = append(result, domain.DashboardTrendPoint{
			Hour:     hourStr,
			Requests: hourMap[hourStr],
		})
	}

	return result
}

// statsToProviderStats 将 UsageStats 列表转换为 Provider 统计
func (r *UsageStatsRepository) statsToProviderStats(stats []*domain.UsageStats) map[uint64]domain.DashboardProviderStats {
	// 按 Provider 聚合
	providerMap := make(map[uint64]*struct {
		requests   uint64
		successful uint64
	})

	for _, s := range stats {
		if s.ProviderID == 0 {
			continue
		}
		if _, ok := providerMap[s.ProviderID]; !ok {
			providerMap[s.ProviderID] = &struct {
				requests   uint64
				successful uint64
			}{}
		}
		providerMap[s.ProviderID].requests += s.TotalRequests
		providerMap[s.ProviderID].successful += s.SuccessfulRequests
	}

	// 转换为结果
	result := make(map[uint64]domain.DashboardProviderStats)
	for providerID, data := range providerMap {
		var successRate float64
		if data.requests > 0 {
			successRate = float64(data.successful) / float64(data.requests) * 100
		}
		result[providerID] = domain.DashboardProviderStats{
			Requests:    data.requests,
			SuccessRate: successRate,
		}
	}

	return result
}

// queryDashboardAllTimeStats 查询全量统计和首次使用日期
func (r *UsageStatsRepository) queryDashboardAllTimeStats() (domain.DashboardDaySummary, *time.Time, error) {
	var result domain.DashboardDaySummary

	// 查询全量统计（使用 month 粒度）
	query := `
		SELECT
			COALESCE(SUM(total_requests), 0),
			COALESCE(SUM(input_tokens + output_tokens + cache_read + cache_write), 0),
			COALESCE(SUM(cost), 0),
			MIN(time_bucket)
		FROM usage_stats
		WHERE granularity = 'month'
	`

	var totalRequests, tokens, cost uint64
	var minBucket *int64
	err := r.db.gorm.Raw(query).Row().Scan(&totalRequests, &tokens, &cost, &minBucket)
	if err != nil {
		return result, nil, err
	}

	result.Requests = totalRequests
	result.Tokens = tokens
	result.Cost = cost

	var firstUse *time.Time
	if minBucket != nil && *minBucket > 0 {
		t := fromTimestamp(*minBucket)
		firstUse = &t
	}

	return result, firstUse, nil
}

// queryDashboardHeatmap 查询热力图数据
// 历史数据用 day 粒度预聚合，今天用 QueryWithRealtime 获取实时数据
func (r *UsageStatsRepository) queryDashboardHeatmap(start, todayStart, end time.Time) ([]domain.DashboardHeatmapPoint, error) {
	// 初始化所有日期
	days := int(end.Sub(start).Hours()/24) + 1
	dateMap := make(map[string]uint64, days)
	for i := 0; i < days; i++ {
		date := start.Add(time.Duration(i) * 24 * time.Hour)
		dateStr := date.Format("2006-01-02")
		dateMap[dateStr] = 0
	}

	// 1. 查询历史天数据（今天之前，使用 day 粒度预聚合）
	if todayStart.After(start) {
		query := `
			SELECT time_bucket, SUM(total_requests) as count
			FROM usage_stats
			WHERE granularity = 'day'
			AND time_bucket >= ? AND time_bucket < ?
			GROUP BY time_bucket
		`
		rows, err := r.db.gorm.Raw(query, toTimestamp(start), toTimestamp(todayStart)).Rows()
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var bucket int64
			var count uint64
			if err := rows.Scan(&bucket, &count); err != nil {
				continue
			}
			dateStr := fromTimestamp(bucket).Format("2006-01-02")
			dateMap[dateStr] += count
		}
	}

	// 2. 查询今天的实时数据（使用 QueryWithRealtime）
	todayFilter := repository.UsageStatsFilter{
		Granularity: domain.GranularityDay,
		StartTime:   &todayStart,
	}
	todayStats, err := r.QueryWithRealtime(todayFilter)
	if err != nil {
		return nil, err
	}

	// 聚合今天的数据
	todayDateStr := todayStart.Format("2006-01-02")
	for _, s := range todayStats {
		dateMap[todayDateStr] += s.TotalRequests
	}

	// 转换为有序数组
	result := make([]domain.DashboardHeatmapPoint, 0, days)
	for i := 0; i < days; i++ {
		date := start.Add(time.Duration(i) * 24 * time.Hour)
		dateStr := date.Format("2006-01-02")
		result = append(result, domain.DashboardHeatmapPoint{
			Date:  dateStr,
			Count: dateMap[dateStr],
		})
	}

	return result, nil
}

// queryDashboardTopModels 查询 Top N 模型
func (r *UsageStatsRepository) queryDashboardTopModels(limit int) ([]domain.DashboardModelStats, error) {
	query := `
		SELECT
			model,
			SUM(total_requests) as requests
		FROM usage_stats
		WHERE granularity = 'month' AND model != ''
		GROUP BY model
		ORDER BY requests DESC
		LIMIT ?
	`

	rows, err := r.db.gorm.Raw(query, limit).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.DashboardModelStats
	for rows.Next() {
		var model string
		var requests uint64
		if err := rows.Scan(&model, &requests); err != nil {
			continue
		}
		result = append(result, domain.DashboardModelStats{
			Model:    model,
			Requests: requests,
		})
	}

	return result, nil
}
