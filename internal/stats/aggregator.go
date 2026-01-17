package stats

import (
	"github.com/awsl-project/maxx/internal/repository"
)

// StatsAggregator 统计数据聚合器
// 仅支持定时同步模式，实时数据由 QueryWithRealtime 直接查询
type StatsAggregator struct {
	usageStatsRepo repository.UsageStatsRepository
}

// NewStatsAggregator 创建统计聚合器
func NewStatsAggregator(usageStatsRepo repository.UsageStatsRepository) *StatsAggregator {
	return &StatsAggregator{
		usageStatsRepo: usageStatsRepo,
	}
}

// RunPeriodicSync 定期同步分钟级数据
func (sa *StatsAggregator) RunPeriodicSync() {
	_, _ = sa.usageStatsRepo.AggregateMinute()
}
