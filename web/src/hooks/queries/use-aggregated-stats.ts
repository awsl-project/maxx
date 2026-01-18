/**
 * Aggregated Stats Hooks
 * 基于 useUsageStats 的聚合统计 hooks，避免全量聚合
 */

import { useMemo } from 'react';
import { useUsageStats, getTimeRange, type TimeRangePreset } from './use-usage-stats';
import type {
  UsageStatsFilter,
  ProviderStats,
  UsageStats,
  StatsGranularity,
} from '@/lib/transport';

// Route 统计数据类型
export interface RouteStats {
  routeID: number;
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  successRate: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCacheRead: number;
  totalCacheWrite: number;
  totalCost: number;
}

// 将 UsageStats 数组按 provider 聚合
function aggregateByProvider(stats: UsageStats[] | undefined): Record<number, ProviderStats> {
  if (!stats || stats.length === 0) return {};

  const result: Record<number, ProviderStats> = {};

  for (const s of stats) {
    if (s.providerID === 0) continue;

    if (!result[s.providerID]) {
      result[s.providerID] = {
        providerID: s.providerID,
        totalRequests: 0,
        successfulRequests: 0,
        failedRequests: 0,
        successRate: 0,
        activeRequests: 0,
        totalInputTokens: 0,
        totalOutputTokens: 0,
        totalCacheRead: 0,
        totalCacheWrite: 0,
        totalCost: 0,
      };
    }

    const ps = result[s.providerID];
    ps.totalRequests += s.totalRequests;
    ps.successfulRequests += s.successfulRequests;
    ps.failedRequests += s.failedRequests;
    ps.totalInputTokens += s.inputTokens;
    ps.totalOutputTokens += s.outputTokens;
    ps.totalCacheRead += s.cacheRead;
    ps.totalCacheWrite += s.cacheWrite;
    ps.totalCost += s.cost;
  }

  // 计算成功率
  for (const ps of Object.values(result)) {
    if (ps.totalRequests > 0) {
      ps.successRate = (ps.successfulRequests / ps.totalRequests) * 100;
    }
  }

  return result;
}

// 将 UsageStats 数组按 route 聚合
function aggregateByRoute(stats: UsageStats[] | undefined): Record<number, RouteStats> {
  if (!stats || stats.length === 0) return {};

  const result: Record<number, RouteStats> = {};

  for (const s of stats) {
    if (s.routeID === 0) continue;

    if (!result[s.routeID]) {
      result[s.routeID] = {
        routeID: s.routeID,
        totalRequests: 0,
        successfulRequests: 0,
        failedRequests: 0,
        successRate: 0,
        totalInputTokens: 0,
        totalOutputTokens: 0,
        totalCacheRead: 0,
        totalCacheWrite: 0,
        totalCost: 0,
      };
    }

    const rs = result[s.routeID];
    rs.totalRequests += s.totalRequests;
    rs.successfulRequests += s.successfulRequests;
    rs.failedRequests += s.failedRequests;
    rs.totalInputTokens += s.inputTokens;
    rs.totalOutputTokens += s.outputTokens;
    rs.totalCacheRead += s.cacheRead;
    rs.totalCacheWrite += s.cacheWrite;
    rs.totalCost += s.cost;
  }

  // 计算成功率
  for (const rs of Object.values(result)) {
    if (rs.totalRequests > 0) {
      rs.successRate = (rs.successfulRequests / rs.totalRequests) * 100;
    }
  }

  return result;
}

/**
 * 获取 Provider 统计数据（基于 usage_stats 预聚合数据）
 * 默认使用 day 粒度聚合全部历史数据
 */
export function useProviderStatsFromUsageStats(options?: {
  clientType?: string;
  projectId?: number;
  granularity?: StatsGranularity;
  preset?: TimeRangePreset;
}) {
  const clientType = options?.clientType;
  const projectId = options?.projectId;
  const granularity = options?.granularity;
  const preset = options?.preset;

  const filter = useMemo<UsageStatsFilter>(() => {
    // 如果指定了预设，使用预设的时间范围和粒度
    if (preset) {
      const timeRange = getTimeRange(preset);
      return {
        granularity: timeRange.granularity,
        start: timeRange.start.toISOString(),
        end: timeRange.end.toISOString(),
        clientType,
        projectId,
      };
    }

    // 否则使用指定的粒度（默认 day）查询全部
    return {
      granularity: granularity ?? 'day',
      clientType,
      projectId,
    };
  }, [clientType, projectId, granularity, preset]);

  const { data: stats, isLoading, error } = useUsageStats(filter);

  const providerStats = useMemo(() => aggregateByProvider(stats), [stats]);

  return {
    data: providerStats,
    isLoading,
    error,
  };
}

/**
 * 获取全局 Provider 统计数据（不区分 clientType/projectId）
 * 默认使用 day 粒度聚合全部历史数据
 */
export function useAllProviderStatsFromUsageStats(options?: {
  granularity?: StatsGranularity;
  preset?: TimeRangePreset;
}) {
  return useProviderStatsFromUsageStats(options);
}

/**
 * 获取 Route 统计数据（基于 usage_stats 预聚合数据）
 * 默认使用 day 粒度聚合全部历史数据
 */
export function useRouteStatsFromUsageStats(options?: {
  clientType?: string;
  projectId?: number;
  granularity?: StatsGranularity;
  preset?: TimeRangePreset;
}) {
  const clientType = options?.clientType;
  const projectId = options?.projectId;
  const granularity = options?.granularity;
  const preset = options?.preset;

  const filter = useMemo<UsageStatsFilter>(() => {
    // 如果指定了预设，使用预设的时间范围和粒度
    if (preset) {
      const timeRange = getTimeRange(preset);
      return {
        granularity: timeRange.granularity,
        start: timeRange.start.toISOString(),
        end: timeRange.end.toISOString(),
        clientType,
        projectId,
      };
    }

    // 否则使用指定的粒度（默认 day）查询全部
    return {
      granularity: granularity ?? 'day',
      clientType,
      projectId,
    };
  }, [clientType, projectId, granularity, preset]);

  const { data: stats, isLoading, error } = useUsageStats(filter);

  const routeStats = useMemo(() => aggregateByRoute(stats), [stats]);

  return {
    data: routeStats,
    isLoading,
    error,
  };
}
