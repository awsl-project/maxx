package sqlite

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
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
// filter: 可选的过滤条件
// 注意：列表查询不返回 request_info 和 response_info 大字段
func (r *ProxyRequestRepository) ListCursor(limit int, before, after uint64, filter *repository.ProxyRequestFilter) ([]*domain.ProxyRequest, error) {
	// 使用 Select 排除大字段
	query := r.db.gorm.Model(&ProxyRequest{}).
		Select("id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, ttft_ms, is_stream, status, status_code, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost, api_token_id")

	if after > 0 {
		query = query.Where("id > ?", after)
	} else if before > 0 {
		query = query.Where("id < ?", before)
	}

	// 应用过滤条件
	if filter != nil {
		if filter.ProviderID != nil {
			query = query.Where("provider_id = ?", *filter.ProviderID)
		}
		if filter.Status != nil {
			query = query.Where("status = ?", *filter.Status)
		}
	}

	var models []ProxyRequest
	// 按结束时间排序：未完成的请求（end_time=0）在最前面，已完成的按 end_time DESC 排序
	// SQLite 不支持 NULLS FIRST，使用 CASE WHEN 实现
	if err := query.Order("CASE WHEN end_time = 0 THEN 0 ELSE 1 END, end_time DESC, id DESC").Limit(limit).Find(&models).Error; err != nil {
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

// CountWithFilter 带过滤条件的计数
func (r *ProxyRequestRepository) CountWithFilter(filter *repository.ProxyRequestFilter) (int64, error) {
	// 如果没有过滤条件，使用缓存的总数
	if filter == nil || (filter.ProviderID == nil && filter.Status == nil) {
		return atomic.LoadInt64(&r.count), nil
	}

	// 有过滤条件时需要查询数据库
	var count int64
	query := r.db.gorm.Model(&ProxyRequest{})
	if filter.ProviderID != nil {
		query = query.Where("provider_id = ?", *filter.ProviderID)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// MarkStaleAsFailed marks all IN_PROGRESS/PENDING requests from other instances as FAILED
// Also marks requests that have been IN_PROGRESS for too long (> 30 minutes) as timed out
// Sets proper end_time and duration_ms for complete failure handling
func (r *ProxyRequestRepository) MarkStaleAsFailed(currentInstanceID string) (int64, error) {
	timeoutThreshold := time.Now().Add(-30 * time.Minute).UnixMilli()
	now := time.Now().UnixMilli()

	// Use raw SQL for complex CASE expression
	// Sets end_time = now and calculates duration_ms = now - start_time
	result := r.db.gorm.Exec(`
		UPDATE proxy_requests
		SET status = 'FAILED',
		    error = CASE
		        WHEN instance_id IS NULL OR instance_id != ? THEN 'Server restarted'
		        ELSE 'Request timed out (stuck in progress)'
		    END,
		    end_time = ?,
		    duration_ms = CASE
		        WHEN start_time > 0 THEN ? - start_time
		        ELSE 0
		    END,
		    updated_at = ?
		WHERE status IN ('PENDING', 'IN_PROGRESS')
		  AND (
		      (instance_id IS NULL OR instance_id != ?)
		      OR (start_time < ? AND start_time > 0)
		  )`,
		currentInstanceID, now, now, now, currentInstanceID, timeoutThreshold,
	)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// FixFailedRequestsWithoutEndTime fixes FAILED requests that have no end_time set
// This handles legacy data where end_time was not properly set
func (r *ProxyRequestRepository) FixFailedRequestsWithoutEndTime() (int64, error) {
	now := time.Now().UnixMilli()

	result := r.db.gorm.Exec(`
		UPDATE proxy_requests
		SET end_time = CASE
		        WHEN start_time > 0 THEN start_time
		        ELSE ?
		    END,
		    duration_ms = 0,
		    updated_at = ?
		WHERE status = 'FAILED'
		  AND end_time = 0`,
		now, now,
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

// HasRecentRequests 检查指定时间之后是否有请求记录
func (r *ProxyRequestRepository) HasRecentRequests(since time.Time) (bool, error) {
	sinceTs := toTimestamp(since)
	var count int64
	if err := r.db.gorm.Model(&ProxyRequest{}).Where("created_at >= ?", sinceTs).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateCost updates only the cost field of a request
func (r *ProxyRequestRepository) UpdateCost(id uint64, cost uint64) error {
	return r.db.gorm.Model(&ProxyRequest{}).Where("id = ?", id).Update("cost", cost).Error
}

// AddCost adds a delta to the cost field of a request (can be negative)
func (r *ProxyRequestRepository) AddCost(id uint64, delta int64) error {
	return r.db.gorm.Model(&ProxyRequest{}).Where("id = ?", id).
		Update("cost", gorm.Expr("cost + ?", delta)).Error
}

// BatchUpdateCosts updates costs for multiple requests in a single transaction
func (r *ProxyRequestRepository) BatchUpdateCosts(updates map[uint64]uint64) error {
	if len(updates) == 0 {
		return nil
	}

	return r.db.gorm.Transaction(func(tx *gorm.DB) error {
		// Use CASE WHEN for batch update
		const batchSize = 500
		ids := make([]uint64, 0, len(updates))
		for id := range updates {
			ids = append(ids, id)
		}

		for i := 0; i < len(ids); i += batchSize {
			end := i + batchSize
			if end > len(ids) {
				end = len(ids)
			}
			batchIDs := ids[i:end]

			// Build CASE WHEN statement
			var cases strings.Builder
			cases.WriteString("CASE id ")
			args := make([]interface{}, 0, len(batchIDs)*3+1)

			// First: CASE WHEN pairs (id, cost)
			for _, id := range batchIDs {
				cases.WriteString("WHEN ? THEN ? ")
				args = append(args, id, updates[id])
			}
			cases.WriteString("END")

			// Second: timestamp for updated_at
			args = append(args, time.Now().UnixMilli())

			// Third: WHERE IN ids
			for _, id := range batchIDs {
				args = append(args, id)
			}

			sql := fmt.Sprintf("UPDATE proxy_requests SET cost = %s, updated_at = ? WHERE id IN (?%s)",
				cases.String(), strings.Repeat(",?", len(batchIDs)-1))

			if err := tx.Exec(sql, args...).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RecalculateCostsFromAttempts recalculates all request costs by summing their attempt costs
func (r *ProxyRequestRepository) RecalculateCostsFromAttempts() (int64, error) {
	return r.RecalculateCostsFromAttemptsWithProgress(nil)
}

// RecalculateCostsFromAttemptsWithProgress recalculates all request costs with progress reporting via channel
func (r *ProxyRequestRepository) RecalculateCostsFromAttemptsWithProgress(progress chan<- domain.Progress) (int64, error) {
	sendProgress := func(current, total int, message string) {
		if progress == nil {
			return
		}
		percentage := 0
		if total > 0 {
			percentage = current * 100 / total
		}
		progress <- domain.Progress{
			Phase:      "updating_requests",
			Current:    current,
			Total:      total,
			Percentage: percentage,
			Message:    message,
		}
	}

	// 1. 获取所有 request IDs
	var requestIDs []uint64
	err := r.db.gorm.Model(&ProxyRequest{}).Pluck("id", &requestIDs).Error
	if err != nil {
		return 0, err
	}

	total := len(requestIDs)
	if total == 0 {
		return 0, nil
	}

	// 报告初始进度
	sendProgress(0, total, fmt.Sprintf("Updating %d requests...", total))

	// 2. 分批处理
	const batchSize = 100
	now := time.Now().UnixMilli()
	var totalUpdated int64

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}
		batchIDs := requestIDs[i:end]

		// 使用子查询批量更新
		placeholders := make([]string, len(batchIDs))
		args := make([]interface{}, 0, len(batchIDs)+1)
		args = append(args, now)
		for j, id := range batchIDs {
			placeholders[j] = "?"
			args = append(args, id)
		}

		sql := fmt.Sprintf(`
			UPDATE proxy_requests
			SET cost = (
				SELECT COALESCE(SUM(cost), 0)
				FROM proxy_upstream_attempts
				WHERE proxy_request_id = proxy_requests.id
			),
			updated_at = ?
			WHERE id IN (%s)
		`, strings.Join(placeholders, ","))

		result := r.db.gorm.Exec(sql, args...)
		if result.Error != nil {
			return totalUpdated, result.Error
		}
		totalUpdated += result.RowsAffected

		// 报告进度
		sendProgress(end, total, fmt.Sprintf("Updating requests: %d/%d", end, total))
	}

	return totalUpdated, nil
}

// ClearDetailOlderThan 清理指定时间之前请求的详情字段（request_info 和 response_info）
func (r *ProxyRequestRepository) ClearDetailOlderThan(before time.Time) (int64, error) {
	beforeTs := toTimestamp(before)
	now := time.Now().UnixMilli()

	result := r.db.gorm.Model(&ProxyRequest{}).
		Where("created_at < ? AND (request_info IS NOT NULL OR response_info IS NOT NULL)", beforeTs).
		Updates(map[string]any{
			"request_info":  nil,
			"response_info": nil,
			"updated_at":    now,
		})

	return result.RowsAffected, result.Error
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
		TTFTMs:                     p.TTFT.Milliseconds(),
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
		ModelPriceID:               p.ModelPriceID,
		Multiplier:                 p.Multiplier,
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
		TTFT:                        time.Duration(m.TTFTMs) * time.Millisecond,
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
		ModelPriceID:                m.ModelPriceID,
		Multiplier:                  m.Multiplier,
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
