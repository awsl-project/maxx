import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { apiTokenKeys } from './use-api-tokens';
import { getTransport } from '@/lib/transport';

export const userPanelTokenKeys = {
  all: ['user-panel-token'] as const,
  detail: () => [...userPanelTokenKeys.all, 'detail'] as const,
};

async function refreshUserPanelTokenDependents(queryClient: QueryClient) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: userPanelTokenKeys.all }),
    queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() }),
  ]);
  await queryClient.refetchQueries({
    predicate: (query) => query.queryKey[0] === 'usageStats',
    type: 'active',
  });
}

export function useUserPanelAPIToken() {
  return useQuery({
    queryKey: userPanelTokenKeys.detail(),
    queryFn: () => getTransport().getUserPanelAPIToken(),
  });
}

export function useCreateUserPanelAPIToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().createUserPanelAPIToken(),
    onSuccess: () => refreshUserPanelTokenDependents(queryClient),
  });
}

export function useRegenerateUserPanelAPIToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().regenerateUserPanelAPIToken(),
    onSuccess: () => refreshUserPanelTokenDependents(queryClient),
  });
}

export function useRevealUserPanelAPIToken() {
  return useMutation({
    mutationFn: () => getTransport().revealUserPanelAPIToken(),
  });
}

export function useUserPanelDailyCheckIn() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().checkInUserPanelDailyQuota(),
    onSuccess: () => refreshUserPanelTokenDependents(queryClient),
  });
}
