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

	// Antigravity 配额刷新任务（动态间隔）
	if deps.AntigravityTaskSvc != nil {
		go deps.runAntigravityQuotaRefresh()
	}

	log.Println("[Task] Background tasks started (aggregation:30s, cleanup:1h)")
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
