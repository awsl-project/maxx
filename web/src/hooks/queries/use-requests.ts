/**
 * ProxyRequest React Query Hooks
 */

import { useQuery, useQueryClient, useInfiniteQuery, useMutation } from '@tanstack/react-query';
import { useEffect } from 'react';
import {
  getTransport,
  type ProxyRequest,
  type ProxyUpstreamAttempt,
  type ProxyRequestErrorMode,
  type CursorPaginationParams,
  type CursorPaginationResult,
} from '@/lib/transport';
import { prioritizeActiveRequests } from '@/lib/request-order';

const REQUESTS_INFINITE_PAGE_LIMIT = 100;

/** Query key factory for proxy request related queries. */
export const requestKeys = {
  all: ['requests'] as const,
  lists: () => [...requestKeys.all, 'list'] as const,
  list: (params?: CursorPaginationParams) => [...requestKeys.lists(), params] as const,
  infinite: (
    providerId?: number,
    status?: string,
    apiTokenId?: number,
    projectId?: number,
    startTime?: string,
    endTime?: string,
    errorMode?: ProxyRequestErrorMode,
  ) =>
    [
      ...requestKeys.all,
      'infinite',
      providerId,
      status,
      apiTokenId,
      projectId,
      startTime,
      endTime,
      errorMode,
    ] as const,
  errorStats: (params?: CursorPaginationParams) =>
    [...requestKeys.all, 'error-stats', params] as const,
  cleanupFailedCount: (params?: CursorPaginationParams) =>
    [...requestKeys.all, 'cleanup-failed-count', params] as const,
  cleanupFailedCounts: () => [...requestKeys.all, 'cleanup-failed-count'] as const,
  details: () => [...requestKeys.all, 'detail'] as const,
  detail: (id: number) => [...requestKeys.details(), id] as const,
  attempts: (id: number) => [...requestKeys.detail(id), 'attempts'] as const,
};

export function isProxyRequestError(request: ProxyRequest): boolean {
  return request.status === 'FAILED' || request.status === 'REJECTED' || request.statusCode >= 400;
}

export function shouldRefetchCleanupFailedCount(request: ProxyRequest): boolean {
  return (
    isProxyRequestError(request) || request.status === 'COMPLETED' || request.status === 'CANCELLED'
  );
}

function matchesRequestTimeRange(
  request: ProxyRequest,
  startTime?: string,
  endTime?: string,
): boolean {
  const createdAtMs = new Date(request.createdAt).getTime();
  if (!Number.isFinite(createdAtMs)) {
    return true;
  }
  if (startTime !== undefined && createdAtMs < new Date(startTime).getTime()) {
    return false;
  }
  if (endTime !== undefined && createdAtMs > new Date(endTime).getTime()) {
    return false;
  }
  return true;
}

export function mergeProxyRequestAttemptUpdate(
  request: ProxyRequest,
  attempt: ProxyUpstreamAttempt,
): ProxyRequest {
  if (request.id !== attempt.proxyRequestID) {
    return request;
  }

  return {
    ...request,
    updatedAt: attempt.updatedAt || request.updatedAt,
    routeID: attempt.routeID ?? request.routeID,
    providerID: attempt.providerID ?? request.providerID,
    mappedModel: attempt.mappedModel || request.mappedModel,
    responseModel: attempt.responseModel || request.responseModel,
    ttft: attempt.ttft ?? request.ttft,
    inputTokenCount: attempt.inputTokenCount ?? request.inputTokenCount,
    outputTokenCount: attempt.outputTokenCount ?? request.outputTokenCount,
    cacheReadCount: attempt.cacheReadCount ?? request.cacheReadCount,
    cacheWriteCount: attempt.cacheWriteCount ?? request.cacheWriteCount,
    cache5mWriteCount: attempt.cache5mWriteCount ?? request.cache5mWriteCount,
    cache1hWriteCount: attempt.cache1hWriteCount ?? request.cache1hWriteCount,
    modelPriceId: attempt.modelPriceId ?? request.modelPriceId,
    multiplier: attempt.multiplier ?? request.multiplier,
    cost: attempt.cost ?? request.cost,
  };
}

function matchesProxyRequestParams(
  request: ProxyRequest,
  params: CursorPaginationParams | undefined,
): boolean {
  if (params?.providerId !== undefined && request.providerID !== params.providerId) {
    return false;
  }
  if (params?.status !== undefined && request.status !== params.status) {
    return false;
  }
  if (params?.apiTokenId !== undefined && request.apiTokenID !== params.apiTokenId) {
    return false;
  }
  if (params?.projectId !== undefined && request.projectID !== params.projectId) {
    return false;
  }
  if (!matchesRequestTimeRange(request, params?.startTime, params?.endTime)) {
    return false;
  }
  if (params?.errorMode === 'only' && !isProxyRequestError(request)) {
    return false;
  }
  if (params?.errorMode === 'exclude' && isProxyRequestError(request)) {
    return false;
  }
  return true;
}

export type RequestsCountQueryKey = readonly [
  'requestsCount',
  number | undefined,
  string | undefined,
  number | undefined,
  number | undefined,
  string | undefined,
  string | undefined,
  ProxyRequestErrorMode | undefined,
];

export function matchesProxyRequestCountQuery(
  request: ProxyRequest,
  queryKey: RequestsCountQueryKey,
): boolean {
  return matchesProxyRequestParams(request, {
    providerId: queryKey[1],
    status: queryKey[2],
    apiTokenId: queryKey[3],
    projectId: queryKey[4],
    startTime: queryKey[5],
    endTime: queryKey[6],
    errorMode: queryKey[7],
  });
}

export function getProxyRequestCountDelta(
  previousRequest: ProxyRequest | undefined,
  updatedRequest: ProxyRequest,
  queryKey: RequestsCountQueryKey,
): number {
  const matchedBefore = previousRequest
    ? matchesProxyRequestCountQuery(previousRequest, queryKey)
    : false;
  const matchesAfter = matchesProxyRequestCountQuery(updatedRequest, queryKey);

  if (matchedBefore === matchesAfter) {
    return 0;
  }
  return matchesAfter ? 1 : -1;
}

export function normalizeProxyRequestPage<T extends ProxyRequest>(
  page: CursorPaginationResult<T>,
  items: T[],
  limit?: number,
): CursorPaginationResult<T> {
  let nextItems = prioritizeActiveRequests(items);
  let hasMore = page.hasMore;

  if (typeof limit === 'number' && limit > 0 && nextItems.length > limit) {
    nextItems = nextItems.slice(0, limit);
    hasMore = true;
  }

  return {
    ...page,
    items: nextItems,
    hasMore,
    firstId: nextItems[0]?.id,
    lastId: nextItems[nextItems.length - 1]?.id,
  };
}

export function normalizeProxyRequestPages<T extends ProxyRequest>(
  pages: CursorPaginationResult<T>[],
  items: T[],
  limit: number,
): CursorPaginationResult<T>[] {
  if (pages.length === 0) {
    return pages;
  }

  const orderedItems = prioritizeActiveRequests(items);
  let offset = 0;

  return pages.map((page, index) => {
    const pageLimit = limit > 0 ? limit : page.items.length;
    const nextItems = orderedItems.slice(offset, offset + pageLimit);
    offset += pageLimit;

    const droppedItems = index === pages.length - 1 && offset < orderedItems.length;
    return {
      ...page,
      items: nextItems,
      hasMore: page.hasMore || droppedItems,
      firstId: nextItems[0]?.id,
      lastId: nextItems[nextItems.length - 1]?.id,
    };
  });
}

/** Fetches proxy requests with cursor-based pagination. */
export function useProxyRequests(params?: CursorPaginationParams) {
  return useQuery({
    queryKey: requestKeys.list(params),
    queryFn: () => getTransport().getProxyRequests(params),
  });
}

/**
 * Fetches proxy requests using infinite scroll pagination.
 * Uses staleTime to avoid redundant refetches within a short window.
 */
export function useInfiniteProxyRequests(
  providerId?: number,
  status?: string,
  apiTokenId?: number,
  projectId?: number,
  startTime?: string,
  endTime?: string,
  errorMode?: ProxyRequestErrorMode,
  enabled = true,
) {
  return useInfiniteQuery({
    queryKey: requestKeys.infinite(
      providerId,
      status,
      apiTokenId,
      projectId,
      startTime,
      endTime,
      errorMode,
    ),
    queryFn: ({ pageParam }) =>
      getTransport().getProxyRequests({
        limit: REQUESTS_INFINITE_PAGE_LIMIT,
        before: pageParam,
        providerId,
        status,
        apiTokenId,
        projectId,
        startTime,
        endTime,
        errorMode: errorMode === 'all' ? undefined : errorMode,
      }),
    getNextPageParam: (lastPage) => (lastPage.hasMore ? lastPage.lastId : undefined),
    initialPageParam: undefined as number | undefined,
    enabled,
    staleTime: 5_000,
  });
}

/**
 * Fetches the total count of proxy requests matching the given filters.
 * Polls every 10s as a safety net for missed WebSocket events.
 */
export function useProxyRequestsCount(
  providerId?: number,
  status?: string,
  apiTokenId?: number,
  projectId?: number,
  startTime?: string,
  endTime?: string,
  errorMode?: ProxyRequestErrorMode,
  enabled = true,
) {
  return useQuery({
    queryKey: [
      'requestsCount',
      providerId,
      status,
      apiTokenId,
      projectId,
      startTime,
      endTime,
      errorMode,
    ] as const,
    queryFn: () =>
      getTransport().getProxyRequestsCount(
        providerId,
        status,
        apiTokenId,
        projectId,
        startTime,
        endTime,
        errorMode,
      ),
    enabled,
    staleTime: 5_000,
    refetchInterval: enabled ? 10_000 : false,
    refetchIntervalInBackground: false,
  });
}

export function useProxyRequestErrorStats(params?: CursorPaginationParams, enabled = true) {
  return useQuery({
    queryKey: requestKeys.errorStats(params),
    queryFn: () => getTransport().getProxyRequestErrorStats(params),
    enabled,
    staleTime: 5_000,
  });
}

export function useCleanupFailedProxyRequestsCount(
  params?: CursorPaginationParams,
  enabled = true,
) {
  return useQuery({
    queryKey: requestKeys.cleanupFailedCount(params),
    queryFn: () => getTransport().getCleanupFailedProxyRequestsCount(params),
    enabled,
    staleTime: 5_000,
  });
}

export function useCleanupFailedProxyRequests() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (params?: CursorPaginationParams) =>
      getTransport().cleanupFailedProxyRequests(params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: requestKeys.all });
      queryClient.invalidateQueries({ queryKey: ['requestsCount'] });
      queryClient.invalidateQueries({ queryKey: requestKeys.cleanupFailedCounts() });
      void queryClient.refetchQueries({
        queryKey: requestKeys.cleanupFailedCounts(),
        type: 'active',
      });
    },
  });
}

/** Fetches a single proxy request by ID. */
export function useProxyRequest(id: number) {
  return useQuery({
    queryKey: requestKeys.detail(id),
    queryFn: () => getTransport().getProxyRequest(id),
    enabled: id > 0,
  });
}

/** Fetches upstream attempts for a given proxy request. */
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

    const patchRequestCachesFromAttempt = (updatedAttempt: ProxyUpstreamAttempt) => {
      const requestId = updatedAttempt.proxyRequestID;

      const detailKey = requestKeys.detail(requestId);
      const detailQuery = queryCache.find({ queryKey: detailKey, exact: true });
      if (detailQuery && detailQuery.getObserversCount() > 0) {
        queryClient.setQueryData<ProxyRequest>(detailKey, (old) =>
          old ? mergeProxyRequestAttemptUpdate(old, updatedAttempt) : old,
        );
      }

      const listQueries = queryCache
        .findAll({ queryKey: requestKeys.lists() })
        .filter((q) => q.getObserversCount() > 0);
      for (const query of listQueries) {
        const queryKey = query.queryKey as ReturnType<typeof requestKeys.list>;
        const params = queryKey[2] as CursorPaginationParams | undefined;

        queryClient.setQueryData<CursorPaginationResult<ProxyRequest>>(queryKey, (old) => {
          if (!old || !old.items) return old;
          const index = old.items.findIndex((r) => r.id === requestId);
          if (index < 0) return old;

          const updatedRequest = mergeProxyRequestAttemptUpdate(old.items[index], updatedAttempt);
          if (!matchesProxyRequestParams(updatedRequest, params)) {
            const items = prioritizeActiveRequests(old.items.filter((r) => r.id !== requestId));
            return {
              ...old,
              items,
              firstId: items[0]?.id,
              lastId: items[items.length - 1]?.id,
            };
          }

          const items = [...old.items];
          items[index] = updatedRequest;
          const normalized = prioritizeActiveRequests(items);
          return {
            ...old,
            items: normalized,
            firstId: normalized[0]?.id,
            lastId: normalized[normalized.length - 1]?.id,
          };
        });
      }

      const infiniteQueries = queryCache
        .findAll({ queryKey: [...requestKeys.all, 'infinite'] })
        .filter((q) => q.getObserversCount() > 0);
      for (const query of infiniteQueries) {
        const queryKey = query.queryKey as ReturnType<typeof requestKeys.infinite>;
        const params: CursorPaginationParams = {
          providerId: queryKey[2] as number | undefined,
          status: queryKey[3] as string | undefined,
          apiTokenId: queryKey[4] as number | undefined,
          projectId: queryKey[5] as number | undefined,
          startTime: queryKey[6] as string | undefined,
          endTime: queryKey[7] as string | undefined,
          errorMode: queryKey[8] as ProxyRequestErrorMode | undefined,
        };

        queryClient.setQueryData<{
          pages: CursorPaginationResult<ProxyRequest>[];
          pageParams: (number | undefined)[];
        }>(queryKey, (old) => {
          if (!old || !old.pages || old.pages.length === 0) return old;

          let changed = false;
          const pages = old.pages.map((page) => {
            const index = page.items.findIndex((r) => r.id === requestId);
            if (index < 0) return page;

            changed = true;
            const updatedRequest = mergeProxyRequestAttemptUpdate(
              page.items[index],
              updatedAttempt,
            );
            if (!matchesProxyRequestParams(updatedRequest, params)) {
              return {
                ...page,
                items: prioritizeActiveRequests(page.items.filter((r) => r.id !== requestId)),
              };
            }

            const items = [...page.items];
            items[index] = updatedRequest;
            return { ...page, items: prioritizeActiveRequests(items) };
          });

          return changed ? { ...old, pages } : old;
        });
      }
    };

    const flushAttempts = () => {
      if (pendingAttemptsByRequest.size === 0) {
        return;
      }

      const entries = Array.from(pendingAttemptsByRequest.entries());
      pendingAttemptsByRequest.clear();

      for (const [proxyRequestID, attemptsById] of entries) {
        const attemptsKey = requestKeys.attempts(proxyRequestID);
        const attemptsQuery = queryCache.find({ queryKey: attemptsKey, exact: true });
        const updates = Array.from(attemptsById.values());

        for (const updatedAttempt of updates) {
          patchRequestCachesFromAttempt(updatedAttempt);
        }

        if (!attemptsQuery || attemptsQuery.getObserversCount() === 0) {
          continue;
        }

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
      let refetchCleanupFailedCount = false;

      for (const updatedRequest of updates) {
        const requestId = updatedRequest.id;
        let isKnown = knownRequestIds.has(requestId);
        let previousRequest: ProxyRequest | undefined;

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
            previousRequest ??= old;
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
          const filterProjectId = params?.projectId;
          const filterStartTime = params?.startTime;
          const filterEndTime = params?.endTime;
          const filterErrorMode = params?.errorMode;

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
            if (!matchesRequestTimeRange(request, filterStartTime, filterEndTime)) {
              return false;
            }
            if (filterErrorMode === 'only' && !isProxyRequestError(request)) {
              return false;
            }
            if (filterErrorMode === 'exclude' && isProxyRequestError(request)) {
              return false;
            }
            return true;
          };

          queryClient.setQueryData<CursorPaginationResult<ProxyRequest>>(queryKey, (old) => {
            if (!old || !old.items) return old;
            const limit = typeof params?.limit === 'number' ? params.limit : undefined;

            const normalizePage = (items: ProxyRequest[]) =>
              normalizeProxyRequestPage(old, items, limit);

            const index = old.items.findIndex((r) => r.id === requestId);
            if (index >= 0) {
              isKnown = true;
              previousRequest ??= old.items[index];
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
          const filterStartTime = queryKey[6] as string | undefined;
          const filterEndTime = queryKey[7] as string | undefined;
          const filterErrorMode = queryKey[8] as ProxyRequestErrorMode | undefined;

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
            if (!matchesRequestTimeRange(request, filterStartTime, filterEndTime)) {
              return false;
            }
            if (filterErrorMode === 'only' && !isProxyRequestError(request)) {
              return false;
            }
            if (filterErrorMode === 'exclude' && isProxyRequestError(request)) {
              return false;
            }
            return true;
          };

          queryClient.setQueryData<{
            pages: CursorPaginationResult<ProxyRequest>[];
            pageParams: (number | undefined)[];
          }>(queryKey, (old) => {
            if (!old || !old.pages || old.pages.length === 0) return old;

            const items = old.pages.flatMap((page) => page.items);
            const index = items.findIndex((r) => r.id === requestId);

            if (index >= 0) {
              isKnown = true;
              previousRequest ??= items[index];

              const nextItems = matchesFilter(updatedRequest)
                ? [...items.slice(0, index), updatedRequest, ...items.slice(index + 1)]
                : items.filter((r) => r.id !== requestId);

              return {
                ...old,
                pages: normalizeProxyRequestPages(
                  old.pages,
                  nextItems,
                  REQUESTS_INFINITE_PAGE_LIMIT,
                ),
              };
            }

            if (!matchesFilter(updatedRequest)) {
              return old;
            }

            // 仅在第一页插入“新请求”，但对已加载页做级联规范化，
            // 避免第一页满载时把尾部记录直接丢掉造成分页空洞。
            return {
              ...old,
              pages: normalizeProxyRequestPages(
                old.pages,
                [updatedRequest, ...items],
                REQUESTS_INFINITE_PAGE_LIMIT,
              ),
            };
          });
        }

        // 同步 count 查询：已知请求跨过滤器移动时要做 old -> new 差量；
        // 新请求仍只对最近事件乐观 +1，避免重连后把历史广播重复计数。
        const startTimeMs = new Date(updatedRequest.startTime).getTime();
        const looksLikeRecentRequest =
          Number.isFinite(startTimeMs) && Date.now() - startTimeMs < 15_000;
        const shouldReconcileUnknownRequest = !isKnown && looksLikeRecentRequest;
        if (previousRequest || shouldReconcileUnknownRequest) {
          for (const query of countQueries) {
            const queryKey = query.queryKey as RequestsCountQueryKey;
            const delta = getProxyRequestCountDelta(previousRequest, updatedRequest, queryKey);
            if (delta === 0) {
              continue;
            }
            queryClient.setQueryData<number>(queryKey, (old) => Math.max(0, (old ?? 0) + delta));
          }
        } else if (isKnown) {
          // 这个请求此前只被其他过滤器看见过；当前 count 查询没有旧行可对比，
          // 直接补偿 refetch，避免“进入当前过滤器”的计数被永远漏掉。
          void queryClient.refetchQueries({ queryKey: ['requestsCount'], type: 'active' });
        }

        knownRequestIds.add(requestId);

        if (updatedRequest.status === 'COMPLETED' || updatedRequest.status === 'FAILED') {
          invalidateDashboard = true;
          invalidateProviderStats = true;
          invalidateCooldowns = true;
        }
        if (shouldRefetchCleanupFailedCount(updatedRequest)) {
          refetchCleanupFailedCount = true;
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
      queryClient.invalidateQueries({ queryKey: [...requestKeys.all, 'error-stats'] });
      if (refetchCleanupFailedCount) {
        void queryClient.refetchQueries({
          queryKey: requestKeys.cleanupFailedCounts(),
          type: 'active',
        });
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

    const unsubscribeRequest = transport.subscribe<ProxyRequest>(
      'proxy_request_update',
      (updatedRequest) => {
        pendingRequests.set(updatedRequest.id, updatedRequest);
        scheduleFlush();
      },
    );

    // 订阅 ProxyUpstreamAttempt 更新事件
    const unsubscribeAttempt = transport.subscribe<ProxyUpstreamAttempt>(
      'proxy_upstream_attempt_update',
      (updatedAttempt) => {
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
      void queryClient.refetchQueries({
        queryKey: [...requestKeys.all, 'infinite'],
        type: 'active',
      });
      void queryClient.refetchQueries({ queryKey: requestKeys.details(), type: 'active' });
      void queryClient.refetchQueries({ queryKey: ['requestsCount'], type: 'active' });
      void queryClient.refetchQueries({
        queryKey: requestKeys.cleanupFailedCounts(),
        type: 'active',
      });
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
