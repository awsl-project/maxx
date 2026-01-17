package core

import (
	"log"
	"strconv"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

const (
	defaultRequestRetentionDays = 7
	defaultStatsRetentionDays   = 30
)

// BackgroundTaskDeps 后台任务依赖
type BackgroundTaskDeps struct {
	UsageStats   repository.UsageStatsRepository
	ProxyRequest repository.ProxyRequestRepository
	Settings     repository.SystemSettingRepository
}

// StartBackgroundTasks 启动所有后台任务
func StartBackgroundTasks(deps BackgroundTaskDeps) {
	go func() {
		time.Sleep(2 * time.Second)
		deps.runTasks()

		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			deps.runTasks()
		}
	}()

	log.Println("[Task] Background tasks started")
}

// runTasks 执行所有后台任务
func (d *BackgroundTaskDeps) runTasks() {
	log.Println("[Task] Starting background tasks")

	// 聚合统计数据
	count, err := d.UsageStats.Aggregate()
	if err != nil {
		log.Printf("[Task] Failed to aggregate usage stats: %v", err)
	} else {
		log.Printf("[Task] Aggregated %d usage stats records", count)
	}

	// 清理过期请求
	d.cleanupOldRequests()

	// 清理过期统计
	d.cleanupOldStats()

	log.Println("[Task] All background tasks completed")
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

// cleanupOldStats 清理过期的统计数据
func (d *BackgroundTaskDeps) cleanupOldStats() {
	retentionDays := defaultStatsRetentionDays

	if val, err := d.Settings.Get(domain.SettingKeyStatsRetentionDays); err == nil && val != "" {
		if days, err := strconv.Atoi(val); err == nil {
			retentionDays = days
		}
	}

	if retentionDays <= 0 {
		return // 0 表示不清理
	}

	before := time.Now().AddDate(0, 0, -retentionDays)
	if deleted, err := d.UsageStats.DeleteOlderThan(before); err != nil {
		log.Printf("[Task] Failed to delete old stats: %v", err)
	} else if deleted > 0 {
		log.Printf("[Task] Deleted %d stats older than %d days", deleted, retentionDays)
	}
}
