import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiTokenKeys } from './use-api-tokens';
import { getTransport } from '@/lib/transport';

export const userPanelTokenKeys = {
  all: ['user-panel-token'] as const,
  detail: () => [...userPanelTokenKeys.all, 'detail'] as const,
};

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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userPanelTokenKeys.all });
      queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() });
    },
  });
}

export function useRegenerateUserPanelAPIToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().regenerateUserPanelAPIToken(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userPanelTokenKeys.all });
      queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() });
    },
  });
}
