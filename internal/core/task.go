package core

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

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
}

// StartBackgroundTasks 启动所有后台任务
func StartBackgroundTasks(deps BackgroundTaskDeps) {
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

// runCleanupTasks 清理任务：清理过期数据
func (d *BackgroundTaskDeps) runCleanupTasks() {
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
// 仅当对应保留秒数 > 0 时执行；split=false 时按统一保留时间清理全部状态
func (d *BackgroundTaskDeps) cleanupOldRequestDetails() {
	successSec, failedSec, split := d.requestDetailRetentionConfig()

	clear := func(seconds int, statuses []string, label string) {
		if seconds <= 0 {
			return
		}
		before := time.Now().Add(-time.Duration(seconds) * time.Second)
		if deleted, err := d.ProxyRequest.ClearDetailOlderThan(before, statuses); err != nil {
			log.Printf("[Task] Failed to clear %s request details: %v", label, err)
		} else if deleted > 0 {
			log.Printf("[Task] Cleared details for %d %s requests older than %d seconds", deleted, label, seconds)
		}
		if d.AttemptRepo != nil {
			if deleted, err := d.AttemptRepo.ClearDetailOlderThan(before, statuses); err != nil {
				log.Printf("[Task] Failed to clear %s attempt details: %v", label, err)
			} else if deleted > 0 {
				log.Printf("[Task] Cleared details for %d %s attempts older than %d seconds", deleted, label, seconds)
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
// 间隔取 success/failed 中较小的正值，未配置时每分钟检查一次
func (d *BackgroundTaskDeps) runRequestDetailCleanup() {
	time.Sleep(10 * time.Second) // 初始延迟

	for {
		successSec, failedSec, _ := d.requestDetailRetentionConfig()

		// 取两者中 > 0 的最小值作为间隔；都不需要清理则每分钟轮询一次配置
		var seconds int
		for _, s := range []int{successSec, failedSec} {
			if s > 0 && (seconds == 0 || s < seconds) {
				seconds = s
			}
		}
		if seconds <= 0 {
			time.Sleep(1 * time.Minute)
			continue
		}

		d.cleanupOldRequestDetails()

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
