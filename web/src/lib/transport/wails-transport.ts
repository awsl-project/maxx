/**
 * Wails Transport 实现
 * 使用 Wails bindings 调用 Go 方法，Wails Events 接收实时推送
 */

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
  EventCallback,
  UnsubscribeFn,
} from './types';

// Wails 事件 API 类型
type WailsEventCallback = (data: unknown) => void;
type WailsUnsubscribeFn = () => void;

export class WailsTransport implements Transport {
  private connected = false;
  private eventUnsubscribers: Map<string, WailsUnsubscribeFn> = new Map();
  private eventCallbacks: Map<WSMessageType, Set<EventCallback>> = new Map();

  constructor(_config: TransportConfig = {}) {
    // Wails 模式下配置通常不需要
  }

  // 辅助方法：调用 Go 服务方法
  private async call<T>(method: string, ...args: unknown[]): Promise<T> {
    if (!window.wails) {
      throw new Error('Wails runtime not available');
    }
    return window.wails.Call(method, ...args) as Promise<T>;
  }

  // ===== Provider API =====

  async getProviders(): Promise<Provider[]> {
    return this.call<Provider[]>('AdminService.GetProviders');
  }

  async getProvider(id: number): Promise<Provider> {
    return this.call<Provider>('AdminService.GetProvider', id);
  }

  async createProvider(data: CreateProviderData): Promise<Provider> {
    return this.call<Provider>('AdminService.CreateProvider', data);
  }

  async updateProvider(id: number, data: Partial<Provider>): Promise<Provider> {
    return this.call<Provider>('AdminService.UpdateProvider', id, data);
  }

  async deleteProvider(id: number): Promise<void> {
    await this.call<void>('AdminService.DeleteProvider', id);
  }

  // ===== Project API =====

  async getProjects(): Promise<Project[]> {
    return this.call<Project[]>('AdminService.GetProjects');
  }

  async getProject(id: number): Promise<Project> {
    return this.call<Project>('AdminService.GetProject', id);
  }

  async createProject(data: CreateProjectData): Promise<Project> {
    return this.call<Project>('AdminService.CreateProject', data);
  }

  async updateProject(id: number, data: Partial<Project>): Promise<Project> {
    return this.call<Project>('AdminService.UpdateProject', id, data);
  }

  async deleteProject(id: number): Promise<void> {
    await this.call<void>('AdminService.DeleteProject', id);
  }

  // ===== Route API =====

  async getRoutes(): Promise<Route[]> {
    return this.call<Route[]>('AdminService.GetRoutes');
  }

  async getRoute(id: number): Promise<Route> {
    return this.call<Route>('AdminService.GetRoute', id);
  }

  async createRoute(data: CreateRouteData): Promise<Route> {
    return this.call<Route>('AdminService.CreateRoute', data);
  }

  async updateRoute(id: number, data: Partial<Route>): Promise<Route> {
    return this.call<Route>('AdminService.UpdateRoute', id, data);
  }

  async deleteRoute(id: number): Promise<void> {
    await this.call<void>('AdminService.DeleteRoute', id);
  }

  // ===== Session API =====

  async getSessions(): Promise<Session[]> {
    return this.call<Session[]>('AdminService.GetSessions');
  }

  // ===== RetryConfig API =====

  async getRetryConfigs(): Promise<RetryConfig[]> {
    return this.call<RetryConfig[]>('AdminService.GetRetryConfigs');
  }

  async getRetryConfig(id: number): Promise<RetryConfig> {
    return this.call<RetryConfig>('AdminService.GetRetryConfig', id);
  }

  async createRetryConfig(data: CreateRetryConfigData): Promise<RetryConfig> {
    return this.call<RetryConfig>('AdminService.CreateRetryConfig', data);
  }

  async updateRetryConfig(id: number, data: Partial<RetryConfig>): Promise<RetryConfig> {
    return this.call<RetryConfig>('AdminService.UpdateRetryConfig', id, data);
  }

  async deleteRetryConfig(id: number): Promise<void> {
    await this.call<void>('AdminService.DeleteRetryConfig', id);
  }

  // ===== RoutingStrategy API =====

  async getRoutingStrategies(): Promise<RoutingStrategy[]> {
    return this.call<RoutingStrategy[]>('AdminService.GetRoutingStrategies');
  }

  async getRoutingStrategy(id: number): Promise<RoutingStrategy> {
    return this.call<RoutingStrategy>('AdminService.GetRoutingStrategy', id);
  }

  async createRoutingStrategy(data: CreateRoutingStrategyData): Promise<RoutingStrategy> {
    return this.call<RoutingStrategy>('AdminService.CreateRoutingStrategy', data);
  }

  async updateRoutingStrategy(id: number, data: Partial<RoutingStrategy>): Promise<RoutingStrategy> {
    return this.call<RoutingStrategy>('AdminService.UpdateRoutingStrategy', id, data);
  }

  async deleteRoutingStrategy(id: number): Promise<void> {
    await this.call<void>('AdminService.DeleteRoutingStrategy', id);
  }

  // ===== ProxyRequest API =====

  async getProxyRequests(params?: PaginationParams): Promise<ProxyRequest[]> {
    return this.call<ProxyRequest[]>(
      'AdminService.GetProxyRequests',
      params?.limit ?? 100,
      params?.offset ?? 0
    );
  }

  async getProxyRequest(id: number): Promise<ProxyRequest> {
    return this.call<ProxyRequest>('AdminService.GetProxyRequest', id);
  }

  async getProxyUpstreamAttempts(proxyRequestId: number): Promise<ProxyUpstreamAttempt[]> {
    return this.call<ProxyUpstreamAttempt[]>('AdminService.GetProxyUpstreamAttempts', proxyRequestId);
  }

  // ===== Proxy Status API =====

  async getProxyStatus(): Promise<ProxyStatus> {
    // Wails 模式下，调用 Go 方法获取代理状态
    return this.call<ProxyStatus>('AdminService.GetProxyStatus');
  }

  // ===== Provider Stats API =====

  async getProviderStats(clientType?: string): Promise<Record<number, ProviderStats>> {
    return this.call<Record<number, ProviderStats>>('AdminService.GetProviderStats', clientType ?? '');
  }

  // ===== Settings API =====

  async getSettings(): Promise<Record<string, string>> {
    return this.call<Record<string, string>>('AdminService.GetSettings');
  }

  async getSetting(key: string): Promise<{ key: string; value: string }> {
    return this.call<{ key: string; value: string }>('AdminService.GetSetting', key);
  }

  async updateSetting(key: string, value: string): Promise<{ key: string; value: string }> {
    return this.call<{ key: string; value: string }>('AdminService.UpdateSetting', key, value);
  }

  async deleteSetting(key: string): Promise<void> {
    await this.call<void>('AdminService.DeleteSetting', key);
  }

  // ===== Logs API =====

  async getLogs(limit = 100): Promise<{ lines: string[]; count: number }> {
    return this.call<{ lines: string[]; count: number }>('AdminService.GetLogs', limit);
  }

  // ===== Wails Events 订阅 =====

  subscribe<T = unknown>(eventType: WSMessageType, callback: EventCallback<T>): UnsubscribeFn {
    // 保存回调
    if (!this.eventCallbacks.has(eventType)) {
      this.eventCallbacks.set(eventType, new Set());
    }
    this.eventCallbacks.get(eventType)!.add(callback as EventCallback);

    // 如果这是该事件类型的第一个订阅者，设置 Wails 事件监听
    if (!this.eventUnsubscribers.has(eventType)) {
      this.setupWailsEventListener(eventType);
    }

    return () => {
      this.eventCallbacks.get(eventType)?.delete(callback as EventCallback);

      // 如果没有更多订阅者，取消 Wails 事件监听
      if (this.eventCallbacks.get(eventType)?.size === 0) {
        this.eventUnsubscribers.get(eventType)?.();
        this.eventUnsubscribers.delete(eventType);
      }
    };
  }

  private setupWailsEventListener(eventType: WSMessageType): void {
    // 动态导入 Wails runtime 并设置监听
    import('@wailsio/runtime')
      .then((runtime) => {
        const unsubscribe = runtime.Events.On(eventType, ((data: unknown) => {
          const callbacks = this.eventCallbacks.get(eventType);
          callbacks?.forEach((callback) => callback(data));
        }) as WailsEventCallback);

        this.eventUnsubscribers.set(eventType, unsubscribe);
      })
      .catch((e) => {
        console.warn('Failed to setup Wails event listener:', e);
      });
  }

  // ===== 生命周期 =====

  async connect(): Promise<void> {
    this.connected = true;
  }

  disconnect(): void {
    // 清理所有事件监听
    this.eventUnsubscribers.forEach((unsubscribe) => unsubscribe());
    this.eventUnsubscribers.clear();
    this.eventCallbacks.clear();
    this.connected = false;
  }

  isConnected(): boolean {
    return this.connected && !!window.wails;
  }
}
