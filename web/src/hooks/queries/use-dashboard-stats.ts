/**
 * Dashboard Stats React Query Hooks
 * 专门为 Dashboard 页面设计的统计 hooks
 * 使用单个 API 请求获取所有数据，减少网络开销
 */

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';
import type { DashboardData, DashboardHeatmapPoint as APIDashboardHeatmapPoint, DashboardModelStats, DashboardTrendPoint, DashboardProviderStats as APIDashboardProviderStats } from '@/lib/transport';

// ===== 类型定义 =====

export interface DashboardSummary {
  // 今日统计
  todayRequests: number;
  todayTokens: number; // input + output + cacheRead + cacheWrite
  todayCost: number; // 微美元
  todaySuccessRate: number; // 0-100
  rpm?: number; // Requests Per Minute (今日平均)
  tpm?: number; // Tokens Per Minute (今日平均)

  // 昨日统计（用于对比）
  yesterdayRequests: number;
  yesterdayTokens: number;
  yesterdayCost: number;

  // 变化率
  requestsChange: number; // 百分比变化
  tokensChange: number;
  costChange: number;
}

export interface HeatmapDataPoint {
  date: string; // YYYY-MM-DD
  count: number; // 请求数
  tokens: number; // Token 总量 (not available from new API, kept for compatibility)
}

export interface ModelRanking {
  model: string;
  requests: number;
  tokens: number;
  cost: number; // not available from new API
}

// ===== 工具函数 =====

function calculateChange(current: number, previous: number): number {
  if (previous === 0) return current > 0 ? 100 : 0;
  return ((current - previous) / previous) * 100;
}

// ===== 核心 Hook =====

/**
 * 获取 Dashboard 所有数据（单个 API 请求）
 * 这是核心 hook，其他 hooks 从这里派生数据
 */
export function useDashboardData() {
  return useQuery<DashboardData>({
    queryKey: ['dashboard'],
    queryFn: () => getTransport().getDashboardData(),
    staleTime: 5 * 1000, // 5 seconds
    refetchInterval: 5 * 1000, // 每 5 秒自动刷新
    refetchOnWindowFocus: false,
  });
}

// ===== 派生 Hooks（保持向后兼容） =====

/**
 * 获取 Dashboard 核心统计数据（今日 vs 昨日）
 */
export function useDashboardSummary() {
  const { data: dashboardData, isLoading } = useDashboardData();

  const summary = useMemo<DashboardSummary>(() => {
    if (!dashboardData) {
      return {
        todayRequests: 0,
        todayTokens: 0,
        todayCost: 0,
        todaySuccessRate: 0,
        rpm: undefined,
        tpm: undefined,
        yesterdayRequests: 0,
        yesterdayTokens: 0,
        yesterdayCost: 0,
        requestsChange: 0,
        tokensChange: 0,
        costChange: 0,
      };
    }

    const { today, yesterday } = dashboardData;

    return {
      todayRequests: today.requests,
      todayTokens: today.tokens,
      todayCost: today.cost,
      todaySuccessRate: today.successRate || 0,
      rpm: today.rpm,
      tpm: today.tpm,

      yesterdayRequests: yesterday.requests,
      yesterdayTokens: yesterday.tokens,
      yesterdayCost: yesterday.cost,

      requestsChange: calculateChange(today.requests, yesterday.requests),
      tokensChange: calculateChange(today.tokens, yesterday.tokens),
      costChange: calculateChange(today.cost, yesterday.cost),
    };
  }, [dashboardData]);

  return {
    data: summary,
    isLoading,
  };
}

/**
 * 获取累计统计数据（全部时间）
 */
export function useAllTimeStats() {
  const { data: dashboardData, isLoading } = useDashboardData();

  const totals = useMemo(() => {
    if (!dashboardData) {
      return {
        totalRequests: 0,
        totalTokens: 0,
        totalCost: 0,
      };
    }

    const { allTime } = dashboardData;
    return {
      totalRequests: allTime.requests,
      totalTokens: allTime.tokens,
      totalCost: allTime.cost,
    };
  }, [dashboardData]);

  return { data: totals, isLoading };
}

/**
 * 获取活动热力图数据（近 30 天）
 * 注意：新 API 固定返回 30 天数据
 */
export function useActivityHeatmap() {
  const { data: dashboardData, isLoading } = useDashboardData();

  const heatmapData = useMemo<HeatmapDataPoint[]>(() => {
    if (!dashboardData?.heatmap) return [];

    return dashboardData.heatmap.map((point: APIDashboardHeatmapPoint) => ({
      date: point.date,
      count: point.count,
      tokens: 0, // Not available from new API
    }));
  }, [dashboardData]);

  const timezone = dashboardData?.timezone || 'Asia/Shanghai';

  return { data: heatmapData, isLoading, timezone };
}

/**
 * 获取 Top 模型排名
 * 注意：新 API 固定返回 top 5
 */
export function useTopModels() {
  const { data: dashboardData, isLoading } = useDashboardData();

  const rankings = useMemo<ModelRanking[]>(() => {
    if (!dashboardData?.topModels) return [];

    return dashboardData.topModels.map((model: DashboardModelStats) => ({
      model: model.model,
      requests: model.requests,
      tokens: model.tokens,
      cost: 0, // Not available from new API
    }));
  }, [dashboardData]);

  return { data: rankings, isLoading };
}

/**
 * 获取 24 小时趋势数据
 */
export function use24HourTrend() {
  const { data: dashboardData, isLoading } = useDashboardData();

  const trendData = useMemo(() => {
    if (!dashboardData?.trend24h) return [];

    return dashboardData.trend24h.map((point: DashboardTrendPoint) => ({
      hour: point.hour,
      requests: point.requests,
      cost: 0, // Not available from new API
    }));
  }, [dashboardData]);

  return { data: trendData, isLoading };
}

/**
 * 获取首次使用日期
 */
export function useFirstUseDate() {
  const { data: dashboardData, isLoading } = useDashboardData();

  const firstUseInfo = useMemo(() => {
    if (!dashboardData?.allTime) {
      return { firstUseDate: null, daysSinceFirstUse: 0 };
    }

    const { firstUseDate, daysSinceFirstUse } = dashboardData.allTime;

    return {
      firstUseDate: firstUseDate ? new Date(firstUseDate) : null,
      daysSinceFirstUse,
    };
  }, [dashboardData]);

  return { data: firstUseInfo, isLoading };
}

/**
 * 获取 Provider 统计数据（从 Dashboard API）
 * 用于替代 useAllProviderStatsFromUsageStats
 */
export function useDashboardProviderStats() {
  const { data: dashboardData, isLoading } = useDashboardData();

  const providerStats = useMemo(() => {
    if (!dashboardData?.providerStats) return {};

    // 转换为与旧 API 兼容的格式
    const result: Record<number, {
      totalRequests: number;
      successRate: number;
      rpm?: number;
      tpm?: number;
    }> = {};

    (Object.entries(dashboardData.providerStats) as [string, APIDashboardProviderStats][]).forEach(([id, stats]) => {
      result[Number(id)] = {
        totalRequests: stats.requests,
        successRate: stats.successRate,
        rpm: stats.rpm,
        tpm: stats.tpm,
      };
    });

    return result;
  }, [dashboardData]);

  return { data: providerStats, isLoading };
}
