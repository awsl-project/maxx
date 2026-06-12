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

/**
 * 获取 Proxy 状态
 * 注意：maxx 的 proxy 总是运行的，不支持 start/stop
 */
export function useProxyStatus() {
  return useQuery({
    queryKey: proxyKeys.status(),
    queryFn: () => getTransport().getPublicProxyStatus(),
    staleTime: Infinity, // Proxy 状态不会变化
  });
}
