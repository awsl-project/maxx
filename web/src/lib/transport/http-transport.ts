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
  AntigravityGlobalSettings,
  ImportResult,
  Cooldown,
  KiroTokenValidationResult,
  KiroQuotaData,
} from './types';

export class HttpTransport implements Transport {
  private client: AxiosInstance;
  private ws: WebSocket | null = null;
  private config: Required<TransportConfig>;
  private eventListeners: Map<WSMessageType, Set<EventCallback>> = new Map();
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private connectPromise: Promise<void> | null = null;

  constructor(config: TransportConfig = {}) {
    this.config = {
      baseURL: config.baseURL ?? '/api/admin',
      wsURL: config.wsURL ?? `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`,
      reconnectInterval: config.reconnectInterval ?? 3000,
      maxReconnectAttempts: config.maxReconnectAttempts ?? 10,
    };

    this.client = axios.create({
      baseURL: this.config.baseURL,
      headers: {
        'Content-Type': 'application/json',
      },
    });
  }

  // ===== Provider API =====

  async getProviders(): Promise<Provider[]> {
    const { data } = await this.client.get<Provider[]>('/providers');
    return data ?? [];
  }

  async getProvider(id: number): Promise<Provider> {
    const { data } = await this.client.get<Provider>(`/providers/${id}`);
    return data;
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
    return data ?? [];
  }

  async importProviders(providers: Provider[]): Promise<ImportResult> {
    const { data } = await this.client.post<ImportResult>('/providers/import', providers);
    return data;
  }

  // ===== Project API =====

  async getProjects(): Promise<Project[]> {
    const { data } = await this.client.get<Project[]>('/projects');
    return data ?? [];
  }

  async getProject(id: number): Promise<Project> {
    const { data } = await this.client.get<Project>(`/projects/${id}`);
    return data;
  }

  async getProjectBySlug(slug: string): Promise<Project> {
    const { data } = await this.client.get<Project>(`/projects/by-slug/${slug}`);
    return data;
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
    return data ?? [];
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

  // ===== Session API =====

  async getSessions(): Promise<Session[]> {
    const { data } = await this.client.get<Session[]>('/sessions');
    return data ?? [];
  }

  async updateSessionProject(sessionID: string, projectID: number): Promise<{ session: Session; updatedRequests: number }> {
    const { data } = await this.client.put<{ session: Session; updatedRequests: number }>(
      `/sessions/${encodeURIComponent(sessionID)}/project`,
      { projectID }
    );
    return data;
  }

  async rejectSession(sessionID: string): Promise<Session> {
    const { data } = await this.client.post<Session>(
      `/sessions/${encodeURIComponent(sessionID)}/reject`
    );
    return data;
  }

  // ===== RetryConfig API =====

  async getRetryConfigs(): Promise<RetryConfig[]> {
    const { data } = await this.client.get<RetryConfig[]>('/retry-configs');
    return data ?? [];
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
    const { data } = await this.client.get<RoutingStrategy[]>('/routing-strategies');
    return data ?? [];
  }

  async getRoutingStrategy(id: number): Promise<RoutingStrategy> {
    const { data } = await this.client.get<RoutingStrategy>(`/routing-strategies/${id}`);
    return data;
  }

  async createRoutingStrategy(payload: CreateRoutingStrategyData): Promise<RoutingStrategy> {
    const { data } = await this.client.post<RoutingStrategy>('/routing-strategies', payload);
    return data;
  }

  async updateRoutingStrategy(id: number, payload: Partial<RoutingStrategy>): Promise<RoutingStrategy> {
    const { data } = await this.client.put<RoutingStrategy>(`/routing-strategies/${id}`, payload);
    return data;
  }

  async deleteRoutingStrategy(id: number): Promise<void> {
    await this.client.delete(`/routing-strategies/${id}`);
  }

  // ===== ProxyRequest API =====

  async getProxyRequests(params?: CursorPaginationParams): Promise<CursorPaginationResult<ProxyRequest>> {
    const { data } = await this.client.get<CursorPaginationResult<ProxyRequest>>('/requests', { params });
    return data ?? { items: [], hasMore: false };
  }

  async getProxyRequestsCount(): Promise<number> {
    const { data } = await this.client.get<number>('/requests/count');
    return data ?? 0;
  }

  async getProxyRequest(id: number): Promise<ProxyRequest> {
    const { data } = await this.client.get<ProxyRequest>(`/requests/${id}`);
    return data;
  }

  async getProxyUpstreamAttempts(proxyRequestId: number): Promise<ProxyUpstreamAttempt[]> {
    const { data } = await this.client.get<ProxyUpstreamAttempt[]>(`/requests/${proxyRequestId}/attempts`);
    return data ?? [];
  }

  // ===== Proxy Status API =====

  async getProxyStatus(): Promise<ProxyStatus> {
    const { data } = await this.client.get<ProxyStatus>('/proxy-status');
    return data;
  }

  // ===== Provider Stats API =====

  async getProviderStats(clientType?: string, projectId?: number): Promise<Record<number, ProviderStats>> {
    const params: Record<string, string | number> = {};
    if (clientType) params.client_type = clientType;
    if (projectId !== undefined) params.project_id = projectId;
    const { data } = await this.client.get<Record<number, ProviderStats>>('/provider-stats', { params: Object.keys(params).length > 0 ? params : undefined });
    return data ?? {};
  }

  // ===== Settings API =====

  async getSettings(): Promise<Record<string, string>> {
    const { data } = await this.client.get<Record<string, string>>('/settings');
    return data ?? {};
  }

  async getSetting(key: string): Promise<{ key: string; value: string }> {
    const { data } = await this.client.get<{ key: string; value: string }>(`/settings/${key}`);
    return data;
  }

  async updateSetting(key: string, value: string): Promise<{ key: string; value: string }> {
    const { data } = await this.client.put<{ key: string; value: string }>(`/settings/${key}`, { value });
    return data;
  }

  async deleteSetting(key: string): Promise<void> {
    await this.client.delete(`/settings/${key}`);
  }

  // ===== Logs API =====

  async getLogs(limit = 100): Promise<{ lines: string[]; count: number }> {
    const { data } = await this.client.get<{ lines: string[]; count: number }>('/logs', {
      params: { limit },
    });
    return data ?? { lines: [], count: 0 };
  }

  // ===== Antigravity API =====

  async validateAntigravityToken(refreshToken: string): Promise<AntigravityTokenValidationResult> {
    const { data } = await axios.post<AntigravityTokenValidationResult>(
      '/api/antigravity/validate-token',
      { refreshToken }
    );
    return data;
  }

  async validateAntigravityTokens(tokens: string[]): Promise<AntigravityBatchValidationResult> {
    const { data } = await axios.post<AntigravityBatchValidationResult>(
      '/api/antigravity/validate-tokens',
      { tokens }
    );
    return data;
  }

  async validateAntigravityTokenText(tokenText: string): Promise<AntigravityBatchValidationResult> {
    const { data } = await axios.post<AntigravityBatchValidationResult>(
      '/api/antigravity/validate-tokens',
      { tokenText }
    );
    return data;
  }

  async getAntigravityProviderQuota(providerId: number, forceRefresh?: boolean): Promise<AntigravityQuotaData> {
    const params = forceRefresh ? { refresh: 'true' } : undefined;
    const { data } = await axios.get<AntigravityQuotaData>(
      `/api/antigravity/providers/${providerId}/quota`,
      { params }
    );
    return data;
  }

  async startAntigravityOAuth(): Promise<{ authURL: string; state: string }> {
    const { data } = await axios.post<{ authURL: string; state: string }>(
      '/api/antigravity/oauth/start'
    );
    return data;
  }

  async getAntigravityGlobalSettings(): Promise<AntigravityGlobalSettings> {
    const { data } = await this.client.get<AntigravityGlobalSettings>('/antigravity-settings');
    return data;
  }

  async updateAntigravityGlobalSettings(settings: AntigravityGlobalSettings): Promise<AntigravityGlobalSettings> {
    const { data } = await this.client.put<AntigravityGlobalSettings>('/antigravity-settings', settings);
    return data;
  }

  async resetAntigravityGlobalSettings(): Promise<AntigravityGlobalSettings> {
    const { data } = await this.client.post<AntigravityGlobalSettings>('/antigravity-settings-reset');
    return data;
  }

  // ===== Kiro API =====

  async validateKiroSocialToken(refreshToken: string): Promise<KiroTokenValidationResult> {
    const { data } = await axios.post<KiroTokenValidationResult>(
      '/api/kiro/validate-social-token',
      { refreshToken }
    );
    return data;
  }

  async getKiroProviderQuota(providerId: number): Promise<KiroQuotaData> {
    const { data } = await axios.get<KiroQuotaData>(
      `/api/kiro/providers/${providerId}/quota`
    );
    return data;
  }

  // ===== Cooldown API =====

  async getCooldowns(): Promise<Cooldown[]> {
    const { data } = await this.client.get<Cooldown[]>('/cooldowns');
    return data ?? [];
  }

  async clearCooldown(providerId: number): Promise<void> {
    await this.client.delete(`/cooldowns/${providerId}`);
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

      this.ws.onopen = () => {
        const isReconnect = this.reconnectAttempts > 0;
        this.reconnectAttempts = 0;
        this.connectPromise = null;

        // 如果是重连，发送内部事件通知前端清理状态
        if (isReconnect) {
          const listeners = this.eventListeners.get('_ws_reconnected');
          listeners?.forEach((callback) => callback({}));
        }

        resolve();
      };

      this.ws.onerror = (error) => {
        this.connectPromise = null;
        reject(error);
      };

      this.ws.onclose = () => {
        this.connectPromise = null;
        this.scheduleReconnect();
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
