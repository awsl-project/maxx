/**
 * Streaming Requests Hook
 * 追踪实时活动请求状态
 */

import { useState, useEffect, useCallback, useRef } from 'react';
import { getTransport, type ProxyRequest, type ClientType } from '@/lib/transport';

export interface StreamingState {
  /** 当前活动请求总数 */
  total: number;
  /** 活动请求列表 */
  requests: ProxyRequest[];
  /** 按 clientType 统计的活动请求数 */
  countsByClient: Map<ClientType, number>;
  /** 按 providerID 统计的活动请求数 */
  countsByProvider: Map<number, number>;
  /** 按 providerID + clientType 组合统计的活动请求数 (key: `${providerID}:${clientType}`) */
  countsByProviderAndClient: Map<string, number>;
  /** 按 routeID 统计的活动请求数 */
  countsByRoute: Map<number, number>;
}

/**
 * 判断请求是否为活跃状态
 */
function isActiveRequest(request: ProxyRequest): boolean {
  return request.status === 'PENDING' || request.status === 'IN_PROGRESS';
}

/**
 * 追踪实时活动的 streaming 请求
 * 通过 WebSocket 事件更新状态
 * 注意：React Query 缓存更新由 useProxyRequestUpdates 处理
 */
export function useStreamingRequests(): StreamingState {
  const [activeRequests, setActiveRequests] = useState<Map<string, ProxyRequest>>(new Map());
  const isInitialized = useRef(false);

  // 从 API 加载当前活跃请求
  const loadActiveRequests = useCallback(async () => {
    try {
      const transport = getTransport();
      const activeList = await transport.getActiveProxyRequests();
      const activeMap = new Map<string, ProxyRequest>();

      for (const request of activeList) {
        activeMap.set(request.requestID, request);
      }

      setActiveRequests(activeMap);
    } catch (error) {
      console.error('Failed to load active requests:', error);
    }
  }, []);

  // 处理请求更新
  const handleRequestUpdate = useCallback((request: ProxyRequest) => {
    setActiveRequests((prev) => {
      const next = new Map(prev);

      if (isActiveRequest(request)) {
        // PENDING 或 IN_PROGRESS 的请求添加到活动列表
        next.set(request.requestID, request);
      } else {
        // 已完成、失败、取消或拒绝的请求从活动列表中移除
        next.delete(request.requestID);
      }

      return next;
    });
    // 注意：不要在这里调用 invalidateQueries，会导致重复请求
    // React Query 缓存更新由 useProxyRequestUpdates 处理
  }, []);

  useEffect(() => {
    const transport = getTransport();

    // 初始化时加载当前活跃请求
    if (!isInitialized.current) {
      isInitialized.current = true;
      loadActiveRequests();
    }

    // 订阅请求更新事件 (连接由 main.tsx 统一管理)
    const unsubscribe = transport.subscribe<ProxyRequest>(
      'proxy_request_update',
      handleRequestUpdate,
    );

    // 订阅 WebSocket 重连事件，重新加载活跃请求
    // 因为断开期间可能有请求完成或新增
    const unsubscribeReconnect = transport.subscribe('_ws_reconnected', () => {
      loadActiveRequests();
    });

    return () => {
      unsubscribe();
      unsubscribeReconnect();
    };
  }, [handleRequestUpdate, loadActiveRequests]);

  // 计算按 clientType 和 providerID 的统计
  const countsByClient = new Map<ClientType, number>();
  const countsByProvider = new Map<number, number>();
  const countsByProviderAndClient = new Map<string, number>();
  const countsByRoute = new Map<number, number>();

  for (const request of activeRequests.values()) {
    // 按 clientType 统计
    const clientCount = countsByClient.get(request.clientType) || 0;
    countsByClient.set(request.clientType, clientCount + 1);

    // 按 routeID 统计
    if (request.routeID > 0) {
      const routeCount = countsByRoute.get(request.routeID) || 0;
      countsByRoute.set(request.routeID, routeCount + 1);
    }

    // 按 providerID 统计
    if (request.providerID > 0) {
      const providerCount = countsByProvider.get(request.providerID) || 0;
      countsByProvider.set(request.providerID, providerCount + 1);

      // 按 providerID + clientType 组合统计
      const key = `${request.providerID}:${request.clientType}`;
      const combinedCount = countsByProviderAndClient.get(key) || 0;
      countsByProviderAndClient.set(key, combinedCount + 1);
    }
  }

  return {
    total: activeRequests.size,
    requests: Array.from(activeRequests.values()),
    countsByClient,
    countsByProvider,
    countsByProviderAndClient,
    countsByRoute,
  };
}

/**
 * 获取特定客户端的 streaming 请求数
 */
export function useClientStreamingCount(clientType: ClientType): number {
  const { countsByClient } = useStreamingRequests();
  return countsByClient.get(clientType) || 0;
}

/**
 * 获取特定 Provider 的 streaming 请求数
 */
export function useProviderStreamingCount(providerId: number): number {
  const { countsByProvider } = useStreamingRequests();
  return countsByProvider.get(providerId) || 0;
}

/**
 * 获取特定 Provider 在特定 ClientType 下的 streaming 请求数
 */
export function useProviderClientStreamingCount(
  providerId: number,
  clientType: ClientType,
): number {
  const { countsByProviderAndClient } = useStreamingRequests();
  return countsByProviderAndClient.get(`${providerId}:${clientType}`) || 0;
}
