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
			ID                    uint64 `gorm:"column:id"`
			ProxyRequestID        uint64 `gorm:"column:proxy_request_id"`
			ResponseModel         string `gorm:"column:response_model"`
			MappedModel           string `gorm:"column:mapped_model"`
			RequestModel          string `gorm:"column:request_model"`
			InputTokenCount       uint64 `gorm:"column:input_token_count"`
			OutputTokenCount      uint64 `gorm:"column:output_token_count"`
			InputImageTokenCount  uint64 `gorm:"column:input_image_token_count"`
			OutputImageTokenCount uint64 `gorm:"column:output_image_token_count"`
			CacheReadCount        uint64 `gorm:"column:cache_read_count"`
			CacheWriteCount       uint64 `gorm:"column:cache_write_count"`
			Cache5mWriteCount     uint64 `gorm:"column:cache_5m_write_count"`
			Cache1hWriteCount     uint64 `gorm:"column:cache_1h_write_count"`
			Cost                  uint64 `gorm:"column:cost"`
			Multiplier            uint64 `gorm:"column:multiplier"`
			ModelPriceID          uint64 `gorm:"column:model_price_id"`
		}

		err := r.db.gorm.Table("proxy_upstream_attempts").
			Select("id, proxy_request_id, response_model, mapped_model, request_model, input_token_count, output_token_count, input_image_token_count, output_image_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost, multiplier, model_price_id").
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
				ID:                    r.ID,
				ProxyRequestID:        r.ProxyRequestID,
				ResponseModel:         r.ResponseModel,
				MappedModel:           r.MappedModel,
				RequestModel:          r.RequestModel,
				InputTokenCount:       r.InputTokenCount,
				OutputTokenCount:      r.OutputTokenCount,
				InputImageTokenCount:  r.InputImageTokenCount,
				OutputImageTokenCount: r.OutputImageTokenCount,
				CacheReadCount:        r.CacheReadCount,
				CacheWriteCount:       r.CacheWriteCount,
				Cache5mWriteCount:     r.Cache5mWriteCount,
				Cache1hWriteCount:     r.Cache1hWriteCount,
				Cost:                  r.Cost,
				Multiplier:            r.Multiplier,
				ModelPriceID:          r.ModelPriceID,
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

// BatchUpdateCosts 批量更新 attempt 的 cost 和 model_price_id。
// model_price_id 在重算时会跟着 cost 一起更新到当前匹配的价格记录,二者保持一致。
//
// 内部走 r.db.gorm.Transaction;若调用方已经持有 tx,改用 batchUpdateAttemptCostsInTx 直接拼进去。
func (r *ProxyUpstreamAttemptRepository) BatchUpdateCosts(updates map[uint64]domain.AttemptCostUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return r.db.gorm.Transaction(func(tx *gorm.DB) error {
		return batchUpdateAttemptCostsInTx(tx, updates)
	})
}

// batchUpdateAttemptCostsInTx 用调用方提供的 tx 执行批量 cost+model_price_id 更新。
// 抽出来供需要跨表事务的路径(例如 ProxyRequestRepository.UpdateCostAtomically)调用,
// 让父请求 cost 和 attempt cost 在一个事务里写,避免 partial-state window。
func batchUpdateAttemptCostsInTx(tx *gorm.DB, updates map[uint64]domain.AttemptCostUpdate) error {
	if len(updates) == 0 {
		return nil
	}
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

		var costCases, priceIDCases strings.Builder
		costCases.WriteString("CASE id ")
		priceIDCases.WriteString("CASE id ")
		args := make([]interface{}, 0, len(batchIDs)*4)

		for _, id := range batchIDs {
			costCases.WriteString("WHEN ? THEN ? ")
			args = append(args, id, updates[id].Cost)
		}
		costCases.WriteString("END")

		for _, id := range batchIDs {
			priceIDCases.WriteString("WHEN ? THEN ? ")
			args = append(args, id, updates[id].ModelPriceID)
		}
		priceIDCases.WriteString("END")

		// timestamp for updated_at
		args = append(args, time.Now().UnixMilli())

		// WHERE IN ids
		for _, id := range batchIDs {
			args = append(args, id)
		}

		sql := fmt.Sprintf(
			"UPDATE proxy_upstream_attempts SET cost = %s, model_price_id = %s, updated_at = ? WHERE id IN (?%s)",
			costCases.String(), priceIDCases.String(), strings.Repeat(",?", len(batchIDs)-1),
		)

		if err := tx.Exec(sql, args...).Error; err != nil {
			return err
		}
	}
	return nil
}

// ClearDetailOlderThan 清理指定时间之前 attempt 的详情字段（request_info 和 response_info）
// statuses 为空时不按状态过滤；非空时仅清理所属 ProxyRequest.status IN (statuses) 的 attempt
//
// 设计三件套（cursor + sentinel + cap）见 ProxyRequestRepository.ClearDetailOlderThan
// 的详细注释。这里只描述 attempt 特有的逻辑:
//
//   - **父表过滤用 EXISTS**:SQLite/MySQL planner 都把 EXISTS 当 anti-join,先驱
//     attempt 端的 (detail_cleared, created_at, id) 索引,再对每个候选 PK 查父行。
//     原先 IN(subquery) 形式 SQLite 会从父表驱动,绕开 attempt 端的索引,稳态退化。
//   - **sentinel**:同 request 表,detail_cleared = 0 是 SELECT 的 leading-column
//     等值匹配,索引 idx_proxy_upstream_attempts_detail_cleared (detail_cleared,
//     created_at, id) 让范围扫不回行。
func (r *ProxyUpstreamAttemptRepository) ClearDetailOlderThan(before time.Time, statuses []string) (int64, error) {
	batchSize, batchSleep := detailCleanupBatchParams(r.db.Dialector())
	beforeTs := toTimestamp(before)
	useSentinel := detailClearedColumnAvailable()
	var total int64
	var lastCreatedAt int64
	var lastID uint64

	// Sentinel vs legacy 谓词分支,见 proxy_request.go 同名函数的注释。
	selectPred := "detail_cleared = 0 AND created_at < ?"
	updatePred := "id IN ? AND detail_cleared = 0 AND created_at < ?"
	updateMap := map[string]any{
		"request_info":   nil,
		"response_info":  nil,
		"detail_cleared": 1,
	}
	if !useSentinel {
		selectPred = "(request_info IS NOT NULL OR response_info IS NOT NULL) AND created_at < ?"
		updatePred = "id IN ? AND (request_info IS NOT NULL OR response_info IS NOT NULL) AND created_at < ?"
		updateMap = map[string]any{
			"request_info":  nil,
			"response_info": nil,
		}
	}

	// parentExistsClause 构造相关 EXISTS：通过 attempt.proxy_request_id 关联父行，
	// 然后过滤父行 dev_mode/status。返回 SQL 片段 + 对应 args。
	parentExistsClause := func() (string, []any) {
		sql := "EXISTS (SELECT 1 FROM proxy_requests WHERE proxy_requests.id = proxy_upstream_attempts.proxy_request_id AND proxy_requests.dev_mode = 0"
		args := []any{}
		if len(statuses) > 0 {
			sql += " AND proxy_requests.status IN ?"
			args = append(args, statuses)
		}
		sql += ")"
		return sql, args
	}

	type cursorRow struct {
		ID        uint64 `gorm:"column:id"`
		CreatedAt int64  `gorm:"column:created_at"`
	}

	for batchIdx := 0; batchIdx < maxCleanupBatchesPerCall; batchIdx++ {
		var rows []cursorRow
		existsSQL, existsArgs := parentExistsClause()
		if err := r.db.gorm.Model(&ProxyUpstreamAttempt{}).
			Select("id, created_at").
			Where(selectPred, beforeTs).
			Where("(created_at > ? OR (created_at = ? AND id > ?))", lastCreatedAt, lastCreatedAt, lastID).
			Where(existsSQL, existsArgs...).
			Order("created_at, id").
			Limit(batchSize).
			Scan(&rows).Error; err != nil {
			return total, err
		}
		if len(rows) == 0 {
			return total, nil
		}
		ids := make([]uint64, len(rows))
		for i, row := range rows {
			ids[i] = row.ID
		}
		last := rows[len(rows)-1]
		lastCreatedAt = last.CreatedAt
		lastID = last.ID

		updateMap["updated_at"] = time.Now().UnixMilli()
		result := r.db.gorm.Model(&ProxyUpstreamAttempt{}).
			Where(updatePred, ids, beforeTs).
			Where(existsSQL, existsArgs...).
			Updates(updateMap)
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected

		if len(rows) < batchSize {
			return total, nil
		}
		time.Sleep(batchSleep)
	}
	return total, nil
}

func (r *ProxyUpstreamAttemptRepository) toModel(a *domain.ProxyUpstreamAttempt) *ProxyUpstreamAttempt {
	return &ProxyUpstreamAttempt{
		BaseModel: BaseModel{
			ID:        a.ID,
			CreatedAt: toTimestamp(a.CreatedAt),
			UpdatedAt: toTimestamp(a.UpdatedAt),
		},
		TenantID:          a.TenantID,
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
		RouteID:               a.RouteID,
		ProviderID:            a.ProviderID,
		InputTokenCount:       a.InputTokenCount,
		OutputTokenCount:      a.OutputTokenCount,
		InputImageTokenCount:  a.InputImageTokenCount,
		OutputImageTokenCount: a.OutputImageTokenCount,
		CacheReadCount:        a.CacheReadCount,
		CacheWriteCount:       a.CacheWriteCount,
		Cache5mWriteCount:     a.Cache5mWriteCount,
		Cache1hWriteCount:     a.Cache1hWriteCount,
		ModelPriceID:          a.ModelPriceID,
		Multiplier:            a.Multiplier,
		Cost:                  a.Cost,
	}
}

func (r *ProxyUpstreamAttemptRepository) toDomain(m *ProxyUpstreamAttempt) *domain.ProxyUpstreamAttempt {
	return &domain.ProxyUpstreamAttempt{
		ID:                m.ID,
		CreatedAt:         fromTimestamp(m.CreatedAt),
		UpdatedAt:         fromTimestamp(m.UpdatedAt),
		TenantID:          m.TenantID,
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
		RouteID:               m.RouteID,
		ProviderID:            m.ProviderID,
		InputTokenCount:       m.InputTokenCount,
		OutputTokenCount:      m.OutputTokenCount,
		InputImageTokenCount:  m.InputImageTokenCount,
		OutputImageTokenCount: m.OutputImageTokenCount,
		CacheReadCount:        m.CacheReadCount,
		CacheWriteCount:       m.CacheWriteCount,
		Cache5mWriteCount:     m.Cache5mWriteCount,
		Cache1hWriteCount:     m.Cache1hWriteCount,
		ModelPriceID:          m.ModelPriceID,
		Multiplier:            m.Multiplier,
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
