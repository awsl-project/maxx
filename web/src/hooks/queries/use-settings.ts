/**
 * Settings API Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';
import type { ModelMappingInput } from '@/lib/transport';

export const settingsKeys = {
  all: ['settings'] as const,
  detail: (key: string) => ['settings', key] as const,
  modelMappings: ['model-mappings'] as const,
};

export function useSettings() {
  return useQuery({
    queryKey: settingsKeys.all,
    queryFn: () => getTransport().getSettings(),
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all });
    },
  });
}

export function useDeleteSetting() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (key: string) => getTransport().deleteSetting(key),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all });
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
    mutationFn: (data: ModelMappingInput) =>
      getTransport().createModelMapping(data),
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
