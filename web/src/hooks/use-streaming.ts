/**
 * Streaming Requests Hook
 * 追踪实时活动请求状态
 */

import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
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

export interface StreamingOptions {
  /** 事件更新节流间隔（毫秒），0 表示不节流 */
  throttleMs?: number;
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
export function useStreamingRequests(options: StreamingOptions = {}): StreamingState {
  const [activeRequests, setActiveRequests] = useState<Map<string, ProxyRequest>>(new Map());
  const activeRequestsRef = useRef<Map<string, ProxyRequest>>(new Map());
  const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isInitialized = useRef(false);
  const throttleMs = options.throttleMs ?? 0;

  const scheduleFlush = useCallback(() => {
    if (throttleMs <= 0) {
      return;
    }
    if (flushTimerRef.current) {
      return;
    }
    flushTimerRef.current = setTimeout(() => {
      flushTimerRef.current = null;
      setActiveRequests(new Map(activeRequestsRef.current));
    }, throttleMs);
  }, [throttleMs]);

  const applyState = useCallback(
    (next: Map<string, ProxyRequest>) => {
      activeRequestsRef.current = next;
      if (throttleMs <= 0) {
        setActiveRequests(next);
        return;
      }
      scheduleFlush();
    },
    [scheduleFlush, throttleMs],
  );

  // 从 API 加载当前活跃请求
  const loadActiveRequests = useCallback(async () => {
    try {
      const transport = getTransport();
      const activeList = await transport.getActiveProxyRequests();
      const activeMap = new Map<string, ProxyRequest>();

      // Ensure activeList is an array before iterating
      if (Array.isArray(activeList)) {
        for (const request of activeList) {
          activeMap.set(request.requestID, request);
        }
      } else {
        console.warn('getActiveProxyRequests returned non-array:', activeList);
      }

      applyState(activeMap);
    } catch (error) {
      console.error('Failed to load active requests:', error);
    }
  }, [applyState]);

  // 处理请求更新
  const handleRequestUpdate = useCallback((request: ProxyRequest) => {
    const next = new Map(activeRequestsRef.current);

    if (isActiveRequest(request)) {
      // PENDING 或 IN_PROGRESS 的请求添加到活动列表
      next.set(request.requestID, request);
    } else {
      // 已完成、失败、取消或拒绝的请求从活动列表中移除
      next.delete(request.requestID);
    }

    applyState(next);
    // 注意：不要在这里调用 invalidateQueries，会导致重复请求
    // React Query 缓存更新由 useProxyRequestUpdates 处理
  }, [applyState]);

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
      if (flushTimerRef.current) {
        clearTimeout(flushTimerRef.current);
        flushTimerRef.current = null;
      }
    };
  }, [handleRequestUpdate, loadActiveRequests]);

  useEffect(() => {
    if (flushTimerRef.current) {
      clearTimeout(flushTimerRef.current);
      flushTimerRef.current = null;
    }
    // throttleMs 变更时立即刷出缓冲状态，避免节流切换时丢失更新。
    setActiveRequests(new Map(activeRequestsRef.current));
  }, [throttleMs]);

  return useMemo((): StreamingState => {
    // 计算按 clientType 和 providerID 的统计
    const countsByClient = new Map<ClientType, number>();
    const countsByProvider = new Map<number, number>();
    const countsByProviderAndClient = new Map<string, number>();
    const countsByRoute = new Map<number, number>();
    const requests = Array.from(activeRequests.values());

    for (const request of requests) {
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
      requests,
      countsByClient,
      countsByProvider,
      countsByProviderAndClient,
      countsByRoute,
    };
  }, [activeRequests]);
}

/**
 * 获取特定客户端的 streaming 请求数
 */
export function useClientStreamingCount(
  clientType: ClientType,
  options?: StreamingOptions,
): number {
  const { countsByClient } = useStreamingRequests(options);
  return countsByClient.get(clientType) || 0;
}

/**
 * 获取特定 Provider 的 streaming 请求数
 */
export function useProviderStreamingCount(
  providerId: number,
  options?: StreamingOptions,
): number {
  const { countsByProvider } = useStreamingRequests(options);
  return countsByProvider.get(providerId) || 0;
}

/**
 * 获取特定 Provider 在特定 ClientType 下的 streaming 请求数
 */
export function useProviderClientStreamingCount(
  providerId: number,
  clientType: ClientType,
  options?: StreamingOptions,
): number {
  const { countsByProviderAndClient } = useStreamingRequests(options);
  return countsByProviderAndClient.get(`${providerId}:${clientType}`) || 0;
}
