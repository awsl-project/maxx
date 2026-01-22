package sqlite

import (
	"fmt"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
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

	model := r.toModel(a)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	a.ID = model.ID
	return nil
}

func (r *ProxyUpstreamAttemptRepository) Update(a *domain.ProxyUpstreamAttempt) error {
	a.UpdatedAt = time.Now()
	model := r.toModel(a)
	return r.db.gorm.Save(model).Error
}

func (r *ProxyUpstreamAttemptRepository) ListByProxyRequestID(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	var models []ProxyUpstreamAttempt
	if err := r.db.gorm.Where("proxy_request_id = ?", proxyRequestID).Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ProxyUpstreamAttemptRepository) ListAll() ([]*domain.ProxyUpstreamAttempt, error) {
	var models []ProxyUpstreamAttempt
	if err := r.db.gorm.Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ProxyUpstreamAttemptRepository) CountAll() (int64, error) {
	var count int64
	if err := r.db.gorm.Model(&ProxyUpstreamAttempt{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// StreamForCostCalc iterates through all attempts in batches for cost calculation
// Only fetches fields needed for cost calculation, avoiding expensive JSON parsing
func (r *ProxyUpstreamAttemptRepository) StreamForCostCalc(batchSize int, callback func(batch []*domain.AttemptCostData) error) error {
	var lastID uint64 = 0

	for {
		var results []struct {
			ID                uint64 `gorm:"column:id"`
			ProxyRequestID    uint64 `gorm:"column:proxy_request_id"`
			ResponseModel     string `gorm:"column:response_model"`
			MappedModel       string `gorm:"column:mapped_model"`
			RequestModel      string `gorm:"column:request_model"`
			InputTokenCount   uint64 `gorm:"column:input_token_count"`
			OutputTokenCount  uint64 `gorm:"column:output_token_count"`
			CacheReadCount    uint64 `gorm:"column:cache_read_count"`
			CacheWriteCount   uint64 `gorm:"column:cache_write_count"`
			Cache5mWriteCount uint64 `gorm:"column:cache_5m_write_count"`
			Cache1hWriteCount uint64 `gorm:"column:cache_1h_write_count"`
			Cost              uint64 `gorm:"column:cost"`
		}

		err := r.db.gorm.Table("proxy_upstream_attempts").
			Select("id, proxy_request_id, response_model, mapped_model, request_model, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost").
			Where("id > ?", lastID).
			Order("id").
			Limit(batchSize).
			Find(&results).Error

		if err != nil {
			return err
		}

		if len(results) == 0 {
			break
		}

		// Convert to domain type
		batch := make([]*domain.AttemptCostData, len(results))
		for i, r := range results {
			batch[i] = &domain.AttemptCostData{
				ID:                r.ID,
				ProxyRequestID:    r.ProxyRequestID,
				ResponseModel:     r.ResponseModel,
				MappedModel:       r.MappedModel,
				RequestModel:      r.RequestModel,
				InputTokenCount:   r.InputTokenCount,
				OutputTokenCount:  r.OutputTokenCount,
				CacheReadCount:    r.CacheReadCount,
				CacheWriteCount:   r.CacheWriteCount,
				Cache5mWriteCount: r.Cache5mWriteCount,
				Cache1hWriteCount: r.Cache1hWriteCount,
				Cost:              r.Cost,
			}
		}

		if err := callback(batch); err != nil {
			return err
		}

		lastID = results[len(results)-1].ID

		if len(results) < batchSize {
			break
		}
	}

	return nil
}

func (r *ProxyUpstreamAttemptRepository) UpdateCost(id uint64, cost uint64) error {
	return r.db.gorm.Model(&ProxyUpstreamAttempt{}).Where("id = ?", id).Update("cost", cost).Error
}

// MarkStaleAttemptsFailed marks all IN_PROGRESS/PENDING attempts belonging to stale requests as FAILED
// This should be called after MarkStaleAsFailed on proxy_requests to clean up orphaned attempts
// Sets proper end_time and duration_ms for complete failure handling
func (r *ProxyUpstreamAttemptRepository) MarkStaleAttemptsFailed() (int64, error) {
	now := time.Now().UnixMilli()

	// Update attempts that belong to FAILED requests but are still in progress
	result := r.db.gorm.Exec(`
		UPDATE proxy_upstream_attempts
		SET status = 'FAILED',
		    end_time = ?,
		    duration_ms = CASE
		        WHEN start_time > 0 THEN ? - start_time
		        ELSE 0
		    END,
		    updated_at = ?
		WHERE status IN ('PENDING', 'IN_PROGRESS')
		  AND proxy_request_id IN (
		      SELECT id FROM proxy_requests WHERE status = 'FAILED'
		  )`,
		now, now, now,
	)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// FixFailedAttemptsWithoutEndTime fixes FAILED attempts that have no end_time set
// This handles legacy data where end_time was not properly set
func (r *ProxyUpstreamAttemptRepository) FixFailedAttemptsWithoutEndTime() (int64, error) {
	now := time.Now().UnixMilli()

	result := r.db.gorm.Exec(`
		UPDATE proxy_upstream_attempts
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

// BatchUpdateCosts updates costs for multiple attempts in a single transaction
func (r *ProxyUpstreamAttemptRepository) BatchUpdateCosts(updates map[uint64]uint64) error {
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

			sql := fmt.Sprintf("UPDATE proxy_upstream_attempts SET cost = %s, updated_at = ? WHERE id IN (?%s)",
				cases.String(), strings.Repeat(",?", len(batchIDs)-1))

			if err := tx.Exec(sql, args...).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ClearDetailOlderThan 清理指定时间之前 attempt 的详情字段（request_info 和 response_info）
func (r *ProxyUpstreamAttemptRepository) ClearDetailOlderThan(before time.Time) (int64, error) {
	beforeTs := toTimestamp(before)
	now := time.Now().UnixMilli()

	result := r.db.gorm.Model(&ProxyUpstreamAttempt{}).
		Where("created_at < ? AND (request_info IS NOT NULL OR response_info IS NOT NULL)", beforeTs).
		Updates(map[string]any{
			"request_info":  nil,
			"response_info": nil,
			"updated_at":    now,
		})

	return result.RowsAffected, result.Error
}

func (r *ProxyUpstreamAttemptRepository) toModel(a *domain.ProxyUpstreamAttempt) *ProxyUpstreamAttempt {
	return &ProxyUpstreamAttempt{
		BaseModel: BaseModel{
			ID:        a.ID,
			CreatedAt: toTimestamp(a.CreatedAt),
			UpdatedAt: toTimestamp(a.UpdatedAt),
		},
		StartTime:         toTimestamp(a.StartTime),
		EndTime:           toTimestamp(a.EndTime),
		DurationMs:        a.Duration.Milliseconds(),
		TTFTMs:            a.TTFT.Milliseconds(),
		Status:            a.Status,
		ProxyRequestID:    a.ProxyRequestID,
		IsStream:          boolToInt(a.IsStream),
		RequestModel:      a.RequestModel,
		MappedModel:       a.MappedModel,
		ResponseModel:     a.ResponseModel,
		RequestInfo:       LongText(toJSON(a.RequestInfo)),
		ResponseInfo:      LongText(toJSON(a.ResponseInfo)),
		RouteID:           a.RouteID,
		ProviderID:        a.ProviderID,
		InputTokenCount:   a.InputTokenCount,
		OutputTokenCount:  a.OutputTokenCount,
		CacheReadCount:    a.CacheReadCount,
		CacheWriteCount:   a.CacheWriteCount,
		Cache5mWriteCount: a.Cache5mWriteCount,
		Cache1hWriteCount: a.Cache1hWriteCount,
		ModelPriceID:      a.ModelPriceID,
		Multiplier:        a.Multiplier,
		Cost:              a.Cost,
	}
}

func (r *ProxyUpstreamAttemptRepository) toDomain(m *ProxyUpstreamAttempt) *domain.ProxyUpstreamAttempt {
	return &domain.ProxyUpstreamAttempt{
		ID:                m.ID,
		CreatedAt:         fromTimestamp(m.CreatedAt),
		UpdatedAt:         fromTimestamp(m.UpdatedAt),
		StartTime:         fromTimestamp(m.StartTime),
		EndTime:           fromTimestamp(m.EndTime),
		Duration:          time.Duration(m.DurationMs) * time.Millisecond,
		TTFT:              time.Duration(m.TTFTMs) * time.Millisecond,
		Status:            m.Status,
		ProxyRequestID:    m.ProxyRequestID,
		IsStream:          m.IsStream == 1,
		RequestModel:      m.RequestModel,
		MappedModel:       m.MappedModel,
		ResponseModel:     m.ResponseModel,
		RequestInfo:       fromJSON[*domain.RequestInfo](string(m.RequestInfo)),
		ResponseInfo:      fromJSON[*domain.ResponseInfo](string(m.ResponseInfo)),
		RouteID:           m.RouteID,
		ProviderID:        m.ProviderID,
		InputTokenCount:   m.InputTokenCount,
		OutputTokenCount:  m.OutputTokenCount,
		CacheReadCount:    m.CacheReadCount,
		CacheWriteCount:   m.CacheWriteCount,
		Cache5mWriteCount: m.Cache5mWriteCount,
		Cache1hWriteCount: m.Cache1hWriteCount,
		ModelPriceID:      m.ModelPriceID,
		Multiplier:        m.Multiplier,
		Cost:              m.Cost,
	}
}

func (r *ProxyUpstreamAttemptRepository) toDomainList(models []ProxyUpstreamAttempt) []*domain.ProxyUpstreamAttempt {
	attempts := make([]*domain.ProxyUpstreamAttempt, len(models))
	for i, m := range models {
		attempts[i] = r.toDomain(&m)
	}
	return attempts
}
