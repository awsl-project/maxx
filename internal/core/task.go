package core

import (
	"context"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/service"
)

const (
	defaultRequestRetentionHours = 168 // 默认保留 168 小时（7天）
	defaultSessionRetentionHours = 168 // 默认保留 168 小时（7天）

	// sqlite maintenance throttle
	sqliteVacuumMinInterval = time.Hour
)

var (
	sqliteMaintenanceMu            sync.Mutex
	sqliteMaintenanceLastCompleted time.Time
	sqliteMaintenanceInProgress    bool
)

// BackgroundTaskDeps 后台任务依赖
type BackgroundTaskDeps struct {
	// DB is optional. When provided and the dialector is sqlite, cleanup tasks will
	// best-effort run WAL checkpoint + VACUUM with throttling.
	DB *sqlite.DB

	UsageStats         repository.UsageStatsRepository
	ProxyRequest       repository.ProxyRequestRepository
	AttemptRepo        repository.ProxyUpstreamAttemptRepository
	SessionRepo        repository.SessionRepository
	Settings           repository.SystemSettingRepository
	AntigravityTaskSvc *service.AntigravityTaskService
	CodexTaskSvc       *service.CodexTaskService

	// Coordinator 可选。提供时,数据维护类清理任务(retention purge / detail cleanup)
	// 只在 leader 实例上跑——多副本共享同一 RDS 时,6 副本同时清理会把 IOPS 放大 6 倍互相 race。
	// 选举规则:对活实例 ID 排序,字典序最小的是 leader。简单、无外部依赖、TTL=heartbeat。
	// Coordinator=nil 时退化为"总是 leader"(单实例 / 老路径)。
	Coordinator coordinator.Coordinator
}

// leaderCheckTimeout 限制 coordinator 查询活实例列表的耗时;超时则保守退化为"非 leader",
// 宁可这一轮不跑也不要让 coordinator 异常拖住后台任务。
const leaderCheckTimeout = 2 * time.Second

// isCleanupLeader 判断当前实例是否应当跑数据维护清理任务。
//
//   - Coordinator 为 nil → 单实例部署,总是 leader。
//   - Coordinator.ListAliveInstances 失败/超时 → 保守返回 false。coordinator 不可用时
//     宁可不清理也不要 6 副本一起跑(回到反馈中提到的 IOPS race 老问题)。
//   - 选举:排序后最小 ID 是 leader,与本实例 ID 比较。
//     无锁、无任期、无 split-brain 保护——清理任务幂等(再跑一次只是重复 WHERE 没行可清),
//     即使瞬时两实例都自认 leader 也只是少量重复 IOPS,不会数据错乱。
func (d *BackgroundTaskDeps) isCleanupLeader() bool {
	if d.Coordinator == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), leaderCheckTimeout)
	defer cancel()
	alive, err := d.Coordinator.ListAliveInstances(ctx)
	if err != nil {
		log.Printf("[Task] leader check: ListAliveInstances failed (%v); skipping this tick", err)
		return false
	}
	if len(alive) == 0 {
		// 包括本实例都没注册的极端情况——保守跳过。
		return false
	}
	sort.Strings(alive)
	return alive[0] == d.Coordinator.InstanceID()
}

// StartBackgroundTasks 启动所有后台任务
func StartBackgroundTasks(deps BackgroundTaskDeps) {
	// 启动时校验:多副本部署日志一行,方便运维判断生效模式。
	if deps.Coordinator != nil {
		log.Printf("[Task] cleanup leader gating enabled (instance=%s)", deps.Coordinator.InstanceID())
	} else {
		log.Printf("[Task] cleanup leader gating disabled (single-instance mode)")
	}
	// 启动时校验:MySQL 上 detail-cleanup 索引若缺失,稳态 SELECT 会全表扫,
	// 配合大 batch 反而把 IOPS 放大。打印明显告警 + manual SQL,让运维不至于错过。
	deps.checkDetailCleanupIndexHealth()
	deps.checkDetailClearedColumnHealth()
	// 统计聚合任务（每 30 秒）- 聚合原始数据并自动 rollup 到各粒度
	go func() {
		time.Sleep(5 * time.Second) // 初始延迟
		for range deps.UsageStats.AggregateAndRollUp(0) {
			// drain the channel to wait for completion
		}

		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			for range deps.UsageStats.AggregateAndRollUp(0) {
				// drain the channel to wait for completion
			}
		}
	}()

	// 清理任务（每小时）- 清理过期的分钟/小时数据和请求记录
	go func() {
		time.Sleep(20 * time.Second) // 初始延迟
		deps.runCleanupTasks()

		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			deps.runCleanupTasks()
		}
	}()

	// 请求详情清理任务（动态间隔）- 根据配置的保留秒数动态调整
	go deps.runRequestDetailCleanup()

	// Antigravity 配额刷新任务（动态间隔）
	if deps.AntigravityTaskSvc != nil {
		go deps.runAntigravityQuotaRefresh()
	}

	// Codex 配额刷新任务（动态间隔）
	if deps.CodexTaskSvc != nil {
		go deps.runCodexQuotaRefresh()
	}

	log.Println("[Task] Background tasks started (aggregation:30s, cleanup:1h, detail-cleanup:dynamic)")
}

// checkDetailClearedColumnHealth 在所有 dialect 上验证 v15 detail_cleared 列是否就绪。
//
// 背景:v15 migration 在 proxy_requests / proxy_upstream_attempts 行数 > 500k 时跳过
// ADD COLUMN(避免大表升级阻塞写入)。如果运维忽略 manual SQL,稳态 ClearDetailOlderThan
// 的 SELECT 会引用不存在的列 → 每个 tick 都报错 → cleanup 功能直接挂掉(不是变慢)。
// 启动时探测列是否存在,缺失则置位 flag,sentinel-aware 的清理代码退化到 v13/v14 legacy
// 谓词(`request_info IS NOT NULL OR response_info IS NOT NULL`),保证功能正常。
//
// 检查为 best-effort:DB 字段为 nil(测试场景)直接跳过。任一表缺列即视为 missing。
func (d *BackgroundTaskDeps) checkDetailClearedColumnHealth() {
	if d.DB == nil {
		return
	}
	// 列类型与 migration v15 的 dialect 分支保持一致——否则运维复制 manual SQL 时
	// Postgres 上 TINYINT 直接 1064 语法错(awsl233777 在 PR #568 catch)。
	columnType := "TINYINT"
	if d.DB.Dialector() == "postgres" {
		columnType = "SMALLINT"
	}
	tables := []string{"proxy_requests", "proxy_upstream_attempts"}
	missing := false
	for _, table := range tables {
		exists, err := detailClearedColumnExists(d.DB, table)
		if err != nil {
			log.Printf("[Task] WARNING: failed to probe %s.detail_cleared (%v); assuming missing", table, err)
			missing = true
			break
		}
		if !exists {
			missing = true
			log.Printf("[Task] WARNING: %s.detail_cleared column missing — cleanup will use legacy IS NOT NULL predicate (slow but functional). Apply manually:\n"+
				"  ALTER TABLE %s ADD COLUMN detail_cleared %s NOT NULL DEFAULT 0;", table, table, columnType)
			break
		}
	}
	sqlite.SetDetailClearedColumnMissing(missing)
	if !missing {
		log.Printf("[Task] detail_cleared sentinel column present on both tables (fast path enabled)")
	}
}

// detailClearedColumnExists 跨 dialect 探测列是否存在。
//
// 注意 Postgres 分支:不能 fall through 到 SQLite PRAGMA 路径。Postgres 会以
// SQLSTATE 42601 拒绝 PRAGMA 并 abort 当前事务,后续查询全部失败(multiinstance CI 抓到)。
func detailClearedColumnExists(db *sqlite.DB, table string) (bool, error) {
	switch db.Dialector() {
	case "mysql":
		var n int64
		err := db.GormDB().Raw(`
			SELECT COUNT(*) FROM information_schema.COLUMNS
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = 'detail_cleared'
		`, table).Scan(&n).Error
		return n > 0, err
	case "postgres":
		var n int64
		err := db.GormDB().Raw(`
			SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = 'detail_cleared'
		`, table).Scan(&n).Error
		return n > 0, err
	default:
		// SQLite
		rows, err := db.GormDB().Raw("PRAGMA table_info(" + table + ")").Rows()
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				return false, err
			}
			if name == "detail_cleared" {
				return true, nil
			}
		}
		return false, nil
	}
}

// checkDetailCleanupIndexHealth 在 MySQL 上验证 detail-cleanup 索引是否就绪。
//
// 三档状态:
//   - v14 (_v2) 索引存在 → fast path (1000/20ms)。生产 EXPLAIN filtered ~99%,
//     是 PR 设计目标。
//   - 仅 v13 旧索引 (_no _v2) → degraded。v13 的 (created_at, id) 列序对清理
//     SELECT filtered 只有 ~10%,在大表上把 1000 batch 跑成全扫等价物。退到保守批次
//     200/50ms,并打印 manual v14 SQL。maintainer awsl233777 在 PR #566 catch 的关键
//     场景:v13 自动建成 + v14 threshold-skip 时,以前 health check 把它当作就绪,
//     会用错配的 fast path。
//   - 都不存在 → 最强警告 + 保守批次。
//
// 检查为 best-effort:DB 字段为 nil(测试场景)或非 MySQL(SQLite 用 partial index)
// 直接跳过。INFORMATION_SCHEMA 查询失败也不阻塞启动——这只是健康提示,不是 gate。
func (d *BackgroundTaskDeps) checkDetailCleanupIndexHealth() {
	if d.DB == nil || d.DB.Dialector() != "mysql" {
		return
	}
	type indexRow struct {
		IndexName string `gorm:"column:index_name"`
	}
	var rows []indexRow
	err := d.DB.GormDB().Raw(`
		SELECT DISTINCT index_name FROM information_schema.STATISTICS
		WHERE table_schema = DATABASE()
		  AND table_name = 'proxy_requests'
		  AND index_name IN ('idx_proxy_requests_detail_cleanup', 'idx_proxy_requests_detail_cleanup_v2')
	`).Scan(&rows).Error
	if err != nil {
		log.Printf("[Task] WARNING: failed to verify detail-cleanup index health: %v", err)
		return
	}
	hasV13, hasV14 := false, false
	for _, r := range rows {
		switch r.IndexName {
		case "idx_proxy_requests_detail_cleanup":
			hasV13 = true
		case "idx_proxy_requests_detail_cleanup_v2":
			hasV14 = true
		}
	}
	switch {
	case hasV14:
		sqlite.SetDetailCleanupIndexMissing(false)
		log.Printf("[Task] MySQL detail-cleanup v14 index present (fast path enabled)")
	case hasV13:
		// v13 索引存在但 v14 缺失。v13 列序只让 ~10% 行有效过滤,大 batch 等同全扫。
		// 退到保守批次,并提示运维补 v14。
		sqlite.SetDetailCleanupIndexMissing(true)
		log.Printf("[Task] WARNING: MySQL proxy_requests has only the v13 detail-cleanup index "+
			"(created_at, id). v14 reorder is the actual perf fix; cleanup batch falls back to "+
			"conservative size (200/50ms) until you apply:\n"+
			"  CREATE INDEX idx_proxy_requests_detail_cleanup_v2 ON proxy_requests(status, dev_mode, created_at, id);\n"+
			"  -- then optionally:\n"+
			"  -- DROP INDEX idx_proxy_requests_detail_cleanup ON proxy_requests;")
	default:
		sqlite.SetDetailCleanupIndexMissing(true)
		log.Printf("[Task] WARNING: MySQL proxy_requests has NO detail-cleanup index. "+
			"Cleanup batch falls back to conservative size (200/50ms) until you apply:\n"+
			"  CREATE INDEX idx_proxy_requests_detail_cleanup_v2 ON proxy_requests(status, dev_mode, created_at, id);")
	}
}

// runCleanupTasks 清理任务：清理过期数据
//
// 多实例:仅在 leader 实例上跑。usage_stats / proxy_requests / sessions 是共享存储,
// 6 副本同时跑只会把 IOPS 放大 6 倍且互相 race。leader gate 失败时跳过这一轮。
func (d *BackgroundTaskDeps) runCleanupTasks() {
	if !d.isCleanupLeader() {
		return
	}
	// 1. 清理过期的分钟数据（保留 1 天）
	before := time.Now().UTC().AddDate(0, 0, -1)
	_, _ = d.UsageStats.DeleteOlderThan(domain.GranularityMinute, before)

	// 2. 清理过期的小时数据（保留 1 个月）
	before = time.Now().UTC().AddDate(0, -1, 0)
	_, _ = d.UsageStats.DeleteOlderThan(domain.GranularityHour, before)

	// 3. 清理过期请求记录
	d.cleanupOldRequests()

	// 4. 清理过期会话
	d.cleanupOldSessions()

	// 注：请求详情清理由独立的 runRequestDetailCleanup 任务处理（动态间隔）
}

// cleanupOldRequests 清理过期的请求记录
func (d *BackgroundTaskDeps) cleanupOldRequests() {
	retentionHours := defaultRequestRetentionHours

	if val, err := d.Settings.Get(domain.SettingKeyRequestRetentionHours); err == nil && val != "" {
		if hours, err := strconv.Atoi(val); err == nil {
			retentionHours = hours
		}
	}

	if retentionHours <= 0 {
		return // 0 表示不清理
	}

	before := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
	deletedRequests, err := d.ProxyRequest.DeleteOlderThan(before)
	if err != nil {
		log.Printf("[Task] Failed to delete old requests: %v", err)
		return
	}
	if deletedRequests > 0 {
		log.Printf("[Task] Deleted %d requests older than %d hours", deletedRequests, retentionHours)
	}

	// Best-effort: SQLite needs VACUUM (and WAL checkpoint) to reclaim disk space after deletes.
	// Only attempt when using sqlite dialector and when throttling allows.
	// Failure (e.g. database is locked) must not affect the cleanup flow.
	d.maybeSQLiteCheckpointAndVacuum(deletedRequests)
}

// cleanupOldSessions 清理过期的请求会话
func (d *BackgroundTaskDeps) cleanupOldSessions() {
	if d.SessionRepo == nil || d.Settings == nil {
		return
	}

	retentionHours := defaultSessionRetentionHours
	if val, err := d.Settings.Get(domain.SettingKeySessionRetentionHours); err == nil && val != "" {
		if hours, err := strconv.Atoi(val); err == nil {
			retentionHours = hours
		}
	}

	if retentionHours <= 0 {
		return
	}

	before := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
	deletedSessions, err := d.SessionRepo.DeleteOlderThan(before)
	if err != nil {
		log.Printf("[Task] Failed to delete old sessions: %v", err)
		return
	}
	if deletedSessions > 0 {
		log.Printf("[Task] Deleted %d sessions older than %d hours", deletedSessions, retentionHours)
	}

	d.maybeSQLiteCheckpointAndVacuum(deletedSessions)
}

func (d *BackgroundTaskDeps) maybeSQLiteCheckpointAndVacuum(deletedRequests int64) {
	if d.DB == nil || d.DB.Dialector() != "sqlite" {
		return
	}
	if deletedRequests <= 0 {
		return
	}

	// Combine throttle check (due) + in-progress mark under one lock to avoid race window.
	sqliteMaintenanceMu.Lock()
	if sqliteMaintenanceInProgress {
		sqliteMaintenanceMu.Unlock()
		return
	}
	if !sqliteMaintenanceLastCompleted.IsZero() && time.Since(sqliteMaintenanceLastCompleted) < sqliteVacuumMinInterval {
		sqliteMaintenanceMu.Unlock()
		return
	}
	sqliteMaintenanceInProgress = true
	sqliteMaintenanceMu.Unlock()
	defer func() {
		sqliteMaintenanceMu.Lock()
		sqliteMaintenanceInProgress = false
		sqliteMaintenanceMu.Unlock()
	}()

	// Best-effort WAL checkpoint to also shrink - ignore errors.
	if err := d.DB.GormDB().Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		log.Printf("[Task] SQLite wal_checkpoint(TRUNCATE) failed (ignored): %v", err)
	}

	// VACUUM cannot run within a transaction.
	if err := d.DB.GormDB().Exec("VACUUM").Error; err != nil {
		log.Printf("[Task] SQLite VACUUM failed (best-effort): %v", err)
		return
	}

	sqliteMaintenanceMu.Lock()
	sqliteMaintenanceLastCompleted = time.Now()
	sqliteMaintenanceMu.Unlock()
	log.Printf("[Task] SQLite maintenance completed (best-effort)")
}

// 成功 / 失败 状态分组（用于 split 模式下的按状态清理）
//
// 故意不把 PENDING / IN_PROGRESS 纳入任一桶——长流式请求可能在飞超过
// failed 保留时间，若按 created_at < cutoff 误判会把仍在写入的 body 清空。
// 卡死的孤儿 PENDING/IN_PROGRESS 行会在下次启动时被 MarkStaleAsFailed
// 转成 FAILED，从而被失败桶覆盖；接受这点权衡换在飞行的安全。
var (
	successRequestStatuses = []string{"COMPLETED"}
	failedRequestStatuses  = []string{"FAILED", "CANCELLED", "REJECTED"}
)

// requestDetailRetentionConfig 解析当前生效的请求详情保留配置
// 返回三元组 (successSeconds, failedSeconds, split)
//   - split=false 时 success 与 failed 同值（统一键），按全表清理（statuses=nil）
//   - split=true 时分别取 success/failed 键，未设置回退到统一键
//   - 任一字段为 -1 表示永久保存（不清理），0 表示由 executor 即时清理
func (d *BackgroundTaskDeps) requestDetailRetentionConfig() (successSec, failedSec int, split bool) {
	parse := func(key string, fallback int) int {
		v, err := d.Settings.Get(key)
		if err != nil || v == "" {
			return fallback
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return fallback
		}
		return n
	}

	unified := parse(domain.SettingKeyRequestDetailRetentionSeconds, -1)

	splitVal, _ := d.Settings.Get(domain.SettingKeyRequestDetailRetentionSplitEnabled)
	if splitVal != "true" {
		return unified, unified, false
	}
	return parse(domain.SettingKeyRequestDetailRetentionSecondsSuccess, unified),
		parse(domain.SettingKeyRequestDetailRetentionSecondsFailed, unified),
		true
}

// cleanupOldRequestDetails 清理过期的请求详情（request_info 和 response_info）
//
// 保留秒数语义：
//   - < 0：永久保存（跳过）
//   - = 0：不保存（cutoff = now，eligible 行立即清理）—— 这是 split 模式下
//     side=0 与另一 side != 0 共存时唯一不"漏存"的处理：ingress 不能按状态判断，
//     所以先写入 DB，由本任务在下一次 tick 时清掉。
//   - > 0：cutoff = now - seconds
//
// split=false 时按统一保留时间清理全部状态（statuses=nil）
func (d *BackgroundTaskDeps) cleanupOldRequestDetails() {
	successSec, failedSec, split := d.requestDetailRetentionConfig()
	now := time.Now() // 两个桶共享同一时刻，cutoff 计算一致

	clear := func(seconds int, statuses []string, label string) {
		if seconds < 0 {
			return // 永久保存
		}
		before := now
		if seconds > 0 {
			before = now.Add(-time.Duration(seconds) * time.Second)
		}
		if deleted, err := d.ProxyRequest.ClearDetailOlderThan(before, statuses); err != nil {
			log.Printf("[Task] Failed to clear %s request details: %v", label, err)
		} else if deleted > 0 {
			log.Printf("[Task] Cleared details for %d %s requests (retention=%ds)", deleted, label, seconds)
		}
		if d.AttemptRepo != nil {
			if deleted, err := d.AttemptRepo.ClearDetailOlderThan(before, statuses); err != nil {
				log.Printf("[Task] Failed to clear %s attempt details: %v", label, err)
			} else if deleted > 0 {
				log.Printf("[Task] Cleared details for %d %s attempts (retention=%ds)", deleted, label, seconds)
			}
		}
	}

	if !split {
		clear(successSec, nil, "all")
		return
	}
	clear(successSec, successRequestStatuses, "success")
	clear(failedSec, failedRequestStatuses, "failed")
}

// runRequestDetailCleanup 动态间隔清理请求详情
//
// 间隔决策：
//   - 任一 side >= 0（含 0）→ 该 side 需要清理，纳入间隔计算（0 取最小间隔 10s 以快速回收）
//   - 两 side 都 < 0（都永久）→ 每分钟轮询一次配置
//
// 实际间隔取 max(min(>0 sides), 10s)；存在 0 时直接锁到 10s 以贴近"不保存"语义
func (d *BackgroundTaskDeps) runRequestDetailCleanup() {
	time.Sleep(10 * time.Second) // 初始延迟

	for {
		successSec, failedSec, _ := d.requestDetailRetentionConfig()

		needsRun := successSec >= 0 || failedSec >= 0
		if !needsRun {
			time.Sleep(1 * time.Minute)
			continue
		}

		// 多实例:仅在 leader 上跑 detail cleanup。详见 isCleanupLeader 注释。
		// 注意 sleep 调度仍然继续——非 leader 也要按相同 cadence 重新评估,
		// 一旦 leader 切换不会延迟到下一个长间隔。
		if d.isCleanupLeader() {
			d.cleanupOldRequestDetails()
		}

		// 取所有 >0 side 的最小值；任一 side == 0 则锁到 10s
		var seconds int
		hasZero := successSec == 0 || failedSec == 0
		if !hasZero {
			for _, s := range []int{successSec, failedSec} {
				if s > 0 && (seconds == 0 || s < seconds) {
					seconds = s
				}
			}
		}
		if seconds <= 0 {
			seconds = 10
		}
		interval := time.Duration(seconds) * time.Second
		if interval < 10*time.Second {
			interval = 10 * time.Second
		}
		time.Sleep(interval)
	}
}

// runAntigravityQuotaRefresh 定期刷新 Antigravity 配额
func (d *BackgroundTaskDeps) runAntigravityQuotaRefresh() {
	time.Sleep(30 * time.Second) // 初始延迟

	for {
		interval := d.AntigravityTaskSvc.GetRefreshInterval()
		if interval <= 0 {
			// 禁用状态，每分钟检查一次配置
			time.Sleep(1 * time.Minute)
			continue
		}

		// 执行刷新
		ctx := context.Background()
		d.AntigravityTaskSvc.RefreshQuotas(ctx)

		// 等待下一次刷新
		time.Sleep(time.Duration(interval) * time.Minute)
	}
}

// runCodexQuotaRefresh 定期刷新 Codex 配额
func (d *BackgroundTaskDeps) runCodexQuotaRefresh() {
	time.Sleep(30 * time.Second) // 初始延迟

	for {
		interval := d.CodexTaskSvc.GetRefreshInterval()
		if interval <= 0 {
			// 禁用状态，每分钟检查一次配置
			time.Sleep(1 * time.Minute)
			continue
		}

		// 执行刷新
		ctx := context.Background()
		d.CodexTaskSvc.RefreshQuotas(ctx)

		// 等待下一次刷新
		time.Sleep(time.Duration(interval) * time.Minute)
	}
}
