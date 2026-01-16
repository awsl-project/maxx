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

export interface ProviderConfigKiro {
  authMethod: 'social' | 'idc';
  email?: string;
  refreshToken: string;
  region?: string;
  clientID?: string;
  clientSecret?: string;
  modelMapping?: Record<string, string>;
}

export interface ProviderConfig {
  custom?: ProviderConfigCustom;
  antigravity?: ProviderConfigAntigravity;
  kiro?: ProviderConfigKiro;
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

export type ProxyRequestStatus = 'PENDING' | 'IN_PROGRESS' | 'COMPLETED' | 'FAILED' | 'CANCELLED' | 'REJECTED';

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
  // 模型信息
  requestModel: string;  // 客户端请求的原始模型
  mappedModel: string;   // 映射后实际发送的模型
  responseModel: string; // 上游响应中返回的模型名称
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
  | 'antigravity_oauth_result'
  | 'new_session_pending'
  | 'session_pending_cancelled'
  | '_ws_reconnected'; // 内部事件：WebSocket 重连成功

export interface WSMessage<T = unknown> {
  type: WSMessageType;
  data: T;
}

// New session pending event (for force project binding)
export interface NewSessionPendingEvent {
  sessionID: string;
  clientType: ClientType;
  createdAt: string;
}

// Session pending cancelled event (client disconnected)
export interface SessionPendingCancelledEvent {
  sessionID: string;
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
  models: AntigravityModelQuota[] | null;
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

// Antigravity 模型映射规则
export interface ModelMappingRule {
  pattern: string; // 源模式，支持 * 通配符
  target: string;  // 目标模型名
}

// Antigravity 全局设置
export interface AntigravityGlobalSettings {
  modelMappingRules: ModelMappingRule[];
  availableTargetModels?: string[]; // 只在响应中返回，更新时不需要
}

// ===== Kiro 类型 =====

export interface KiroTokenValidationResult {
  valid: boolean;
  error?: string;
  email?: string;
  userId?: string;
  subscriptionType?: string; // FREE, PRO, etc.
  usageLimit?: number;
  currentUsage?: number;
  daysUntilReset?: number;
  isBanned: boolean;
  banReason?: string;
  profileArn?: string;
  accessToken?: string;
  refreshToken?: string;
}

export interface KiroQuotaData {
  total_limit: number;      // 总额度（包括基础+免费试用）
  available: number;        // 可用额度
  used: number;             // 已使用额度
  days_until_reset: number;
  subscription_type: string;
  free_trial_status?: string;
  email?: string;
  is_banned: boolean;
  ban_reason?: string;
  last_updated: number;
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

/**
 * Cooldown 类型 - 与 Go domain.Cooldown 同步
 * 注意：providerName 和 remaining 需要在前端计算
 */
export interface Cooldown {
  id: number;
  createdAt: string;
  updatedAt: string;
  providerID: number;
  clientType: string; // 'all' for global cooldown, or specific client type
  untilTime: string; // ISO 8601 timestamp (Go time.Time)
  reason: CooldownReason;
}
