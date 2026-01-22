package core

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/service"
)

const (
	defaultRequestRetentionHours = 168 // 默认保留 168 小时（7天）
)

// BackgroundTaskDeps 后台任务依赖
type BackgroundTaskDeps struct {
	UsageStats          repository.UsageStatsRepository
	ProxyRequest        repository.ProxyRequestRepository
	AttemptRepo         repository.ProxyUpstreamAttemptRepository
	Settings            repository.SystemSettingRepository
	AntigravityTaskSvc  *service.AntigravityTaskService
}

// StartBackgroundTasks 启动所有后台任务
func StartBackgroundTasks(deps BackgroundTaskDeps) {
	// 统计聚合任务（每 30 秒）- 聚合原始数据并自动 rollup 到各粒度
	go func() {
		time.Sleep(5 * time.Second) // 初始延迟
		for range deps.UsageStats.AggregateAndRollUp() {
			// drain the channel to wait for completion
		}

		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			for range deps.UsageStats.AggregateAndRollUp() {
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
	if deleted, err := d.ProxyRequest.DeleteOlderThan(before); err != nil {
		log.Printf("[Task] Failed to delete old requests: %v", err)
	} else if deleted > 0 {
		log.Printf("[Task] Deleted %d requests older than %d hours", deleted, retentionHours)
	}
}

// cleanupOldRequestDetails 清理过期的请求详情（request_info 和 response_info）
// 仅当 request_detail_retention_seconds > 0 时执行
func (d *BackgroundTaskDeps) cleanupOldRequestDetails() {
	val, err := d.Settings.Get(domain.SettingKeyRequestDetailRetentionSeconds)
	if err != nil || val == "" {
		return // 未设置或读取失败，不清理（默认 -1 永久保存）
	}

	seconds, err := strconv.Atoi(val)
	if err != nil || seconds <= 0 {
		return // -1 永久保存，0 在 executor 中处理，不需要后台清理
	}

	before := time.Now().Add(-time.Duration(seconds) * time.Second)

	// 清理 ProxyRequest 详情
	if deleted, err := d.ProxyRequest.ClearDetailOlderThan(before); err != nil {
		log.Printf("[Task] Failed to clear request details: %v", err)
	} else if deleted > 0 {
		log.Printf("[Task] Cleared details for %d requests older than %d seconds", deleted, seconds)
	}

	// 清理 ProxyUpstreamAttempt 详情
	if d.AttemptRepo != nil {
		if deleted, err := d.AttemptRepo.ClearDetailOlderThan(before); err != nil {
			log.Printf("[Task] Failed to clear attempt details: %v", err)
		} else if deleted > 0 {
			log.Printf("[Task] Cleared details for %d attempts older than %d seconds", deleted, seconds)
		}
	}
}

// runRequestDetailCleanup 动态间隔清理请求详情
// 根据 request_detail_retention_seconds 配置动态调整清理间隔
func (d *BackgroundTaskDeps) runRequestDetailCleanup() {
	time.Sleep(10 * time.Second) // 初始延迟

	for {
		// 读取配置
		val, err := d.Settings.Get(domain.SettingKeyRequestDetailRetentionSeconds)
		if err != nil || val == "" {
			// 未设置，每分钟检查一次配置
			time.Sleep(1 * time.Minute)
			continue
		}

		seconds, err := strconv.Atoi(val)
		if err != nil || seconds <= 0 {
			// -1 永久保存或 0 在 executor 中处理，每分钟检查一次配置变更
			time.Sleep(1 * time.Minute)
			continue
		}

		// 执行清理
		d.cleanupOldRequestDetails()

		// 按配置的秒数作为间隔等待（最小 10 秒，防止过于频繁）
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
