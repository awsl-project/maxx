/**
 * Proxy Status Hooks
 * 获取代理服务器状态
 */

import { useQuery } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';

export const proxyKeys = {
  all: ['proxy'] as const,
  status: () => [...proxyKeys.all, 'status'] as const,
};

interface QueryRefreshOptions {
  staleTime?: number;
  refetchInterval?: number | false;
}

/**
 * 获取 Proxy 状态
 */
export function useProxyStatus(options?: QueryRefreshOptions) {
  return useQuery({
    queryKey: proxyKeys.status(),
    queryFn: () => getTransport().getProxyStatus(),
    staleTime: options?.staleTime ?? 5000,
    refetchInterval: options?.refetchInterval,
  });
}
