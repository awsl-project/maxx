/**
 * Shared Client Type Routes Content Component
 * Used by both global routes and project routes
 */

import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Plus, RefreshCw, Zap } from 'lucide-react';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
  DragOverlay,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import {
  useRoutes,
  useProviders,
  useCreateRoute,
  useToggleRoute,
  useDeleteRoute,
  useUpdateRoutePositions,
  useProviderStats,
  useProxyRequestUpdates,
  routeKeys,
} from '@/hooks/queries';
import { useQueryClient } from '@tanstack/react-query';
import { useStreamingRequests } from '@/hooks/use-streaming';
import { getClientName, getClientColor } from '@/components/icons/client-icons';
import { getProviderColor, type ProviderType } from '@/lib/theme';
import type { ClientType, Provider } from '@/lib/transport';
import {
  SortableProviderRow,
  ProviderRowContent,
} from '@/pages/client-routes/components/provider-row';
import type { ProviderConfigItem } from '@/pages/client-routes/types';
import { Button } from '../ui';
import { AntigravityQuotasProvider } from '@/contexts/antigravity-quotas-context';
import { CooldownsProvider } from '@/contexts/cooldowns-context';

type ProviderTypeKey = 'antigravity' | 'kiro' | 'codex' | 'custom';

const PROVIDER_TYPE_ORDER: ProviderTypeKey[] = ['antigravity', 'kiro', 'codex', 'custom'];

const PROVIDER_TYPE_LABELS: Record<Exclude<ProviderTypeKey, 'custom'>, string> = {
  antigravity: 'Antigravity',
  kiro: 'Kiro',
  codex: 'Codex',
};

interface ClientTypeRoutesContentProps {
  clientType: ClientType;
  projectID: number; // 0 for global routes
  searchQuery?: string; // Optional search query from parent
}

// Wrapper component that provides the AntigravityQuotasProvider and CooldownsProvider
export function ClientTypeRoutesContent(props: ClientTypeRoutesContentProps) {
  return (
    <AntigravityQuotasProvider>
      <CooldownsProvider>
        <ClientTypeRoutesContentInner {...props} />
      </CooldownsProvider>
    </AntigravityQuotasProvider>
  );
}

// Inner component that can access the contexts
function ClientTypeRoutesContentInner({
  clientType,
  projectID,
  searchQuery = '',
}: ClientTypeRoutesContentProps) {
  const { t } = useTranslation();
  const [activeId, setActiveId] = useState<string | null>(null);
  const { data: providerStats = {} } = useProviderStats(clientType, projectID || undefined);
  const queryClient = useQueryClient();

  // 订阅请求更新事件，确保 providerStats 实时刷新
  useProxyRequestUpdates();

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const { data: allRoutes, isLoading: routesLoading } = useRoutes();
  const { data: providers = [], isLoading: providersLoading } = useProviders();
  const { countsByProviderAndClient } = useStreamingRequests();

  const createRoute = useCreateRoute();
  const toggleRoute = useToggleRoute();
  const deleteRoute = useDeleteRoute();
  const updatePositions = useUpdateRoutePositions();

  const loading = routesLoading || providersLoading;

  // Get routes for this clientType and projectID
  const clientRoutes = useMemo(() => {
    return allRoutes?.filter((r) => r.clientType === clientType && r.projectID === projectID) || [];
  }, [allRoutes, clientType, projectID]);

  // Build provider config items
  const items = useMemo((): ProviderConfigItem[] => {
    const allItems = providers.map((provider) => {
      const route = clientRoutes.find((r) => Number(r.providerID) === Number(provider.id)) || null;
      const isNative = (provider.supportedClientTypes || []).includes(clientType);
      return {
        id: `${clientType}-provider-${provider.id}`,
        provider,
        route,
        enabled: route?.isEnabled ?? false,
        isNative,
      };
    });

    // Only show providers that have routes
    let filteredItems = allItems.filter((item) => item.route);

    // Apply search filter
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      filteredItems = filteredItems.filter(
        (item) =>
          item.provider.name.toLowerCase().includes(query) ||
          item.provider.type.toLowerCase().includes(query),
      );
    }

    return filteredItems.sort((a, b) => {
      if (a.route && b.route) return a.route.position - b.route.position;
      if (a.route && !b.route) return -1;
      if (!a.route && b.route) return 1;
      if (a.isNative && !b.isNative) return -1;
      if (!a.isNative && b.isNative) return 1;
      return a.provider.name.localeCompare(b.provider.name);
    });
  }, [providers, clientRoutes, clientType, searchQuery]);

  // Get available providers (without routes yet), grouped by type and sorted alphabetically
  const groupedAvailableProviders = useMemo((): Record<ProviderTypeKey, Provider[]> => {
    const groups: Record<ProviderTypeKey, Provider[]> = {
      antigravity: [],
      kiro: [],
      codex: [],
      custom: [],
    };

    let available = providers.filter((p) => {
      const hasRoute = clientRoutes.some((r) => Number(r.providerID) === Number(p.id));
      return !hasRoute;
    });

    // Apply search filter
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      available = available.filter(
        (p) => p.name.toLowerCase().includes(query) || p.type.toLowerCase().includes(query),
      );
    }

    // Group by type
    available.forEach((p) => {
      const type = p.type as ProviderTypeKey;
      if (groups[type]) {
        groups[type].push(p);
      } else {
        groups.custom.push(p);
      }
    });

    // Sort alphabetically within each group
    for (const key of Object.keys(groups) as ProviderTypeKey[]) {
      groups[key].sort((a, b) => a.name.localeCompare(b.name));
    }

    return groups;
  }, [providers, clientRoutes, searchQuery]);

  // Check if there are any available providers
  const hasAvailableProviders = useMemo(() => {
    return PROVIDER_TYPE_ORDER.some((type) => groupedAvailableProviders[type].length > 0);
  }, [groupedAvailableProviders]);

  const activeItem = activeId ? items.find((item) => item.id === activeId) : null;

  const handleToggle = (item: ProviderConfigItem) => {
    if (item.route) {
      toggleRoute.mutate(item.route.id);
    } else {
      createRoute.mutate({
        isEnabled: true,
        isNative: item.isNative,
        projectID,
        clientType,
        providerID: item.provider.id,
        position: items.length + 1,
        retryConfigID: 0,
      });
    }
  };

  const handleAddRoute = (provider: Provider, isNative: boolean) => {
    createRoute.mutate({
      isEnabled: true,
      isNative,
      projectID,
      clientType,
      providerID: provider.id,
      position: items.length + 1,
      retryConfigID: 0,
    });
  };

  const handleDeleteRoute = (routeId: number) => {
    deleteRoute.mutate(routeId);
  };

  const handleDragStart = (event: DragStartEvent) => {
    setActiveId(event.active.id as string);
    document.body.classList.add('is-dragging');
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    setActiveId(null);
    document.body.classList.remove('is-dragging');

    if (!over || active.id === over.id) return;

    const oldIndex = items.findIndex((item) => item.id === active.id);
    const newIndex = items.findIndex((item) => item.id === over.id);

    if (oldIndex === -1 || newIndex === -1) return;

    const newItems = arrayMove(items, oldIndex, newIndex);

    // Update positions for all items
    const updates: Record<number, number> = {};
    newItems.forEach((item, i) => {
      if (item.route) {
        updates[item.route.id] = i + 1;
      }
    });

    if (Object.keys(updates).length > 0) {
      // 乐观更新：立即更新本地缓存
      queryClient.setQueryData(routeKeys.list(), (oldRoutes: typeof allRoutes) => {
        if (!oldRoutes) return oldRoutes;
        return oldRoutes.map((route) => {
          const newPosition = updates[route.id];
          if (newPosition !== undefined) {
            return { ...route, position: newPosition };
          }
          return route;
        });
      });

      // 发送 API 请求
      updatePositions.mutate(updates, {
        onError: () => {
          // 失败时回滚：重新获取服务器数据
          queryClient.invalidateQueries({ queryKey: routeKeys.list() });
        },
      });
    }
  };

  const color = getClientColor(clientType);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full p-12">
        <div className="text-muted-foreground">{t('common.loading')}</div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full min-h-0">
      <div className="flex-1 overflow-y-auto px-6 py-6">
        <div className="mx-auto max-w-[1400px] space-y-6">
          {/* Routes List */}
          {items.length > 0 ? (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragStart={handleDragStart}
              onDragEnd={handleDragEnd}
            >
              <SortableContext
                items={items.map((item) => item.id)}
                strategy={verticalListSortingStrategy}
              >
                <div className="space-y-2">
                  {items.map((item, index) => (
                    <SortableProviderRow
                      key={item.id}
                      item={item}
                      index={index}
                      clientType={clientType}
                      streamingCount={
                        countsByProviderAndClient.get(`${item.provider.id}:${clientType}`) || 0
                      }
                      stats={providerStats[item.provider.id]}
                      isToggling={toggleRoute.isPending || createRoute.isPending}
                      onToggle={() => handleToggle(item)}
                      onDelete={item.route ? () => handleDeleteRoute(item.route!.id) : undefined}
                    />
                  ))}
                </div>
              </SortableContext>

              <DragOverlay dropAnimation={null}>
                {activeItem && (
                  <ProviderRowContent
                    item={activeItem}
                    index={items.findIndex((i) => i.id === activeItem.id)}
                    clientType={clientType}
                    streamingCount={
                      countsByProviderAndClient.get(`${activeItem.provider.id}:${clientType}`) ||
                      0
                    }
                    stats={providerStats[activeItem.provider.id]}
                    isToggling={false}
                    isOverlay
                    onToggle={() => {}}
                  />
                )}
              </DragOverlay>
            </DndContext>
          ) : (
            <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
              <p className="text-body">
                {t('routes.noRoutesForClient', { client: getClientName(clientType) })}
              </p>
              <p className="text-caption mt-sm">{t('routes.addRouteToGetStarted')}</p>
            </div>
          )}

          {/* Add Route Section - Grouped by Type */}
          {hasAvailableProviders && (
            <div className="pt-4 border-t border-border/50 ">
              <div className="flex items-center gap-2 mb-6">
                <Plus size={14} style={{ color }} />
                <span className="text-caption font-medium text-muted-foreground">
                  {t('routes.availableProviders')}
                </span>
              </div>
              <div className="space-y-6">
                {PROVIDER_TYPE_ORDER.map((typeKey) => {
                  const typeProviders = groupedAvailableProviders[typeKey];
                  if (typeProviders.length === 0) return null;

                  return (
                    <div key={typeKey}>
                      <div className="flex items-center gap-2 mb-3">
                        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                          {typeKey === 'custom'
                            ? t('routes.providerType.custom')
                            : PROVIDER_TYPE_LABELS[typeKey]}
                        </span>
                        <div className="h-px flex-1 bg-border/50" />
                      </div>
                      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                        {typeProviders.map((provider) => {
                          const isNative = (provider.supportedClientTypes || []).includes(clientType);
                          const providerColor = getProviderColor(provider.type as ProviderType);
                          return (
                            <Button
                              key={provider.id}
                              variant={null}
                              onClick={() => handleAddRoute(provider, isNative)}
                              disabled={createRoute.isPending}
                              className="h-auto group relative flex items-center justify-between gap-4 p-4 rounded-xl border border-border/40 bg-background hover:bg-secondary/50 hover:border-border shadow-sm hover:shadow transition-all duration-300 text-left disabled:opacity-50 disabled:cursor-not-allowed overflow-hidden"
                            >
                              {/* Left: Provider Icon & Info */}
                              <div className="flex items-center gap-3 flex-1 min-w-0">
                                <div
                                  className="relative w-11 h-11 rounded-lg flex items-center justify-center shrink-0 transition-all duration-300 group-hover:scale-105"
                                  style={{
                                    backgroundColor: `${providerColor}20`,
                                    color: providerColor,
                                  }}
                                >
                                  <span className="relative text-xl font-black">
                                    {provider.name.charAt(0).toUpperCase()}
                                  </span>
                                </div>
                                <div className="flex-1 min-w-0">
                                  <div className="text-[14px] font-semibold text-foreground truncate leading-tight mb-1">
                                    {provider.name}
                                  </div>
                                  <div className="flex items-center gap-2">
                                    {isNative ? (
                                      <span className="flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-bold bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 whitespace-nowrap">
                                        <Zap size={10} className="fill-current opacity-30" />
                                        NATIVE
                                      </span>
                                    ) : (
                                      <span className="flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-bold bg-amber-500/15 text-amber-600 dark:text-amber-400 whitespace-nowrap">
                                        <RefreshCw size={10} />
                                        CONV
                                      </span>
                                    )}
                                  </div>
                                </div>
                              </div>

                              {/* Right: Add Icon */}
                              <Plus
                                size={20}
                                style={{ color: providerColor }}
                                className="opacity-50 group-hover:opacity-100 group-hover:scale-110 transition-all duration-300 shrink-0"
                              />
                            </Button>
                          );
                        })}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
