package sqlite

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type UsageStatsRepository struct {
	db *DB
}

func NewUsageStatsRepository(db *DB) *UsageStatsRepository {
	return &UsageStatsRepository{db: db}
}

// Upsert 更新或插入统计记录（直接替换，不累加）
func (r *UsageStatsRepository) Upsert(stats *domain.UsageStats) error {
	now := time.Now()
	stats.CreatedAt = now

	_, err := r.db.Exec(`
		INSERT INTO usage_stats (
			created_at, hour, route_id, provider_id, project_id, api_token_id, client_type,
			total_requests, successful_requests, failed_requests,
			input_tokens, output_tokens, cache_read, cache_write, cost
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, route_id, provider_id, project_id, api_token_id, client_type) DO UPDATE SET
			total_requests = excluded.total_requests,
			successful_requests = excluded.successful_requests,
			failed_requests = excluded.failed_requests,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			cache_read = excluded.cache_read,
			cache_write = excluded.cache_write,
			cost = excluded.cost
	`,
		stats.CreatedAt, stats.Hour, stats.RouteID, stats.ProviderID, stats.ProjectID, stats.APITokenID, stats.ClientType,
		stats.TotalRequests, stats.SuccessfulRequests, stats.FailedRequests,
		stats.InputTokens, stats.OutputTokens, stats.CacheRead, stats.CacheWrite, stats.Cost,
	)
	return err
}

// Query 查询统计数据
func (r *UsageStatsRepository) Query(filter repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	currentHour := time.Now().Truncate(time.Hour)

	// 判断是否需要包含当前小时的实时数据
	needRealtime := filter.EndTime == nil || !filter.EndTime.Before(currentHour)

	var conditions []string
	var args []interface{}

	if filter.StartTime != nil {
		conditions = append(conditions, "hour >= ?")
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		conditions = append(conditions, "hour <= ?")
		args = append(args, *filter.EndTime)
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

	query := `SELECT id, created_at, hour, route_id, provider_id, project_id, api_token_id, client_type,
		total_requests, successful_requests, failed_requests,
		input_tokens, output_tokens, cache_read, cache_write, cost
		FROM usage_stats`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY hour DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.UsageStats
	for rows.Next() {
		var s domain.UsageStats
		err := rows.Scan(
			&s.ID, &s.CreatedAt, &s.Hour, &s.RouteID, &s.ProviderID, &s.ProjectID, &s.APITokenID, &s.ClientType,
			&s.TotalRequests, &s.SuccessfulRequests, &s.FailedRequests,
			&s.InputTokens, &s.OutputTokens, &s.CacheRead, &s.CacheWrite, &s.Cost,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 如果需要当前小时的实时数据，从 proxy_upstream_attempts 查询
	if needRealtime {
		realtimeStats, err := r.queryRealtimeStats(currentHour, filter)
		if err != nil {
			return nil, err
		}
		// 合并实时数据：按 key 替换或新增
		if len(realtimeStats) > 0 {
			// 建立已有数据的索引 (hour + route + provider + project + token + client)
			existingIdx := make(map[string]int)
			for i, s := range results {
				key := statsKey(s)
				existingIdx[key] = i
			}
			// 合并
			for _, rt := range realtimeStats {
				key := statsKey(rt)
				if idx, ok := existingIdx[key]; ok {
					// 替换已有记录
					results[idx] = rt
				} else {
					// 新增到最前面
					results = append([]*domain.UsageStats{rt}, results...)
				}
			}
		}
	}

	return results, nil
}

// statsKey 生成统计数据的唯一键
func statsKey(s *domain.UsageStats) string {
	return fmt.Sprintf("%s_%d_%d_%d_%d_%s",
		s.Hour.Format("2006010215"),
		s.RouteID, s.ProviderID, s.ProjectID, s.APITokenID, s.ClientType)
}

// queryRealtimeStats 从 proxy_upstream_attempts 查询当前小时的实时统计数据
func (r *UsageStatsRepository) queryRealtimeStats(currentHour time.Time, filter repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	var conditions []string
	var args []interface{}

	// 当前小时的时间范围
	nextHour := currentHour.Add(time.Hour)
	conditions = append(conditions, "a.created_at >= ?", "a.created_at < ?")
	args = append(args, currentHour, nextHour)

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

	query := `
		SELECT
			COALESCE(r.route_id, 0), COALESCE(a.provider_id, 0),
			COALESCE(r.project_id, 0), COALESCE(r.api_token_id, 0), COALESCE(r.client_type, ''),
			COUNT(*),
			SUM(CASE WHEN a.status = 'COMPLETED' THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.status IN ('FAILED', 'CANCELLED') THEN 1 ELSE 0 END),
			COALESCE(SUM(a.input_token_count), 0),
			COALESCE(SUM(a.output_token_count), 0),
			COALESCE(SUM(a.cache_read_count), 0),
			COALESCE(SUM(a.cache_write_count), 0),
			COALESCE(SUM(a.cost), 0)
		FROM proxy_upstream_attempts a
		LEFT JOIN proxy_requests r ON a.proxy_request_id = r.id
		WHERE ` + strings.Join(conditions, " AND ") + `
		GROUP BY r.route_id, a.provider_id, r.project_id, r.api_token_id, r.client_type
	`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.UsageStats
	for rows.Next() {
		s := &domain.UsageStats{Hour: currentHour}
		err := rows.Scan(
			&s.RouteID, &s.ProviderID, &s.ProjectID, &s.APITokenID, &s.ClientType,
			&s.TotalRequests, &s.SuccessfulRequests, &s.FailedRequests,
			&s.InputTokens, &s.OutputTokens, &s.CacheRead, &s.CacheWrite, &s.Cost,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// DeleteOlderThan 删除指定时间之前的统计记录
func (r *UsageStatsRepository) DeleteOlderThan(before time.Time) (int64, error) {
	result, err := r.db.Exec(`DELETE FROM usage_stats WHERE hour < ?`, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetLatestHour 获取最新的聚合小时
func (r *UsageStatsRepository) GetLatestHour() (*time.Time, error) {
	rows, err := r.db.Query(`SELECT MAX(hour) FROM usage_stats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	var hourStr *string
	if err := rows.Scan(&hourStr); err != nil {
		return nil, err
	}
	if hourStr == nil || *hourStr == "" {
		return nil, nil
	}

	// 尝试多种格式解析
	hour, err := time.Parse(time.RFC3339, *hourStr)
	if err != nil {
		hour, err = time.Parse("2006-01-02 15:04:05", *hourStr)
		if err != nil {
			hour, err = time.Parse("2006-01-02 15", *hourStr)
			if err != nil {
				return nil, err
			}
		}
	}
	return &hour, nil
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

	rows, err := r.db.Query(query, args...)
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

// Aggregate 聚合统计数据（从 proxy_upstream_attempts 聚合到 usage_stats）
func (r *UsageStatsRepository) Aggregate() (int, error) {
	currentHour := time.Now().Truncate(time.Hour)

	// 增量聚合：找到最新的聚合时间
	var startTime time.Time
	latestHour, err := r.GetLatestHour()
	if err != nil {
		log.Printf("[Aggregate] GetLatestHour error: %v", err)
		startTime = time.Now().AddDate(0, 0, -30)
	} else if latestHour == nil {
		log.Printf("[Aggregate] No existing stats, using 30 days ago")
		startTime = time.Now().AddDate(0, 0, -30)
	} else {
		log.Printf("[Aggregate] Latest hour: %v", *latestHour)
		startTime = latestHour.Add(-time.Hour)
	}

	log.Printf("[Aggregate] Query range: %v ~ %v", startTime, currentHour)

	// 聚合数据
	rows, err := r.db.Query(`
		SELECT
			substr(a.created_at, 1, 13) as hour,
			COALESCE(r.route_id, 0), COALESCE(a.provider_id, 0),
			COALESCE(r.project_id, 0), COALESCE(r.api_token_id, 0), COALESCE(r.client_type, ''),
			COUNT(*),
			SUM(CASE WHEN a.status = 'COMPLETED' THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.status IN ('FAILED', 'CANCELLED') THEN 1 ELSE 0 END),
			COALESCE(SUM(a.input_token_count), 0),
			COALESCE(SUM(a.output_token_count), 0),
			COALESCE(SUM(a.cache_read_count), 0),
			COALESCE(SUM(a.cache_write_count), 0),
			COALESCE(SUM(a.cost), 0)
		FROM proxy_upstream_attempts a
		LEFT JOIN proxy_requests r ON a.proxy_request_id = r.id
		WHERE a.created_at >= ? AND a.created_at < ?
		GROUP BY hour, r.route_id, a.provider_id, r.project_id, r.api_token_id, r.client_type
	`, startTime, currentHour)
	if err != nil {
		log.Printf("[Aggregate] Query error: %v", err)
		return 0, err
	}
	defer rows.Close()

	// 收集所有待插入的数据
	var statsList []domain.UsageStats
	for rows.Next() {
		var hourStr string
		var stats domain.UsageStats
		err := rows.Scan(
			&hourStr, &stats.RouteID, &stats.ProviderID, &stats.ProjectID, &stats.APITokenID, &stats.ClientType,
			&stats.TotalRequests, &stats.SuccessfulRequests, &stats.FailedRequests,
			&stats.InputTokens, &stats.OutputTokens, &stats.CacheRead, &stats.CacheWrite, &stats.Cost,
		)
		if err != nil {
			log.Printf("[Aggregate] Scan error: %v", err)
			continue
		}
		stats.Hour, _ = time.Parse("2006-01-02 15", hourStr)
		statsList = append(statsList, stats)
	}

	log.Printf("[Aggregate] Found %d records to upsert", len(statsList))

	if len(statsList) == 0 {
		return 0, nil
	}

	// 使用事务批量插入
	if err := r.batchUpsert(statsList); err != nil {
		return 0, err
	}
	return len(statsList), nil
}

// batchUpsert 批量插入统计数据
func (r *UsageStatsRepository) batchUpsert(statsList []domain.UsageStats) error {
	now := time.Now()
	for i := range statsList {
		s := &statsList[i]
		_, err := r.db.Exec(`
			INSERT INTO usage_stats (
				created_at, hour, route_id, provider_id, project_id, api_token_id, client_type,
				total_requests, successful_requests, failed_requests,
				input_tokens, output_tokens, cache_read, cache_write, cost
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(hour, route_id, provider_id, project_id, api_token_id, client_type) DO UPDATE SET
				total_requests = excluded.total_requests,
				successful_requests = excluded.successful_requests,
				failed_requests = excluded.failed_requests,
				input_tokens = excluded.input_tokens,
				output_tokens = excluded.output_tokens,
				cache_read = excluded.cache_read,
				cache_write = excluded.cache_write,
				cost = excluded.cost
		`, now, s.Hour, s.RouteID, s.ProviderID, s.ProjectID, s.APITokenID, s.ClientType,
			s.TotalRequests, s.SuccessfulRequests, s.FailedRequests,
			s.InputTokens, s.OutputTokens, s.CacheRead, s.CacheWrite, s.Cost)
		if err != nil {
			return err
		}
	}
	return nil
}
