/**
 * 领域模型类型定义
 * 与 Go internal/domain/model.go 保持同步
 */

// ===== 基础类型 =====

export type ClientType = 'claude' | 'codex' | 'gemini' | 'openai';

// ===== Provider 相关 =====

export interface ProviderConfigCustom {
  baseURL: string;
  apiKey: string;
  clientBaseURL?: Partial<Record<ClientType, string>>;
  modelMapping?: Record<string, string>;
}

export interface ProviderConfigAntigravity {
  email: string;
  refreshToken: string;
  projectID: string;
  endpoint: string;
  modelMapping?: Record<string, string>;
}

export interface ProviderConfig {
  custom?: ProviderConfigCustom;
  antigravity?: ProviderConfigAntigravity;
}

export interface Provider {
  id: number;
  createdAt: string;
  updatedAt: string;
  type: string;
  name: string;
  config: ProviderConfig | null;
  supportedClientTypes: ClientType[];
}

// supportedClientTypes 可选，后端会根据 provider type 自动设置
export type CreateProviderData = Omit<Provider, 'id' | 'createdAt' | 'updatedAt' | 'supportedClientTypes'> & {
  supportedClientTypes?: ClientType[];
};

// ===== Project =====

export interface Project {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  slug: string;
  enabledCustomRoutes: ClientType[];
}

export type CreateProjectData = Omit<Project, 'id' | 'createdAt' | 'updatedAt' | 'slug'> & {
  slug?: string;
};

// ===== Session =====

export interface Session {
  id: number;
  createdAt: string;
  updatedAt: string;
  sessionID: string;
  clientType: ClientType;
  projectID: number;
}

// ===== Route =====

export interface Route {
  id: number;
  createdAt: string;
  updatedAt: string;
  isEnabled: boolean;
  isNative: boolean; // 是否为原生支持（自动创建），false 表示转换支持（手动创建）
  projectID: number;
  clientType: ClientType;
  providerID: number;
  position: number;
  retryConfigID: number;
  modelMapping?: Record<string, string>;
}

export type CreateRouteData = Omit<Route, 'id' | 'createdAt' | 'updatedAt'>;

// ===== RetryConfig =====

export interface RetryConfig {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  isDefault: boolean;
  maxRetries: number;
  initialInterval: number; // nanoseconds
  backoffRate: number;
  maxInterval: number; // nanoseconds
}

export type CreateRetryConfigData = Omit<RetryConfig, 'id' | 'createdAt' | 'updatedAt'>;

// ===== RoutingStrategy =====

export type RoutingStrategyType = 'priority' | 'weighted_random';

export interface RoutingStrategyConfig {
  // 扩展字段
}

export interface RoutingStrategy {
  id: number;
  createdAt: string;
  updatedAt: string;
  projectID: number;
  type: RoutingStrategyType;
  config: RoutingStrategyConfig | null;
}

export type CreateRoutingStrategyData = Omit<RoutingStrategy, 'id' | 'createdAt' | 'updatedAt'>;

// ===== ProxyRequest =====

export interface RequestInfo {
  method: string;
  headers: Record<string, string>;
  url: string;
  body: string;
}

export interface ResponseInfo {
  status: number;
  headers: Record<string, string>;
  body: string;
}

export type ProxyRequestStatus = 'PENDING' | 'IN_PROGRESS' | 'COMPLETED' | 'FAILED' | 'CANCELLED';

export interface ProxyRequest {
  id: number;
  createdAt: string;
  updatedAt: string;
  instanceID: string;
  requestID: string;
  sessionID: string;
  clientType: ClientType;
  requestModel: string;
  responseModel: string;
  startTime: string;
  endTime: string;
  duration: number; // nanoseconds
  isStream: boolean; // 是否为 SSE 流式请求
  status: ProxyRequestStatus;
  statusCode: number; // HTTP 状态码（冗余存储，用于列表查询优化）
  requestInfo: RequestInfo | null;
  responseInfo: ResponseInfo | null;
  error: string;
  proxyUpstreamAttemptCount: number;
  finalProxyUpstreamAttemptID: number;
  // 当前使用的 Route 和 Provider (用于实时追踪)
  routeID: number;
  providerID: number;
  projectID: number;
  inputTokenCount: number;
  outputTokenCount: number;
  cacheReadCount: number;
  cacheWriteCount: number;
  cache5mWriteCount: number;
  cache1hWriteCount: number;
  cost: number;
}

// ===== ProxyUpstreamAttempt =====

export type ProxyUpstreamAttemptStatus = 'PENDING' | 'IN_PROGRESS' | 'COMPLETED' | 'FAILED' | 'CANCELLED';

export interface ProxyUpstreamAttempt {
  id: number;
  createdAt: string;
  updatedAt: string;
  startTime: string;
  endTime: string;
  duration: number; // nanoseconds
  status: ProxyUpstreamAttemptStatus;
  proxyRequestID: number;
  isStream: boolean; // 是否为 SSE 流式请求
  requestInfo: RequestInfo | null;
  responseInfo: ResponseInfo | null;
  routeID: number;
  providerID: number;
  inputTokenCount: number;
  outputTokenCount: number;
  cacheReadCount: number;
  cacheWriteCount: number;
  cache5mWriteCount: number;
  cache1hWriteCount: number;
  cost: number;
}

// ===== 分页 =====

export interface PaginationParams {
  limit?: number;
  offset?: number;
}

/** 基于游标的分页参数 (用于大数据量场景) */
export interface CursorPaginationParams {
  limit?: number;
  /** 获取 id 小于此值的记录 (向后翻页) */
  before?: number;
  /** 获取 id 大于此值的记录 (向前翻页/获取新数据) */
  after?: number;
}

/** 游标分页响应 */
export interface CursorPaginationResult<T> {
  items: T[];
  hasMore: boolean;
  /** 当前页第一条记录的 id */
  firstId?: number;
  /** 当前页最后一条记录的 id */
  lastId?: number;
}

// ===== WebSocket 消息 =====

export type WSMessageType =
  | 'proxy_request_update'
  | 'proxy_upstream_attempt_update'
  | 'stats_update'
  | 'log_message'
  | 'antigravity_oauth_result';

export interface WSMessage<T = unknown> {
  type: WSMessageType;
  data: T;
}

// ===== Proxy Status =====

export interface ProxyStatus {
  running: boolean;
  address: string;
  port: number;
}

// ===== Provider Stats =====

export interface ProviderStats {
  providerID: number;
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  successRate: number; // 0-100
  activeRequests: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCacheRead: number;
  totalCacheWrite: number;
  totalCost: number; // 微美元
}

// ===== Antigravity 相关 =====

export interface AntigravityUserInfo {
  email: string;
  name: string;
  picture: string;
}

export interface AntigravityModelQuota {
  name: string;
  percentage: number; // 0-100
  resetTime: string;
}

export interface AntigravityQuotaData {
  models: AntigravityModelQuota[];
  lastUpdated: number;
  isForbidden: boolean;
  subscriptionTier: string; // FREE/PRO/ULTRA
}

export interface AntigravityTokenValidationResult {
  valid: boolean;
  error?: string;
  userInfo?: AntigravityUserInfo;
  projectID?: string;
  quota?: AntigravityQuotaData;
}

export interface AntigravityBatchValidationResult {
  results: AntigravityTokenValidationResult[];
  total: number;
}

export interface AntigravityOAuthResult {
  state: string;        // 用于前端匹配会话
  success: boolean;
  accessToken?: string;
  refreshToken?: string;
  email?: string;
  projectID?: string;
  userInfo?: AntigravityUserInfo;
  quota?: AntigravityQuotaData;
  error?: string;
}

// ===== 回调类型 =====

export type EventCallback<T = unknown> = (data: T) => void;
export type UnsubscribeFn = () => void;

// ===== Import Result =====

export interface ImportResult {
  imported: number;
  skipped: number;
  errors: string[];
}

// ===== Cooldown =====

export type CooldownReason =
  | 'server_error'
  | 'network_error'
  | 'quota_exhausted'
  | 'rate_limit_exceeded'
  | 'concurrent_limit'
  | 'unknown';

export interface Cooldown {
  providerID: number;
  providerName: string;
  clientType: string; // 'all' for global cooldown, or specific client type
  until: string; // ISO 8601 timestamp
  remaining: string; // Human-readable duration like "15m30s"
  reason: CooldownReason; // Cooldown reason
}
