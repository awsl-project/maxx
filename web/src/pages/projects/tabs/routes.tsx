/**
 * Project Routes Tab
 * 显示项目特定的路由配置 - 左侧 ClientType Sidebar + 右侧拖拽卡片布局
 */

import { useState, useMemo } from 'react';
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
import { useRoutes, useProviders, useCreateRoute, useToggleRoute, useDeleteRoute, useUpdateRoutePositions, useProviderStats, useUpdateProject, projectKeys } from '@/hooks/queries';
import { useStreamingRequests } from '@/hooks/use-streaming';
import { ClientIcon, getClientName, getClientColor } from '@/components/icons/client-icons';
import { getProviderColor } from '@/lib/provider-colors';
import { cn } from '@/lib/utils';
import { Switch } from '@/components/ui';
import type { Project, ClientType, Provider, Route } from '@/lib/transport';
import { SortableProviderRow, ProviderRowContent } from '@/pages/client-routes/components/provider-row';
import type { ProviderConfigItem } from '@/pages/client-routes/types';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { useQueryClient } from '@tanstack/react-query';

// 支持的客户端类型列表
const CLIENT_TYPES: ClientType[] = ['claude', 'openai', 'codex', 'gemini'];

interface RoutesTabProps {
  project: Project;
}

// 单个客户端类型的路由内容
interface ClientTypeContentProps {
  clientType: ClientType;
  project: Project;
  projectRoutes: Route[];
  providers: Provider[];
  createRoute: ReturnType<typeof useCreateRoute>;
  toggleRoute: ReturnType<typeof useToggleRoute>;
  deleteRoute: ReturnType<typeof useDeleteRoute>;
  updatePositions: ReturnType<typeof useUpdateRoutePositions>;
  countsByProviderAndClient: Map<string, number>;
  isCustomRoutesEnabled: boolean;
  onToggleCustomRoutes: (enabled: boolean) => void;
}

function ClientTypeContent({
  clientType,
  project,
  projectRoutes,
  providers,
  createRoute,
  toggleRoute,
  deleteRoute,
  updatePositions,
  countsByProviderAndClient,
  isCustomRoutesEnabled,
  onToggleCustomRoutes,
}: ClientTypeContentProps) {
  const [activeId, setActiveId] = useState<string | null>(null);
  const { data: providerStats = {} } = useProviderStats(clientType, project.id);

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

  // 为当前客户端类型构建 ProviderConfigItem 列表
  const items = useMemo((): ProviderConfigItem[] => {
    const clientRoutes = projectRoutes.filter((r) => r.clientType === clientType);

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

    // 只显示有路由的 provider
    const filteredItems = allItems.filter((item) => item.route);

    return filteredItems.sort((a, b) => {
      if (a.route && b.route) return a.route.position - b.route.position;
      if (a.route && !b.route) return -1;
      if (!a.route && b.route) return 1;
      if (a.isNative && !b.isNative) return -1;
      if (!a.isNative && b.isNative) return 1;
      return a.provider.name.localeCompare(b.provider.name);
    });
  }, [projectRoutes, providers, clientType]);

  // 获取可以添加路由的 Provider
  const availableProviders = useMemo((): Provider[] => {
    const clientRoutes = projectRoutes.filter((r) => r.clientType === clientType);
    return providers.filter((p) => {
      const hasRoute = clientRoutes.some((r) => Number(r.providerID) === Number(p.id));
      return !hasRoute;
    });
  }, [projectRoutes, providers, clientType]);

  const activeItem = activeId ? items.find((item) => item.id === activeId) : null;

  const handleToggle = (item: ProviderConfigItem) => {
    if (item.route) {
      toggleRoute.mutate(item.route.id);
    } else {
      createRoute.mutate({
        isEnabled: true,
        isNative: item.isNative,
        projectID: project.id,
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
      projectID: project.id,
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
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    setActiveId(null);

    if (!over || active.id === over.id) return;

    const oldIndex = items.findIndex((item) => item.id === active.id);
    const newIndex = items.findIndex((item) => item.id === over.id);

    if (oldIndex === -1 || newIndex === -1) return;

    const newItems = arrayMove(items, oldIndex, newIndex);

    // 创建缺失的路由
    for (let i = 0; i < newItems.length; i++) {
      const item = newItems[i];
      if (!item.route) {
        await createRoute.mutateAsync({
          isEnabled: false,
          isNative: item.isNative,
          projectID: project.id,
          clientType,
          providerID: item.provider.id,
          position: i + 1,
          retryConfigID: 0,
        });
      }
    }

    // 更新位置
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

  const color = getClientColor(clientType);

  return (
    <div className="flex flex-col h-full">
      {/* Header with Toggle */}
      <div className="flex items-center justify-between px-lg py-4 border-b border-border bg-surface-primary">
        <div className="flex items-center gap-md">
          <ClientIcon type={clientType} size={32} />
          <div>
            <h2 className="text-lg font-semibold text-text-primary">{getClientName(clientType)}</h2>
            <p className="text-xs text-text-muted">
              {isCustomRoutesEnabled
                ? `${items.length} route${items.length !== 1 ? 's' : ''} configured for this project`
                : 'Using global routes'
              }
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm text-text-secondary">Custom Routes</span>
          <Switch
            checked={isCustomRoutesEnabled}
            onCheckedChange={onToggleCustomRoutes}
          />
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-lg py-6">
        <div className="mx-auto max-w-[1400px] space-y-6">
          {/* Disabled state - show prompt */}
          {!isCustomRoutesEnabled ? (
            <div className="flex flex-col items-center justify-center py-24 text-center space-y-6">
              <div className="p-6 rounded-full bg-surface-secondary/50">
                <ClientIcon type={clientType} size={48} className="opacity-30" />
              </div>
              <div className="space-y-3">
                <h3 className="text-lg font-semibold text-text-primary">
                  Custom Routes Disabled
                </h3>
                <p className="text-sm text-text-secondary leading-relaxed">
                  {getClientName(clientType)} is currently using global routes.
                  Toggle the switch above to enable project-specific routing for this client type.
                </p>
              </div>
            </div>
          ) : (
            <>
              {/* Routes List */}
              {items.length > 0 ? (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragStart={handleDragStart}
              onDragEnd={handleDragEnd}
            >
              <SortableContext items={items} strategy={verticalListSortingStrategy}>
                <div className="space-y-sm">
                  {items.map((item, index) => (
                    <SortableProviderRow
                      key={item.id}
                      item={item}
                      index={index}
                      clientType={clientType}
                      streamingCount={countsByProviderAndClient.get(`${item.provider.id}:${clientType}`) || 0}
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
                    streamingCount={countsByProviderAndClient.get(`${activeItem.provider.id}:${clientType}`) || 0}
                    stats={providerStats[activeItem.provider.id]}
                    isToggling={false}
                    isOverlay
                    onToggle={() => {}}
                  />
                )}
              </DragOverlay>
            </DndContext>
          ) : (
            <div className="flex flex-col items-center justify-center py-16 text-text-muted">
              <p className="text-body">No routes configured for {getClientName(clientType)}</p>
              <p className="text-caption mt-sm">Add a route below to get started</p>
            </div>
          )}

          {/* Add Route Section */}
          {availableProviders.length > 0 && (
            <div className="pt-4 border-t border-border/50">
              <div className="flex items-center gap-2 mb-md">
                <Plus size={14} style={{ color }} />
                <span className="text-caption font-medium text-text-muted">Add Route</span>
              </div>
              <div className="space-y-sm">
                {availableProviders.map((provider) => {
                  const isNative = (provider.supportedClientTypes || []).includes(clientType);
                  return (
                    <button
                      key={provider.id}
                      onClick={() => handleAddRoute(provider, isNative)}
                      disabled={createRoute.isPending}
                      className="w-full flex items-center gap-md p-md rounded-lg border border-dashed border-border/50 bg-surface-secondary/30 hover:bg-surface-hover transition-all text-left"
                      style={{ '--hover-border-color': `${color}40` } as React.CSSProperties}
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
                        <div className="flex items-center gap-2">
                          <span className="text-body text-text-muted">{provider.name}</span>
                          {isNative ? (
                            <span className="text-[9px] font-bold text-emerald-500">NATIVE</span>
                          ) : (
                            <span className="flex items-center gap-0.5 text-[9px] font-bold text-amber-500">
                              <RefreshCw size={8} /> CONV
                            </span>
                          )}
                        </div>
                        <div className="text-caption text-text-muted/50">Click to add route</div>
                      </div>
                      <Plus size={16} className="text-text-muted" />
                    </button>
                  );
                })}
              </div>
            </div>
          )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

// ClientType Sidebar Item
function ClientTypeSidebarItem({
  clientType,
  isActive,
  onClick,
  routeCount,
  streamingCount,
}: {
  clientType: ClientType;
  isActive: boolean;
  onClick: () => void;
  routeCount: number;
  streamingCount: number;
}) {
  const color = getClientColor(clientType);

  return (
    <button
      onClick={onClick}
      className={cn(
        'sidebar-item text-left relative overflow-hidden',
        isActive && 'sidebar-item-active'
      )}
    >
      {/* Marquee 背景动画 (仅在有 streaming 请求且未激活时显示) */}
      {streamingCount > 0 && !isActive && (
        <div
          className="absolute inset-0 animate-marquee pointer-events-none opacity-50"
          style={{ backgroundColor: `${color}10` }}
        />
      )}
      <ClientIcon type={clientType} size={18} className="relative z-10" />
      <span className="flex-1 relative z-10 text-body">{getClientName(clientType)}</span>
      {routeCount > 0 && (
        <span className="text-[10px] font-mono text-text-muted relative z-10">{routeCount}</span>
      )}
      <StreamingBadge count={streamingCount} color={color} />
    </button>
  );
}

export function RoutesTab({ project }: RoutesTabProps) {
  const [activeClientType, setActiveClientType] = useState<ClientType>('claude');
  const queryClient = useQueryClient();

  const { data: allRoutes, isLoading: routesLoading } = useRoutes();
  const { data: providers = [], isLoading: providersLoading } = useProviders();
  const { countsByRoute, countsByProviderAndClient } = useStreamingRequests();

  const createRoute = useCreateRoute();
  const toggleRoute = useToggleRoute();
  const deleteRoute = useDeleteRoute();
  const updatePositions = useUpdateRoutePositions();
  const updateProject = useUpdateProject();

  const loading = routesLoading || providersLoading;

  // 获取项目的路由
  const projectRoutes = useMemo(() => {
    return allRoutes?.filter((r) => r.projectID === project.id) || [];
  }, [allRoutes, project.id]);

  // 获取每个 ClientType 的路由数量
  const getRouteCount = (clientType: ClientType) => {
    return projectRoutes.filter((r) => r.clientType === clientType).length;
  };

  // 获取每个 ClientType 的 streaming 请求数（只统计当前项目的路由）
  const getStreamingCount = (clientType: ClientType) => {
    const clientRoutes = projectRoutes.filter((r) => r.clientType === clientType);
    let count = 0;
    for (const route of clientRoutes) {
      count += countsByRoute.get(route.id) || 0;
    }
    return count;
  };

  // 检查某个 ClientType 是否启用了自定义路由
  const isCustomRoutesEnabled = (clientType: ClientType): boolean => {
    return (project.enabledCustomRoutes || []).includes(clientType);
  };

  // 切换 ClientType 的自定义路由开关
  const handleToggleCustomRoutes = (clientType: ClientType, enabled: boolean) => {
    const currentEnabled = project.enabledCustomRoutes || [];
    let newEnabled: ClientType[];

    if (enabled) {
      // 添加到列表
      newEnabled = [...currentEnabled, clientType];
    } else {
      // 从列表移除
      newEnabled = currentEnabled.filter(ct => ct !== clientType);
    }

    updateProject.mutate(
      { id: project.id, data: { name: project.name, slug: project.slug, enabledCustomRoutes: newEnabled } },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
          queryClient.invalidateQueries({ queryKey: projectKeys.slug(project.slug) });
        },
      }
    );
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full p-6">
        <div className="text-text-muted">Loading...</div>
      </div>
    );
  }

  return (
    <div className="flex h-full overflow-hidden">
      {/* Left Sidebar - ClientType List */}
      <aside className="w-[200px] shrink-0 flex flex-col border-r border-border bg-surface-primary">
        <div className="sidebar-section-title">Client Types</div>
        <nav className="flex-1 flex flex-col overflow-y-auto py-md">
          {CLIENT_TYPES.map((clientType) => (
            <ClientTypeSidebarItem
              key={clientType}
              clientType={clientType}
              isActive={activeClientType === clientType}
              onClick={() => setActiveClientType(clientType)}
              routeCount={getRouteCount(clientType)}
              streamingCount={getStreamingCount(clientType)}
            />
          ))}
        </nav>
      </aside>

      {/* Main Content */}
      <main className="flex-1 min-w-0 overflow-hidden">
        <ClientTypeContent
          clientType={activeClientType}
          project={project}
          projectRoutes={projectRoutes}
          providers={providers}
          createRoute={createRoute}
          toggleRoute={toggleRoute}
          deleteRoute={deleteRoute}
          updatePositions={updatePositions}
          countsByProviderAndClient={countsByProviderAndClient}
          isCustomRoutesEnabled={isCustomRoutesEnabled(activeClientType)}
          onToggleCustomRoutes={(enabled) => handleToggleCustomRoutes(activeClientType, enabled)}
        />
      </main>
    </div>
  );
}
