import { useEffect, useRef } from 'react';
import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { apiTokenKeys } from './use-api-tokens';
import { getTransport } from '@/lib/transport';

export const userPanelTokenKeys = {
  all: ['user-panel-token'] as const,
  detail: () => [...userPanelTokenKeys.all, 'detail'] as const,
  availableModels: () => [...userPanelTokenKeys.all, 'available-models'] as const,
  availableModelRoutes: () => [...userPanelTokenKeys.all, 'available-model-routes'] as const,
  dailyCheckInStatus: (dayKey?: string) =>
    [...userPanelTokenKeys.all, 'daily-check-in-status', dayKey] as const,
  announcement: () => [...userPanelTokenKeys.all, 'announcement'] as const,
  consumptionLeaderboard: () => [...userPanelTokenKeys.all, 'consumption-leaderboard'] as const,
};

async function refreshUserPanelTokenDependents(queryClient: QueryClient) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: userPanelTokenKeys.all }),
    queryClient.invalidateQueries({ queryKey: apiTokenKeys.lists() }),
  ]);
  await queryClient.refetchQueries({
    predicate: (query) => query.queryKey[0] === 'usageStats',
    type: 'active',
  });
}

export function useUserPanelAPIToken() {
  return useQuery({
    queryKey: userPanelTokenKeys.detail(),
    queryFn: () => getTransport().getUserPanelAPIToken(),
  });
}

export function useCreateUserPanelAPIToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().createUserPanelAPIToken(),
    onSuccess: () => refreshUserPanelTokenDependents(queryClient),
  });
}

export function useRegenerateUserPanelAPIToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().regenerateUserPanelAPIToken(),
    onSuccess: () => refreshUserPanelTokenDependents(queryClient),
  });
}

export function useRevealUserPanelAPIToken() {
  return useMutation({
    mutationFn: () => getTransport().revealUserPanelAPIToken(),
  });
}

export function useUserPanelAvailableModels(enabled = true) {
  return useQuery({
    queryKey: userPanelTokenKeys.availableModels(),
    queryFn: () => getTransport().getUserPanelAvailableModels(),
    enabled,
  });
}

export function useUserPanelDailyCheckInStatus(enabled = true, dayKey?: string) {
  return useQuery({
    queryKey: userPanelTokenKeys.dailyCheckInStatus(dayKey),
    queryFn: () => getTransport().getUserPanelDailyCheckInStatus(),
    enabled,
  });
}

export function useUserPanelDailyCheckIn() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => getTransport().checkInUserPanelDailyQuota(),
    onSuccess: () => refreshUserPanelTokenDependents(queryClient),
  });
}

export function useUserPanelAvailableModelRoutes(enabled = true) {
  return useQuery({
    queryKey: userPanelTokenKeys.availableModelRoutes(),
    queryFn: () => getTransport().getUserPanelAvailableModelRoutes(),
    enabled,
  });
}

export function useUserPanelAnnouncement(enabled = true) {
  return useQuery({
    queryKey: userPanelTokenKeys.announcement(),
    queryFn: () => getTransport().getUserPanelAnnouncement(),
    enabled,
  });
}

export function useUserPanelConsumptionLeaderboard(enabled = true) {
  const queryClient = useQueryClient();
  const refetchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!enabled) return;

    const transport = getTransport();
    const invalidateLeaderboard = () => {
      void queryClient.invalidateQueries({
        queryKey: userPanelTokenKeys.consumptionLeaderboard(),
      });
    };
    const unsubscribeDirty = transport.subscribe('user_panel_consumption_leaderboard_dirty', () => {
      if (refetchTimerRef.current) return;
      refetchTimerRef.current = setTimeout(() => {
        refetchTimerRef.current = null;
        invalidateLeaderboard();
      }, 1000);
    });
    const unsubscribeReconnect = transport.subscribe('_ws_reconnected', invalidateLeaderboard);

    return () => {
      unsubscribeDirty();
      unsubscribeReconnect();
      if (refetchTimerRef.current) {
        clearTimeout(refetchTimerRef.current);
        refetchTimerRef.current = null;
      }
    };
  }, [enabled, queryClient]);

  return useQuery({
    queryKey: userPanelTokenKeys.consumptionLeaderboard(),
    queryFn: () => getTransport().getUserPanelConsumptionLeaderboard(),
    enabled,
  });
}
