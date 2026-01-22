/**
 * ProxyRequest React Query Hooks
 */

import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import {
  getTransport,
  type ProxyRequest,
  type ProxyUpstreamAttempt,
  type CursorPaginationParams,
  type CursorPaginationResult,
} from '@/lib/transport';

// Query Keys
export const requestKeys = {
  all: ['requests'] as const,
  lists: () => [...requestKeys.all, 'list'] as const,
  list: (params?: CursorPaginationParams) => [...requestKeys.lists(), params] as const,
  details: () => [...requestKeys.all, 'detail'] as const,
  detail: (id: number) => [...requestKeys.details(), id] as const,
  attempts: (id: number) => [...requestKeys.detail(id), 'attempts'] as const,
};

// 获取 ProxyRequests (游标分页)
export function useProxyRequests(params?: CursorPaginationParams) {
  return useQuery({
    queryKey: requestKeys.list(params),
    queryFn: () => getTransport().getProxyRequests(params),
  });
}

// 获取 ProxyRequests 总数
export function useProxyRequestsCount(providerId?: number, status?: string) {
  return useQuery({
    queryKey: ['requestsCount', providerId, status] as const,
    queryFn: () => getTransport().getProxyRequestsCount(providerId, status),
  });
}

// 获取单个 ProxyRequest
export function useProxyRequest(id: number) {
  return useQuery({
    queryKey: requestKeys.detail(id),
    queryFn: () => getTransport().getProxyRequest(id),
    enabled: id > 0,
  });
}

// 获取 ProxyRequest 的 Attempts
export function useProxyUpstreamAttempts(proxyRequestId: number) {
  return useQuery({
    queryKey: requestKeys.attempts(proxyRequestId),
    queryFn: () => getTransport().getProxyUpstreamAttempts(proxyRequestId),
    enabled: proxyRequestId > 0,
  });
}

// 订阅 ProxyRequest 实时更新
export function useProxyRequestUpdates() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const transport = getTransport();

    // 订阅 ProxyRequest 更新事件 (连接由 main.tsx 统一管理)
    const unsubscribeRequest = transport.subscribe<ProxyRequest>(
      'proxy_request_update',
      (updatedRequest) => {
        // 检查是否是新请求（通过详情缓存判断）
        const existingDetail = queryClient.getQueryData(requestKeys.detail(updatedRequest.id));
        const isNewRequest = !existingDetail;

        // 更新单个请求的缓存
        queryClient.setQueryData(requestKeys.detail(updatedRequest.id), updatedRequest);

        // 更新列表缓存（乐观更新）- 适配 CursorPaginationResult 结构
        // 使用 queryCache 遍历所有匹配的查询，以获取每个查询的过滤参数
        const queryCache = queryClient.getQueryCache();
        const listQueries = queryCache.findAll({ queryKey: requestKeys.lists() });

        for (const query of listQueries) {
          const queryKey = query.queryKey as ReturnType<typeof requestKeys.list>;
          // 从 queryKey 中提取过滤参数: ['requests', 'list', params]
          const params = queryKey[2] as CursorPaginationParams | undefined;
          const filterProviderId = params?.providerId;
          const filterStatus = params?.status;

          // 检查是否匹配过滤条件的辅助函数
          const matchesFilter = (request: ProxyRequest) => {
            if (filterProviderId !== undefined && request.providerID !== filterProviderId) {
              return false;
            }
            if (filterStatus !== undefined && request.status !== filterStatus) {
              return false;
            }
            return true;
          };

          queryClient.setQueryData<CursorPaginationResult<ProxyRequest>>(queryKey, (old) => {
            if (!old || !old.items) return old;

            const index = old.items.findIndex((r) => r.id === updatedRequest.id);
            if (index >= 0) {
              // 已存在的请求：检查是否仍然匹配过滤条件
              if (!matchesFilter(updatedRequest)) {
                // 不再匹配过滤条件，从列表中移除
                const newItems = old.items.filter((r) => r.id !== updatedRequest.id);
                return { ...old, items: newItems };
              }
              // 仍然匹配，更新
              const newItems = [...old.items];
              newItems[index] = updatedRequest;
              return { ...old, items: newItems };
            }

            // 新请求：检查是否匹配过滤条件
            if (!matchesFilter(updatedRequest)) {
              // 不匹配过滤条件，不添加
              return old;
            }

            // 新请求添加到列表开头（只在首页，即没有 before 参数的查询）
            if (params?.before) {
              // 不是首页，不添加新请求
              return old;
            }

            return {
              ...old,
              items: [updatedRequest, ...old.items],
              firstId: updatedRequest.id,
            };
          });
        }

        // 新请求时乐观更新 count（需要考虑每个 count 查询的过滤条件）
        if (isNewRequest) {
          // 遍历所有 requestsCount 缓存
          const countQueries = queryCache.findAll({ queryKey: ['requestsCount'] });
          for (const query of countQueries) {
            // queryKey: ['requestsCount', providerId, status]
            const filterProviderId = query.queryKey[1] as number | undefined;
            const filterStatus = query.queryKey[2] as string | undefined;
            // 如果有过滤条件且不匹配，不更新计数
            if (filterProviderId !== undefined && updatedRequest.providerID !== filterProviderId) {
              continue;
            }
            if (filterStatus !== undefined && updatedRequest.status !== filterStatus) {
              continue;
            }
            queryClient.setQueryData<number>(query.queryKey, (old) => (old ?? 0) + 1);
          }
        }

        // 请求完成或失败时刷新相关数据
        if (updatedRequest.status === 'COMPLETED' || updatedRequest.status === 'FAILED') {
          // 刷新 dashboard 数据
          queryClient.invalidateQueries({ queryKey: ['dashboard'] });
          // 刷新 provider stats（因为统计数据变化了）
          queryClient.invalidateQueries({ queryKey: ['providers', 'stats'] });
          // 刷新 cooldowns（请求可能触发了冷却，即使最终成功也可能有 provider 进入冷却）
          queryClient.invalidateQueries({ queryKey: ['cooldowns'] });
        }
      },
    );

    // 订阅 ProxyUpstreamAttempt 更新事件
    const unsubscribeAttempt = transport.subscribe<ProxyUpstreamAttempt>(
      'proxy_upstream_attempt_update',
      (updatedAttempt) => {
        // 更新 Attempts 缓存
        queryClient.setQueryData<ProxyUpstreamAttempt[]>(
          requestKeys.attempts(updatedAttempt.proxyRequestID),
          (old) => {
            if (!old) return [updatedAttempt];
            const index = old.findIndex((a) => a.id === updatedAttempt.id);
            if (index >= 0) {
              const newList = [...old];
              newList[index] = updatedAttempt;
              return newList;
            }
            return [...old, updatedAttempt];
          },
        );
      },
    );

    return () => {
      unsubscribeRequest();
      unsubscribeAttempt();
    };
  }, [queryClient]);
}
