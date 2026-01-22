/**
 * Model Price React Query Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport, type ModelPriceInput } from '@/lib/transport';
import { pricingKeys } from './use-pricing';

// Query Keys
export const modelPriceKeys = {
  all: ['model-prices'] as const,
  lists: () => [...modelPriceKeys.all, 'list'] as const,
  list: () => [...modelPriceKeys.lists()] as const,
  details: () => [...modelPriceKeys.all, 'detail'] as const,
  detail: (id: number) => [...modelPriceKeys.details(), id] as const,
};

// 获取所有 Model Prices
export function useModelPrices() {
  return useQuery({
    queryKey: modelPriceKeys.list(),
    queryFn: () => getTransport().getModelPrices(),
  });
}

// 获取单个 Model Price
export function useModelPrice(id: number) {
  return useQuery({
    queryKey: modelPriceKeys.detail(id),
    queryFn: () => getTransport().getModelPrice(id),
    enabled: id > 0,
  });
}

// 创建 Model Price
export function useCreateModelPrice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: ModelPriceInput) => getTransport().createModelPrice(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: modelPriceKeys.lists() });
      // Also invalidate the pricing query since it may use database prices
      queryClient.invalidateQueries({ queryKey: pricingKeys.all });
    },
  });
}

// 更新 Model Price
export function useUpdateModelPrice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: ModelPriceInput }) =>
      getTransport().updateModelPrice(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: modelPriceKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: modelPriceKeys.lists() });
      // Also invalidate the pricing query since it may use database prices
      queryClient.invalidateQueries({ queryKey: pricingKeys.all });
    },
  });
}

// 删除 Model Price
export function useDeleteModelPrice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => getTransport().deleteModelPrice(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: modelPriceKeys.lists() });
      // Also invalidate the pricing query since it may use database prices
      queryClient.invalidateQueries({ queryKey: pricingKeys.all });
    },
  });
}

// 重置 Model Prices 为默认值
export function useResetModelPricesToDefaults() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().resetModelPricesToDefaults(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: modelPriceKeys.lists() });
      // Also invalidate the pricing query since it may use database prices
      queryClient.invalidateQueries({ queryKey: pricingKeys.all });
    },
  });
}
