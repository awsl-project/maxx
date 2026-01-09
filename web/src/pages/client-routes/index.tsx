/**
 * Client Routes Page
 * 显示特定客户端类型的路由配置 - 使用拖拽卡片布局
 */

import { useState, useMemo } from 'react';
import { useParams } from 'react-router-dom';
import { Plus, RefreshCw } from 'lucide-react';
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
import { useRoutes, useProviders, useCreateRoute, useToggleRoute, useDeleteRoute, useUpdateRoutePositions, useProviderStats } from '@/hooks/queries';
import { useStreamingRequests } from '@/hooks/use-streaming';
import { ClientIcon, getClientName } from '@/components/icons/client-icons';
import { getProviderColor } from '@/lib/provider-colors';
import type { ClientType, Provider } from '@/lib/transport';
import type { ProviderConfigItem } from './types';
import { SortableProviderRow, ProviderRowContent } from './components/provider-row';

export function ClientRoutesPage() {
  const { clientType } = useParams<{ clientType: string }>();
  const [activeId, setActiveId] = useState<string | null>(null);

  const { data: allRoutes, isLoading: routesLoading } = useRoutes();
  const { data: providers = [], isLoading: providersLoading } = useProviders();
  const { data: providerStats = {} } = useProviderStats(clientType);
  const { countsByProvider } = useStreamingRequests();

  const createRoute = useCreateRoute();
  const toggleRoute = useToggleRoute();
  const deleteRoute = useDeleteRoute();
  const updatePositions = useUpdateRoutePositions();

  const loading = routesLoading || providersLoading;

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  // 获取所有 Provider，区分原生支持和转换支持
  const sortedItems = useMemo((): ProviderConfigItem[] => {
    if (!clientType) return [];

    const clientRoutes = allRoutes?.filter((r) => r.clientType === clientType) || [];

    const items = providers.map((provider) => {
      const route = clientRoutes.find((r) => Number(r.providerID) === Number(provider.id)) || null;
      const isNative = provider.supportedClientTypes.includes(clientType as ClientType);
      return {
        id: `provider-${provider.id}`,
        provider,
        route,
        enabled: route?.isEnabled ?? false,
        isNative,
      };
    });

    const filteredItems = items.filter((item) => item.isNative || item.route);

    return filteredItems.sort((a, b) => {
      if (a.route && b.route) return a.route.position - b.route.position;
      if (a.route && !b.route) return -1;
      if (!a.route && b.route) return 1;
      if (a.isNative && !b.isNative) return -1;
      if (!a.isNative && b.isNative) return 1;
      return a.provider.name.localeCompare(b.provider.name);
    });
  }, [clientType, providers, allRoutes]);

  // 获取可以添加转换 Route 的 Provider
  const availableForConversion = useMemo(() => {
    if (!clientType) return [];
    const clientRoutes = allRoutes?.filter((r) => r.clientType === clientType) || [];
    return providers.filter((p) => {
      const hasRoute = clientRoutes.some((r) => Number(r.providerID) === Number(p.id));
      const isNative = p.supportedClientTypes.includes(clientType as ClientType);
      return !isNative && !hasRoute;
    });
  }, [clientType, providers, allRoutes]);

  const activeItem = activeId ? sortedItems.find((item) => item.id === activeId) : null;

  const handleToggle = (item: ProviderConfigItem) => {
    if (item.route) {
      toggleRoute.mutate(item.route.id);
    } else {
      createRoute.mutate({
        isEnabled: true,
        isNative: item.isNative,
        projectID: 0,
        clientType: clientType as ClientType,
        providerID: item.provider.id,
        position: sortedItems.length + 1,
        retryConfigID: 0,
      });
    }
  };

  const handleAddConvertedRoute = (provider: Provider) => {
    createRoute.mutate({
      isEnabled: true,
      isNative: false,
      projectID: 0,
      clientType: clientType as ClientType,
      providerID: provider.id,
      position: sortedItems.length + 1,
      retryConfigID: 0,
    });
  };

  const handleDeleteRoute = (routeId: number) => {
    deleteRoute.mutate(routeId);
  };

  const handleDragStart = (event: DragStartEvent) => {
    setActiveId(event.active.id as string);
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    setActiveId(null);

    if (!over || active.id === over.id) return;

    const oldIndex = sortedItems.findIndex((item) => item.id === active.id);
    const newIndex = sortedItems.findIndex((item) => item.id === over.id);

    if (oldIndex === -1 || newIndex === -1) return;

    const newItems = arrayMove(sortedItems, oldIndex, newIndex);

    for (let i = 0; i < newItems.length; i++) {
      const item = newItems[i];
      if (!item.route) {
        await createRoute.mutateAsync({
          isEnabled: false,
          isNative: item.isNative,
          projectID: 0,
          clientType: clientType as ClientType,
          providerID: item.provider.id,
          position: i + 1,
          retryConfigID: 0,
        });
      }
    }

    const updates: Record<number, number> = {};
    newItems.forEach((item, i) => {
      if (item.route) {
        updates[item.route.id] = i + 1;
      }
    });

    if (Object.keys(updates).length > 0) {
      updatePositions.mutate(updates);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-text-muted">Loading...</div>
      </div>
    );
  }

  if (!clientType) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-text-muted">Client type not found</div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between p-lg border-b border-border bg-surface-primary">
        <div className="flex items-center gap-md">
          <ClientIcon type={clientType as ClientType} size={32} />
          <div>
            <h2 className="text-headline font-semibold text-text-primary">{getClientName(clientType as ClientType)}</h2>
            <p className="text-caption text-text-muted">
              Configure routing priority for {getClientName(clientType as ClientType)} requests
            </p>
          </div>
        </div>
      </div>

      {/* Provider List */}
      <div className="flex-1 overflow-y-auto p-lg">
        {sortedItems.length === 0 && availableForConversion.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-text-muted">
            <p className="text-body">No providers available</p>
            <p className="text-caption mt-sm">Add a provider first to configure routes</p>
          </div>
        ) : (
          <>
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragStart={handleDragStart}
              onDragEnd={handleDragEnd}
            >
              <SortableContext items={sortedItems} strategy={verticalListSortingStrategy}>
                <div className="space-y-sm">
                  {sortedItems.map((item, index) => (
                    <SortableProviderRow
                      key={item.id}
                      item={item}
                      index={index}
                      clientType={clientType as ClientType}
                      streamingCount={item.route ? countsByProvider.get(item.route.id) || 0 : 0}
                      stats={providerStats[item.provider.id]}
                      isToggling={toggleRoute.isPending || createRoute.isPending}
                      onToggle={() => handleToggle(item)}
                      onDelete={item.route && !item.isNative ? () => handleDeleteRoute(item.route!.id) : undefined}
                    />
                  ))}
                </div>
              </SortableContext>

              <DragOverlay dropAnimation={null}>
                {activeItem && (
                  <ProviderRowContent
                    item={activeItem}
                    index={sortedItems.findIndex((i) => i.id === activeItem.id)}
                    clientType={clientType as ClientType}
                    streamingCount={activeItem.route ? countsByProvider.get(activeItem.route.id) || 0 : 0}
                    stats={providerStats[activeItem.provider.id]}
                    isToggling={false}
                    isOverlay
                    onToggle={() => {}}
                  />
                )}
              </DragOverlay>
            </DndContext>

            {/* Add Converted Route Section */}
            {availableForConversion.length > 0 && (
              <div className="mt-lg pt-lg border-t border-border/50">
                <div className="flex items-center gap-2 mb-md">
                  <RefreshCw size={14} className="text-amber-400" />
                  <span className="text-caption font-medium text-text-muted">添加转换路由 (API Conversion)</span>
                </div>
                <div className="space-y-sm">
                  {availableForConversion.map((provider) => (
                    <button
                      key={provider.id}
                      onClick={() => handleAddConvertedRoute(provider)}
                      disabled={createRoute.isPending}
                      className="w-full flex items-center gap-md p-md rounded-lg border border-dashed border-border/50 bg-surface-secondary/30 hover:bg-surface-hover hover:border-amber-400/30 transition-all text-left"
                    >
                      <div
                        className="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 opacity-50"
                        style={{
                          backgroundColor: `${getProviderColor(provider.type)}15`,
                          color: getProviderColor(provider.type),
                        }}
                      >
                        <span className="text-sm font-bold">{provider.name.charAt(0).toUpperCase()}</span>
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="text-body text-text-muted">{provider.name}</div>
                        <div className="text-caption text-text-muted/50">点击添加为转换路由</div>
                      </div>
                      <Plus size={16} className="text-text-muted" />
                    </button>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
