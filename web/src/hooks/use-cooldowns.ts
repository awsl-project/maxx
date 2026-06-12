import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';
import type { Cooldown, ProviderHealthLevel } from '@/lib/transport';
import { useEffect, useState, useCallback } from 'react';
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

export function useCooldowns() {
  const queryClient = useQueryClient();
  const [refreshKey, setRefreshKey] = useState(0);

  const {
    data: cooldowns = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ['cooldowns'],
    queryFn: () => getTransport().getCooldowns(),
    staleTime: 3000,
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

  // Setup timeouts for each cooldown to force re-render when they expire
  useEffect(() => {
    if (cooldowns.length === 0) return;
    const timeouts: number[] = [];
    cooldowns.forEach((cooldown) => {
      const until = new Date(cooldown.until).getTime();
      const delay = until - Date.now();
      if (delay > 0) {
        const timeout = setTimeout(() => {
          setRefreshKey((prev) => prev + 1);
        }, delay + 100);
        timeouts.push(timeout);
      }
    });
    return () => timeouts.forEach((timeout) => clearTimeout(timeout));
  }, [cooldowns]);

  // Get all active cooldowns for a provider, optionally filtered by clientType.
  // When clientType is given, returns cooldowns that affect that clientType:
  //   - provider-level (clientType="") — always included
  //   - matching clientType
  //   - model-level with matching clientType or no clientType
  // When clientType is omitted, returns all cooldowns for the provider.
  const getCooldownsForProvider = useCallback(
    (providerId: number, clientType?: string): Cooldown[] => {
      const now = Date.now();
      return cooldowns.filter((cd) => {
        if (cd.providerID !== providerId) return false;
        if (new Date(cd.until).getTime() <= now) return false;
        if (!clientType) return true; // no filter, return all
        // provider-level cooldown (empty clientType) affects all
        if (!cd.clientType) return true;
        // match specific clientType
        return cd.clientType === clientType;
      });
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [cooldowns, refreshKey],
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [cooldowns, refreshKey],
  );

  // Get remaining seconds for a cooldown
  const getRemainingSeconds = useCallback((cooldown: Cooldown) => {
    if (!cooldown.until) return 0;
    const diff = new Date(cooldown.until).getTime() - Date.now();
    return Math.max(0, Math.floor(diff / 1000));
  }, []);

  // Format remaining time
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
      }
      return `${String(secs).padStart(2, '0')}s`;
    },
    [getRemainingSeconds],
  );

  // Clear cooldown (with optional model)
  const clearCooldown = useCallback(
    (providerId: number, options?: { clientType?: string; model?: string }) => {
      clearCooldownMutation.mutate({ providerId, options });
    },
    [clearCooldownMutation],
  );

  return {
    cooldowns,
    isLoading,
    error,
    getCooldownsForProvider,
    getProviderHealthLevel,
    isProviderInCooldown,
    getRemainingSeconds,
    formatRemaining,
    clearCooldown,
    isClearingCooldown: clearCooldownMutation.isPending,
  };
}
