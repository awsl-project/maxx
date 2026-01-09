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
  PaginationParams,
  WSMessageType,
  WSMessage,
  EventCallback,
  UnsubscribeFn,
} from './types';

export class HttpTransport implements Transport {
  private client: AxiosInstance;
  private ws: WebSocket | null = null;
  private config: Required<TransportConfig>;
  private eventListeners: Map<WSMessageType, Set<EventCallback>> = new Map();
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(config: TransportConfig = {}) {
    this.config = {
      baseURL: config.baseURL ?? '/admin',
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

  // ===== Project API =====

  async getProjects(): Promise<Project[]> {
    const { data } = await this.client.get<Project[]>('/projects');
    return data ?? [];
  }

  async getProject(id: number): Promise<Project> {
    const { data } = await this.client.get<Project>(`/projects/${id}`);
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

  async getProxyRequests(params?: PaginationParams): Promise<ProxyRequest[]> {
    const { data } = await this.client.get<ProxyRequest[]>('/requests', { params });
    return data ?? [];
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

  async getProviderStats(clientType?: string): Promise<Record<number, ProviderStats>> {
    const params = clientType ? { client_type: clientType } : undefined;
    const { data } = await this.client.get<Record<number, ProviderStats>>('/provider-stats', { params });
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
    return new Promise((resolve, reject) => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        resolve();
        return;
      }

      this.ws = new WebSocket(this.config.wsURL);

      this.ws.onopen = () => {
        this.reconnectAttempts = 0;
        resolve();
      };

      this.ws.onerror = (error) => {
        reject(error);
      };

      this.ws.onclose = () => {
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
