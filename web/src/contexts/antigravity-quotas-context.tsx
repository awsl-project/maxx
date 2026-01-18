/**
 * Antigravity Quotas Context
 * 提供批量获取的 Antigravity 配额数据，减少重复请求
 */

import { createContext, useContext, type ReactNode } from 'react';
import type { AntigravityQuotaData } from '@/lib/transport';
import { useAntigravityBatchQuotas } from '@/hooks/queries';

interface AntigravityQuotasContextValue {
  quotas: Record<number, AntigravityQuotaData> | undefined;
  isLoading: boolean;
  getQuotaForProvider: (providerId: number) => AntigravityQuotaData | undefined;
}

const AntigravityQuotasContext = createContext<AntigravityQuotasContextValue | null>(null);

interface AntigravityQuotasProviderProps {
  children: ReactNode;
  enabled?: boolean;
}

export function AntigravityQuotasProvider({
  children,
  enabled = true,
}: AntigravityQuotasProviderProps) {
  const { data: quotas, isLoading } = useAntigravityBatchQuotas(enabled);

  const getQuotaForProvider = (providerId: number): AntigravityQuotaData | undefined => {
    return quotas?.[providerId];
  };

  return (
    <AntigravityQuotasContext.Provider value={{ quotas, isLoading, getQuotaForProvider }}>
      {children}
    </AntigravityQuotasContext.Provider>
  );
}

export function useAntigravityQuotasContext() {
  const context = useContext(AntigravityQuotasContext);
  if (!context) {
    throw new Error('useAntigravityQuotasContext must be used within AntigravityQuotasProvider');
  }
  return context;
}

// 可选的 hook，用于在没有 Provider 时不抛出错误
export function useAntigravityQuotaFromContext(
  providerId: number,
): AntigravityQuotaData | undefined {
  const context = useContext(AntigravityQuotasContext);
  return context?.getQuotaForProvider(providerId);
}
