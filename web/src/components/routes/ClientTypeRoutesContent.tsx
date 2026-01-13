/**
 * Shared Client Type Routes Content Component
 * Used by both global routes and project routes
 */

import { useState, useMemo } from 'react'
import { Plus, RefreshCw, Zap } from 'lucide-react'
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
} from '@dnd-kit/core'
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import {
  useRoutes,
  useProviders,
  useCreateRoute,
  useToggleRoute,
  useDeleteRoute,
  useUpdateRoutePositions,
  useProviderStats,
} from '@/hooks/queries'
import { useStreamingRequests } from '@/hooks/use-streaming'
import { getClientName, getClientColor } from '@/components/icons/client-icons'
import { getProviderColor, type ProviderType } from '@/lib/theme'
import type { ClientType, Provider } from '@/lib/transport'
import {
  SortableProviderRow,
  ProviderRowContent,
} from '@/pages/client-routes/components/provider-row'
import type { ProviderConfigItem } from '@/pages/client-routes/types'
import { Button } from '../ui'

interface ClientTypeRoutesContentProps {
  clientType: ClientType
  projectID: number // 0 for global routes
}

export function ClientTypeRoutesContent({
  clientType,
  projectID,
}: ClientTypeRoutesContentProps) {
  const [activeId, setActiveId] = useState<string | null>(null)
  const { data: providerStats = {} } = useProviderStats(clientType, projectID)

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  )

  const { data: allRoutes, isLoading: routesLoading } = useRoutes()
  const { data: providers = [], isLoading: providersLoading } = useProviders()
  const { countsByProviderAndClient } = useStreamingRequests()

  const createRoute = useCreateRoute()
  const toggleRoute = useToggleRoute()
  const deleteRoute = useDeleteRoute()
  const updatePositions = useUpdateRoutePositions()

  const loading = routesLoading || providersLoading

  // Get routes for this clientType and projectID
  const clientRoutes = useMemo(() => {
    return (
      allRoutes?.filter(
        r => r.clientType === clientType && r.projectID === projectID
      ) || []
    )
  }, [allRoutes, clientType, projectID])

  // Build provider config items
  const items = useMemo((): ProviderConfigItem[] => {
    const allItems = providers.map(provider => {
      const route =
        clientRoutes.find(r => Number(r.providerID) === Number(provider.id)) ||
        null
      const isNative = (provider.supportedClientTypes || []).includes(
        clientType
      )
      return {
        id: `${clientType}-provider-${provider.id}`,
        provider,
        route,
        enabled: route?.isEnabled ?? false,
        isNative,
      }
    })

    // Only show providers that have routes
    const filteredItems = allItems.filter(item => item.route)

    return filteredItems.sort((a, b) => {
      if (a.route && b.route) return a.route.position - b.route.position
      if (a.route && !b.route) return -1
      if (!a.route && b.route) return 1
      if (a.isNative && !b.isNative) return -1
      if (!a.isNative && b.isNative) return 1
      return a.provider.name.localeCompare(b.provider.name)
    })
  }, [providers, clientRoutes, clientType])

  // Get available providers (without routes yet)
  const availableProviders = useMemo((): Provider[] => {
    return providers.filter(p => {
      const hasRoute = clientRoutes.some(
        r => Number(r.providerID) === Number(p.id)
      )
      return !hasRoute
    })
  }, [providers, clientRoutes])

  const activeItem = activeId ? items.find(item => item.id === activeId) : null

  const handleToggle = (item: ProviderConfigItem) => {
    if (item.route) {
      toggleRoute.mutate(item.route.id)
    } else {
      createRoute.mutate({
        isEnabled: true,
        isNative: item.isNative,
        projectID,
        clientType,
        providerID: item.provider.id,
        position: items.length + 1,
        retryConfigID: 0,
      })
    }
  }

  const handleAddRoute = (provider: Provider, isNative: boolean) => {
    createRoute.mutate({
      isEnabled: true,
      isNative,
      projectID,
      clientType,
      providerID: provider.id,
      position: items.length + 1,
      retryConfigID: 0,
    })
  }

  const handleDeleteRoute = (routeId: number) => {
    deleteRoute.mutate(routeId)
  }

  const handleDragStart = (event: DragStartEvent) => {
    setActiveId(event.active.id as string)
  }

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event
    setActiveId(null)

    if (!over || active.id === over.id) return

    const oldIndex = items.findIndex(item => item.id === active.id)
    const newIndex = items.findIndex(item => item.id === over.id)

    if (oldIndex === -1 || newIndex === -1) return

    const newItems = arrayMove(items, oldIndex, newIndex)

    // Create missing routes
    for (let i = 0; i < newItems.length; i++) {
      const item = newItems[i]
      if (!item.route) {
        await createRoute.mutateAsync({
          isEnabled: false,
          isNative: item.isNative,
          projectID,
          clientType,
          providerID: item.provider.id,
          position: i + 1,
          retryConfigID: 0,
        })
      }
    }

    // Update positions
    const updates: Record<number, number> = {}
    newItems.forEach((item, i) => {
      if (item.route) {
        updates[item.route.id] = i + 1
      }
    })

    if (Object.keys(updates).length > 0) {
      updatePositions.mutate(updates)
    }
  }

  const color = getClientColor(clientType)

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full p-12">
        <div className="text-text-muted">Loading...</div>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full px-6">
      <div className="flex-1 overflow-y-auto px-lg py-6">
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
                items={items}
                strategy={verticalListSortingStrategy}
              >
                <div className="space-y-sm">
                  {items.map((item, index) => (
                    <SortableProviderRow
                      key={item.id}
                      item={item}
                      index={index}
                      clientType={clientType}
                      streamingCount={
                        countsByProviderAndClient.get(
                          `${item.provider.id}:${clientType}`
                        ) || 0
                      }
                      stats={providerStats[item.provider.id]}
                      isToggling={
                        toggleRoute.isPending || createRoute.isPending
                      }
                      onToggle={() => handleToggle(item)}
                      onDelete={
                        item.route
                          ? () => handleDeleteRoute(item.route!.id)
                          : undefined
                      }
                    />
                  ))}
                </div>
              </SortableContext>

              <DragOverlay dropAnimation={null}>
                {activeItem && (
                  <ProviderRowContent
                    item={activeItem}
                    index={items.findIndex(i => i.id === activeItem.id)}
                    clientType={clientType}
                    streamingCount={
                      countsByProviderAndClient.get(
                        `${activeItem.provider.id}:${clientType}`
                      ) || 0
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
            <div className="flex flex-col items-center justify-center py-16 text-text-muted">
              <p className="text-body">
                No routes configured for {getClientName(clientType)}
              </p>
              <p className="text-caption mt-sm">
                Add a route below to get started
              </p>
            </div>
          )}

          {/* Add Route Section - Card Style */}
          {availableProviders.length > 0 && (
            <div className="pt-4 border-t border-border/50 ">
              <div className="flex items-center gap-2 mb-6">
                <Plus size={14} style={{ color }} />
                <span className="text-caption font-medium text-text-muted">
                  Available Providers
                </span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {availableProviders.map(provider => {
                  const isNative = (
                    provider.supportedClientTypes || []
                  ).includes(clientType)
                  const providerColor = getProviderColor(
                    provider.type as ProviderType
                  )
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
                          <div className="text-[14px] font-semibold text-text-primary truncate leading-tight mb-1">
                            {provider.name}
                          </div>
                          <div className="flex items-center gap-2">
                            <span className="text-[11px] font-medium text-text-muted/80 capitalize leading-tight">
                              {provider.type}
                            </span>
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
                  )
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
