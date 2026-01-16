package sqlite

import (
	"database/sql"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
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
		`INSERT INTO proxy_upstream_attempts (created_at, updated_at, start_time, end_time, duration_ms, status, proxy_request_id, is_stream, request_model, mapped_model, response_model, request_info, response_info, route_id, provider_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.CreatedAt, a.UpdatedAt, a.StartTime, a.EndTime, a.Duration.Milliseconds(), a.Status, a.ProxyRequestID, a.IsStream, a.RequestModel, a.MappedModel, a.ResponseModel, toJSON(a.RequestInfo), toJSON(a.ResponseInfo), a.RouteID, a.ProviderID, a.InputTokenCount, a.OutputTokenCount, a.CacheReadCount, a.CacheWriteCount, a.Cache5mWriteCount, a.Cache1hWriteCount, a.Cost,
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
		`UPDATE proxy_upstream_attempts SET updated_at = ?, start_time = ?, end_time = ?, duration_ms = ?, status = ?, is_stream = ?, request_model = ?, mapped_model = ?, response_model = ?, request_info = ?, response_info = ?, route_id = ?, provider_id = ?, input_token_count = ?, output_token_count = ?, cache_read_count = ?, cache_write_count = ?, cache_5m_write_count = ?, cache_1h_write_count = ?, cost = ? WHERE id = ?`,
		a.UpdatedAt, a.StartTime, a.EndTime, a.Duration.Milliseconds(), a.Status, a.IsStream, a.RequestModel, a.MappedModel, a.ResponseModel, toJSON(a.RequestInfo), toJSON(a.ResponseInfo), a.RouteID, a.ProviderID, a.InputTokenCount, a.OutputTokenCount, a.CacheReadCount, a.CacheWriteCount, a.Cache5mWriteCount, a.Cache1hWriteCount, a.Cost, a.ID,
	)
	return err
}

func (r *ProxyUpstreamAttemptRepository) ListByProxyRequestID(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, start_time, end_time, duration_ms, status, proxy_request_id, is_stream, request_model, mapped_model, response_model, request_info, response_info, route_id, provider_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost FROM proxy_upstream_attempts WHERE proxy_request_id = ? ORDER BY id`, proxyRequestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attempts := make([]*domain.ProxyUpstreamAttempt, 0)
	for rows.Next() {
		var a domain.ProxyUpstreamAttempt
		var reqInfoJSON, respInfoJSON string
		var startTime, endTime sql.NullTime
		var durationMs int64
		err := rows.Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt, &startTime, &endTime, &durationMs, &a.Status, &a.ProxyRequestID, &a.IsStream, &a.RequestModel, &a.MappedModel, &a.ResponseModel, &reqInfoJSON, &respInfoJSON, &a.RouteID, &a.ProviderID, &a.InputTokenCount, &a.OutputTokenCount, &a.CacheReadCount, &a.CacheWriteCount, &a.Cache5mWriteCount, &a.Cache1hWriteCount, &a.Cost)
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
