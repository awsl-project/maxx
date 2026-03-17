/**
 * ProxyRequest React Query Hooks
 */

import { useQuery, useQueryClient, useInfiniteQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import {
  getTransport,
  type ProxyRequest,
  type ProxyUpstreamAttempt,
  type CursorPaginationParams,
  type CursorPaginationResult,
} from '@/lib/transport';
import { prioritizeActiveRequests } from '@/lib/request-order';

// Query Keys
export const requestKeys = {
  all: ['requests'] as const,
  lists: () => [...requestKeys.all, 'list'] as const,
  list: (params?: CursorPaginationParams) => [...requestKeys.lists(), params] as const,
  infinite: (providerId?: number, status?: string, apiTokenId?: number, projectId?: number) =>
    [...requestKeys.all, 'infinite', providerId, status, apiTokenId, projectId] as const,
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

// 获取 ProxyRequests (无限滚动)
export function useInfiniteProxyRequests(
  providerId?: number,
  status?: string,
  apiTokenId?: number,
  projectId?: number,
  enabled = true,
) {
  return useInfiniteQuery({
    queryKey: requestKeys.infinite(providerId, status, apiTokenId, projectId),
    queryFn: ({ pageParam }) =>
      getTransport().getProxyRequests({
        limit: 100,
        before: pageParam,
        providerId,
        status,
        apiTokenId,
        projectId,
      }),
    getNextPageParam: (lastPage) => (lastPage.hasMore ? lastPage.lastId : undefined),
    initialPageParam: undefined as number | undefined,
    enabled,
  });
}

// 获取 ProxyRequests 总数
export function useProxyRequestsCount(
  providerId?: number,
  status?: string,
  apiTokenId?: number,
  projectId?: number,
  enabled = true,
) {
  return useQuery({
    queryKey: ['requestsCount', providerId, status, apiTokenId, projectId] as const,
    queryFn: () => getTransport().getProxyRequestsCount(providerId, status, apiTokenId, projectId),
    enabled,
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

/**
 * Subscribes to live request events and reconciles React Query caches after
 * request updates or WebSocket reconnects.
 */
export function useProxyRequestUpdates() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const transport = getTransport();
    const queryCache = queryClient.getQueryCache();

    const flushIntervalMs = 250;
    const pendingRequests = new Map<number, ProxyRequest>();
    const pendingAttemptsByRequest = new Map<number, Map<number, ProxyUpstreamAttempt>>();
    const knownRequestIds = new Set<number>();
    let flushTimer: ReturnType<typeof setTimeout> | null = null;

    const flushAttempts = () => {
      if (pendingAttemptsByRequest.size === 0) {
        return;
      }

      const entries = Array.from(pendingAttemptsByRequest.entries());
      pendingAttemptsByRequest.clear();

      for (const [proxyRequestID, attemptsById] of entries) {
        const attemptsKey = requestKeys.attempts(proxyRequestID);
        const attemptsQuery = queryCache.find({ queryKey: attemptsKey, exact: true });
        if (!attemptsQuery || attemptsQuery.getObserversCount() === 0) {
          continue;
        }

        const updates = Array.from(attemptsById.values());

        queryClient.setQueryData<ProxyUpstreamAttempt[]>(attemptsKey, (old) => {
          const list = old ? [...old] : [];

          for (const updatedAttempt of updates) {
            const index = list.findIndex((a) => a.id === updatedAttempt.id);
            if (index >= 0) {
              const prev = list[index];
              list[index] = {
                ...prev,
                ...updatedAttempt,
                requestInfo: updatedAttempt.requestInfo ?? prev.requestInfo,
                responseInfo: updatedAttempt.responseInfo ?? prev.responseInfo,
              };
              continue;
            }
            list.push(updatedAttempt);
          }

          return list;
        });
      }
    };

    const flush = () => {
      if (pendingRequests.size === 0 && pendingAttemptsByRequest.size === 0) {
        return;
      }

      if (pendingRequests.size === 0) {
        flushAttempts();
        return;
      }

      const updates = Array.from(pendingRequests.values());
      pendingRequests.clear();

      const listQueries = queryCache
        .findAll({ queryKey: requestKeys.lists() })
        .filter((q) => q.getObserversCount() > 0);
      const infiniteQueries = queryCache
        .findAll({ queryKey: [...requestKeys.all, 'infinite'] })
        .filter((q) => q.getObserversCount() > 0);
      const countQueries = queryCache
        .findAll({ queryKey: ['requestsCount'] })
        .filter((q) => q.getObserversCount() > 0);

      let invalidateDashboard = false;
      let invalidateProviderStats = false;
      let invalidateCooldowns = false;

      for (const updatedRequest of updates) {
        const requestId = updatedRequest.id;
        let isKnown = knownRequestIds.has(requestId);

        // 仅当详情查询正在被观察时才更新详情缓存，避免列表页“写缓存造内存”
        const detailKey = requestKeys.detail(requestId);
        const detailQuery = queryCache.find({ queryKey: detailKey, exact: true });
        if (detailQuery && detailQuery.getObserversCount() > 0) {
          // 后端可能会对 WS 广播做“瘦身”（不带 requestInfo/responseInfo 大字段），
          // 这里合并旧值，避免把详情页已加载的内容覆盖成空。
          queryClient.setQueryData<ProxyRequest>(detailKey, (old) => {
            if (!old) {
              return updatedRequest;
            }
            return {
              ...old,
              ...updatedRequest,
              requestInfo: updatedRequest.requestInfo ?? old.requestInfo,
              responseInfo: updatedRequest.responseInfo ?? old.responseInfo,
            };
          });
          isKnown = true;
        }

        // 更新 Cursor 列表查询（仅更新正在被观察的 query）
        for (const query of listQueries) {
          const queryKey = query.queryKey as ReturnType<typeof requestKeys.list>;
          const params = queryKey[2] as CursorPaginationParams | undefined;
          const filterProviderId = params?.providerId;
          const filterStatus = params?.status;
          const filterAPITokenId = params?.apiTokenId;

          const matchesFilter = (request: ProxyRequest) => {
            if (filterProviderId !== undefined && request.providerID !== filterProviderId) {
              return false;
            }
            if (filterStatus !== undefined && request.status !== filterStatus) {
              return false;
            }
            if (filterAPITokenId !== undefined && request.apiTokenID !== filterAPITokenId) {
              return false;
            }
            return true;
          };

          queryClient.setQueryData<CursorPaginationResult<ProxyRequest>>(queryKey, (old) => {
            if (!old || !old.items) return old;
            const limit = typeof params?.limit === 'number' ? params.limit : undefined;

            const normalizePage = (items: ProxyRequest[]) => {
              // 首屏列表和 WS 增量更新都统一走这里，避免同一页里出现
              // “已完成/失败”仍停留在活跃请求前面的短暂错序。
              let nextItems = prioritizeActiveRequests(items);
              let hasMore = old.hasMore;

              if (typeof limit === 'number' && limit > 0 && nextItems.length > limit) {
                nextItems = nextItems.slice(0, limit);
                hasMore = true;
              }

              const firstId = nextItems[0]?.id;
              const lastId = nextItems[nextItems.length - 1]?.id;

              return {
                ...old,
                items: nextItems,
                hasMore,
                firstId,
                lastId,
              };
            };

            const index = old.items.findIndex((r) => r.id === requestId);
            if (index >= 0) {
              isKnown = true;
              if (!matchesFilter(updatedRequest)) {
                const newItems = old.items.filter((r) => r.id !== requestId);
                return normalizePage(newItems);
              }
              const newItems = [...old.items];
              newItems[index] = updatedRequest;
              return normalizePage(newItems);
            }

            if (!matchesFilter(updatedRequest)) {
              return old;
            }

            if (params?.before) {
              return old;
            }

            return normalizePage([updatedRequest, ...old.items]);
          });
        }

        // 更新 Infinite Queries（仅更新正在被观察的 query）
        for (const query of infiniteQueries) {
          const queryKey = query.queryKey as ReturnType<typeof requestKeys.infinite>;
          const filterProviderId = queryKey[2] as number | undefined;
          const filterStatus = queryKey[3] as string | undefined;
          const filterAPITokenId = queryKey[4] as number | undefined;
          const filterProjectId = queryKey[5] as number | undefined;

          const matchesFilter = (request: ProxyRequest) => {
            if (filterProviderId !== undefined && request.providerID !== filterProviderId) {
              return false;
            }
            if (filterStatus !== undefined && request.status !== filterStatus) {
              return false;
            }
            if (filterAPITokenId !== undefined && request.apiTokenID !== filterAPITokenId) {
              return false;
            }
            if (filterProjectId !== undefined && request.projectID !== filterProjectId) {
              return false;
            }
            return true;
          };

          queryClient.setQueryData<{
            pages: CursorPaginationResult<ProxyRequest>[];
            pageParams: (number | undefined)[];
          }>(queryKey, (old) => {
            if (!old || !old.pages || old.pages.length === 0) return old;

            let hasExisting = false;

            const updatedPages = old.pages.map((page) => {
              const index = page.items.findIndex((r) => r.id === requestId);
              if (index < 0) {
                return page;
              }

              hasExisting = true;

              if (!matchesFilter(updatedRequest)) {
                const newItems = page.items.filter((r) => r.id !== requestId);
                return { ...page, items: prioritizeActiveRequests(newItems) };
              }

              const newItems = [...page.items];
              newItems[index] = updatedRequest;
              return { ...page, items: prioritizeActiveRequests(newItems) };
            });

            if (hasExisting) {
              isKnown = true;
              return { ...old, pages: updatedPages };
            }

            if (!matchesFilter(updatedRequest)) {
              return { ...old, pages: updatedPages };
            }

            // 仅在第一页插入“新请求”，避免重复插入导致列表膨胀
            const firstPage = updatedPages[0];
            if (!firstPage) {
              return { ...old, pages: updatedPages };
            }

            return {
              ...old,
              pages: [
                {
                  ...firstPage,
                  items: prioritizeActiveRequests([updatedRequest, ...firstPage.items]),
                },
                ...updatedPages.slice(1),
              ],
            };
          });
        }

        // 新请求时乐观更新 count（增加保护：避免因“未观察详情缓存”导致重复 +1）
        if (!isKnown) {
          const startTimeMs = new Date(updatedRequest.startTime).getTime();
          const looksLikeNewRequest =
            updatedRequest.status === 'PENDING' &&
            Number.isFinite(startTimeMs) &&
            Date.now() - startTimeMs < 15_000;

          if (looksLikeNewRequest) {
            for (const query of countQueries) {
              const filterProviderId = query.queryKey[1] as number | undefined;
              const filterStatus = query.queryKey[2] as string | undefined;
              const filterAPITokenId = query.queryKey[3] as number | undefined;
              const filterProjectId = query.queryKey[4] as number | undefined;
              if (filterProviderId !== undefined && updatedRequest.providerID !== filterProviderId) {
                continue;
              }
              if (filterStatus !== undefined && updatedRequest.status !== filterStatus) {
                continue;
              }
              if (filterAPITokenId !== undefined && updatedRequest.apiTokenID !== filterAPITokenId) {
                continue;
              }
              if (filterProjectId !== undefined && updatedRequest.projectID !== filterProjectId) {
                continue;
              }
              queryClient.setQueryData<number>(query.queryKey, (old) => (old ?? 0) + 1);
            }
          }
        }

        knownRequestIds.add(requestId);

        if (updatedRequest.status === 'COMPLETED' || updatedRequest.status === 'FAILED') {
          invalidateDashboard = true;
          invalidateProviderStats = true;
          invalidateCooldowns = true;
        }
      }

      if (invalidateDashboard) {
        queryClient.invalidateQueries({ queryKey: ['dashboard'] });
      }
      if (invalidateProviderStats) {
        queryClient.invalidateQueries({ queryKey: ['providers', 'stats'] });
      }
      if (invalidateCooldowns) {
        queryClient.invalidateQueries({ queryKey: ['cooldowns'] });
      }

      flushAttempts();
    };

    const scheduleFlush = () => {
      if (flushTimer) {
        return;
      }
      flushTimer = setTimeout(() => {
        flushTimer = null;
        flush();
      }, flushIntervalMs);
    };

    const unsubscribeRequest = transport.subscribe<ProxyRequest>('proxy_request_update', (updatedRequest) => {
      pendingRequests.set(updatedRequest.id, updatedRequest);
      scheduleFlush();
    });

    // 订阅 ProxyUpstreamAttempt 更新事件
    const unsubscribeAttempt = transport.subscribe<ProxyUpstreamAttempt>(
      'proxy_upstream_attempt_update',
      (updatedAttempt) => {
        // 仅当 attempts 查询正在被观察时才更新，避免列表页“写缓存造内存”
        const attemptsKey = requestKeys.attempts(updatedAttempt.proxyRequestID);
        const attemptsQuery = queryCache.find({ queryKey: attemptsKey, exact: true });
        if (!attemptsQuery || attemptsQuery.getObserversCount() === 0) {
          return;
        }

        let perRequest = pendingAttemptsByRequest.get(updatedAttempt.proxyRequestID);
        if (!perRequest) {
          perRequest = new Map<number, ProxyUpstreamAttempt>();
          pendingAttemptsByRequest.set(updatedAttempt.proxyRequestID, perRequest);
        }
        perRequest.set(updatedAttempt.id, updatedAttempt);
        scheduleFlush();
      },
    );

    const unsubscribeReconnect = transport.subscribe('_ws_reconnected', () => {
      if (flushTimer) {
        clearTimeout(flushTimer);
        flushTimer = null;
      }

      // 断线窗口里的 WS 事件已经不可信，重连后统一走 Query 补偿同步。
      pendingRequests.clear();
      pendingAttemptsByRequest.clear();
      knownRequestIds.clear();

      void queryClient.refetchQueries({ queryKey: requestKeys.lists(), type: 'active' });
      void queryClient.refetchQueries({ queryKey: [...requestKeys.all, 'infinite'], type: 'active' });
      void queryClient.refetchQueries({ queryKey: requestKeys.details(), type: 'active' });
      void queryClient.refetchQueries({ queryKey: ['requestsCount'], type: 'active' });
      void queryClient.refetchQueries({ queryKey: ['dashboard'], type: 'active' });
      void queryClient.refetchQueries({ queryKey: ['providers', 'stats'], type: 'active' });
      void queryClient.refetchQueries({ queryKey: ['cooldowns'], type: 'active' });
    });

    return () => {
      if (flushTimer) {
        clearTimeout(flushTimer);
        flushTimer = null;
      }
      pendingRequests.clear();
      pendingAttemptsByRequest.clear();
      unsubscribeRequest();
      unsubscribeAttempt();
      unsubscribeReconnect();
    };
  }, [queryClient]);
}
