/**
 * Invite Code React Query Hooks
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  getTransport,
  type InviteCode,
  type CreateInviteCodeData,
  type UpdateInviteCodeData,
  type InviteCodeUsage,
  type InviteCodeCreateResult,
} from '@/lib/transport';

export const inviteCodeKeys = {
  all: ['invite-codes'] as const,
  lists: () => [...inviteCodeKeys.all, 'list'] as const,
  details: () => [...inviteCodeKeys.all, 'detail'] as const,
  detail: (id: number) => [...inviteCodeKeys.details(), id] as const,
  usages: (id: number) => [...inviteCodeKeys.all, 'usages', id] as const,
};

export function useInviteCodes() {
  return useQuery({
    queryKey: inviteCodeKeys.lists(),
    queryFn: () => getTransport().getInviteCodes(),
  });
}

export function useInviteCode(id: number) {
  return useQuery({
    queryKey: inviteCodeKeys.detail(id),
    queryFn: () => getTransport().getInviteCode(id),
    enabled: id > 0,
  });
}

export function useInviteCodeUsages(id: number) {
  return useQuery<InviteCodeUsage[]>({
    queryKey: inviteCodeKeys.usages(id),
    queryFn: () => getTransport().getInviteCodeUsages(id),
    enabled: id > 0,
  });
}

export function useCreateInviteCodes() {
  const queryClient = useQueryClient();

  return useMutation<InviteCodeCreateResult, unknown, CreateInviteCodeData>({
    mutationFn: (data) => getTransport().createInviteCodes(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: inviteCodeKeys.lists() });
    },
  });
}

export function useUpdateInviteCode() {
  const queryClient = useQueryClient();

  return useMutation<InviteCode, unknown, { id: number; data: UpdateInviteCodeData }>({
    mutationFn: ({ id, data }) => getTransport().updateInviteCode(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: inviteCodeKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: inviteCodeKeys.lists() });
    },
  });
}

export function useDeleteInviteCode() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => getTransport().deleteInviteCode(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: inviteCodeKeys.lists() });
    },
  });
}
