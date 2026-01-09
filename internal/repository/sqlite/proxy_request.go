package sqlite

import (
	"database/sql"
	"time"

	"github.com/Bowl42/maxx-next/internal/domain"
)

type ProxyRequestRepository struct {
	db *DB
}

func NewProxyRequestRepository(db *DB) *ProxyRequestRepository {
	return &ProxyRequestRepository{db: db}
}

func (r *ProxyRequestRepository) Create(p *domain.ProxyRequest) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	result, err := r.db.db.Exec(
		`INSERT INTO proxy_requests (created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, status, request_info, response_info, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.CreatedAt, p.UpdatedAt, p.InstanceID, p.RequestID, p.SessionID, p.ClientType, p.RequestModel, p.ResponseModel,
		nullTime(p.StartTime), nullTime(p.EndTime), p.Duration.Milliseconds(), p.Status,
		toJSON(p.RequestInfo), toJSON(p.ResponseInfo), p.Error,
		p.ProxyUpstreamAttemptCount, p.FinalProxyUpstreamAttemptID,
		p.InputTokenCount, p.OutputTokenCount, p.CacheReadCount, p.CacheWriteCount, p.Cache5mWriteCount, p.Cache1hWriteCount, p.Cost,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = uint64(id)
	return nil
}

func (r *ProxyRequestRepository) Update(p *domain.ProxyRequest) error {
	p.UpdatedAt = time.Now()
	_, err := r.db.db.Exec(
		`UPDATE proxy_requests SET updated_at = ?, instance_id = ?, request_id = ?, session_id = ?, client_type = ?, request_model = ?, response_model = ?, start_time = ?, end_time = ?, duration_ms = ?, status = ?, request_info = ?, response_info = ?, error = ?, proxy_upstream_attempt_count = ?, final_proxy_upstream_attempt_id = ?, input_token_count = ?, output_token_count = ?, cache_read_count = ?, cache_write_count = ?, cache_5m_write_count = ?, cache_1h_write_count = ?, cost = ? WHERE id = ?`,
		p.UpdatedAt, p.InstanceID, p.RequestID, p.SessionID, p.ClientType, p.RequestModel, p.ResponseModel,
		nullTime(p.StartTime), nullTime(p.EndTime), p.Duration.Milliseconds(), p.Status,
		toJSON(p.RequestInfo), toJSON(p.ResponseInfo), p.Error,
		p.ProxyUpstreamAttemptCount, p.FinalProxyUpstreamAttemptID,
		p.InputTokenCount, p.OutputTokenCount, p.CacheReadCount, p.CacheWriteCount, p.Cache5mWriteCount, p.Cache1hWriteCount, p.Cost, p.ID,
	)
	return err
}

func (r *ProxyRequestRepository) GetByID(id uint64) (*domain.ProxyRequest, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, status, request_info, response_info, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost FROM proxy_requests WHERE id = ?`, id)
	return r.scanRequest(row)
}

func (r *ProxyRequestRepository) List(limit, offset int) ([]*domain.ProxyRequest, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, status, request_info, response_info, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost FROM proxy_requests ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []*domain.ProxyRequest
	for rows.Next() {
		p, err := r.scanRequestRows(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, p)
	}
	return requests, rows.Err()
}

// MarkStaleAsFailed marks all IN_PROGRESS/PENDING requests from other instances as FAILED
func (r *ProxyRequestRepository) MarkStaleAsFailed(currentInstanceID string) (int64, error) {
	result, err := r.db.db.Exec(
		`UPDATE proxy_requests SET status = 'FAILED', error = 'Server restarted', updated_at = ? WHERE status IN ('PENDING', 'IN_PROGRESS') AND (instance_id IS NULL OR instance_id != ?)`,
		time.Now(), currentInstanceID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *ProxyRequestRepository) scanRequest(row *sql.Row) (*domain.ProxyRequest, error) {
	var p domain.ProxyRequest
	var startTime, endTime sql.NullTime
	var durationMs int64
	var reqInfoJSON, respInfoJSON string
	var instanceID sql.NullString
	err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &instanceID, &p.RequestID, &p.SessionID, &p.ClientType, &p.RequestModel, &p.ResponseModel, &startTime, &endTime, &durationMs, &p.Status, &reqInfoJSON, &respInfoJSON, &p.Error, &p.ProxyUpstreamAttemptCount, &p.FinalProxyUpstreamAttemptID, &p.InputTokenCount, &p.OutputTokenCount, &p.CacheReadCount, &p.CacheWriteCount, &p.Cache5mWriteCount, &p.Cache1hWriteCount, &p.Cost)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if instanceID.Valid {
		p.InstanceID = instanceID.String
	}
	p.StartTime = parseTime(startTime)
	p.EndTime = parseTime(endTime)
	p.Duration = time.Duration(durationMs) * time.Millisecond
	p.RequestInfo = fromJSON[*domain.RequestInfo](reqInfoJSON)
	p.ResponseInfo = fromJSON[*domain.ResponseInfo](respInfoJSON)
	return &p, nil
}

func (r *ProxyRequestRepository) scanRequestRows(rows *sql.Rows) (*domain.ProxyRequest, error) {
	var p domain.ProxyRequest
	var startTime, endTime sql.NullTime
	var durationMs int64
	var reqInfoJSON, respInfoJSON string
	var instanceID sql.NullString
	err := rows.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &instanceID, &p.RequestID, &p.SessionID, &p.ClientType, &p.RequestModel, &p.ResponseModel, &startTime, &endTime, &durationMs, &p.Status, &reqInfoJSON, &respInfoJSON, &p.Error, &p.ProxyUpstreamAttemptCount, &p.FinalProxyUpstreamAttemptID, &p.InputTokenCount, &p.OutputTokenCount, &p.CacheReadCount, &p.CacheWriteCount, &p.Cache5mWriteCount, &p.Cache1hWriteCount, &p.Cost)
	if err != nil {
		return nil, err
	}
	if instanceID.Valid {
		p.InstanceID = instanceID.String
	}
	p.StartTime = parseTime(startTime)
	p.EndTime = parseTime(endTime)
	p.Duration = time.Duration(durationMs) * time.Millisecond
	p.RequestInfo = fromJSON[*domain.RequestInfo](reqInfoJSON)
	p.ResponseInfo = fromJSON[*domain.ResponseInfo](respInfoJSON)
	return &p, nil
}
