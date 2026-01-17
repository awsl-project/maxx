/**
 * Response Models React Query Hook
 * 获取所有已使用的响应模型名称列表
 */

import { useQuery } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';

// Query Keys
export const responseModelKeys = {
  all: ['responseModels'] as const,
  list: () => [...responseModelKeys.all, 'list'] as const,
};

/**
 * 获取响应模型名称列表
 */
export function useResponseModels() {
  return useQuery({
    queryKey: responseModelKeys.list(),
    queryFn: () => getTransport().getResponseModels(),
    staleTime: 5 * 60 * 1000, // 5 分钟
  });
}
