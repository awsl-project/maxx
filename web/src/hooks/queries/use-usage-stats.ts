/**
 * UsageStats React Query Hooks
 * 支持多层级时间粒度聚合
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport, type UsageStatsFilter, type StatsGranularity } from '@/lib/transport';

// Query Keys
export const usageStatsKeys = {
  all: ['usageStats'] as const,
  list: (filter?: UsageStatsFilter) => [...usageStatsKeys.all, filter] as const,
};

/**
 * 根据时间范围自动选择合适的粒度
 * | 时间范围 | 推荐粒度 |
 * |---------|---------|
 * | ≤ 2 小时 | minute |
 * | 2 小时 - 7 天 | hour |
 * | 7 天 - 90 天 | day |
 * | 90 天 - 1 年 | week |
 * | > 1 年 | month |
 */
export function selectGranularity(start?: Date, end?: Date): StatsGranularity {
  if (!start || !end) {
    return 'hour'; // 默认小时粒度
  }

  const diffMs = end.getTime() - start.getTime();
  const diffHours = diffMs / (1000 * 60 * 60);
  const diffDays = diffHours / 24;

  if (diffHours <= 2) {
    return 'minute';
  } else if (diffDays <= 7) {
    return 'hour';
  } else if (diffDays <= 90) {
    return 'day';
  } else if (diffDays <= 365) {
    return 'week';
  } else {
    return 'month';
  }
}

/**
 * 时间范围预设
 */
export type TimeRangePreset =
  | 'last_hour'
  | 'last_2_hours'
  | 'last_24_hours'
  | 'last_7_days'
  | 'last_30_days'
  | 'last_90_days'
  | 'last_year'
  | 'all_time';

/**
 * 获取预设时间范围的起止时间和推荐粒度
 */
export function getTimeRange(preset: TimeRangePreset): {
  start: Date;
  end: Date;
  granularity: StatsGranularity;
} {
  const now = new Date();
  let start: Date;
  let granularity: StatsGranularity;

  switch (preset) {
    case 'last_hour':
      start = new Date(now.getTime() - 60 * 60 * 1000);
      granularity = 'minute';
      break;
    case 'last_2_hours':
      start = new Date(now.getTime() - 2 * 60 * 60 * 1000);
      granularity = 'minute';
      break;
    case 'last_24_hours':
      start = new Date(now.getTime() - 24 * 60 * 60 * 1000);
      granularity = 'hour';
      break;
    case 'last_7_days':
      start = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
      granularity = 'hour';
      break;
    case 'last_30_days':
      start = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
      granularity = 'day';
      break;
    case 'last_90_days':
      start = new Date(now.getTime() - 90 * 24 * 60 * 60 * 1000);
      granularity = 'day';
      break;
    case 'last_year':
      start = new Date(now.getTime() - 365 * 24 * 60 * 60 * 1000);
      granularity = 'week';
      break;
    case 'all_time':
    default:
      // 从很早开始，使用月粒度
      start = new Date('2020-01-01');
      granularity = 'month';
      break;
  }

  return { start, end: now, granularity };
}

/**
 * 获取统计数据
 * @param filter 过滤条件，granularity 为必填字段
 */
export function useUsageStats(filter?: UsageStatsFilter) {
  return useQuery({
    queryKey: usageStatsKeys.list(filter),
    queryFn: () => getTransport().getUsageStats(filter),
  });
}

/**
 * 使用预设时间范围获取统计数据
 */
export function useUsageStatsWithPreset(
  preset: TimeRangePreset,
  additionalFilter?: Omit<UsageStatsFilter, 'granularity' | 'start' | 'end'>,
) {
  const { start, end, granularity } = getTimeRange(preset);

  const filter: UsageStatsFilter = {
    ...additionalFilter,
    granularity,
    start: start.toISOString(),
    end: end.toISOString(),
  };

  return useUsageStats(filter);
}

/**
 * 清空并重新聚合统计数据（不重算成本）
 */
export function useRecalculateUsageStats() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().recalculateUsageStats(),
    onSuccess: () => {
      // 使所有 usageStats 查询失效，触发重新获取
      queryClient.invalidateQueries({ queryKey: usageStatsKeys.all });
    },
  });
}

/**
 * 重新计算所有请求的成本
 */
export function useRecalculateCosts() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().recalculateCosts(),
    onSuccess: () => {
      // 使所有 usageStats 查询失效，触发重新获取
      queryClient.invalidateQueries({ queryKey: usageStatsKeys.all });
    },
  });
}
