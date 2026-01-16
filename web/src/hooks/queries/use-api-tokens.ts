/**
 * API Token React Query Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport, type APIToken, type CreateAPITokenData } from '@/lib/transport';

// Query Keys
export const apiTokenKeys = {
  all: ['api-tokens'] as const,
  lists: () => [...apiTokenKeys.all, 'list'] as const,
  list: () => [...apiTokenKeys.lists()] as const,
  details: () => [...apiTokenKeys.all, 'detail'] as const,
  detail: (id: number) => [...apiTokenKeys.details(), id] as const,
};

// 获取所有 API Tokens
export function useAPITokens() {
  return useQuery({
    queryKey: apiTokenKeys.list(),
    queryFn: () => getTransport().getAPITokens(),
  });
}

// 获取单个 API Token
export function useAPIToken(id: number) {
  return useQuery({
    queryKey: apiTokenKeys.detail(id),
    queryFn: () => getTransport().getAPIToken(id),
    enabled: id > 0,
  });
}

// 创建 API Token
export function useCreateAPIToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateAPITokenData) => getTransport().createAPIToken(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() });
    },
  });
}

// 更新 API Token
export function useUpdateAPIToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<APIToken> }) =>
      getTransport().updateAPIToken(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: apiTokenKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() });
    },
  });
}

// 删除 API Token
export function useDeleteAPIToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => getTransport().deleteAPIToken(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() });
    },
  });
}
