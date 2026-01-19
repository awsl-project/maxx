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
  logo?: string; // Logo URL or data URI
  config: ProviderConfig | null;
  supportedClientTypes: ClientType[];
  supportModels?: string[]; // 支持的模型列表（通配符模式），空数组表示支持所有模型
}

// supportedClientTypes 可选，后端会根据 provider type 自动设置
export type CreateProviderData = Omit<
  Provider,
  'id' | 'createdAt' | 'updatedAt' | 'supportedClientTypes'
> & {
  supportedClientTypes?: ClientType[];
  supportModels?: string[];
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

export interface RoutePositionUpdate {
  id: number;
  position: number;
}

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

export type ProxyRequestStatus =
  | 'PENDING'
  | 'IN_PROGRESS'
  | 'COMPLETED'
  | 'FAILED'
  | 'CANCELLED'
  | 'REJECTED';

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
  // API Token ID
  apiTokenID: number;
}

// ===== ProxyUpstreamAttempt =====

export type ProxyUpstreamAttemptStatus =
  | 'PENDING'
  | 'IN_PROGRESS'
  | 'COMPLETED'
  | 'FAILED'
  | 'CANCELLED';

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
  requestModel: string; // 客户端请求的原始模型
  mappedModel: string; // 映射后实际发送的模型
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
  | 'cooldown_update'
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
  version: string;
  commit: string;
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

// 批量配额查询结果
export interface AntigravityBatchQuotaResult {
  quotas: Record<number, AntigravityQuotaData>; // providerId -> quota
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
  state: string; // 用于前端匹配会话
  success: boolean;
  accessToken?: string;
  refreshToken?: string;
  email?: string;
  projectID?: string;
  userInfo?: AntigravityUserInfo;
  quota?: AntigravityQuotaData;
  error?: string;
}

// ===== 模型映射 =====

// 模型映射作用域
export type ModelMappingScope = 'global' | 'provider' | 'route';

// 模型映射规则
export interface ModelMapping {
  id: number;
  createdAt: string;
  updatedAt: string;
  scope: ModelMappingScope; // 作用域类型
  clientType: string; // 客户端类型，空表示所有
  providerType: string; // 供应商类型（如 antigravity, kiro, custom），空表示所有
  providerID: number; // 供应商 ID，0 表示所有
  projectID: number; // 项目 ID，0 表示所有
  routeID: number; // 路由 ID，0 表示所有
  apiTokenID: number; // Token ID，0 表示所有
  pattern: string; // 源模式，支持 * 通配符
  target: string; // 目标模型名
  priority: number; // 优先级，数字越小优先级越高
  isEnabled: boolean; // 是否启用
  isBuiltin: boolean; // 是否为内置规则
}

// 创建/更新模型映射的请求
export interface ModelMappingInput {
  scope?: ModelMappingScope;
  clientType?: string;
  providerType?: string;
  providerID?: number;
  projectID?: number;
  routeID?: number;
  apiTokenID?: number;
  pattern: string;
  target: string;
  priority?: number;
  isEnabled?: boolean;
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
  total_limit: number; // 总额度（包括基础+免费试用）
  available: number; // 可用额度
  used: number; // 已使用额度
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

// ===== Auth 相关 =====

export interface AuthStatus {
  authEnabled: boolean;
}

export interface AuthVerifyResult {
  success: boolean;
  token?: string;
  error?: string;
}

// ===== API Token =====

export interface APIToken {
  id: number;
  createdAt: string;
  updatedAt: string;
  token: string;
  tokenPrefix: string;
  name: string;
  description: string;
  projectID: number;
  isEnabled: boolean;
  expiresAt?: string;
  lastUsedAt?: string;
  useCount: number;
}

export interface APITokenCreateResult {
  token: string; // 明文 Token（仅创建时返回）
  apiToken: APIToken;
}

export interface CreateAPITokenData {
  name: string;
  description?: string;
  projectID?: number;
  expiresAt?: string;
}

// ===== Usage Stats =====

/** 统计数据时间粒度 */
export type StatsGranularity = 'minute' | 'hour' | 'day' | 'week' | 'month';

export interface UsageStats {
  id: number;
  createdAt: string;
  timeBucket: string; // 时间桶（根据粒度截断）
  granularity: StatsGranularity; // 时间粒度
  routeID: number;
  providerID: number;
  projectID: number;
  apiTokenID: number;
  clientType: string;
  model: string; // 请求的模型名称
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  totalDurationMs: number; // 累计请求耗时（毫秒）
  inputTokens: number;
  outputTokens: number;
  cacheRead: number;
  cacheWrite: number;
  cost: number;
}

/** 统计数据汇总 */
export interface UsageStatsSummary {
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  successRate: number; // 0-100
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCacheRead: number;
  totalCacheWrite: number;
  totalCost: number; // 微美元
}

export interface UsageStatsFilter {
  granularity?: StatsGranularity; // 时间粒度（必填）
  start?: string; // 开始时间 ISO8601
  end?: string; // 结束时间 ISO8601
  routeId?: number;
  providerId?: number;
  projectId?: number;
  apiTokenId?: number;
  clientType?: string;
  model?: string; // 模型名称
}

/** Response Model - 记录所有出现过的 response model */
export interface ResponseModel {
  id: number;
  createdAt: string;
  name: string;
  lastSeenAt: string;
  useCount: number;
}

// ===== Backup API Types =====

/** 备份文件结构 */
export interface BackupFile {
  version: string;
  exportedAt: string;
  appVersion: string;
  data: BackupData;
}

export interface BackupData {
  systemSettings?: BackupSystemSetting[];
  providers?: BackupProvider[];
  projects?: BackupProject[];
  retryConfigs?: BackupRetryConfig[];
  routes?: BackupRoute[];
  routingStrategies?: BackupRoutingStrategy[];
  apiTokens?: BackupAPIToken[];
  modelMappings?: BackupModelMapping[];
}

export interface BackupSystemSetting {
  key: string;
  value: string;
}

export interface BackupProvider {
  name: string;
  type: string;
  config?: ProviderConfig;
  supportedClientTypes?: ClientType[];
  supportModels?: string[];
}

export interface BackupProject {
  name: string;
  slug: string;
  enabledCustomRoutes?: ClientType[];
}

export interface BackupRetryConfig {
  name: string;
  isDefault: boolean;
  maxRetries: number;
  initialIntervalMs: number;
  backoffRate: number;
  maxIntervalMs: number;
}

export interface BackupRoute {
  isEnabled: boolean;
  isNative: boolean;
  projectSlug: string;
  clientType: ClientType;
  providerName: string;
  position: number;
  retryConfigName: string;
}

export interface BackupRoutingStrategy {
  projectSlug: string;
  type: RoutingStrategyType;
  config?: RoutingStrategyConfig;
}

export interface BackupAPIToken {
  name: string;
  description: string;
  projectSlug: string;
  isEnabled: boolean;
  expiresAt?: string;
}

export interface BackupModelMapping {
  scope: ModelMappingScope;
  clientType?: ClientType;
  providerType?: string;
  providerName?: string;
  projectSlug?: string;
  routeName?: string;
  apiTokenName?: string;
  pattern: string;
  target: string;
  priority: number;
}

/** 导入选项 */
export interface BackupImportOptions {
  conflictStrategy?: 'skip' | 'overwrite' | 'error';
  dryRun?: boolean;
}

/** 导入摘要 */
export interface BackupImportSummary {
  imported: number;
  skipped: number;
  updated: number;
}

/** 导入结果 */
export interface BackupImportResult {
  success: boolean;
  summary: Record<string, BackupImportSummary>;
  errors: string[];
  warnings: string[];
}

// ===== Dashboard API Types =====

/** Dashboard 日统计摘要 */
export interface DashboardDaySummary {
  requests: number;
  tokens: number;
  cost: number;
  successRate?: number;
  rpm?: number; // Requests Per Minute (今日平均)
  tpm?: number; // Tokens Per Minute (今日平均)
}

/** Dashboard 全量统计摘要 */
export interface DashboardAllTimeSummary {
  requests: number;
  tokens: number;
  cost: number;
  firstUseDate?: string;
  daysSinceFirstUse: number;
}

/** Dashboard 热力图数据点 */
export interface DashboardHeatmapPoint {
  date: string;
  count: number;
}

/** Dashboard 模型统计 */
export interface DashboardModelStats {
  model: string;
  requests: number;
  tokens: number;
}

/** Dashboard 趋势数据点 */
export interface DashboardTrendPoint {
  hour: string;
  requests: number;
}

/** Dashboard Provider 统计 */
export interface DashboardProviderStats {
  requests: number;
  successRate: number;
  rpm?: number; // Requests Per Minute (今日平均)
  tpm?: number; // Tokens Per Minute (今日平均)
}

/** Dashboard 聚合数据 - 单个 API 返回所有 Dashboard 所需数据 */
export interface DashboardData {
  today: DashboardDaySummary;
  yesterday: DashboardDaySummary;
  allTime: DashboardAllTimeSummary;
  heatmap: DashboardHeatmapPoint[];
  topModels: DashboardModelStats[];
  trend24h: DashboardTrendPoint[];
  providerStats: Record<number, DashboardProviderStats>;
  timezone: string; // 配置的时区，如 "Asia/Shanghai"
}
