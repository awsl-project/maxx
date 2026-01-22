/**
 * Pricing API Hooks
 */

import { useQuery } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';

export const pricingKeys = {
  all: ['pricing'] as const,
};

/**
 * 获取价格表
 * 价格表较少变化，使用较长的 staleTime
 */
export function usePricing() {
  return useQuery({
    queryKey: pricingKeys.all,
    queryFn: () => getTransport().getPricing(),
    staleTime: 1000 * 60 * 60, // 1 hour
  });
}
