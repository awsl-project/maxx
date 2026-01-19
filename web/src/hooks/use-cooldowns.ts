import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query';
import { getTransport } from '@/lib/transport';
import type { Cooldown } from '@/lib/transport';
import { useEffect, useState, useCallback } from 'react';

export function useCooldowns() {
  const queryClient = useQueryClient();
  // Force re-render counter to trigger updates when cooldowns expire
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
    const transport = getTransport();
    const unsubscribe = transport.subscribe('cooldown_update', () => {
      // Invalidate and refetch cooldowns when a cooldown update is received
      queryClient.invalidateQueries({ queryKey: ['cooldowns'] });
    });

    return () => {
      unsubscribe();
    };
  }, [queryClient]);

  // Mutation for clearing cooldown
  const clearCooldownMutation = useMutation({
    mutationFn: (providerId: number) => getTransport().clearCooldown(providerId),
    onSuccess: () => {
      // Invalidate and refetch cooldowns after successful deletion
      queryClient.invalidateQueries({ queryKey: ['cooldowns'] });
    },
  });

  // Setup timeouts for each cooldown to force re-render when they expire
  useEffect(() => {
    if (cooldowns.length === 0) {
      return;
    }

    const timeouts: number[] = [];

    cooldowns.forEach((cooldown) => {
      const until = new Date(cooldown.untilTime).getTime();
      const now = Date.now();
      const delay = until - now;

      // If cooldown will expire in the future, set a timeout
      if (delay > 0) {
        const timeout = setTimeout(() => {
          // Force re-render when cooldown expires
          // This ensures getCooldownForProvider returns undefined for expired cooldowns
          setRefreshKey((prev) => prev + 1);
        }, delay + 100); // Add small buffer to ensure time has passed
        timeouts.push(timeout);
      }
    });

    return () => {
      // Clear all timeouts on cleanup
      timeouts.forEach((timeout) => clearTimeout(timeout));
    };
  }, [cooldowns]);

  // Helper to get cooldown for a specific provider (excludes expired cooldowns)
  // Use useCallback with refreshKey to ensure new reference when cooldowns expire
  const getCooldownForProvider = useCallback((providerId: number, clientType?: string) => {
    return cooldowns.find((cd: Cooldown) => {
      // Check if cooldown matches provider and client type
      const matchesProvider = cd.providerID === providerId;
      const matchesClientType =
        cd.clientType === '' ||
        cd.clientType === 'all' ||
        (clientType && cd.clientType === clientType);

      if (!matchesProvider || !matchesClientType) {
        return false;
      }

      // Check if cooldown is still active (not expired)
      const untilTime =
        cd.untilTime || ((cd as unknown as Record<string, unknown>).until as string);
      if (!untilTime) {
        return false;
      }
      const until = new Date(untilTime).getTime();
      const now = Date.now();
      return until > now;
    });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cooldowns, refreshKey]);

  // Helper to check if provider is in cooldown
  const isProviderInCooldown = useCallback((providerId: number, clientType?: string) => {
    return !!getCooldownForProvider(providerId, clientType);
  }, [getCooldownForProvider]);

  // Helper to get remaining time as seconds
  const getRemainingSeconds = useCallback((cooldown: Cooldown) => {
    // Handle both 'untilTime' and 'until' field names for backward compatibility
    const untilTime =
      cooldown.untilTime || ((cooldown as unknown as Record<string, unknown>).until as string);
    if (!untilTime) return 0;

    const until = new Date(untilTime);
    const now = new Date();
    const diff = until.getTime() - now.getTime();
    return Math.max(0, Math.floor(diff / 1000));
  }, []);

  // Helper to format remaining time
  const formatRemaining = useCallback((cooldown: Cooldown) => {
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
  }, [getRemainingSeconds]);

  // Helper to clear cooldown
  const clearCooldown = useCallback((providerId: number) => {
    clearCooldownMutation.mutate(providerId);
  }, [clearCooldownMutation]);

  return {
    cooldowns,
    isLoading,
    error,
    getCooldownForProvider,
    isProviderInCooldown,
    getRemainingSeconds,
    formatRemaining,
    clearCooldown,
    isClearingCooldown: clearCooldownMutation.isPending,
  };
}
