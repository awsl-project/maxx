package sqlite

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
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
	if err := r.db.gorm.Model(&ProxyRequest{}).Count(&count).Error; err == nil {
		atomic.StoreInt64(&r.count, count)
	}
}

func (r *ProxyRequestRepository) Create(p *domain.ProxyRequest) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	model := r.toModel(p)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	p.ID = model.ID

	// 创建成功后增加计数缓存
	atomic.AddInt64(&r.count, 1)

	return nil
}

func (r *ProxyRequestRepository) Update(p *domain.ProxyRequest) error {
	p.UpdatedAt = time.Now()
	model := r.toModel(p)
	return r.db.gorm.Save(model).Error
}

func (r *ProxyRequestRepository) GetByID(id uint64) (*domain.ProxyRequest, error) {
	var model ProxyRequest
	if err := r.db.gorm.First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *ProxyRequestRepository) List(limit, offset int) ([]*domain.ProxyRequest, error) {
	var models []ProxyRequest
	if err := r.db.gorm.Order("id DESC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

// ListCursor 基于游标的分页查询，比 OFFSET 更高效
// before: 获取 id < before 的记录 (向后翻页)
// after: 获取 id > after 的记录 (向前翻页/获取新数据)
// 注意：列表查询不返回 request_info 和 response_info 大字段
func (r *ProxyRequestRepository) ListCursor(limit int, before, after uint64) ([]*domain.ProxyRequest, error) {
	// 使用 Select 排除大字段
	query := r.db.gorm.Model(&ProxyRequest{}).
		Select("id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, is_stream, status, status_code, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost, api_token_id")

	if after > 0 {
		query = query.Where("id > ?", after)
	} else if before > 0 {
		query = query.Where("id < ?", before)
	}

	var models []ProxyRequest
	if err := query.Order("id DESC").Limit(limit).Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

// ListActive 获取所有活跃请求 (PENDING 或 IN_PROGRESS 状态)
func (r *ProxyRequestRepository) ListActive() ([]*domain.ProxyRequest, error) {
	var models []ProxyRequest
	if err := r.db.gorm.Model(&ProxyRequest{}).
		Select("id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, is_stream, status, status_code, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost, api_token_id").
		Where("status IN ?", []string{"PENDING", "IN_PROGRESS"}).
		Order("id DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ProxyRequestRepository) Count() (int64, error) {
	return atomic.LoadInt64(&r.count), nil
}

// MarkStaleAsFailed marks all IN_PROGRESS/PENDING requests from other instances as FAILED
// Also marks requests that have been IN_PROGRESS for too long (> 30 minutes) as timed out
func (r *ProxyRequestRepository) MarkStaleAsFailed(currentInstanceID string) (int64, error) {
	timeoutThreshold := time.Now().Add(-30 * time.Minute).UnixMilli()
	now := time.Now().UnixMilli()

	// Use raw SQL for complex CASE expression
	result := r.db.gorm.Exec(`
		UPDATE proxy_requests
		SET status = 'FAILED',
		    error = CASE
		        WHEN instance_id IS NULL OR instance_id != ? THEN 'Server restarted'
		        ELSE 'Request timed out (stuck in progress)'
		    END,
		    updated_at = ?
		WHERE status IN ('PENDING', 'IN_PROGRESS')
		  AND (
		      (instance_id IS NULL OR instance_id != ?)
		      OR (start_time < ? AND start_time > 0)
		  )`,
		currentInstanceID, now, currentInstanceID, timeoutThreshold,
	)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// UpdateProjectIDBySessionID 批量更新指定 sessionID 的所有请求的 projectID
func (r *ProxyRequestRepository) UpdateProjectIDBySessionID(sessionID string, projectID uint64) (int64, error) {
	now := time.Now().UnixMilli()
	result := r.db.gorm.Model(&ProxyRequest{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"project_id": projectID,
			"updated_at": now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// DeleteOlderThan 删除指定时间之前的请求记录
func (r *ProxyRequestRepository) DeleteOlderThan(before time.Time) (int64, error) {
	beforeTs := toTimestamp(before)

	// 先查询需要删除的请求ID列表（兼容MySQL）
	var requestIDs []uint64
	if err := r.db.gorm.Model(&ProxyRequest{}).Where("created_at < ?", beforeTs).Pluck("id", &requestIDs).Error; err != nil {
		return 0, err
	}

	if len(requestIDs) == 0 {
		return 0, nil
	}

	// 删除关联的 attempts
	if err := r.db.gorm.Where("proxy_request_id IN ?", requestIDs).Delete(&ProxyUpstreamAttempt{}).Error; err != nil {
		return 0, err
	}

	// 删除 requests
	result := r.db.gorm.Where("id IN ?", requestIDs).Delete(&ProxyRequest{})
	if result.Error != nil {
		return 0, result.Error
	}

	affected := result.RowsAffected
	// 更新计数缓存
	if affected > 0 {
		atomic.AddInt64(&r.count, -affected)
	}

	return affected, nil
}

func (r *ProxyRequestRepository) toModel(p *domain.ProxyRequest) *ProxyRequest {
	return &ProxyRequest{
		BaseModel: BaseModel{
			ID:        p.ID,
			CreatedAt: toTimestamp(p.CreatedAt),
			UpdatedAt: toTimestamp(p.UpdatedAt),
		},
		InstanceID:                 p.InstanceID,
		RequestID:                  p.RequestID,
		SessionID:                  p.SessionID,
		ClientType:                 string(p.ClientType),
		RequestModel:               p.RequestModel,
		ResponseModel:              p.ResponseModel,
		StartTime:                  toTimestamp(p.StartTime),
		EndTime:                    toTimestamp(p.EndTime),
		DurationMs:                 p.Duration.Milliseconds(),
		IsStream:                   boolToInt(p.IsStream),
		Status:                     p.Status,
		StatusCode:                 p.StatusCode,
		RequestInfo:                LongText(toJSON(p.RequestInfo)),
		ResponseInfo:               LongText(toJSON(p.ResponseInfo)),
		Error:                      LongText(p.Error),
		ProxyUpstreamAttemptCount:  p.ProxyUpstreamAttemptCount,
		FinalProxyUpstreamAttemptID: p.FinalProxyUpstreamAttemptID,
		RouteID:                    p.RouteID,
		ProviderID:                 p.ProviderID,
		ProjectID:                  p.ProjectID,
		InputTokenCount:            p.InputTokenCount,
		OutputTokenCount:           p.OutputTokenCount,
		CacheReadCount:             p.CacheReadCount,
		CacheWriteCount:            p.CacheWriteCount,
		Cache5mWriteCount:          p.Cache5mWriteCount,
		Cache1hWriteCount:          p.Cache1hWriteCount,
		Cost:                       p.Cost,
		APITokenID:                 p.APITokenID,
	}
}

func (r *ProxyRequestRepository) toDomain(m *ProxyRequest) *domain.ProxyRequest {
	return &domain.ProxyRequest{
		ID:                          m.ID,
		CreatedAt:                   fromTimestamp(m.CreatedAt),
		UpdatedAt:                   fromTimestamp(m.UpdatedAt),
		InstanceID:                  m.InstanceID,
		RequestID:                   m.RequestID,
		SessionID:                   m.SessionID,
		ClientType:                  domain.ClientType(m.ClientType),
		RequestModel:                m.RequestModel,
		ResponseModel:               m.ResponseModel,
		StartTime:                   fromTimestamp(m.StartTime),
		EndTime:                     fromTimestamp(m.EndTime),
		Duration:                    time.Duration(m.DurationMs) * time.Millisecond,
		IsStream:                    m.IsStream == 1,
		Status:                      m.Status,
		StatusCode:                  m.StatusCode,
		RequestInfo:                 fromJSON[*domain.RequestInfo](string(m.RequestInfo)),
		ResponseInfo:                fromJSON[*domain.ResponseInfo](string(m.ResponseInfo)),
		Error:                       string(m.Error),
		ProxyUpstreamAttemptCount:   m.ProxyUpstreamAttemptCount,
		FinalProxyUpstreamAttemptID: m.FinalProxyUpstreamAttemptID,
		RouteID:                     m.RouteID,
		ProviderID:                  m.ProviderID,
		ProjectID:                   m.ProjectID,
		InputTokenCount:             m.InputTokenCount,
		OutputTokenCount:            m.OutputTokenCount,
		CacheReadCount:              m.CacheReadCount,
		CacheWriteCount:             m.CacheWriteCount,
		Cache5mWriteCount:           m.Cache5mWriteCount,
		Cache1hWriteCount:           m.Cache1hWriteCount,
		Cost:                        m.Cost,
		APITokenID:                  m.APITokenID,
	}
}

func (r *ProxyRequestRepository) toDomainList(models []ProxyRequest) []*domain.ProxyRequest {
	requests := make([]*domain.ProxyRequest, len(models))
	for i, m := range models {
		requests[i] = r.toDomain(&m)
	}
	return requests
}
