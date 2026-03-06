/**
 * User React Query Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport, type CreateUserData, type UpdateUserData } from '@/lib/transport';

// Query Keys
export const userKeys = {
  all: ['users'] as const,
  lists: () => [...userKeys.all, 'list'] as const,
  list: () => [...userKeys.lists()] as const,
  details: () => [...userKeys.all, 'detail'] as const,
  detail: (id: number) => [...userKeys.details(), id] as const,
  passkeys: () => [...userKeys.all, 'passkeys'] as const,
};

export function useUsers() {
  return useQuery({
    queryKey: userKeys.list(),
    queryFn: () => getTransport().getUsers(),
  });
}

export function useUser(id: number) {
  return useQuery({
    queryKey: userKeys.detail(id),
    queryFn: () => getTransport().getUser(id),
    enabled: id > 0,
  });
}

export function useCreateUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateUserData) => getTransport().createUser(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userKeys.lists() });
    },
  });
}

export function useUpdateUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: UpdateUserData }) =>
      getTransport().updateUser(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: userKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: userKeys.lists() });
    },
  });
}

export function useDeleteUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => getTransport().deleteUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userKeys.lists() });
    },
  });
}

export function useApproveUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => getTransport().approveUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userKeys.lists() });
    },
  });
}

export function useChangeMyPassword() {
  return useMutation({
    mutationFn: ({ oldPassword, newPassword }: { oldPassword: string; newPassword: string }) =>
      getTransport().changeMyPassword(oldPassword, newPassword),
  });
}

export function usePasskeyCredentials(enabled = true) {
  return useQuery({
    queryKey: userKeys.passkeys(),
    queryFn: () => getTransport().listPasskeyCredentials(),
    enabled,
  });
}

export function useDeletePasskeyCredential() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => getTransport().deletePasskeyCredential(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userKeys.passkeys() });
    },
  });
}

export function useRegisterPasskey() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const { startRegistration, browserSupportsWebAuthn } = await import(
        '@simplewebauthn/browser'
      );
      if (!browserSupportsWebAuthn()) {
        throw new Error('PASSKEY_NOT_SUPPORTED');
      }

      const transport = getTransport();
      const beginResult = await transport.startPasskeyRegistration();
      if (!beginResult.success || !beginResult.sessionID || !beginResult.options) {
        throw new Error(beginResult.error || 'Failed to start passkey registration');
      }

      const attResp = await startRegistration({ optionsJSON: beginResult.options! });

      const finishResult = await transport.finishPasskeyRegistration(
        beginResult.sessionID,
        attResp,
      );
      if (!finishResult.success) {
        throw new Error(finishResult.error || 'Failed to finish passkey registration');
      }
      return finishResult;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userKeys.passkeys() });
    },
  });
}
