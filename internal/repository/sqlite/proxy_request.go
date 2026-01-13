package sqlite

import (
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/Bowl42/maxx-next/internal/domain"
)

type ProxyRequestRepository struct {
	db    *DB
	count int64 // 缓存的请求总数，使用原子操作
}

func NewProxyRequestRepository(db *DB) *ProxyRequestRepository {
	r := &ProxyRequestRepository{db: db}
	// 初始化时从数据库加载计数
	r.initCount()
	return r
}

// initCount 从数据库初始化计数缓存
func (r *ProxyRequestRepository) initCount() {
	var count int64
	if err := r.db.db.QueryRow(`SELECT COUNT(*) FROM proxy_requests`).Scan(&count); err == nil {
		atomic.StoreInt64(&r.count, count)
	}
}

func (r *ProxyRequestRepository) Create(p *domain.ProxyRequest) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	result, err := r.db.db.Exec(
		`INSERT INTO proxy_requests (created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, is_stream, status, status_code, request_info, response_info, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.CreatedAt, p.UpdatedAt, p.InstanceID, p.RequestID, p.SessionID, p.ClientType, p.RequestModel, p.ResponseModel,
		nullTime(p.StartTime), nullTime(p.EndTime), p.Duration.Milliseconds(), p.IsStream, p.Status, p.StatusCode,
		toJSON(p.RequestInfo), toJSON(p.ResponseInfo), p.Error,
		p.ProxyUpstreamAttemptCount, p.FinalProxyUpstreamAttemptID, p.RouteID, p.ProviderID, p.ProjectID,
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

	// 创建成功后增加计数缓存
	atomic.AddInt64(&r.count, 1)

	return nil
}

func (r *ProxyRequestRepository) Update(p *domain.ProxyRequest) error {
	p.UpdatedAt = time.Now()
	_, err := r.db.db.Exec(
		`UPDATE proxy_requests SET updated_at = ?, instance_id = ?, request_id = ?, session_id = ?, client_type = ?, request_model = ?, response_model = ?, start_time = ?, end_time = ?, duration_ms = ?, is_stream = ?, status = ?, status_code = ?, request_info = ?, response_info = ?, error = ?, proxy_upstream_attempt_count = ?, final_proxy_upstream_attempt_id = ?, route_id = ?, provider_id = ?, project_id = ?, input_token_count = ?, output_token_count = ?, cache_read_count = ?, cache_write_count = ?, cache_5m_write_count = ?, cache_1h_write_count = ?, cost = ? WHERE id = ?`,
		p.UpdatedAt, p.InstanceID, p.RequestID, p.SessionID, p.ClientType, p.RequestModel, p.ResponseModel,
		nullTime(p.StartTime), nullTime(p.EndTime), p.Duration.Milliseconds(), p.IsStream, p.Status, p.StatusCode,
		toJSON(p.RequestInfo), toJSON(p.ResponseInfo), p.Error,
		p.ProxyUpstreamAttemptCount, p.FinalProxyUpstreamAttemptID, p.RouteID, p.ProviderID, p.ProjectID,
		p.InputTokenCount, p.OutputTokenCount, p.CacheReadCount, p.CacheWriteCount, p.Cache5mWriteCount, p.Cache1hWriteCount, p.Cost, p.ID,
	)
	return err
}

func (r *ProxyRequestRepository) GetByID(id uint64) (*domain.ProxyRequest, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, is_stream, status, request_info, response_info, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost FROM proxy_requests WHERE id = ?`, id)
	return r.scanRequest(row)
}

func (r *ProxyRequestRepository) List(limit, offset int) ([]*domain.ProxyRequest, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, is_stream, status, request_info, response_info, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost FROM proxy_requests ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
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

// ListCursor 基于游标的分页查询，比 OFFSET 更高效
// before: 获取 id < before 的记录 (向后翻页)
// after: 获取 id > after 的记录 (向前翻页/获取新数据)
// 注意：列表查询不返回 request_info 和 response_info 大字段
func (r *ProxyRequestRepository) ListCursor(limit int, before, after uint64) ([]*domain.ProxyRequest, error) {
	// 列表查询使用精简字段，不包含 request_info 和 response_info
	const listColumns = `id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, is_stream, status, status_code, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost`

	var query string
	var args []interface{}

	if after > 0 {
		query = `SELECT ` + listColumns + ` FROM proxy_requests WHERE id > ? ORDER BY id DESC LIMIT ?`
		args = []interface{}{after, limit}
	} else if before > 0 {
		query = `SELECT ` + listColumns + ` FROM proxy_requests WHERE id < ? ORDER BY id DESC LIMIT ?`
		args = []interface{}{before, limit}
	} else {
		query = `SELECT ` + listColumns + ` FROM proxy_requests ORDER BY id DESC LIMIT ?`
		args = []interface{}{limit}
	}

	rows, err := r.db.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []*domain.ProxyRequest
	for rows.Next() {
		p, err := r.scanRequestRowsLite(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, p)
	}
	return requests, rows.Err()
}

func (r *ProxyRequestRepository) Count() (int64, error) {
	return atomic.LoadInt64(&r.count), nil
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
	var routeID, providerID, projectID sql.NullInt64
	var isStream sql.NullBool
	err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &instanceID, &p.RequestID, &p.SessionID, &p.ClientType, &p.RequestModel, &p.ResponseModel, &startTime, &endTime, &durationMs, &isStream, &p.Status, &reqInfoJSON, &respInfoJSON, &p.Error, &p.ProxyUpstreamAttemptCount, &p.FinalProxyUpstreamAttemptID, &routeID, &providerID, &projectID, &p.InputTokenCount, &p.OutputTokenCount, &p.CacheReadCount, &p.CacheWriteCount, &p.Cache5mWriteCount, &p.Cache1hWriteCount, &p.Cost)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if instanceID.Valid {
		p.InstanceID = instanceID.String
	}
	if routeID.Valid {
		p.RouteID = uint64(routeID.Int64)
	}
	if providerID.Valid {
		p.ProviderID = uint64(providerID.Int64)
	}
	if projectID.Valid {
		p.ProjectID = uint64(projectID.Int64)
	}
	if isStream.Valid {
		p.IsStream = isStream.Bool
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
	var routeID, providerID, projectID sql.NullInt64
	var isStream sql.NullBool
	err := rows.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &instanceID, &p.RequestID, &p.SessionID, &p.ClientType, &p.RequestModel, &p.ResponseModel, &startTime, &endTime, &durationMs, &isStream, &p.Status, &reqInfoJSON, &respInfoJSON, &p.Error, &p.ProxyUpstreamAttemptCount, &p.FinalProxyUpstreamAttemptID, &routeID, &providerID, &projectID, &p.InputTokenCount, &p.OutputTokenCount, &p.CacheReadCount, &p.CacheWriteCount, &p.Cache5mWriteCount, &p.Cache1hWriteCount, &p.Cost)
	if err != nil {
		return nil, err
	}
	if instanceID.Valid {
		p.InstanceID = instanceID.String
	}
	if routeID.Valid {
		p.RouteID = uint64(routeID.Int64)
	}
	if providerID.Valid {
		p.ProviderID = uint64(providerID.Int64)
	}
	if projectID.Valid {
		p.ProjectID = uint64(projectID.Int64)
	}
	if isStream.Valid {
		p.IsStream = isStream.Bool
	}
	p.StartTime = parseTime(startTime)
	p.EndTime = parseTime(endTime)
	p.Duration = time.Duration(durationMs) * time.Millisecond
	p.RequestInfo = fromJSON[*domain.RequestInfo](reqInfoJSON)
	p.ResponseInfo = fromJSON[*domain.ResponseInfo](respInfoJSON)
	return &p, nil
}

// scanRequestRowsLite 精简版扫描，不包含 request_info 和 response_info
func (r *ProxyRequestRepository) scanRequestRowsLite(rows *sql.Rows) (*domain.ProxyRequest, error) {
	var p domain.ProxyRequest
	var startTime, endTime sql.NullTime
	var durationMs int64
	var instanceID sql.NullString
	var routeID, providerID, projectID sql.NullInt64
	var isStream sql.NullBool
	err := rows.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &instanceID, &p.RequestID, &p.SessionID, &p.ClientType, &p.RequestModel, &p.ResponseModel, &startTime, &endTime, &durationMs, &isStream, &p.Status, &p.StatusCode, &p.Error, &p.ProxyUpstreamAttemptCount, &p.FinalProxyUpstreamAttemptID, &routeID, &providerID, &projectID, &p.InputTokenCount, &p.OutputTokenCount, &p.CacheReadCount, &p.CacheWriteCount, &p.Cache5mWriteCount, &p.Cache1hWriteCount, &p.Cost)
	if err != nil {
		return nil, err
	}
	if instanceID.Valid {
		p.InstanceID = instanceID.String
	}
	if routeID.Valid {
		p.RouteID = uint64(routeID.Int64)
	}
	if providerID.Valid {
		p.ProviderID = uint64(providerID.Int64)
	}
	if projectID.Valid {
		p.ProjectID = uint64(projectID.Int64)
	}
	if isStream.Valid {
		p.IsStream = isStream.Bool
	}
	p.StartTime = parseTime(startTime)
	p.EndTime = parseTime(endTime)
	p.Duration = time.Duration(durationMs) * time.Millisecond
	return &p, nil
}

// UpdateProjectIDBySessionID 批量更新指定 sessionID 的所有请求的 projectID
func (r *ProxyRequestRepository) UpdateProjectIDBySessionID(sessionID string, projectID uint64) (int64, error) {
	result, err := r.db.db.Exec(
		`UPDATE proxy_requests SET project_id = ?, updated_at = ? WHERE session_id = ?`,
		projectID, time.Now(), sessionID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
