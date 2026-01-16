/**
 * Route React Query Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport, type Route, type CreateRouteData } from '@/lib/transport';

// Query Keys
export const routeKeys = {
  all: ['routes'] as const,
  lists: () => [...routeKeys.all, 'list'] as const,
  list: () => [...routeKeys.lists()] as const,
  details: () => [...routeKeys.all, 'detail'] as const,
  detail: (id: number) => [...routeKeys.details(), id] as const,
};

// 获取所有 Routes
export function useRoutes() {
  return useQuery({
    queryKey: routeKeys.list(),
    queryFn: () => getTransport().getRoutes(),
  });
}

// 获取单个 Route
export function useRoute(id: number) {
  return useQuery({
    queryKey: routeKeys.detail(id),
    queryFn: () => getTransport().getRoute(id),
    enabled: id > 0,
  });
}

// 创建 Route
export function useCreateRoute() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateRouteData) => getTransport().createRoute(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}

// 更新 Route
export function useUpdateRoute() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Route> }) =>
      getTransport().updateRoute(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: routeKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}

// 删除 Route
export function useDeleteRoute() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => getTransport().deleteRoute(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}

// 切换 Route 启用状态
export function useToggleRoute() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => {
      const transport = getTransport();
      // 先获取当前状态，然后切换
      return transport.getRoute(id).then((route) =>
        transport.updateRoute(id, { isEnabled: !route.isEnabled })
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}

// 批量更新 Route 位置
export function useUpdateRoutePositions() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (updates: Record<number, number>) => {
      const transport = getTransport();
      // 转换为 { id, position } 数组
      const positionUpdates = Object.entries(updates).map(([id, position]) => ({
        id: Number(id),
        position,
      }));
      return transport.batchUpdateRoutePositions(positionUpdates);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: routeKeys.lists() });
    },
  });
}
