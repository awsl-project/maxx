package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"github.com/Bowl42/maxx-next/internal/domain"
)

type ProxyUpstreamAttemptRepository struct {
	db *DB
}

func NewProxyUpstreamAttemptRepository(db *DB) *ProxyUpstreamAttemptRepository {
	return &ProxyUpstreamAttemptRepository{db: db}
}

func (r *ProxyUpstreamAttemptRepository) Create(a *domain.ProxyUpstreamAttempt) error {
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now

	result, err := r.db.db.Exec(
		`INSERT INTO proxy_upstream_attempts (created_at, updated_at, start_time, end_time, duration_ms, status, proxy_request_id, is_stream, request_info, response_info, route_id, provider_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.CreatedAt, a.UpdatedAt, a.StartTime, a.EndTime, a.Duration.Milliseconds(), a.Status, a.ProxyRequestID, a.IsStream, toJSON(a.RequestInfo), toJSON(a.ResponseInfo), a.RouteID, a.ProviderID, a.InputTokenCount, a.OutputTokenCount, a.CacheReadCount, a.CacheWriteCount, a.Cache5mWriteCount, a.Cache1hWriteCount, a.Cost,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	a.ID = uint64(id)
	return nil
}

func (r *ProxyUpstreamAttemptRepository) Update(a *domain.ProxyUpstreamAttempt) error {
	a.UpdatedAt = time.Now()
	_, err := r.db.db.Exec(
		`UPDATE proxy_upstream_attempts SET updated_at = ?, start_time = ?, end_time = ?, duration_ms = ?, status = ?, is_stream = ?, request_info = ?, response_info = ?, route_id = ?, provider_id = ?, input_token_count = ?, output_token_count = ?, cache_read_count = ?, cache_write_count = ?, cache_5m_write_count = ?, cache_1h_write_count = ?, cost = ? WHERE id = ?`,
		a.UpdatedAt, a.StartTime, a.EndTime, a.Duration.Milliseconds(), a.Status, a.IsStream, toJSON(a.RequestInfo), toJSON(a.ResponseInfo), a.RouteID, a.ProviderID, a.InputTokenCount, a.OutputTokenCount, a.CacheReadCount, a.CacheWriteCount, a.Cache5mWriteCount, a.Cache1hWriteCount, a.Cost, a.ID,
	)
	return err
}

func (r *ProxyUpstreamAttemptRepository) ListByProxyRequestID(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, start_time, end_time, duration_ms, status, proxy_request_id, is_stream, request_info, response_info, route_id, provider_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost FROM proxy_upstream_attempts WHERE proxy_request_id = ? ORDER BY id`, proxyRequestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []*domain.ProxyUpstreamAttempt
	for rows.Next() {
		var a domain.ProxyUpstreamAttempt
		var reqInfoJSON, respInfoJSON string
		var startTime, endTime sql.NullTime
		var durationMs int64
		err := rows.Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt, &startTime, &endTime, &durationMs, &a.Status, &a.ProxyRequestID, &a.IsStream, &reqInfoJSON, &respInfoJSON, &a.RouteID, &a.ProviderID, &a.InputTokenCount, &a.OutputTokenCount, &a.CacheReadCount, &a.CacheWriteCount, &a.Cache5mWriteCount, &a.Cache1hWriteCount, &a.Cost)
		if err != nil {
			return nil, err
		}
		if startTime.Valid {
			a.StartTime = startTime.Time
		}
		if endTime.Valid {
			a.EndTime = endTime.Time
		}
		a.Duration = time.Duration(durationMs) * time.Millisecond
		a.RequestInfo = fromJSON[*domain.RequestInfo](reqInfoJSON)
		a.ResponseInfo = fromJSON[*domain.ResponseInfo](respInfoJSON)
		attempts = append(attempts, &a)
	}
	return attempts, rows.Err()
}

// GetProviderStats returns aggregated statistics per provider, optionally filtered by client type and project ID
func (r *ProxyUpstreamAttemptRepository) GetProviderStats(clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error) {
	var query string
	var args []interface{}

	// Build WHERE conditions
	conditions := []string{"a.provider_id > 0"}
	needJoin := false

	if clientType != "" {
		conditions = append(conditions, "r.client_type = ?")
		args = append(args, clientType)
		needJoin = true
	}
	if projectID > 0 {
		conditions = append(conditions, "r.project_id = ?")
		args = append(args, projectID)
		needJoin = true
	}

	if needJoin {
		query = `
			SELECT
				a.provider_id,
				COUNT(*) as total_requests,
				SUM(CASE WHEN a.status = 'COMPLETED' THEN 1 ELSE 0 END) as successful_requests,
				SUM(CASE WHEN a.status = 'FAILED' OR a.status = 'CANCELLED' THEN 1 ELSE 0 END) as failed_requests,
				SUM(CASE WHEN a.status = 'IN_PROGRESS' OR a.status = 'PENDING' THEN 1 ELSE 0 END) as active_requests,
				COALESCE(SUM(a.input_token_count), 0) as total_input_tokens,
				COALESCE(SUM(a.output_token_count), 0) as total_output_tokens,
				COALESCE(SUM(a.cache_read_count), 0) as total_cache_read,
				COALESCE(SUM(a.cache_write_count), 0) as total_cache_write,
				COALESCE(SUM(a.cost), 0) as total_cost
			FROM proxy_upstream_attempts a
			INNER JOIN proxy_requests r ON a.proxy_request_id = r.id
			WHERE ` + joinConditions(conditions) + `
			GROUP BY a.provider_id
		`
	} else {
		query = `
			SELECT
				provider_id,
				COUNT(*) as total_requests,
				SUM(CASE WHEN status = 'COMPLETED' THEN 1 ELSE 0 END) as successful_requests,
				SUM(CASE WHEN status = 'FAILED' OR status = 'CANCELLED' THEN 1 ELSE 0 END) as failed_requests,
				SUM(CASE WHEN status = 'IN_PROGRESS' OR status = 'PENDING' THEN 1 ELSE 0 END) as active_requests,
				COALESCE(SUM(input_token_count), 0) as total_input_tokens,
				COALESCE(SUM(output_token_count), 0) as total_output_tokens,
				COALESCE(SUM(cache_read_count), 0) as total_cache_read,
				COALESCE(SUM(cache_write_count), 0) as total_cache_write,
				COALESCE(SUM(cost), 0) as total_cost
			FROM proxy_upstream_attempts
			WHERE provider_id > 0
			GROUP BY provider_id
		`
	}

	rows, err := r.db.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[uint64]*domain.ProviderStats)
	for rows.Next() {
		var s domain.ProviderStats
		err := rows.Scan(
			&s.ProviderID,
			&s.TotalRequests,
			&s.SuccessfulRequests,
			&s.FailedRequests,
			&s.ActiveRequests,
			&s.TotalInputTokens,
			&s.TotalOutputTokens,
			&s.TotalCacheRead,
			&s.TotalCacheWrite,
			&s.TotalCost,
		)
		if err != nil {
			return nil, err
		}
		// Calculate success rate
		if s.TotalRequests > 0 {
			s.SuccessRate = float64(s.SuccessfulRequests) / float64(s.TotalRequests) * 100
		}
		stats[s.ProviderID] = &s
	}
	return stats, rows.Err()
}

// joinConditions joins SQL conditions with AND
func joinConditions(conditions []string) string {
	return strings.Join(conditions, " AND ")
}
