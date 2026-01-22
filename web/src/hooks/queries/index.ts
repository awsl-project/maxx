/**
 * React Query Hooks 导出入口
 */

// Provider hooks
export {
  providerKeys,
  useProviders,
  useProvider,
  useCreateProvider,
  useUpdateProvider,
  useDeleteProvider,
  useProviderStats,
  useAllProviderStats,
  useAntigravityQuota,
  useAntigravityBatchQuotas,
  useKiroQuota,
} from './use-providers';

// Project hooks
export {
  projectKeys,
  useProjects,
  useProject,
  useProjectBySlug,
  useCreateProject,
  useUpdateProject,
  useDeleteProject,
} from './use-projects';

// Route hooks
export {
  routeKeys,
  useRoutes,
  useRoute,
  useCreateRoute,
  useUpdateRoute,
  useDeleteRoute,
  useToggleRoute,
  useUpdateRoutePositions,
} from './use-routes';

// Session hooks
export {
  sessionKeys,
  useSessions,
  useUpdateSessionProject,
  useRejectSession,
} from './use-sessions';

// RetryConfig hooks
export {
  retryConfigKeys,
  useRetryConfigs,
  useRetryConfig,
  useCreateRetryConfig,
  useUpdateRetryConfig,
  useDeleteRetryConfig,
} from './use-retry-configs';

// RoutingStrategy hooks
export {
  routingStrategyKeys,
  useRoutingStrategies,
  useRoutingStrategy,
  useCreateRoutingStrategy,
  useUpdateRoutingStrategy,
  useDeleteRoutingStrategy,
} from './use-routing-strategies';

// ProxyRequest hooks
export {
  requestKeys,
  useProxyRequests,
  useProxyRequestsCount,
  useProxyRequest,
  useProxyUpstreamAttempts,
  useProxyRequestUpdates,
} from './use-requests';

// Proxy hooks
export { proxyKeys, useProxyStatus } from './use-proxy';

// Settings hooks
export {
  settingsKeys,
  useSettings,
  useSetting,
  useUpdateSetting,
  useDeleteSetting,
  useModelMappings,
  useCreateModelMapping,
  useUpdateModelMapping,
  useDeleteModelMapping,
  useClearAllModelMappings,
  useResetModelMappingsToDefaults,
} from './use-settings';

// API Token hooks
export {
  apiTokenKeys,
  useAPITokens,
  useAPIToken,
  useCreateAPIToken,
  useUpdateAPIToken,
  useDeleteAPIToken,
} from './use-api-tokens';

// Usage Stats hooks
export {
  usageStatsKeys,
  useUsageStats,
  useUsageStatsWithPreset,
  useRecalculateUsageStats,
  useRecalculateCosts,
  selectGranularity,
  getTimeRange,
  type TimeRangePreset,
} from './use-usage-stats';

// Aggregated Stats hooks (基于 usage_stats 预聚合数据)
export {
  useProviderStatsFromUsageStats,
  useAllProviderStatsFromUsageStats,
  useRouteStatsFromUsageStats,
  type RouteStats,
} from './use-aggregated-stats';

// Response Model hooks
export { responseModelKeys, useResponseModels } from './use-response-models';

// Dashboard Stats hooks
export {
  useDashboardData,
  useDashboardSummary,
  useAllTimeStats,
  useActivityHeatmap,
  useTopModels,
  use24HourTrend,
  useFirstUseDate,
  useDashboardProviderStats,
  type DashboardSummary,
  type HeatmapDataPoint,
  type ModelRanking,
} from './use-dashboard-stats';

// Pricing hooks
export { pricingKeys, usePricing } from './use-pricing';

// Model Price hooks
export {
  modelPriceKeys,
  useModelPrices,
  useModelPrice,
  useCreateModelPrice,
  useUpdateModelPrice,
  useDeleteModelPrice,
  useResetModelPricesToDefaults,
} from './use-model-prices';
