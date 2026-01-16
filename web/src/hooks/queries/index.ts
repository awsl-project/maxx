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
export { sessionKeys, useSessions, useUpdateSessionProject, useRejectSession } from './use-sessions';

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
  useAntigravityGlobalSettings,
  useUpdateAntigravityGlobalSettings,
  useResetAntigravityGlobalSettings,
} from './use-settings';
