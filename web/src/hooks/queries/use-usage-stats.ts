/**
 * UsageStats React Query Hooks
 */

import { useQuery } from '@tanstack/react-query';
import { getTransport, type UsageStatsFilter } from '@/lib/transport';

// Query Keys
export const usageStatsKeys = {
  all: ['usageStats'] as const,
  list: (filter?: UsageStatsFilter) => [...usageStatsKeys.all, filter] as const,
};

// 获取统计数据
export function useUsageStats(filter?: UsageStatsFilter) {
  return useQuery({
    queryKey: usageStatsKeys.list(filter),
    queryFn: () => getTransport().getUsageStats(filter),
  });
}
