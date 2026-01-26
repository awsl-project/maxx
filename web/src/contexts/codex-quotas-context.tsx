/**
 * Codex Quotas Context
 * 提供批量获取的 Codex 配额数据，减少重复请求
 */

import { createContext, useContext, type ReactNode } from 'react';
import type { CodexQuotaData } from '@/lib/transport';
import { useCodexBatchQuotas } from '@/hooks/queries';

interface CodexQuotasContextValue {
  quotas: Record<number, CodexQuotaData> | undefined;
  isLoading: boolean;
  getQuotaForProvider: (providerId: number) => CodexQuotaData | undefined;
}

const CodexQuotasContext = createContext<CodexQuotasContextValue | null>(null);

interface CodexQuotasProviderProps {
  children: ReactNode;
  enabled?: boolean;
}

export function CodexQuotasProvider({ children, enabled = true }: CodexQuotasProviderProps) {
  const { data: quotas, isLoading } = useCodexBatchQuotas(enabled);

  const getQuotaForProvider = (providerId: number): CodexQuotaData | undefined => {
    return quotas?.[providerId];
  };

  return (
    <CodexQuotasContext.Provider value={{ quotas, isLoading, getQuotaForProvider }}>
      {children}
    </CodexQuotasContext.Provider>
  );
}

export function useCodexQuotasContext() {
  const context = useContext(CodexQuotasContext);
  if (!context) {
    throw new Error('useCodexQuotasContext must be used within CodexQuotasProvider');
  }
  return context;
}

// 可选的 hook，用于在没有 Provider 时不抛出错误
export function useCodexQuotaFromContext(providerId: number): CodexQuotaData | undefined {
  const context = useContext(CodexQuotasContext);
  return context?.getQuotaForProvider(providerId);
}
