package stats

import (
	"github.com/awsl-project/maxx/internal/repository"
)

// StatsAggregator 统计数据聚合器
// 仅支持定时同步模式，实时数据由 Query 方法直接查询
type StatsAggregator struct {
	usageStatsRepo repository.UsageStatsRepository
}

// NewStatsAggregator 创建统计聚合器
func NewStatsAggregator(usageStatsRepo repository.UsageStatsRepository) *StatsAggregator {
	return &StatsAggregator{
		usageStatsRepo: usageStatsRepo,
	}
}

// RunPeriodicSync 定期同步统计数据（聚合 + rollup）
// 通过 range channel 等待所有阶段完成
func (sa *StatsAggregator) RunPeriodicSync() {
	for range sa.usageStatsRepo.AggregateAndRollUp() {
		// drain the channel to wait for completion
	}
}
