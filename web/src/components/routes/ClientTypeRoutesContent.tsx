/**
 * Shared Client Type Routes Content Component
 * Used by both global routes and project routes
 */

import { useState, useMemo } from 'react'
import { Plus, RefreshCw } from 'lucide-react'
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
import { getProviderColor } from '@/lib/theme'
import type { ClientType, Provider } from '@/lib/transport'
import {
  SortableProviderRow,
  ProviderRowContent,
} from '@/pages/client-routes/components/provider-row'
import type { ProviderConfigItem } from '@/pages/client-routes/types'

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
    <div className="flex flex-col h-full">
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
                        item.route && !item.isNative
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
            <div className="pt-4 border-t border-border/50">
              <div className="flex items-center gap-2 mb-md">
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
                  const providerColor = getProviderColor(provider.type)
                  return (
                    <button
                      key={provider.id}
                      onClick={() => handleAddRoute(provider, isNative)}
                      disabled={createRoute.isPending}
                      className="group relative flex flex-col p-4 rounded-lg border border-border bg-surface-secondary hover:bg-surface-hover hover:border-accent/30 transition-all text-left"
                    >
                      <div className="flex items-start gap-3 mb-3">
                        <div
                          className="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0"
                          style={{
                            backgroundColor: `${providerColor}15`,
                            color: providerColor,
                          }}
                        >
                          <span className="text-base font-bold">
                            {provider.name.charAt(0).toUpperCase()}
                          </span>
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="text-sm font-medium text-text-primary truncate">
                            {provider.name}
                          </div>
                          <div className="text-xs text-text-muted capitalize">
                            {provider.type}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center justify-between">
                        {isNative ? (
                          <span className="text-[10px] font-bold text-emerald-500 bg-emerald-500/10 px-2 py-0.5 rounded">
                            NATIVE
                          </span>
                        ) : (
                          <span className="flex items-center gap-1 text-[10px] font-bold text-amber-500 bg-amber-500/10 px-2 py-0.5 rounded">
                            <RefreshCw size={8} /> CONV
                          </span>
                        )}
                        <Plus
                          size={14}
                          className="text-text-muted group-hover:text-accent transition-colors"
                        />
                      </div>
                    </button>
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
