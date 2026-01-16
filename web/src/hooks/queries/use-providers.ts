/**
 * Provider React Query Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport, type Provider, type CreateProviderData } from '@/lib/transport';
import { routeKeys } from './use-routes';

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
    queryFn: () => getTransport().getProviders(),
  });
}

// 获取单个 Provider
export function useProvider(id: number) {
  return useQuery({
    queryKey: providerKeys.detail(id),
    queryFn: () => getTransport().getProvider(id),
    enabled: id > 0,
  });
}

// 创建 Provider
export function useCreateProvider() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateProviderData) => getTransport().createProvider(data),
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
      getTransport().updateProvider(id, data),
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
    mutationFn: (id: number) => getTransport().deleteProvider(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: providerKeys.lists() });
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}

// 获取 Provider 统计信息
export function useProviderStats(clientType?: string, projectId?: number) {
  return useQuery({
    queryKey: [...providerKeys.stats(), clientType, projectId],
    queryFn: () => getTransport().getProviderStats(clientType, projectId),
    // 每 30 秒刷新一次
    refetchInterval: 30000,
    enabled: !!clientType, // 只在有 clientType 时才查询
  });
}

// 获取全局 Provider 统计信息（不区分 clientType）
export function useAllProviderStats() {
  return useQuery({
    queryKey: [...providerKeys.stats(), 'all'],
    queryFn: () => getTransport().getProviderStats(),
    // 每 30 秒刷新一次
    refetchInterval: 30000,
  });
}

// 获取 Antigravity Provider 额度
export function useAntigravityQuota(providerId: number, enabled = true) {
  return useQuery({
    queryKey: [...providerKeys.all, 'antigravity-quota', providerId],
    queryFn: () => getTransport().getAntigravityProviderQuota(providerId, false),
    enabled: enabled && providerId > 0,
    // 每 60 秒刷新一次
    refetchInterval: 60000,
    staleTime: 30000,
  });
}

// 获取 Kiro Provider 额度
export function useKiroQuota(providerId: number, enabled = true) {
  return useQuery({
    queryKey: [...providerKeys.all, 'kiro-quota', providerId],
    queryFn: () => getTransport().getKiroProviderQuota(providerId),
    enabled: enabled && providerId > 0,
    // 每 60 秒刷新一次
    refetchInterval: 60000,
    staleTime: 30000,
  });
}
