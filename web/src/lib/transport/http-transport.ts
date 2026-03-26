/**
 * HTTP Transport 实现
 * 使用 Axios 发送 HTTP 请求，WebSocket 接收实时推送
 */

import axios, { type AxiosInstance } from 'axios';
import type { Transport, TransportConfig } from './interface';
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
  ProxyStatus,
  ProviderStats,
  CursorPaginationParams,
  CursorPaginationResult,
  WSMessageType,
  WSMessage,
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
  CodexTokenValidationResult,
  CodexUsageResponse,
  CodexQuotaData,
  ClaudeTokenValidationResult,
  AuthStatus,
  AuthLoginResult,
  PasskeyRegistrationOptionsResult,
  PasskeyLoginOptionsResult,
  PasskeyRegisterResult,
  PasskeyCredential,
  RegistrationResponseJSON,
  AuthenticationResponseJSON,
  AuthRegisterResult,
  ApplyResult,
  ChangePasswordResult,
  User,
  CreateUserData,
  UpdateUserData,
  InviteCode,
  InviteCodeUsage,
  CreateInviteCodeData,
  UpdateInviteCodeData,
  InviteCodeCreateResult,
  APIToken,
  APITokenCreateResult,
  CreateAPITokenData,
  RoutePositionUpdate,
  UsageStats,
  UsageStatsFilter,
  RecalculateCostsResult,
  RecalculateRequestCostResult,
  DashboardData,
  BackupFile,
  BackupImportOptions,
  BackupImportResult,
  PriceTable,
  ModelPrice,
  ModelPriceInput,
} from './types';

type TransportRuntimeConfig = Required<Omit<TransportConfig, 'adminBaseURL'>> & {
  adminBaseURL: string;
};

export class HttpTransport implements Transport {
  private client: AxiosInstance;
  private adminClient: AxiosInstance;
  private ws: WebSocket | null = null;
  private config: TransportRuntimeConfig;
  private eventListeners: Map<WSMessageType, Set<EventCallback>> = new Map();
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private connectPromise: Promise<void> | null = null;
  private authToken: string | null = null;
  private manualDisconnect = false;
  private connectTimeoutMs = 5000;

  constructor(config: TransportConfig = {}) {
    const requestedBaseURL = (config.baseURL ?? '/api').replace(/\/+$/, '') || '/api';
    const adminBaseURL =
      (config.adminBaseURL ?? `${requestedBaseURL}/admin`).replace(/\/+$/, '') ||
      `${requestedBaseURL}/admin`;

    this.config = {
      baseURL: requestedBaseURL,
      adminBaseURL,
      wsURL:
        config.wsURL ?? `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`,
      reconnectInterval: config.reconnectInterval ?? 3000,
      maxReconnectAttempts: config.maxReconnectAttempts ?? 10,
    };

    this.client = this.createClient(this.config.baseURL);
    this.adminClient = this.createClient(this.config.adminBaseURL);
  }

  private createClient(baseURL: string): AxiosInstance {
    const client = axios.create({
      baseURL,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    client.interceptors.request.use((config) => {
      if (this.authToken) {
        config.headers['Authorization'] = `Bearer ${this.authToken}`;
      }
      return config;
    });

    return client;
  }

  private formatUnexpectedResponseData(data: unknown): string {
    if (typeof data === 'string') {
      return data;
    }
    try {
      return JSON.stringify(data);
    } catch {
      return String(data);
    }
  }

  private isPlainObject(data: unknown): data is Record<string, unknown> {
    if (!data || typeof data !== 'object' || Array.isArray(data)) {
      return false;
    }

    const prototype = Object.getPrototypeOf(data);
    return prototype === Object.prototype || prototype === null;
  }

  private describeUnexpectedResponseType(data: unknown): string {
    if (Array.isArray(data)) {
      return `array(length=${data.length})`;
    }
    if (data === null) {
      return 'null';
    }
    if (typeof data === 'string') {
      return `string(length=${data.length})`;
    }
    if (typeof data === 'object') {
      return 'object';
    }
    return typeof data;
  }

  private shouldLogUnexpectedResponseDetails(): boolean {
    return import.meta.env.DEV || import.meta.env.MODE === 'development';
  }

  private debugUnexpectedResponse(resource: string, expectedType: string, data: unknown): void {
    if (!this.shouldLogUnexpectedResponseDetails()) {
      return;
    }

    console.debug('[HttpTransport] Unexpected response payload', {
      resource,
      expectedType,
      receivedType: this.describeUnexpectedResponseType(data),
      data: this.formatUnexpectedResponseData(data),
    });
  }

  private expectArray<T>(data: unknown, resource: string): T[] {
    if (Array.isArray(data)) {
      return data as T[];
    }
    this.debugUnexpectedResponse(resource, 'array', data);
    throw new Error(
      `[HttpTransport] Expected array response for ${resource}, received: ${this.describeUnexpectedResponseType(data)}`,
    );
  }

  private expectObject<T>(data: unknown, resource: string): T {
    if (this.isPlainObject(data)) {
      return data as T;
    }
    this.debugUnexpectedResponse(resource, 'object', data);
    throw new Error(
      `[HttpTransport] Expected object response for ${resource}, received: ${this.describeUnexpectedResponseType(data)}`,
    );
  }

  private expectStringRecord(data: unknown, resource: string): Record<string, string> {
    const record = this.expectObject<Record<string, unknown>>(data, resource);
    const validatedEntries = Object.entries(record).map(([key, value]) => {
      if (typeof value !== 'string') {
        this.debugUnexpectedResponse(`${resource}.${key}`, 'string', value);
        throw new Error(
          `[HttpTransport] Expected string value for ${resource}.${key}, received: ${this.describeUnexpectedResponseType(value)}`,
        );
      }

      return [key, value] as const;
    });

    return Object.fromEntries(validatedEntries);
  }

  // ===== Provider API =====

  async getProviders(): Promise<Provider[]> {
    const { data } = await this.client.get<Provider[]>('/providers');
    return this.expectArray<Provider>(data, '/providers');
  }

  async getProvider(id: number): Promise<Provider> {
    const { data } = await this.client.get<Provider>(`/providers/${id}`);
    return this.expectObject<Provider>(data, `/providers/${id}`);
  }

  async createProvider(payload: CreateProviderData): Promise<Provider> {
    const { data } = await this.client.post<Provider>('/providers', payload);
    return data;
  }

  async updateProvider(id: number, payload: Partial<Provider>): Promise<Provider> {
    const { data } = await this.client.put<Provider>(`/providers/${id}`, payload);
    return data;
  }

  async deleteProvider(id: number): Promise<void> {
    await this.client.delete(`/providers/${id}`);
  }

  async exportProviders(): Promise<Provider[]> {
    const { data } = await this.client.get<Provider[]>('/providers/export');
    return this.expectArray<Provider>(data, '/providers/export');
  }

  async importProviders(providers: Provider[]): Promise<ImportResult> {
    const { data } = await this.client.post<ImportResult>('/providers/import', providers);
    return data;
  }

  // ===== Project API =====

  async getProjects(): Promise<Project[]> {
    const { data } = await this.client.get<Project[]>('/projects');
    return this.expectArray<Project>(data, '/projects');
  }

  async getProject(id: number): Promise<Project> {
    const { data } = await this.client.get<Project>(`/projects/${id}`);
    return this.expectObject<Project>(data, `/projects/${id}`);
  }

  async getProjectBySlug(slug: string): Promise<Project> {
    const { data } = await this.client.get<Project>(`/projects/by-slug/${slug}`);
    return this.expectObject<Project>(data, `/projects/by-slug/${slug}`);
  }

  async createProject(payload: CreateProjectData): Promise<Project> {
    const { data } = await this.client.post<Project>('/projects', payload);
    return data;
  }

  async updateProject(id: number, payload: Partial<Project>): Promise<Project> {
    const { data } = await this.client.put<Project>(`/projects/${id}`, payload);
    return data;
  }

  async deleteProject(id: number): Promise<void> {
    await this.client.delete(`/projects/${id}`);
  }

  // ===== Route API =====

  async getRoutes(): Promise<Route[]> {
    const { data } = await this.client.get<Route[]>('/routes');
    return this.expectArray<Route>(data, '/routes');
  }

  async getRoute(id: number): Promise<Route> {
    const { data } = await this.client.get<Route>(`/routes/${id}`);
    return data;
  }

  async createRoute(payload: CreateRouteData): Promise<Route> {
    const { data } = await this.client.post<Route>('/routes', payload);
    return data;
  }

  async updateRoute(id: number, payload: Partial<Route>): Promise<Route> {
    const { data } = await this.client.put<Route>(`/routes/${id}`, payload);
    return data;
  }

  async deleteRoute(id: number): Promise<void> {
    await this.client.delete(`/routes/${id}`);
  }

  async batchUpdateRoutePositions(updates: RoutePositionUpdate[]): Promise<void> {
    await this.client.put('/routes/batch-positions', updates);
  }

  // ===== Session API =====

  async getSessions(): Promise<Session[]> {
    const { data } = await this.adminClient.get<Session[]>('/sessions');
    return this.expectArray<Session>(data, '/sessions');
  }

  async updateSessionProject(
    sessionID: string,
    projectID: number,
  ): Promise<{ session: Session; updatedRequests: number }> {
    const { data } = await this.adminClient.put<{
      session: Session;
      updatedRequests: number;
    }>(`/sessions/${encodeURIComponent(sessionID)}/project`, { projectID });
    return data;
  }

  async rejectSession(sessionID: string): Promise<Session> {
    const { data } = await this.adminClient.post<Session>(
      `/sessions/${encodeURIComponent(sessionID)}/reject`,
    );
    return data;
  }

  // ===== RetryConfig API =====

  async getRetryConfigs(): Promise<RetryConfig[]> {
    const { data } = await this.client.get<RetryConfig[]>('/retry-configs');
    return this.expectArray<RetryConfig>(data, '/retry-configs');
  }

  async getRetryConfig(id: number): Promise<RetryConfig> {
    const { data } = await this.client.get<RetryConfig>(`/retry-configs/${id}`);
    return data;
  }

  async createRetryConfig(payload: CreateRetryConfigData): Promise<RetryConfig> {
    const { data } = await this.client.post<RetryConfig>('/retry-configs', payload);
    return data;
  }

  async updateRetryConfig(id: number, payload: Partial<RetryConfig>): Promise<RetryConfig> {
    const { data } = await this.client.put<RetryConfig>(`/retry-configs/${id}`, payload);
    return data;
  }

  async deleteRetryConfig(id: number): Promise<void> {
    await this.client.delete(`/retry-configs/${id}`);
  }

  // ===== RoutingStrategy API =====

  async getRoutingStrategies(): Promise<RoutingStrategy[]> {
    const { data } = await this.adminClient.get<RoutingStrategy[]>('/routing-strategies');
    return this.expectArray<RoutingStrategy>(data, '/routing-strategies');
  }

  async getRoutingStrategy(id: number): Promise<RoutingStrategy> {
    const { data } = await this.adminClient.get<RoutingStrategy>(`/routing-strategies/${id}`);
    return data;
  }

  async createRoutingStrategy(payload: CreateRoutingStrategyData): Promise<RoutingStrategy> {
    const { data } = await this.adminClient.post<RoutingStrategy>('/routing-strategies', payload);
    return data;
  }

  async updateRoutingStrategy(
    id: number,
    payload: Partial<RoutingStrategy>,
  ): Promise<RoutingStrategy> {
    const { data } = await this.adminClient.put<RoutingStrategy>(
      `/routing-strategies/${id}`,
      payload,
    );
    return data;
  }

  async deleteRoutingStrategy(id: number): Promise<void> {
    await this.adminClient.delete(`/routing-strategies/${id}`);
  }

  // ===== ProxyRequest API =====

  async getProxyRequests(
    params?: CursorPaginationParams,
  ): Promise<CursorPaginationResult<ProxyRequest>> {
    const { data } = await this.adminClient.get<CursorPaginationResult<ProxyRequest>>('/requests', {
      params,
    });
    return data ?? { items: [], hasMore: false };
  }

  async getProxyRequestsCount(
    providerId?: number,
    status?: string,
    apiTokenId?: number,
    projectId?: number,
  ): Promise<number> {
    const params: Record<string, string> = {};
    if (providerId !== undefined) {
      params.providerId = String(providerId);
    }
    if (status !== undefined) {
      params.status = status;
    }
    if (apiTokenId !== undefined) {
      params.apiTokenId = String(apiTokenId);
    }
    if (projectId !== undefined) {
      params.projectId = String(projectId);
    }
    const { data } = await this.adminClient.get<number>('/requests/count', { params });
    return data ?? 0;
  }

  async getActiveProxyRequests(): Promise<ProxyRequest[]> {
    const { data } = await this.adminClient.get<ProxyRequest[]>('/requests/active');
    // Ensure we always return an array
    if (!data || !Array.isArray(data)) {
      return [];
    }
    return data;
  }

  async getProxyRequest(id: number): Promise<ProxyRequest> {
    const { data } = await this.adminClient.get<ProxyRequest>(`/requests/${id}`);
    return data;
  }

  async getProxyUpstreamAttempts(proxyRequestId: number): Promise<ProxyUpstreamAttempt[]> {
    const { data } = await this.adminClient.get<ProxyUpstreamAttempt[]>(
      `/requests/${proxyRequestId}/attempts`,
    );
    return data ?? [];
  }

  // ===== Proxy Status API =====

  async getProxyStatus(): Promise<ProxyStatus> {
    const { data } = await this.adminClient.get<ProxyStatus>('/proxy-status');
    return this.expectObject<ProxyStatus>(data, '/admin/proxy-status');
  }

  async getPublicProxyStatus(): Promise<ProxyStatus> {
    const { data } = await this.client.get<ProxyStatus>('/proxy-status');
    return this.expectObject<ProxyStatus>(data, '/proxy-status');
  }

  // ===== System API =====

  async restartServer(): Promise<void> {
    await this.adminClient.post('/restart');
  }

  // ===== Provider Stats API =====

  async getProviderStats(
    clientType?: string,
    projectId?: number,
  ): Promise<Record<number, ProviderStats>> {
    const params: Record<string, string | number> = {};
    if (clientType) params.client_type = clientType;
    if (projectId !== undefined) params.project_id = projectId;
    const { data } = await this.client.get<Record<number, ProviderStats>>('/provider-stats', {
      params: Object.keys(params).length > 0 ? params : undefined,
    });
    return data ?? {};
  }

  // ===== Settings API =====

  async getPublicSettings(): Promise<Record<string, string>> {
    const { data } = await this.client.get<Record<string, string>>('/settings');
    return this.expectStringRecord(data, '/settings');
  }

  async getAdminSettings(): Promise<Record<string, string>> {
    const { data } = await this.adminClient.get<Record<string, string>>('/settings');
    return this.expectStringRecord(data, '/admin/settings');
  }

  async getSetting(key: string): Promise<{ key: string; value: string }> {
    const { data } = await this.adminClient.get<{ key: string; value: string }>(`/settings/${key}`);
    return data;
  }

  async updateSetting(key: string, value: string): Promise<{ key: string; value: string }> {
    const { data } = await this.adminClient.put<{ key: string; value: string }>(
      `/settings/${key}`,
      {
        value,
      },
    );
    return data;
  }

  async deleteSetting(key: string): Promise<void> {
    await this.adminClient.delete(`/settings/${key}`);
  }

  // ===== Logs API =====

  async getLogs(limit = 100): Promise<{ lines: string[]; count: number }> {
    const { data } = await this.adminClient.get<{ lines: string[]; count: number }>('/logs', {
      params: { limit },
    });
    return data ?? { lines: [], count: 0 };
  }

  // ===== Antigravity API =====

  async validateAntigravityToken(refreshToken: string): Promise<AntigravityTokenValidationResult> {
    const { data } = await this.client.post<AntigravityTokenValidationResult>(
      '/antigravity/validate-token',
      { refreshToken },
    );
    return data;
  }

  async validateAntigravityTokens(tokens: string[]): Promise<AntigravityBatchValidationResult> {
    const { data } = await this.client.post<AntigravityBatchValidationResult>(
      '/antigravity/validate-tokens',
      { tokens },
    );
    return data;
  }

  async validateAntigravityTokenText(tokenText: string): Promise<AntigravityBatchValidationResult> {
    const { data } = await this.client.post<AntigravityBatchValidationResult>(
      '/antigravity/validate-tokens',
      { tokenText },
    );
    return data;
  }

  async getAntigravityProviderQuota(
    providerId: number,
    forceRefresh?: boolean,
  ): Promise<AntigravityQuotaData> {
    const params = forceRefresh ? { refresh: 'true' } : undefined;
    const { data } = await this.client.get<AntigravityQuotaData>(
      `/antigravity/providers/${providerId}/quota`,
      { params },
    );
    return data;
  }

  async getAntigravityBatchQuotas(): Promise<Record<number, AntigravityQuotaData>> {
    const { data } = await this.client.get<{ quotas: Record<number, AntigravityQuotaData> }>(
      '/antigravity/providers/quotas',
    );
    return data.quotas;
  }

  async startAntigravityOAuth(): Promise<{ authURL: string; state: string }> {
    const { data } = await this.client.post<{ authURL: string; state: string }>(
      '/antigravity/oauth/start',
    );
    return data;
  }

  async refreshAntigravityQuotas(): Promise<{ success: boolean; refreshed: number }> {
    const { data } = await this.client.post<{ success: boolean; refreshed: number }>(
      '/antigravity/refresh-quotas',
    );
    return data;
  }

  async sortAntigravityRoutes(): Promise<{ success: boolean }> {
    const { data } = await this.client.post<{ success: boolean }>('/antigravity/sort-routes');
    return data;
  }

  // ===== Model Mapping API =====

  async getModelMappings(): Promise<ModelMapping[]> {
    const { data } = await this.client.get<ModelMapping[]>('/model-mappings');
    return this.expectArray<ModelMapping>(data, '/model-mappings');
  }

  async createModelMapping(input: ModelMappingInput): Promise<ModelMapping> {
    const { data } = await this.client.post<ModelMapping>('/model-mappings', input);
    return data;
  }

  async updateModelMapping(id: number, input: ModelMappingInput): Promise<ModelMapping> {
    const { data } = await this.client.put<ModelMapping>(`/model-mappings/${id}`, input);
    return data;
  }

  async deleteModelMapping(id: number): Promise<void> {
    await this.client.delete(`/model-mappings/${id}`);
  }

  async clearAllModelMappings(): Promise<void> {
    await this.client.delete('/model-mappings/clear-all');
  }

  async resetModelMappingsToDefaults(): Promise<void> {
    await this.client.post('/model-mappings/reset-defaults');
  }

  // ===== Kiro API =====

  async validateKiroSocialToken(refreshToken: string): Promise<KiroTokenValidationResult> {
    const { data } = await this.client.post<KiroTokenValidationResult>(
      '/kiro/validate-social-token',
      { refreshToken },
    );
    return data;
  }

  async getKiroProviderQuota(providerId: number): Promise<KiroQuotaData> {
    const { data } = await this.client.get<KiroQuotaData>(`/kiro/providers/${providerId}/quota`);
    return data;
  }

  // ===== Codex API =====

  async validateCodexToken(refreshToken: string): Promise<CodexTokenValidationResult> {
    const { data } = await this.client.post<CodexTokenValidationResult>('/codex/validate-token', {
      refreshToken,
    });
    return data;
  }

  async startCodexOAuth(): Promise<{ authURL: string; state: string }> {
    const { data } = await this.client.post<{ authURL: string; state: string }>(
      '/codex/oauth/start',
    );
    return data;
  }

  async exchangeCodexOAuthCallback(
    code: string,
    state: string,
  ): Promise<import('./types').CodexOAuthResult> {
    const { data } = await this.client.post<import('./types').CodexOAuthResult>(
      '/codex/oauth/exchange',
      { code, state },
    );
    return data;
  }

  async refreshCodexProviderInfo(providerId: number): Promise<CodexTokenValidationResult> {
    const { data } = await this.client.post<CodexTokenValidationResult>(
      `/codex/provider/${providerId}/refresh`,
    );
    return data;
  }

  async getCodexProviderUsage(providerId: number): Promise<CodexUsageResponse> {
    const { data } = await this.client.get<CodexUsageResponse>(
      `/codex/provider/${providerId}/usage`,
    );
    return data;
  }

  async getCodexBatchQuotas(): Promise<Record<number, CodexQuotaData>> {
    const { data } = await this.client.get<{ quotas: Record<number, CodexQuotaData> }>(
      '/codex/providers/quotas',
    );
    return data.quotas ?? {};
  }

  async refreshCodexQuotas(): Promise<{ success: boolean; refreshed: boolean }> {
    const { data } = await this.client.post<{ success: boolean; refreshed: boolean }>(
      '/codex/refresh-quotas',
    );
    return data;
  }

  async sortCodexRoutes(): Promise<{ success: boolean }> {
    const { data } = await this.client.post<{ success: boolean }>('/codex/sort-routes');
    return data;
  }

  // ===== Claude API =====

  async validateClaudeToken(refreshToken: string): Promise<ClaudeTokenValidationResult> {
    const { data } = await this.client.post<ClaudeTokenValidationResult>('/claude/validate-token', {
      refreshToken,
    });
    return data;
  }

  async startClaudeOAuth(): Promise<{ authURL: string; state: string }> {
    const { data } = await this.client.post<{ authURL: string; state: string }>(
      '/claude/oauth/start',
    );
    return data;
  }

  async exchangeClaudeOAuthCallback(
    code: string,
    state: string,
  ): Promise<import('./types').ClaudeOAuthResult> {
    const { data } = await this.client.post<import('./types').ClaudeOAuthResult>(
      '/claude/oauth/exchange',
      { code, state },
    );
    return data;
  }

  async refreshClaudeProviderInfo(providerId: number): Promise<ClaudeTokenValidationResult> {
    const { data } = await this.client.post<ClaudeTokenValidationResult>(
      `/claude/provider/${providerId}/refresh`,
    );
    return data;
  }

  // ===== Cooldown API =====

  async getCooldowns(): Promise<Cooldown[]> {
    const { data } = await this.adminClient.get<Cooldown[]>('/cooldowns');
    return this.expectArray<Cooldown>(data, '/cooldowns');
  }

  async clearCooldown(providerId: number): Promise<void> {
    await this.adminClient.delete(`/cooldowns/${providerId}`);
  }

  async setCooldown(providerId: number, untilTime: string, clientType?: string): Promise<void> {
    await this.adminClient.put(`/cooldowns/${providerId}`, { untilTime, clientType });
  }

  // ===== Auth API =====

  async getAuthStatus(): Promise<AuthStatus> {
    const { data } = await this.adminClient.get<AuthStatus>('/auth/status');
    return data;
  }

  async login(username: string, password: string): Promise<AuthLoginResult> {
    const { data } = await this.adminClient.post<AuthLoginResult>('/auth/login', {
      username,
      password,
    });
    return data;
  }

  async startPasskeyLogin(username?: string): Promise<PasskeyLoginOptionsResult> {
    const { data } = await this.adminClient.post<PasskeyLoginOptionsResult>(
      '/auth/passkey/login/options',
      { username: username || '' },
    );
    return data;
  }

  async finishPasskeyLogin(
    sessionID: string,
    credential: AuthenticationResponseJSON,
  ): Promise<AuthLoginResult> {
    const { data } = await this.adminClient.post<AuthLoginResult>('/auth/passkey/login/verify', {
      sessionID,
      credential,
    });
    return data;
  }

  async startPasskeyRegistration(): Promise<PasskeyRegistrationOptionsResult> {
    const { data } = await this.adminClient.post<PasskeyRegistrationOptionsResult>(
      '/auth/passkey/register/options',
    );
    return data;
  }

  async finishPasskeyRegistration(
    sessionID: string,
    credential: RegistrationResponseJSON,
  ): Promise<PasskeyRegisterResult> {
    const { data } = await this.adminClient.post<PasskeyRegisterResult>(
      '/auth/passkey/register/verify',
      { sessionID, credential },
    );
    return data;
  }

  async listPasskeyCredentials(): Promise<PasskeyCredential[]> {
    const { data } = await this.adminClient.get<{
      success: boolean;
      credentials?: PasskeyCredential[];
    }>('/auth/passkey/credentials');
    return data?.credentials ?? [];
  }

  async deletePasskeyCredential(id: string): Promise<void> {
    await this.adminClient.delete(`/auth/passkey/credentials/${encodeURIComponent(id)}`);
  }

  async register(
    username: string,
    password: string,
    tenantID?: number,
  ): Promise<AuthRegisterResult> {
    const { data } = await this.adminClient.post<AuthRegisterResult>('/auth/register', {
      username,
      password,
      tenantID,
    });
    return data;
  }

  async apply(username: string, password: string, inviteCode: string): Promise<ApplyResult> {
    const { data } = await this.adminClient.post<ApplyResult>('/auth/apply', {
      username,
      password,
      inviteCode,
    });
    return data;
  }

  async changeMyPassword(oldPassword: string, newPassword: string): Promise<ChangePasswordResult> {
    const { data } = await this.adminClient.put<ChangePasswordResult>('/auth/password', {
      oldPassword,
      newPassword,
    });
    return data;
  }

  setAuthToken(token: string): void {
    this.authToken = token;
  }

  clearAuthToken(): void {
    this.authToken = null;
  }

  // ===== User API =====

  async getUsers(): Promise<User[]> {
    const { data } = await this.adminClient.get<User[]>('/users');
    return this.expectArray<User>(data, '/users');
  }

  async getUser(id: number): Promise<User> {
    const { data } = await this.adminClient.get<User>(`/users/${id}`);
    return data;
  }

  async createUser(payload: CreateUserData): Promise<User> {
    const { data } = await this.adminClient.post<User>('/users', payload);
    return data;
  }

  async updateUser(id: number, payload: UpdateUserData): Promise<User> {
    const { data } = await this.adminClient.put<User>(`/users/${id}`, payload);
    return data;
  }

  async deleteUser(id: number): Promise<void> {
    await this.adminClient.delete(`/users/${id}`);
  }

  async updatePassword(userId: number, password: string): Promise<void> {
    await this.adminClient.put(`/users/${userId}/password`, { password });
  }

  async approveUser(id: number): Promise<User> {
    const { data } = await this.adminClient.put<User>(`/users/${id}/approve`);
    return data;
  }

  // ===== API Token API =====

  async getAdminAPITokens(): Promise<APIToken[]> {
    const { data } = await this.adminClient.get<APIToken[]>('/api-tokens');
    return this.expectArray<APIToken>(data, '/api-tokens');
  }

  async getAdminAPIToken(id: number): Promise<APIToken> {
    const { data } = await this.adminClient.get<APIToken>(`/api-tokens/${id}`);
    return this.expectObject<APIToken>(data, `/api-tokens/${id}`);
  }

  async getVisibleAPITokens(): Promise<APIToken[]> {
    const { data } = await this.client.get<APIToken[]>('/api-tokens');
    return this.expectArray<APIToken>(data, '/api-tokens');
  }

  async createAPIToken(payload: CreateAPITokenData): Promise<APITokenCreateResult> {
    const { data } = await this.adminClient.post<APITokenCreateResult>('/api-tokens', payload);
    return data;
  }

  async updateAPIToken(id: number, payload: Partial<APIToken>): Promise<APIToken> {
    const { data } = await this.adminClient.put<APIToken>(`/api-tokens/${id}`, payload);
    return data;
  }

  async deleteAPIToken(id: number): Promise<void> {
    await this.adminClient.delete(`/api-tokens/${id}`);
  }

  // ===== Invite Code API =====

  async getInviteCodes(): Promise<InviteCode[]> {
    const { data } = await this.adminClient.get<InviteCode[]>('/invite-codes');
    return this.expectArray<InviteCode>(data, '/invite-codes');
  }

  async getInviteCode(id: number): Promise<InviteCode> {
    const { data } = await this.adminClient.get<InviteCode>(`/invite-codes/${id}`);
    return data;
  }

  async createInviteCodes(payload: CreateInviteCodeData): Promise<InviteCodeCreateResult> {
    const { data } = await this.adminClient.post<InviteCodeCreateResult>('/invite-codes', payload);
    return data;
  }

  async updateInviteCode(id: number, payload: UpdateInviteCodeData): Promise<InviteCode> {
    const { data } = await this.adminClient.put<InviteCode>(`/invite-codes/${id}`, payload);
    return data;
  }

  async deleteInviteCode(id: number): Promise<void> {
    await this.adminClient.delete(`/invite-codes/${id}`);
  }

  async getInviteCodeUsages(id: number): Promise<InviteCodeUsage[]> {
    const { data } = await this.adminClient.get<InviteCodeUsage[]>(`/invite-codes/${id}/usages`);
    return data ?? [];
  }

  // ===== Usage Stats API =====

  async getUsageStats(filter?: UsageStatsFilter): Promise<UsageStats[]> {
    const params = new URLSearchParams();
    if (filter?.granularity) params.set('granularity', filter.granularity);
    if (filter?.start) params.set('start', filter.start);
    if (filter?.end) params.set('end', filter.end);
    if (filter?.routeId) params.set('routeId', String(filter.routeId));
    if (filter?.providerId) params.set('providerId', String(filter.providerId));
    if (filter?.projectId) params.set('projectId', String(filter.projectId));
    if (filter?.clientType) params.set('clientType', filter.clientType);
    if (filter?.apiTokenId) params.set('apiTokenId', String(filter.apiTokenId));
    if (filter?.model) params.set('model', filter.model);

    const query = params.toString();
    const url = query ? `/usage-stats?${query}` : '/usage-stats';
    const { data } = await this.adminClient.get<UsageStats[]>(url);
    return this.expectArray<UsageStats>(data, '/usage-stats');
  }

  async recalculateUsageStats(): Promise<void> {
    await this.adminClient.post('/usage-stats/recalculate');
  }

  async recalculateCosts(): Promise<RecalculateCostsResult> {
    const { data } = await this.adminClient.post<RecalculateCostsResult>(
      '/usage-stats/recalculate-costs',
    );
    return data;
  }

  async recalculateRequestCost(requestId: number): Promise<RecalculateRequestCostResult> {
    const { data } = await this.adminClient.post<RecalculateRequestCostResult>(
      `/requests/${requestId}/recalculate-cost`,
    );
    return data;
  }

  // ===== Dashboard API =====

  async getDashboardData(): Promise<DashboardData> {
    const { data } = await this.adminClient.get<DashboardData>('/dashboard');
    return data;
  }

  // ===== Response Model API =====

  async getResponseModels(): Promise<string[]> {
    const { data } = await this.client.get<string[]>('/response-models');
    return this.expectArray<string>(data, '/response-models');
  }

  // ===== Backup API =====

  async exportBackup(): Promise<BackupFile> {
    const { data } = await this.adminClient.get<BackupFile>('/backup/export');
    return data;
  }

  async importBackup(
    backup: BackupFile,
    options?: BackupImportOptions,
  ): Promise<BackupImportResult> {
    const params = new URLSearchParams();
    if (options?.conflictStrategy) params.set('conflictStrategy', options.conflictStrategy);
    if (options?.dryRun) params.set('dryRun', 'true');

    const query = params.toString();
    const url = query ? `/backup/import?${query}` : '/backup/import';
    const { data } = await this.adminClient.post<BackupImportResult>(url, backup);
    return data;
  }

  // ===== Pricing API =====

  async getPricing(): Promise<PriceTable> {
    const { data } = await this.adminClient.get<PriceTable>('/pricing');
    return data;
  }

  // ===== Model Price API =====

  async getModelPrices(): Promise<ModelPrice[]> {
    const { data } = await this.client.get<ModelPrice[]>('/model-prices');
    return this.expectArray<ModelPrice>(data, '/model-prices');
  }

  async getModelPrice(id: number): Promise<ModelPrice> {
    const { data } = await this.client.get<ModelPrice>(`/model-prices/${id}`);
    return data;
  }

  async createModelPrice(input: ModelPriceInput): Promise<ModelPrice> {
    const { data } = await this.adminClient.post<ModelPrice>('/model-prices', input);
    return data;
  }

  async updateModelPrice(id: number, input: ModelPriceInput): Promise<ModelPrice> {
    const { data } = await this.adminClient.put<ModelPrice>(`/model-prices/${id}`, input);
    return data;
  }

  async deleteModelPrice(id: number): Promise<void> {
    await this.adminClient.delete(`/model-prices/${id}`);
  }

  async resetModelPricesToDefaults(): Promise<ModelPrice[]> {
    const { data } = await this.adminClient.post<ModelPrice[]>('/model-prices/reset');
    return data;
  }

  // ===== WebSocket 订阅 =====

  subscribe<T = unknown>(eventType: WSMessageType, callback: EventCallback<T>): UnsubscribeFn {
    if (!this.eventListeners.has(eventType)) {
      this.eventListeners.set(eventType, new Set());
    }
    this.eventListeners.get(eventType)!.add(callback as EventCallback);

    return () => {
      this.eventListeners.get(eventType)?.delete(callback as EventCallback);
    };
  }

  // ===== 生命周期 =====

  async connect(): Promise<void> {
    this.manualDisconnect = false;

    // Already connected
    if (this.ws?.readyState === WebSocket.OPEN) {
      return Promise.resolve();
    }

    // Connection in progress, return existing promise to avoid race conditions
    if (this.connectPromise && this.ws?.readyState === WebSocket.CONNECTING) {
      return this.connectPromise;
    }

    this.connectPromise = new Promise((resolve, reject) => {
      this.ws = new WebSocket(this.config.wsURL);

      let opened = false;
      let settled = false;
      let reconnectScheduled = false;
      let timeoutId: ReturnType<typeof setTimeout> | null = null;

      const clearConnectTimeout = () => {
        if (!timeoutId) {
          return;
        }
        clearTimeout(timeoutId);
        timeoutId = null;
      };

      const scheduleReconnectOnce = () => {
        if (reconnectScheduled || this.manualDisconnect) {
          return;
        }
        reconnectScheduled = true;
        this.scheduleReconnect();
      };

      const settleResolve = () => {
        if (settled) {
          return;
        }
        settled = true;
        clearConnectTimeout();
        this.connectPromise = null;
        resolve();
      };

      const settleReject = (error: Error) => {
        if (settled) {
          return;
        }
        settled = true;
        clearConnectTimeout();
        this.connectPromise = null;
        reject(error);
      };

      timeoutId = setTimeout(() => {
        if (opened) {
          return;
        }
        settleReject(new Error(`WebSocket connection timeout after ${this.connectTimeoutMs}ms`));
        this.ws?.close();
        scheduleReconnectOnce();
      }, this.connectTimeoutMs);

      this.ws.onopen = () => {
        opened = true;
        const isReconnect = this.reconnectAttempts > 0;
        this.reconnectAttempts = 0;

        // 如果是重连，发送内部事件通知前端清理状态
        if (isReconnect) {
          const listeners = this.eventListeners.get('_ws_reconnected');
          listeners?.forEach((callback) => callback({}));
        }

        settleResolve();
      };

      this.ws.onerror = () => {
        if (opened) {
          return;
        }
        settleReject(new Error('WebSocket connection error'));
        scheduleReconnectOnce();
      };

      this.ws.onclose = () => {
        if (!opened) {
          settleReject(new Error('WebSocket connection closed before open'));
        }
        scheduleReconnectOnce();
      };

      this.ws.onmessage = (event) => {
        try {
          const message: WSMessage = JSON.parse(event.data);
          const listeners = this.eventListeners.get(message.type);
          listeners?.forEach((callback) => callback(message.data));
        } catch (e) {
          console.error('Failed to parse WebSocket message:', e);
        }
      };
    });

    return this.connectPromise;
  }

  disconnect(): void {
    this.manualDisconnect = true;
    this.reconnectAttempts = 0;

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.ws?.close();
    this.ws = null;
  }

  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.config.maxReconnectAttempts) {
      console.error('Max reconnect attempts reached');
      return;
    }

    this.reconnectTimer = setTimeout(() => {
      this.reconnectAttempts++;
      this.connect().catch(console.error);
    }, this.config.reconnectInterval);
  }
}
