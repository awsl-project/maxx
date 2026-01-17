package core

import (
	"log"
	"strconv"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

const (
	defaultRequestRetentionDays = 0  // 0 表示默认不清理
	defaultStatsRetentionDays   = 0  // 0 表示默认不清理
)

// BackgroundTaskDeps 后台任务依赖
type BackgroundTaskDeps struct {
	UsageStats   repository.UsageStatsRepository
	ProxyRequest repository.ProxyRequestRepository
	Settings     repository.SystemSettingRepository
}

// StartBackgroundTasks 启动所有后台任务
func StartBackgroundTasks(deps BackgroundTaskDeps) {
	// 分钟级聚合任务（每 30 秒）- 实时聚合原始数据到分钟
	go func() {
		time.Sleep(5 * time.Second) // 初始延迟
		deps.runMinuteAggregation()

		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			deps.runMinuteAggregation()
		}
	}()

	// 小时级 Roll-up（每分钟）- 分钟 → 小时
	go func() {
		time.Sleep(10 * time.Second) // 初始延迟
		deps.runHourlyRollup()

		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			deps.runHourlyRollup()
		}
	}()

	// 天级 Roll-up（每 5 分钟）- 小时 → 天/周/月
	go func() {
		time.Sleep(15 * time.Second) // 初始延迟
		deps.runDailyRollup()

		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			deps.runDailyRollup()
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

	log.Println("[Task] Background tasks started (minute:30s, hour:1m, day:5m, cleanup:1h)")
}

// runMinuteAggregation 分钟级聚合：从原始数据聚合到分钟
func (d *BackgroundTaskDeps) runMinuteAggregation() {
	_, _ = d.UsageStats.AggregateMinute()
}

// runHourlyRollup 小时级 Roll-up：分钟 → 小时
func (d *BackgroundTaskDeps) runHourlyRollup() {
	_, _ = d.UsageStats.RollUp(domain.GranularityMinute, domain.GranularityHour)
}

// runDailyRollup 天级 Roll-up：小时 → 天/周/月
func (d *BackgroundTaskDeps) runDailyRollup() {
	// 小时 → 天
	_, _ = d.UsageStats.RollUp(domain.GranularityHour, domain.GranularityDay)
	// 天 → 周
	_, _ = d.UsageStats.RollUp(domain.GranularityDay, domain.GranularityWeek)
	// 天 → 月
	_, _ = d.UsageStats.RollUp(domain.GranularityDay, domain.GranularityMonth)
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
	retentionDays := defaultRequestRetentionDays

	if val, err := d.Settings.Get(domain.SettingKeyRequestRetentionDays); err == nil && val != "" {
		if days, err := strconv.Atoi(val); err == nil {
			retentionDays = days
		}
	}

	if retentionDays <= 0 {
		return // 0 表示不清理
	}

	before := time.Now().AddDate(0, 0, -retentionDays)
	if deleted, err := d.ProxyRequest.DeleteOlderThan(before); err != nil {
		log.Printf("[Task] Failed to delete old requests: %v", err)
	} else if deleted > 0 {
		log.Printf("[Task] Deleted %d requests older than %d days", deleted, retentionDays)
	}
}
