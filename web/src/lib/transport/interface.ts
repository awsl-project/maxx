/**
 * Transport 抽象接口
 * 统一 HTTP/WebSocket 和 Wails 两种通信方式
 */

import type {
  Provider,
  CreateProviderData,
  Project,
  CreateProjectData,
  Session,
  Route,
  CreateRouteData,
  RetryConfig,
  CreateRetryConfigData,
  RoutingStrategy,
  CreateRoutingStrategyData,
  ProxyRequest,
  ProxyUpstreamAttempt,
  CursorPaginationParams,
  CursorPaginationResult,
  ProxyStatus,
  ProviderStats,
  WSMessageType,
  EventCallback,
  UnsubscribeFn,
  AntigravityTokenValidationResult,
  AntigravityBatchValidationResult,
  AntigravityQuotaData,
  ModelMapping,
  ModelMappingInput,
  ImportResult,
  Cooldown,
  KiroTokenValidationResult,
  KiroQuotaData,
  AuthStatus,
  AuthVerifyResult,
  APIToken,
  APITokenCreateResult,
  CreateAPITokenData,
  RoutePositionUpdate,
  UsageStats,
  UsageStatsFilter,
  DashboardData,
  BackupFile,
  BackupImportOptions,
  BackupImportResult,
} from './types';

/**
 * Transport 抽象接口
 */
export interface Transport {
  // ===== Provider API =====
  getProviders(): Promise<Provider[]>;
  getProvider(id: number): Promise<Provider>;
  createProvider(data: CreateProviderData): Promise<Provider>;
  updateProvider(id: number, data: Partial<Provider>): Promise<Provider>;
  deleteProvider(id: number): Promise<void>;
  exportProviders(): Promise<Provider[]>;
  importProviders(providers: Provider[]): Promise<ImportResult>;

  // ===== Project API =====
  getProjects(): Promise<Project[]>;
  getProject(id: number): Promise<Project>;
  getProjectBySlug(slug: string): Promise<Project>;
  createProject(data: CreateProjectData): Promise<Project>;
  updateProject(id: number, data: Partial<Project>): Promise<Project>;
  deleteProject(id: number): Promise<void>;

  // ===== Route API =====
  getRoutes(): Promise<Route[]>;
  getRoute(id: number): Promise<Route>;
  createRoute(data: CreateRouteData): Promise<Route>;
  updateRoute(id: number, data: Partial<Route>): Promise<Route>;
  deleteRoute(id: number): Promise<void>;
  batchUpdateRoutePositions(updates: RoutePositionUpdate[]): Promise<void>;

  // ===== Session API =====
  getSessions(): Promise<Session[]>;
  updateSessionProject(
    sessionID: string,
    projectID: number,
  ): Promise<{ session: Session; updatedRequests: number }>;
  rejectSession(sessionID: string): Promise<Session>;

  // ===== RetryConfig API =====
  getRetryConfigs(): Promise<RetryConfig[]>;
  getRetryConfig(id: number): Promise<RetryConfig>;
  createRetryConfig(data: CreateRetryConfigData): Promise<RetryConfig>;
  updateRetryConfig(id: number, data: Partial<RetryConfig>): Promise<RetryConfig>;
  deleteRetryConfig(id: number): Promise<void>;

  // ===== RoutingStrategy API =====
  getRoutingStrategies(): Promise<RoutingStrategy[]>;
  getRoutingStrategy(id: number): Promise<RoutingStrategy>;
  createRoutingStrategy(data: CreateRoutingStrategyData): Promise<RoutingStrategy>;
  updateRoutingStrategy(id: number, data: Partial<RoutingStrategy>): Promise<RoutingStrategy>;
  deleteRoutingStrategy(id: number): Promise<void>;

  // ===== ProxyRequest API (只读) =====
  getProxyRequests(params?: CursorPaginationParams): Promise<CursorPaginationResult<ProxyRequest>>;
  getProxyRequestsCount(): Promise<number>;
  getActiveProxyRequests(): Promise<ProxyRequest[]>;
  getProxyRequest(id: number): Promise<ProxyRequest>;
  getProxyUpstreamAttempts(proxyRequestId: number): Promise<ProxyUpstreamAttempt[]>;

  // ===== Proxy Status API =====
  getProxyStatus(): Promise<ProxyStatus>;

  // ===== Provider Stats API =====
  getProviderStats(clientType?: string, projectId?: number): Promise<Record<number, ProviderStats>>;

  // ===== Settings API =====
  getSettings(): Promise<Record<string, string>>;
  getSetting(key: string): Promise<{ key: string; value: string }>;
  updateSetting(key: string, value: string): Promise<{ key: string; value: string }>;
  deleteSetting(key: string): Promise<void>;

  // ===== Logs API =====
  getLogs(limit?: number): Promise<{ lines: string[]; count: number }>;

  // ===== Antigravity API =====
  validateAntigravityToken(refreshToken: string): Promise<AntigravityTokenValidationResult>;
  validateAntigravityTokens(tokens: string[]): Promise<AntigravityBatchValidationResult>;
  validateAntigravityTokenText(tokenText: string): Promise<AntigravityBatchValidationResult>;
  getAntigravityProviderQuota(
    providerId: number,
    forceRefresh?: boolean,
  ): Promise<AntigravityQuotaData>;
  getAntigravityBatchQuotas(): Promise<Record<number, AntigravityQuotaData>>;
  startAntigravityOAuth(): Promise<{ authURL: string; state: string }>;

  // ===== Model Mapping API =====
  getModelMappings(): Promise<ModelMapping[]>;
  createModelMapping(data: ModelMappingInput): Promise<ModelMapping>;
  updateModelMapping(id: number, data: ModelMappingInput): Promise<ModelMapping>;
  deleteModelMapping(id: number): Promise<void>;
  clearAllModelMappings(): Promise<void>;
  resetModelMappingsToDefaults(): Promise<void>;

  // ===== Kiro API =====
  validateKiroSocialToken(refreshToken: string): Promise<KiroTokenValidationResult>;
  getKiroProviderQuota(providerId: number): Promise<KiroQuotaData>;

  // ===== Cooldown API =====
  getCooldowns(): Promise<Cooldown[]>;
  clearCooldown(providerId: number): Promise<void>;

  // ===== Auth API =====
  getAuthStatus(): Promise<AuthStatus>;
  verifyPassword(password: string): Promise<AuthVerifyResult>;
  setAuthToken(token: string): void;
  clearAuthToken(): void;

  // ===== API Token API =====
  getAPITokens(): Promise<APIToken[]>;
  getAPIToken(id: number): Promise<APIToken>;
  createAPIToken(data: CreateAPITokenData): Promise<APITokenCreateResult>;
  updateAPIToken(id: number, data: Partial<APIToken>): Promise<APIToken>;
  deleteAPIToken(id: number): Promise<void>;

  // ===== Usage Stats API =====
  getUsageStats(filter?: UsageStatsFilter): Promise<UsageStats[]>;
  recalculateUsageStats(): Promise<void>;

  // ===== Dashboard API =====
  getDashboardData(): Promise<DashboardData>;

  // ===== Response Model API =====
  getResponseModels(): Promise<string[]>;

  // ===== Backup API =====
  exportBackup(): Promise<BackupFile>;
  importBackup(backup: BackupFile, options?: BackupImportOptions): Promise<BackupImportResult>;

  // ===== 实时订阅 =====
  subscribe<T = unknown>(eventType: WSMessageType, callback: EventCallback<T>): UnsubscribeFn;

  // ===== 生命周期 =====
  connect(): Promise<void>;
  disconnect(): void;
  isConnected(): boolean;
}

/**
 * Transport 运行时类型
 */
export type TransportType = 'http' | 'wails';

/**
 * Transport 配置
 */
export interface TransportConfig {
  /** HTTP 模式的 base URL */
  baseURL?: string;
  /** WebSocket URL (HTTP 模式) */
  wsURL?: string;
  /** 重连间隔 (ms) */
  reconnectInterval?: number;
  /** 最大重连次数 */
  maxReconnectAttempts?: number;
}
