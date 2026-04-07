/**
 * Settings API Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';
import type { ModelMappingInput } from '@/lib/transport';

export const settingsKeys = {
  all: ['settings'] as const,
  public: ['public-settings'] as const,
  detail: (key: string) => ['settings', key] as const,
  modelMappings: ['model-mappings'] as const,
};

export function usePublicSettings(enabled = true) {
  return useQuery({
    queryKey: settingsKeys.public,
    queryFn: () => getTransport().getPublicSettings(),
    enabled,
  });
}

export function useSettings(enabled = true) {
  return useQuery({
    queryKey: settingsKeys.all,
    queryFn: () => getTransport().getAdminSettings(),
    enabled,
  });
}

export function useSetting(key: string) {
  return useQuery({
    queryKey: settingsKeys.detail(key),
    queryFn: () => getTransport().getSetting(key),
    enabled: !!key,
  });
}

export function useUpdateSetting() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      getTransport().updateSetting(key, value),
    onMutate: async ({ key, value }) => {
      // Cancel any outgoing refetches
      await queryClient.cancelQueries({ queryKey: settingsKeys.all });

      // Snapshot the previous value
      const previousSettings = queryClient.getQueryData<Record<string, string>>(settingsKeys.all);

      // Optimistically update to the new value
      if (previousSettings) {
        queryClient.setQueryData(settingsKeys.all, {
          ...previousSettings,
          [key]: value,
        });
      }

      return { previousSettings };
    },
    onError: (_err, _variables, context) => {
      // If the mutation fails, use the context returned from onMutate to roll back
      if (context?.previousSettings) {
        queryClient.setQueryData(settingsKeys.all, context.previousSettings);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all });
      queryClient.invalidateQueries({ queryKey: settingsKeys.public });
    },
  });
}

export function useDeleteSetting() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (key: string) => getTransport().deleteSetting(key),
    onMutate: async (key) => {
      await queryClient.cancelQueries({ queryKey: settingsKeys.all });

      const previousSettings = queryClient.getQueryData<Record<string, string>>(settingsKeys.all);

      if (previousSettings) {
        const nextSettings = { ...previousSettings };
        delete nextSettings[key];
        queryClient.setQueryData(settingsKeys.all, nextSettings);
      }

      return { previousSettings };
    },
    onError: (_err, _key, context) => {
      if (context?.previousSettings) {
        queryClient.setQueryData(settingsKeys.all, context.previousSettings);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all });
      queryClient.invalidateQueries({ queryKey: settingsKeys.public });
    },
  });
}

// ===== Model Mapping =====

export function useModelMappings() {
  return useQuery({
    queryKey: settingsKeys.modelMappings,
    queryFn: () => getTransport().getModelMappings(),
  });
}

export function useCreateModelMapping() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: ModelMappingInput) => getTransport().createModelMapping(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.modelMappings });
    },
  });
}

export function useUpdateModelMapping() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: ModelMappingInput }) =>
      getTransport().updateModelMapping(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.modelMappings });
    },
  });
}

export function useDeleteModelMapping() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => getTransport().deleteModelMapping(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.modelMappings });
    },
  });
}

export function useClearAllModelMappings() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().clearAllModelMappings(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.modelMappings });
    },
  });
}

export function useResetModelMappingsToDefaults() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().resetModelMappingsToDefaults(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.modelMappings });
    },
  });
}
