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

var activeProxyRequestStatuses = []string{"PENDING", "IN_PROGRESS"}

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

func (r *ProxyRequestRepository) GetByID(tenantID uint64, id uint64) (*domain.ProxyRequest, error) {
	var model ProxyRequest
	if err := tenantScope(r.db.gorm, tenantID).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *ProxyRequestRepository) List(tenantID uint64, limit, offset int) ([]*domain.ProxyRequest, error) {
	var models []ProxyRequest
	if err := tenantScope(r.db.gorm, tenantID).Order("id DESC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

// ListCursor 基于游标的分页查询，比 OFFSET 更高效
// before: 获取 id < before 的记录 (向后翻页)
// after: 获取 id > after 的记录 (向前翻页/获取新数据)
// filter: 可选的过滤条件
// 注意：列表查询不返回 request_info 和 response_info 大字段
func (r *ProxyRequestRepository) ListCursor(tenantID uint64, limit int, before, after uint64, filter *repository.ProxyRequestFilter) ([]*domain.ProxyRequest, error) {
	// 使用 Select 排除大字段
	baseQuery := tenantScope(r.db.gorm.Model(&ProxyRequest{}), tenantID).
		Select("id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, ttft_ms, is_stream, status, status_code, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost, api_token_id")

	if after > 0 {
		baseQuery = baseQuery.Where("id > ?", after)
	} else if before > 0 {
		baseQuery = baseQuery.Where("id < ?", before)
	}

	// 应用过滤条件
	if filter != nil {
		if filter.ProviderID != nil {
			baseQuery = baseQuery.Where("provider_id = ?", *filter.ProviderID)
		}
		if filter.Status != nil {
			baseQuery = baseQuery.Where("status = ?", *filter.Status)
		}
		if filter.APITokenID != nil {
			baseQuery = baseQuery.Where("api_token_id = ?", *filter.APITokenID)
		}
		if filter.ProjectID != nil {
			baseQuery = baseQuery.Where("project_id = ?", *filter.ProjectID)
		}
	}

	orderBy := "id DESC"
	if after > 0 {
		orderBy = "id ASC"
	}

	// 游标分页必须保持全局单调 ID 顺序，活跃优先排序交给前端展示层处理。
	var models []ProxyRequest
	if err := baseQuery.Order(orderBy).Limit(limit).Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

// ListActive 获取所有活跃请求 (PENDING 或 IN_PROGRESS 状态)
func (r *ProxyRequestRepository) ListActive(tenantID uint64) ([]*domain.ProxyRequest, error) {
	var models []ProxyRequest
	if err := tenantScope(r.db.gorm.Model(&ProxyRequest{}), tenantID).
		Select("id, created_at, updated_at, instance_id, request_id, session_id, client_type, request_model, response_model, start_time, end_time, duration_ms, is_stream, status, status_code, error, proxy_upstream_attempt_count, final_proxy_upstream_attempt_id, route_id, provider_id, project_id, input_token_count, output_token_count, cache_read_count, cache_write_count, cache_5m_write_count, cache_1h_write_count, cost, api_token_id").
		Where("status IN ?", activeProxyRequestStatuses).
		Order("id DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ProxyRequestRepository) Count(tenantID uint64) (int64, error) {
	if tenantID == domain.TenantIDAll {
		return atomic.LoadInt64(&r.count), nil
	}
	var count int64
	if err := tenantScope(r.db.gorm.Model(&ProxyRequest{}), tenantID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountWithFilter 带过滤条件的计数
func (r *ProxyRequestRepository) CountWithFilter(tenantID uint64, filter *repository.ProxyRequestFilter) (int64, error) {
	// 如果没有过滤条件且没有 tenantID 过滤，使用缓存的总数
	if tenantID == domain.TenantIDAll && (filter == nil || (filter.ProviderID == nil && filter.Status == nil && filter.APITokenID == nil && filter.ProjectID == nil)) {
		return atomic.LoadInt64(&r.count), nil
	}

	// 有过滤条件时需要查询数据库
	var count int64
	query := tenantScope(r.db.gorm.Model(&ProxyRequest{}), tenantID)
	if filter != nil {
		if filter.ProviderID != nil {
			query = query.Where("provider_id = ?", *filter.ProviderID)
		}
		if filter.Status != nil {
			query = query.Where("status = ?", *filter.Status)
		}
		if filter.APITokenID != nil {
			query = query.Where("api_token_id = ?", *filter.APITokenID)
		}
		if filter.ProjectID != nil {
			query = query.Where("project_id = ?", *filter.ProjectID)
		}
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// 死实例孤儿请求的回收宽限期。当一个请求的 instance_id 不在活实例列表里时,
// 等它的 start_time 早于 (now - DeadInstanceGracePeriod) 才标记为 FAILED。
// 这个宽限期覆盖以下场景:
//   - 新启动的实例完成 RegisterInstance 之前可能已经下发了少量请求
//   - 实例 ID 一时未来得及同步到 coordinator(网络抖动)
// 选 60s 与心跳 TTL 对齐;单实例重启场景下,旧 in-progress 请求等 60s 后被清理,
// 远好于原行为(立刻杀)和过保守行为(等 30min)之间。
const deadInstanceGraceMillis = int64(60 * 1000)

// MarkStaleAsFailed marks IN_PROGRESS/PENDING requests as FAILED when their
// owning instance is no longer alive, or when the request has been in flight
// for more than 30 minutes (timeout).
//
// 关键安全语义:
//   - aliveInstanceIDs 为空 → 直接返回 0,不做任何回收。防止 coordinator
//     异常导致全表误杀。调用方在 coordinator 健康时才该调用。
//   - 多实例环境下,只清理 (a) instance_id 不在活实例集合且过了 60s 宽限期,
//     或 (b) 任意实例上 start_time 超过 30min 的卡死请求。
func (r *ProxyRequestRepository) MarkStaleAsFailed(aliveInstanceIDs []string) (int64, error) {
	if len(aliveInstanceIDs) == 0 {
		return 0, nil
	}

	nowMs := time.Now().UnixMilli()
	timeoutThreshold := nowMs - int64(30*time.Minute/time.Millisecond)
	deadGraceThreshold := nowMs - deadInstanceGraceMillis

	// 死实例分支用 COALESCE(NULLIF(start_time, 0), created_at):PENDING 状态
	// 请求可能 start_time = 0(还没真正开始处理就被卡在队列),如果只看
	// start_time 这些请求永远不会被回收。fallback 到 created_at 让"创建超过
	// 60s 且实例已死"的请求也能被清。
	//
	// 30min 硬超时分支仍只看 start_time > 0:超时本质上是"已开始但卡死太久",
	// 还没开始的 PENDING 不算超时(它由死实例分支或 hourly cleanup 处理)。
	result := r.db.gorm.Exec(`
		UPDATE proxy_requests
		SET status = 'FAILED',
		    error = CASE
		        WHEN start_time > 0 AND start_time < ? THEN 'Request timed out (stuck in progress)'
		        ELSE 'Instance no longer alive'
		    END,
		    end_time = ?,
		    duration_ms = CASE
		        WHEN start_time > 0 THEN ? - start_time
		        ELSE 0
		    END,
		    updated_at = ?
		WHERE status IN ('PENDING', 'IN_PROGRESS')
		  AND (
		      ((instance_id IS NULL OR instance_id NOT IN (?))
		         AND COALESCE(NULLIF(start_time, 0), created_at) < ?)
		      OR
		      (start_time > 0 AND start_time < ?)
		  )`,
		timeoutThreshold, nowMs, nowMs, nowMs,
		aliveInstanceIDs, deadGraceThreshold,
		timeoutThreshold,
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
func (r *ProxyRequestRepository) UpdateProjectIDBySessionID(tenantID uint64, sessionID string, projectID uint64) (int64, error) {
	now := time.Now().UnixMilli()
	result := tenantScope(r.db.gorm.Model(&ProxyRequest{}), tenantID).
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
//
// 分批处理：避免一次性 pluck 海量 id + 两次大 IN DELETE 产生超大事务。
func (r *ProxyRequestRepository) DeleteOlderThan(before time.Time) (int64, error) {
	const batchSize = 500
	beforeTs := toTimestamp(before)
	var total int64
	var lastID uint64

	for {
		var ids []uint64
		if err := r.db.gorm.Model(&ProxyRequest{}).
			Where("created_at < ? AND id > ?", beforeTs, lastID).
			Order("id").
			Limit(batchSize).
			Pluck("id", &ids).Error; err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		// keyset 推进：即使本批写命中 0 行也保证下一批跳过这段 id 区间，不会 livelock
		lastID = ids[len(ids)-1]

		var batchAffected int64
		if err := r.db.gorm.Transaction(func(tx *gorm.DB) error {
			// 子删除限定到当前仍 eligible 的 parent（防止 parent 不可删时 attempts 成孤儿）
			eligibleParents := tx.Model(&ProxyRequest{}).
				Select("id").
				Where("id IN ? AND created_at < ?", ids, beforeTs)
			if err := tx.Where("proxy_request_id IN (?)", eligibleParents).Delete(&ProxyUpstreamAttempt{}).Error; err != nil {
				return err
			}
			res := tx.Where("id IN ? AND created_at < ?", ids, beforeTs).Delete(&ProxyRequest{})
			if res.Error != nil {
				return res.Error
			}
			batchAffected = res.RowsAffected
			return nil
		}); err != nil {
			return total, err
		}
		if batchAffected > 0 {
			atomic.AddInt64(&r.count, -batchAffected)
		}
		total += batchAffected

		if len(ids) < batchSize {
			return total, nil
		}
	}
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

// UpdateCostAtomically 一个事务里同时更新 proxy_requests.cost 和一批 attempt 的 cost+model_price_id。
// 保证 proxy_requests.cost == SUM(proxy_upstream_attempts.cost) 这条不变量在中途不会被打破:
// 如果只用两步独立写,attempt 写完 / request 写失败 会留下父子不一致的窗口,审计后续很难发现。
func (r *ProxyRequestRepository) UpdateCostAtomically(requestID, requestCost uint64, attemptUpdates map[uint64]domain.AttemptCostUpdate) error {
	return r.db.gorm.Transaction(func(tx *gorm.DB) error {
		if err := batchUpdateAttemptCostsInTx(tx, attemptUpdates); err != nil {
			return err
		}
		return tx.Model(&ProxyRequest{}).Where("id = ?", requestID).Update("cost", requestCost).Error
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

// detailCleanupIndexMissing 由启动时健康检查置位:MySQL detail-cleanup 索引不存在时
// 设为 1。设置后 detailCleanupBatchParams 退化回保守批次(200/50ms),避免在无索引的
// 大表上以 batch=1000 反复触发 full-table-scan 把 IOPS 打满。
//
// 用 atomic.Int32 而非 mutex:写一次(启动),读高频(每次清理批次),无锁读最便宜。
var detailCleanupIndexMissing atomic.Int32

// detailClearedColumnMissing 由启动时检查置位:大表 threshold-skip 导致 v15 没建
// detail_cleared 列时设为 1。ClearDetailOlderThan 据此走 legacy IS NOT NULL 谓词,
// 否则查询会因列不存在而每个 tick 都失败——是功能失效,不是降级变慢(Codex 抓到)。
var detailClearedColumnMissing atomic.Int32

// SetDetailClearedColumnMissing 设置 detail_cleared 列缺失状态。
//
// 调用方:internal/core/task.go:checkDetailClearedColumnHealth 在启动时探测一次。
// 同进程内若列后续被运维手动补建,需重启进程才能恢复 fast-path——这是与
// detailCleanupIndexMissing 一致的语义,避免运行时反复轮询。
func SetDetailClearedColumnMissing(missing bool) {
	if missing {
		detailClearedColumnMissing.Store(1)
	} else {
		detailClearedColumnMissing.Store(0)
	}
}

// detailClearedColumnAvailable 返回是否可以使用 detail_cleared sentinel。
func detailClearedColumnAvailable() bool {
	return detailClearedColumnMissing.Load() == 0
}

// SetDetailCleanupIndexMissing 设置 MySQL detail-cleanup 索引缺失状态。startup
// health-check 调用,见 internal/core/task.go:checkDetailCleanupIndexHealth。
//
// 显式 set(true/false):每次启动健康检查时都覆盖写,避免之前进程态/测试态遗留的 sticky
// 标志位污染后续判断。同进程内若索引被运维补建,需要重启进程才能恢复 fast-path;
// 这是可接受的权衡——避免运行时反复轮询 INFORMATION_SCHEMA 的开销。
func SetDetailCleanupIndexMissing(missing bool) {
	if missing {
		detailCleanupIndexMissing.Store(1)
	} else {
		detailCleanupIndexMissing.Store(0)
	}
}

// detailCleanupBatchParams 返回当前 dialect 下 detail cleanup 批次大小与 batch 间 sleep。
//
//   - SQLite:200 / 50ms。SQLite WAL 是单写者锁,大 batch 会让 API INSERT 长时间等待
//     ("卡死"问题的根因);沿用 v0.13.77 的保守值,验证稳定。
//   - MySQL 且索引就绪:1000 / 20ms。改完 v14 索引后 SELECT 亚秒级,瓶颈转到 UPDATE
//     网络往返,大 batch 把往返摊薄到更多行。
//   - MySQL 但索引缺失(threshold-skip 且没手动建):退化回 200 / 50ms。无索引时 SELECT
//     是 full-scan-per-batch,大 batch 不会摊薄全扫成本,反而每批次锁更多行;保守批次
//     +更长 sleep 减小对在线流量的干扰。startup 已经打了告警日志,运维应当尽快建索引。
func detailCleanupBatchParams(dialector string) (batchSize int, sleep time.Duration) {
	switch dialector {
	case "mysql":
		if detailCleanupIndexMissing.Load() == 1 {
			return 200, 50 * time.Millisecond
		}
		return 1000, 20 * time.Millisecond
	default:
		return 200, 50 * time.Millisecond
	}
}

// maxCleanupBatchesPerCall 限制单次 ClearDetailOlderThan 调用最多处理多少 batch。
//
// 解决"一次调用 drain-to-completion 跑 43 min"的延迟尾问题:
//   - 50 batch × 1000 行(MySQL) = 50k 行 / 调用,单次 wall-clock 几秒
//   - SQLite 200/50ms 时 50 × 200 = 10k 行 / 调用,同样几秒收敛
//   - 配合 sentinel 索引,backlog 在多次 tick 内被分摊处理,不会单次卡死
//
// var 而非 const:测试中需要临时调小验证 cap 行为(TestClearDetailOlderThan_RespectsBatchCap)。
// 生产路径不应修改。
var maxCleanupBatchesPerCall = 50

// ClearDetailOlderThan 清理指定时间之前请求的详情字段（request_info 和 response_info）
// statuses 为空时不按状态过滤；非空时仅清理 status IN (statuses) 的记录
//
// 设计:
//
//  1. **sentinel column (detail_cleared)**:WHERE detail_cleared = 0 是 leading-column
//     等值匹配,planner 在 v15 索引 (detail_cleared, created_at, id) 的 0-段做范围扫,
//     **无需回行评估 LONGTEXT NULL flag**——这是 MySQL 上 43-min stall 的真正根因。
//     UPDATE 同步置 detail_cleared = 1,清完的行自动离开 0-段。稳态下 0-段几乎空。
//
//  2. **within-call cursor (created_at, id)**:同一次函数调用内分批用游标。**不跨调用
//     持久化**——曾经持久化过,会与可变 status 过滤冲突:PENDING 行先被 cursor 越过,
//     之后转 COMPLETED/FAILED 时永远在 cursor 后,清不到(Codex review 抓到)。
//     去掉持久化后每次从 0-段起点扫,稳态依赖 sentinel 让起点接近真实未清行。
//
//  3. **maxCleanupBatchesPerCall 封顶**:即使 backlog 巨大,单次调用 wall-clock 秒级,
//     不再 43-min。下次 tick 接力。
//
// 重应用谓词:Pluck 与 UPDATE 之间行的 status/dev_mode 可能变动,UPDATE WHERE 必须再次
// 校验所有过滤条件,避免错改 dev_mode 行或状态已变更的行。
func (r *ProxyRequestRepository) ClearDetailOlderThan(before time.Time, statuses []string) (int64, error) {
	batchSize, batchSleep := detailCleanupBatchParams(r.db.Dialector())
	beforeTs := toTimestamp(before)
	useSentinel := detailClearedColumnAvailable()
	var total int64
	var lastCreatedAt int64
	var lastID uint64

	// 谓词分支:
	//   - useSentinel=true(常态):detail_cleared = 0,planner 走 v15 sentinel 索引
	//   - useSentinel=false(大表 threshold-skip 列没建):退化到 v13/v14 legacy 谓词,
	//     虽然慢但功能正常。运维补建列+重启即恢复 fast-path。
	selectPred := "detail_cleared = 0 AND created_at < ? AND dev_mode = 0"
	updatePred := "id IN ? AND detail_cleared = 0 AND created_at < ? AND dev_mode = 0"
	updateMap := map[string]any{
		"request_info":   nil,
		"response_info":  nil,
		"detail_cleared": 1,
		"updated_at":     time.Now().UnixMilli(),
	}
	if !useSentinel {
		selectPred = "(request_info IS NOT NULL OR response_info IS NOT NULL) AND created_at < ? AND dev_mode = 0"
		updatePred = "id IN ? AND (request_info IS NOT NULL OR response_info IS NOT NULL) AND created_at < ? AND dev_mode = 0"
		updateMap = map[string]any{
			"request_info":  nil,
			"response_info": nil,
			"updated_at":    time.Now().UnixMilli(),
		}
	}

	type cursorRow struct {
		ID        uint64 `gorm:"column:id"`
		CreatedAt int64  `gorm:"column:created_at"`
	}

	for batchIdx := 0; batchIdx < maxCleanupBatchesPerCall; batchIdx++ {
		var rows []cursorRow
		q := r.db.gorm.Model(&ProxyRequest{}).
			Select("id, created_at").
			Where(selectPred, beforeTs).
			Where("(created_at > ? OR (created_at = ? AND id > ?))", lastCreatedAt, lastCreatedAt, lastID)
		if len(statuses) > 0 {
			q = q.Where("status IN ?", statuses)
		}
		if err := q.Order("created_at, id").Limit(batchSize).Scan(&rows).Error; err != nil {
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

		// 每个 batch 用当前时刻刷新 updated_at;updateMap 在循环外构造时是初始时刻,
		// 长 backlog 下让 updated_at 反映最近一次实际写入更精确。
		updateMap["updated_at"] = time.Now().UnixMilli()
		uq := r.db.gorm.Model(&ProxyRequest{}).Where(updatePred, ids, beforeTs)
		if len(statuses) > 0 {
			uq = uq.Where("status IN ?", statuses)
		}
		result := uq.Updates(updateMap)
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

func (r *ProxyRequestRepository) toModel(p *domain.ProxyRequest) *ProxyRequest {
	return &ProxyRequest{
		BaseModel: BaseModel{
			ID:        p.ID,
			CreatedAt: toTimestamp(p.CreatedAt),
			UpdatedAt: toTimestamp(p.UpdatedAt),
		},
		TenantID:                    p.TenantID,
		InstanceID:                  p.InstanceID,
		RequestID:                   p.RequestID,
		SessionID:                   p.SessionID,
		ClientType:                  string(p.ClientType),
		RequestModel:                p.RequestModel,
		ResponseModel:               p.ResponseModel,
		StartTime:                   toTimestamp(p.StartTime),
		EndTime:                     toTimestamp(p.EndTime),
		DurationMs:                  p.Duration.Milliseconds(),
		TTFTMs:                      p.TTFT.Milliseconds(),
		IsStream:                    boolToInt(p.IsStream),
		Status:                      p.Status,
		StatusCode:                  p.StatusCode,
		RequestInfo:                 LongText(toJSON(p.RequestInfo)),
		ResponseInfo:                LongText(toJSON(p.ResponseInfo)),
		Error:                       LongText(p.Error),
		ProxyUpstreamAttemptCount:   p.ProxyUpstreamAttemptCount,
		FinalProxyUpstreamAttemptID: p.FinalProxyUpstreamAttemptID,
		RouteID:                     p.RouteID,
		ProviderID:                  p.ProviderID,
		ProjectID:                   p.ProjectID,
		InputTokenCount:             p.InputTokenCount,
		OutputTokenCount:            p.OutputTokenCount,
		CacheReadCount:              p.CacheReadCount,
		CacheWriteCount:             p.CacheWriteCount,
		Cache5mWriteCount:           p.Cache5mWriteCount,
		Cache1hWriteCount:           p.Cache1hWriteCount,
		ModelPriceID:                p.ModelPriceID,
		Multiplier:                  p.Multiplier,
		Cost:                        p.Cost,
		APITokenID:                  p.APITokenID,
		DevMode:                     boolToInt(p.DevMode),
	}
}

func (r *ProxyRequestRepository) toDomain(m *ProxyRequest) *domain.ProxyRequest {
	return &domain.ProxyRequest{
		ID:                          m.ID,
		CreatedAt:                   fromTimestamp(m.CreatedAt),
		UpdatedAt:                   fromTimestamp(m.UpdatedAt),
		TenantID:                    m.TenantID,
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
		DevMode:                     m.DevMode == 1,
	}
}

func (r *ProxyRequestRepository) toDomainList(models []ProxyRequest) []*domain.ProxyRequest {
	requests := make([]*domain.ProxyRequest, len(models))
	for i, m := range models {
		requests[i] = r.toDomain(&m)
	}
	return requests
}
