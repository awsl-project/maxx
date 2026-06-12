/**
 * Cooldowns Context
 * 提供共享的 Cooldowns 数据，减少重复请求
 */

import { createContext, useContext, useEffect, useCallback, type ReactNode } from 'react';
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';
import type { Cooldown, ProviderHealthLevel } from '@/lib/transport';
import { subscribeCooldownUpdates } from '@/lib/cooldown-update-subscription';

/** Get provider health level from its cooldowns */
function computeHealthLevel(cooldowns: Cooldown[]): ProviderHealthLevel {
  if (cooldowns.length === 0) return 'healthy';
  const hasProviderLevel = cooldowns.some((cd) => !cd.clientType && !cd.model);
  if (hasProviderLevel) return 'frozen';
  const hasKeyLevel = cooldowns.some((cd) => cd.clientType && !cd.model);
  if (hasKeyLevel) return 'limited';
  return 'degraded';
}

/** Hierarchical cooldown check */
function checkInCooldown(
  cooldowns: Cooldown[],
  providerId: number,
  clientType?: string,
  model?: string,
): boolean {
  const now = Date.now();
  return cooldowns.some((cd) => {
    if (cd.providerID !== providerId) return false;
    if (new Date(cd.until).getTime() <= now) return false;
    // Provider-level: blocks everything
    if (!cd.clientType && !cd.model) return true;
    // ClientType-level: blocks all models for that client type
    if (clientType && cd.clientType === clientType && !cd.model) return true;
    // Model-level (all client types)
    if (model && !cd.clientType && cd.model === model) return true;
    // Model+ClientType level
    if (clientType && model && cd.clientType === clientType && cd.model === model) return true;
    return false;
  });
}

interface CooldownsContextValue {
  cooldowns: Cooldown[];
  isLoading: boolean;
  getCooldownsForProvider: (providerId: number, clientType?: string) => Cooldown[];
  getProviderHealthLevel: (providerId: number, clientType?: string) => ProviderHealthLevel;
  isProviderInCooldown: (providerId: number, clientType?: string, model?: string) => boolean;
  getRemainingSeconds: (cooldown: Cooldown) => number;
  formatRemaining: (cooldown: Cooldown) => string;
  clearCooldown: (providerId: number, options?: { clientType?: string; model?: string }) => void;
  isClearingCooldown: boolean;
  setCooldown: (providerId: number, untilTime: string, clientType?: string, model?: string) => void;
  isSettingCooldown: boolean;
}

const CooldownsContext = createContext<CooldownsContextValue | null>(null);

interface CooldownsProviderProps {
  children: ReactNode;
}

export function CooldownsProvider({ children }: CooldownsProviderProps) {
  const queryClient = useQueryClient();

  const { data: cooldowns = [], isLoading } = useQuery({
    queryKey: ['cooldowns'],
    queryFn: () => getTransport().getCooldowns(),
    staleTime: 5000,
  });

  // Subscribe to cooldown_update WebSocket event
  useEffect(() => {
    return subscribeCooldownUpdates(queryClient);
  }, [queryClient]);

  // Mutation for clearing cooldown
  const clearCooldownMutation = useMutation({
    mutationFn: ({ providerId, options }: { providerId: number; options?: { clientType?: string; model?: string } }) =>
      getTransport().clearCooldown(providerId, options),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cooldowns'] });
    },
  });

  // Mutation for setting cooldown
  const setCooldownMutation = useMutation({
    mutationFn: async ({
      providerId,
      untilTime,
      clientType,
      model,
    }: {
      providerId: number;
      untilTime: string;
      clientType?: string;
      model?: string;
    }) => {
      return getTransport().setCooldown(providerId, untilTime, clientType, model);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cooldowns'] });
    },
    onError: (error) => {
      console.error('Failed to set cooldown:', error);
    },
  });

  // Setup timeouts for each cooldown to force re-render when they expire
  useEffect(() => {
    if (cooldowns.length === 0) {
      return;
    }

    const timeouts: number[] = [];

    cooldowns.forEach((cooldown) => {
      const until = new Date(cooldown.until).getTime();
      const now = Date.now();
      const delay = until - now;

      if (delay > 0) {
        const timeout = setTimeout(() => {
          queryClient.invalidateQueries({ queryKey: ['cooldowns'] });
        }, delay + 100);
        timeouts.push(timeout);
      }
    });

    return () => {
      timeouts.forEach((timeout) => clearTimeout(timeout));
    };
  }, [cooldowns, queryClient]);

  // Get all active cooldowns for a provider
  const getCooldownsForProvider = useCallback(
    (providerId: number, clientType?: string): Cooldown[] => {
      const now = Date.now();
      return cooldowns.filter((cd) => {
        if (cd.providerID !== providerId) return false;
        if (new Date(cd.until).getTime() <= now) return false;
        if (!clientType) return true;
        if (!cd.clientType) return true; // provider-level affects all
        return cd.clientType === clientType;
      });
    },
    [cooldowns],
  );

  // Get health level for a provider (optionally scoped to a clientType)
  const getProviderHealthLevel = useCallback(
    (providerId: number, clientType?: string): ProviderHealthLevel => {
      return computeHealthLevel(getCooldownsForProvider(providerId, clientType));
    },
    [getCooldownsForProvider],
  );

  // Hierarchical check: is a specific (provider, clientType, model) combo frozen?
  const isProviderInCooldown = useCallback(
    (providerId: number, clientType?: string, model?: string): boolean => {
      return checkInCooldown(cooldowns, providerId, clientType, model);
    },
    [cooldowns],
  );

  const getRemainingSeconds = useCallback((cooldown: Cooldown) => {
    if (!cooldown.until) return 0;
    const diff = new Date(cooldown.until).getTime() - Date.now();
    return Math.max(0, Math.floor(diff / 1000));
  }, []);

  const formatRemaining = useCallback(
    (cooldown: Cooldown) => {
      const seconds = getRemainingSeconds(cooldown);

      if (Number.isNaN(seconds) || seconds === 0) return 'Expired';

      const hours = Math.floor(seconds / 3600);
      const minutes = Math.floor((seconds % 3600) / 60);
      const secs = seconds % 60;

      if (hours > 0) {
        return `${String(hours).padStart(2, '0')}h ${String(minutes).padStart(2, '0')}m ${String(secs).padStart(2, '0')}s`;
      } else if (minutes > 0) {
        return `${String(minutes).padStart(2, '0')}m ${String(secs).padStart(2, '0')}s`;
      } else {
        return `${String(secs).padStart(2, '0')}s`;
      }
    },
    [getRemainingSeconds],
  );

  const clearCooldown = useCallback(
    (providerId: number, options?: { clientType?: string; model?: string }) => {
      clearCooldownMutation.mutate({ providerId, options });
    },
    [clearCooldownMutation],
  );

  const setCooldown = useCallback(
    (providerId: number, untilTime: string, clientType?: string, model?: string) => {
      setCooldownMutation.mutate({ providerId, untilTime, clientType, model });
    },
    [setCooldownMutation],
  );

  return (
    <CooldownsContext.Provider
      value={{
        cooldowns,
        isLoading,
        getCooldownsForProvider,
        getProviderHealthLevel,
        isProviderInCooldown,
        getRemainingSeconds,
        formatRemaining,
        clearCooldown,
        isClearingCooldown: clearCooldownMutation.isPending,
        setCooldown,
        isSettingCooldown: setCooldownMutation.isPending,
      }}
    >
      {children}
    </CooldownsContext.Provider>
  );
}

export function useCooldownsContext() {
  const context = useContext(CooldownsContext);
  if (!context) {
    throw new Error('useCooldownsContext must be used within CooldownsProvider');
  }
  return context;
}

// Optional hook that doesn't throw when used outside provider
export function useCooldownFromContext(
  providerId: number,
  clientType?: string,
): Cooldown[] {
  const context = useContext(CooldownsContext);
  return context?.getCooldownsForProvider(providerId, clientType) ?? [];
}
