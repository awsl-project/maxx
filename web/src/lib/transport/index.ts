/**
 * Transport 模块导出入口
 */

// 类型导出
export type {
  // 领域模型
  ClientType,
  Provider,
  ProviderConfig,
  ProviderConfigCustom,
  ProviderConfigAntigravity,
  CreateProviderData,
  Project,
  CreateProjectData,
  Session,
  Route,
  CreateRouteData,
  RetryConfig,
  CreateRetryConfigData,
  RoutingStrategy,
  RoutingStrategyType,
  RoutingStrategyConfig,
  CreateRoutingStrategyData,
  ProxyRequest,
  ProxyRequestStatus,
  ProxyUpstreamAttempt,
  ProxyUpstreamAttemptStatus,
  RequestInfo,
  ResponseInfo,
  ProviderStats,
  // 分页
  PaginationParams,
  // WebSocket
  WSMessageType,
  WSMessage,
  // 回调
  EventCallback,
  UnsubscribeFn,
} from './types';

export type {
  Transport,
  TransportType,
  TransportConfig,
} from './interface';

// 实现导出 - 只导出 HttpTransport
// WailsTransport 通过动态导入加载，避免在 web 模式下导入 @wailsio/runtime
export { HttpTransport } from './http-transport';

// 工厂函数导出
export {
  detectTransportType,
  isWailsEnvironment,
  createTransport,
  createTransportAsync,
  initializeTransport,
  getTransport,
  resetTransport,
} from './factory';
