/**
 * Provider React Query Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport, type Provider, type CreateProviderData } from '@/lib/transport';
import { routeKeys } from './use-routes';

const transport = getTransport();

// Query Keys
export const providerKeys = {
  all: ['providers'] as const,
  lists: () => [...providerKeys.all, 'list'] as const,
  list: () => [...providerKeys.lists()] as const,
  details: () => [...providerKeys.all, 'detail'] as const,
  detail: (id: number) => [...providerKeys.details(), id] as const,
  stats: () => [...providerKeys.all, 'stats'] as const,
};

// 获取所有 Providers
export function useProviders() {
  return useQuery({
    queryKey: providerKeys.list(),
    queryFn: () => transport.getProviders(),
  });
}

// 获取单个 Provider
export function useProvider(id: number) {
  return useQuery({
    queryKey: providerKeys.detail(id),
    queryFn: () => transport.getProvider(id),
    enabled: id > 0,
  });
}

// 创建 Provider
export function useCreateProvider() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateProviderData) => transport.createProvider(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: providerKeys.lists() });
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}

// 更新 Provider
export function useUpdateProvider() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Provider> }) =>
      transport.updateProvider(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: providerKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: providerKeys.lists() });
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}

// 删除 Provider
export function useDeleteProvider() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => transport.deleteProvider(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: providerKeys.lists() });
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}

// 获取 Provider 统计信息
export function useProviderStats(clientType?: string) {
  return useQuery({
    queryKey: [...providerKeys.stats(), clientType],
    queryFn: () => transport.getProviderStats(clientType),
    // 每 30 秒刷新一次
    refetchInterval: 30000,
    enabled: !!clientType, // 只在有 clientType 时才查询
  });
}
