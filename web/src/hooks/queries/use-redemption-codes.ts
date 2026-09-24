import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  getTransport,
  type CreateRedemptionCodeData,
  type RedemptionCode,
  type RedemptionCodeCreateResult,
  type UserPanelCreateRedemptionCodeData,
  type UserPanelCreateRedemptionCodeResult,
  type UpdateRedemptionCodeData,
} from '@/lib/transport';
import { apiTokenKeys } from './use-api-tokens';

export const redemptionCodeKeys = {
  all: ['redemption-codes'] as const,
  lists: () => [...redemptionCodeKeys.all, 'list'] as const,
  list: () => [...redemptionCodeKeys.lists()] as const,
  details: () => [...redemptionCodeKeys.all, 'detail'] as const,
  detail: (id: number) => [...redemptionCodeKeys.details(), id] as const,
};

export function useRedemptionCodes() {
  return useQuery({
    queryKey: redemptionCodeKeys.list(),
    queryFn: () => getTransport().getRedemptionCodes(),
  });
}

export function useRedemptionCode(id: number) {
  return useQuery({
    queryKey: redemptionCodeKeys.detail(id),
    queryFn: () => getTransport().getRedemptionCode(id),
    enabled: id > 0,
  });
}

export function useCreateRedemptionCodes() {
  const queryClient = useQueryClient();
  return useMutation<RedemptionCodeCreateResult, unknown, CreateRedemptionCodeData>({
    mutationFn: (data) => getTransport().createRedemptionCodes(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: redemptionCodeKeys.lists() }),
  });
}

export function useUpdateRedemptionCode() {
  const queryClient = useQueryClient();
  return useMutation<RedemptionCode, unknown, { id: number; data: UpdateRedemptionCodeData }>({
    mutationFn: ({ id, data }) => getTransport().updateRedemptionCode(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: redemptionCodeKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: redemptionCodeKeys.lists() });
    },
  });
}

export function useDeleteRedemptionCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => getTransport().deleteRedemptionCode(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: redemptionCodeKeys.lists() }),
  });
}

export function useCreateUserPanelRedemptionCodes() {
  const queryClient = useQueryClient();
  return useMutation<
    UserPanelCreateRedemptionCodeResult,
    unknown,
    UserPanelCreateRedemptionCodeData
  >({
    mutationFn: (data) => getTransport().createUserPanelRedemptionCodes(data),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() }),
        queryClient.invalidateQueries({ queryKey: ['user-panel-token'] }),
      ]);
    },
  });
}

export function useRedeemUserPanelCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (code: string) => getTransport().redeemUserPanelCode(code),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() }),
        queryClient.invalidateQueries({ queryKey: ['user-panel-token'] }),
      ]);
      await queryClient.refetchQueries({
        predicate: (query) => query.queryKey[0] === 'usageStats',
        type: 'active',
      });
    },
  });
}
