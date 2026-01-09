/**
 * Streaming Requests Hook
 * 追踪实时活动请求状态
 */

import { useState, useEffect, useCallback } from 'react';
import { getTransport, type ProxyRequest, type ClientType } from '@/lib/transport';

const transport = getTransport();

export interface StreamingState {
  /** 当前活动请求总数 */
  total: number;
  /** 活动请求列表 */
  requests: ProxyRequest[];
  /** 按 clientType 统计的活动请求数 */
  countsByClient: Map<ClientType, number>;
  /** 按 providerID 统计的活动请求数 */
  countsByProvider: Map<number, number>;
}

/**
 * 追踪实时活动的 streaming 请求
 * 通过 WebSocket 事件更新状态
 * 注意：React Query 缓存更新由 useProxyRequestUpdates 处理
 */
export function useStreamingRequests(): StreamingState {
  const [activeRequests, setActiveRequests] = useState<Map<string, ProxyRequest>>(new Map());

  // 处理请求更新
  const handleRequestUpdate = useCallback((request: ProxyRequest) => {
    setActiveRequests((prev) => {
      const next = new Map(prev);

      // 已完成、失败或取消的请求从活动列表中移除
      if (request.status === 'COMPLETED' || request.status === 'FAILED' || request.status === 'CANCELLED') {
        next.delete(request.requestID);
      } else {
        // PENDING 或 IN_PROGRESS 的请求添加到活动列表
        next.set(request.requestID, request);
      }

      return next;
    });
    // 注意：不要在这里调用 invalidateQueries，会导致重复请求
    // React Query 缓存更新由 useProxyRequestUpdates 处理
  }, []);

  useEffect(() => {
    // 连接 WebSocket
    transport.connect().catch(console.error);

    // 订阅请求更新事件
    const unsubscribe = transport.subscribe<ProxyRequest>(
      'proxy_request_update',
      handleRequestUpdate
    );

    return () => {
      unsubscribe();
    };
  }, [handleRequestUpdate]);

  // 计算按 clientType 和 providerID 的统计
  const countsByClient = new Map<ClientType, number>();
  const countsByProvider = new Map<number, number>();

  for (const request of activeRequests.values()) {
    // 按 clientType 统计
    const clientCount = countsByClient.get(request.clientType) || 0;
    countsByClient.set(request.clientType, clientCount + 1);

    // 按 providerID 统计
    if (request.providerID > 0) {
      const providerCount = countsByProvider.get(request.providerID) || 0;
      countsByProvider.set(request.providerID, providerCount + 1);
    }
  }

  return {
    total: activeRequests.size,
    requests: Array.from(activeRequests.values()),
    countsByClient,
    countsByProvider,
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
