/**
 * Shared Client Type Routes Content Component
 * Used by both global routes and project routes
 */

import { useEffect, useState, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import {
  AlertTriangle,
  CopyPlus,
  RefreshCw,
  Trash2,
  Zap,
  Workflow,
  Settings2,
  Pin,
} from 'lucide-react';
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
  useBulkDeleteRoutes,
  useSyncRoutesFromProject,
  useUpdateRoutePositions,
  useProviderStats,
  useProxyRequestUpdates,
  useRoutingStrategies,
  useProjects,
  routeKeys,
} from '@/hooks/queries';
import { useQueryClient } from '@tanstack/react-query';
import { useStreamingRequests } from '@/hooks/use-streaming';
import { getClientName, getClientColor } from '@/components/icons/client-icons';
import { getProviderColor, type ProviderType } from '@/lib/theme';
import type {
  ClientType,
  Provider,
  ProviderStats,
  Project,
  Route,
  RouteSyncMode,
  RoutingStrategyType,
  RoutingStickyScope,
} from '@/lib/transport';
import {
  SortableProviderRow,
  ProviderRowContent,
} from '@/pages/client-routes/components/provider-row';
import { ClaudeProviderBatchTestDialog } from '@/pages/client-routes/components/claude-provider-batch-test-dialog';
import type { ProviderConfigItem } from '@/pages/client-routes/types';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Button,
  Badge,
  buttonVariants,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui';
import { cn } from '@/lib/utils';
import { AntigravityQuotasProvider } from '@/contexts/antigravity-quotas-context';
import { CooldownsProvider } from '@/contexts/cooldowns-context';
import {
  PROVIDER_TYPE_CONFIGS,
  PROVIDER_TYPE_ORDER,
  createProviderTypeGroups,
  type ProviderTypeKey,
} from '@/pages/providers/types';
import { invertVisibleProviderSelection } from '@/pages/providers/utils/selection';

function isSameProviderStats(a: ProviderStats, b: ProviderStats): boolean {
  return (
    a.providerID === b.providerID &&
    a.totalRequests === b.totalRequests &&
    a.successfulRequests === b.successfulRequests &&
    a.failedRequests === b.failedRequests &&
    a.successRate === b.successRate &&
    a.activeRequests === b.activeRequests &&
    a.totalInputTokens === b.totalInputTokens &&
    a.totalOutputTokens === b.totalOutputTokens &&
    a.totalCacheRead === b.totalCacheRead &&
    a.totalCacheWrite === b.totalCacheWrite &&
    a.totalCost === b.totalCost
  );
}

function useStableProviderStats(stats: Record<number, ProviderStats>) {
  const prevRef = useRef<Record<number, ProviderStats>>({});

  return useMemo(() => {
    const prev = prevRef.current;
    const next: Record<number, ProviderStats> = {};

    for (const [key, value] of Object.entries(stats)) {
      const id = Number(key);
      const prevValue = prev[id];
      if (prevValue && isSameProviderStats(prevValue, value)) {
        next[id] = prevValue;
      } else {
        next[id] = value;
      }
    }

    prevRef.current = next;
    return next;
  }, [stats]);
}

interface ClientTypeRoutesContentProps {
  clientType: ClientType;
  projectID: number; // 0 for global routes
  searchQuery?: string; // Optional search query from parent
}

// Small banner above the routes list telling the user which routing strategy is
// actually in effect for this scope (project-specific, else inherited global,
// else the priority default) and what that means for ordering. Mirrors the
// backend resolution order in router.getRoutingStrategy.
// Render a sticky TTL as a compact, human-friendly duration (1800 → "30m",
// 3600 → "1h", 90 → "90s"). Non-positive values fall back to the 30m default
// the backend applies (sticky.TTLFromConfig).
function formatTtl(seconds: number): string {
  const s = seconds > 0 ? seconds : 1800;
  if (s % 3600 === 0) return `${s / 3600}h`;
  if (s % 60 === 0) return `${s / 60}m`;
  return `${s}s`;
}

function RoutingStrategyBanner({
  type,
  inherited,
  isDefault,
  stickyEnabled,
  stickyScope,
  stickyTTLSeconds,
}: {
  type: RoutingStrategyType;
  inherited: boolean;
  isDefault: boolean;
  stickyEnabled: boolean;
  stickyScope: RoutingStickyScope;
  stickyTTLSeconds: number;
}) {
  const { t } = useTranslation();
  const isWeighted = type === 'weighted_random';
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 rounded-xl border border-border/60 bg-muted/30 px-4 py-2.5">
      <Workflow className="h-4 w-4 text-cyan-500 shrink-0" />
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
        {t('routes.strategyLabel')}
      </span>
      <Badge variant={isWeighted ? 'warning' : 'info'}>
        {isWeighted
          ? t('routingStrategies.weightedRandom')
          : t('routingStrategies.priorityByPosition')}
      </Badge>
      {/* Session affinity only takes effect under weighted_random, so only
          surface its state there — otherwise the badge would be misleading. */}
      {isWeighted &&
        (stickyEnabled ? (
          <Badge variant="success" title={t('routes.affinityTooltip')}>
            <Pin className="mr-1 h-3 w-3" />
            {t('routes.affinityOn')}
            <span className="ml-1 font-normal opacity-80">
              ·{' '}
              {stickyScope === 'conversation'
                ? t('routes.affinityScopeConversation')
                : t('routes.affinityScopeToken')}{' '}
              · {formatTtl(stickyTTLSeconds)}
            </span>
          </Badge>
        ) : (
          <Badge variant="outline" title={t('routes.affinityTooltip')}>
            <Pin className="mr-1 h-3 w-3 opacity-50" />
            {t('routes.affinityOff')}
          </Badge>
        ))}
      {inherited && (
        <span className="text-[11px] text-muted-foreground/70">
          ({t('routes.strategyInherited')})
        </span>
      )}
      {isDefault && (
        <span className="text-[11px] text-muted-foreground/70">
          ({t('routes.strategyDefault')})
        </span>
      )}
      <span className="text-[11px] text-muted-foreground min-w-0 flex-1 break-words">
        {isWeighted ? t('routes.strategyWeightedHint') : t('routes.strategyPriorityHint')}
      </span>
      <Link
        to="/routing-strategies"
        className={cn(buttonVariants({ variant: 'outline', size: 'sm' }), 'h-7 shrink-0 text-xs')}
      >
        <Settings2 className="mr-1.5 h-3.5 w-3.5" />
        {t('routes.strategyConfigure')}
      </Link>
    </div>
  );
}

function projectHasCustomRoutes(project: Project | undefined, clientType: ClientType): boolean {
  return (project?.enabledCustomRoutes ?? []).includes(clientType);
}

function routesForScope(routes: Route[] | undefined, projectID: number, clientType: ClientType) {
  return (routes ?? [])
    .filter((route) => route.projectID === projectID && route.clientType === clientType)
    .slice()
    .sort((a, b) => (a.position === b.position ? a.id - b.id : a.position - b.position));
}

function routeConfigDiffers(target: Route, source: Route, position: number): boolean {
  return (
    target.isEnabled !== source.isEnabled ||
    target.isNative !== source.isNative ||
    target.position !== position ||
    (target.weight || 1) !== (source.weight || 1) ||
    target.retryConfigID !== source.retryConfigID
  );
}

interface SyncRoutesDialogProps {
  clientType: ClientType;
  projectID: number;
  routes: Route[] | undefined;
  projects: Project[];
}

function SyncRoutesDialog({ clientType, projectID, routes, projects }: SyncRoutesDialogProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const sourceOptions = useMemo(() => {
    const options = projectID === 0 ? [] : [{ id: 0, label: t('common.default'), isGlobal: true }];
    for (const project of projects) {
      if (project.id === projectID) continue;
      options.push({ id: project.id, label: project.name, isGlobal: false });
    }
    return options;
  }, [projectID, projects, t]);
  const initialSourceID = sourceOptions.find((option) => option.id !== projectID)?.id ?? 0;
  const [sourceProjectID, setSourceProjectID] = useState<number>(initialSourceID);
  const [mode, setMode] = useState<RouteSyncMode>('overwrite');
  const syncRoutes = useSyncRoutesFromProject();

  useEffect(() => {
    if (!sourceOptions.some((option) => option.id === sourceProjectID)) {
      setSourceProjectID(sourceOptions[0]?.id ?? 0);
    }
  }, [sourceOptions, sourceProjectID]);

  const sourceProject = projects.find((project) => project.id === sourceProjectID);
  const sourceUsesGlobal =
    sourceProjectID > 0 && !projectHasCustomRoutes(sourceProject, clientType);
  const effectiveSourceProjectID = sourceUsesGlobal ? 0 : sourceProjectID;
  const sourceRoutes = routesForScope(routes, effectiveSourceProjectID, clientType);
  const targetRoutes = routesForScope(routes, projectID, clientType);

  const preview = useMemo(() => {
    const targetByProvider = new Map(targetRoutes.map((route) => [route.providerID, route]));
    const sourceProviderIDs = new Set(sourceRoutes.map((route) => route.providerID));

    if (mode === 'add_missing') {
      let created = 0;
      let skipped = 0;
      for (const source of sourceRoutes) {
        if (targetByProvider.has(source.providerID)) {
          skipped++;
        } else {
          created++;
        }
      }
      return { created, updated: 0, deleted: 0, skipped };
    }

    let created = 0;
    let updated = 0;
    for (const [index, source] of sourceRoutes.entries()) {
      const target = targetByProvider.get(source.providerID);
      if (!target) {
        created++;
      } else if (routeConfigDiffers(target, source, index + 1)) {
        updated++;
      }
    }

    let deleted = 0;
    for (const target of targetRoutes) {
      if (!sourceProviderIDs.has(target.providerID)) deleted++;
    }

    return { created, updated, deleted, skipped: 0 };
  }, [mode, sourceRoutes, targetRoutes]);

  const canSync = sourceOptions.length > 0 && !syncRoutes.isPending;
  const targetLabel =
    projectID === 0 ? t('common.default') : projects.find((p) => p.id === projectID)?.name;

  const handleSync = () => {
    syncRoutes.mutate(
      {
        sourceProjectID,
        targetProjectID: projectID,
        clientType,
        mode,
      },
      {
        onSuccess: () => {
          setOpen(false);
        },
      },
    );
  };

  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
        disabled={sourceOptions.length === 0}
        className="h-7 shrink-0 text-xs"
      >
        <CopyPlus className="mr-1.5 h-3.5 w-3.5" />
        {t('routes.syncFromProject')}
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="grid-cols-[minmax(0,1fr)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('routes.syncDialog.title')}</DialogTitle>
            <DialogDescription>
              {t('routes.syncDialog.description', {
                client: getClientName(clientType),
                target: targetLabel ?? t('common.default'),
              })}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground">
                {t('routes.syncDialog.source')}
              </label>
              <Select
                value={String(sourceProjectID)}
                onValueChange={(value) => setSourceProjectID(Number(value))}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {sourceOptions.map((option) => (
                    <SelectItem key={option.id} value={String(option.id)}>
                      {option.isGlobal ? t('routes.syncDialog.defaultRoutes') : option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {sourceUsesGlobal && (
                <p className="text-xs text-amber-600 dark:text-amber-400">
                  {t('routes.syncDialog.sourceInheritsGlobal')}
                </p>
              )}
            </div>

            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground">
                {t('routes.syncDialog.mode')}
              </label>
              <Select value={mode} onValueChange={(value) => setMode(value as RouteSyncMode)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="overwrite">{t('routes.syncDialog.overwrite')}</SelectItem>
                  <SelectItem value="add_missing">{t('routes.syncDialog.addMissing')}</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="rounded-lg border border-border/60 bg-muted/30 p-3 text-sm">
              <div className="mb-2 font-medium">{t('routes.syncDialog.preview')}</div>
              <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground sm:grid-cols-4">
                <span>{t('routes.syncDialog.created', { count: preview.created })}</span>
                <span>{t('routes.syncDialog.updated', { count: preview.updated })}</span>
                <span>{t('routes.syncDialog.deleted', { count: preview.deleted })}</span>
                <span>{t('routes.syncDialog.skipped', { count: preview.skipped })}</span>
              </div>
            </div>

            {projectID === 0 && (
              <div className="flex gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-300">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{t('routes.syncDialog.globalTargetWarning')}</span>
              </div>
            )}

            {syncRoutes.error && (
              <p className="text-xs text-destructive">
                {syncRoutes.error instanceof Error
                  ? syncRoutes.error.message
                  : t('routes.syncDialog.failed')}
              </p>
            )}
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setOpen(false)}
              disabled={syncRoutes.isPending}
            >
              {t('common.cancel')}
            </Button>
            <Button onClick={handleSync} disabled={!canSync}>
              {syncRoutes.isPending ? t('common.saving') : t('routes.syncDialog.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
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
  const [selectedRouteIds, setSelectedRouteIds] = useState<Set<number>>(() => new Set());
  const [selectedAvailableProviderIds, setSelectedAvailableProviderIds] = useState<Set<number>>(
    () => new Set(),
  );
  const [bulkAddError, setBulkAddError] = useState<string | null>(null);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);
  const { data: providerStats = {} } = useProviderStats(clientType, projectID || undefined);
  const stableProviderStats = useStableProviderStats(providerStats);
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
  const { data: projects = [] } = useProjects();
  const { data: strategies = [] } = useRoutingStrategies();

  // Resolve the effective strategy for this scope, mirroring the backend's
  // order: project-specific first, then the global (projectID 0) strategy,
  // then the built-in priority default.
  const strategyInfo = useMemo(() => {
    const own = strategies.find((s) => s.projectID === projectID);
    const global = strategies.find((s) => s.projectID === 0);
    const resolved = own ?? global;
    const cfg = resolved?.config ?? null;
    return {
      type: (resolved?.type ?? 'priority') as RoutingStrategyType,
      inherited: !own && !!global && projectID !== 0,
      isDefault: !resolved,
      // Sticky / session-affinity is only honoured under weighted_random
      // (priority is already deterministic — see router.go). Surface it so the
      // effect of the routes' weights is understood in context.
      stickyEnabled: !!cfg?.stickyEnabled,
      stickyScope: cfg?.stickyScope ?? 'token',
      stickyTTLSeconds: cfg?.stickyTTLSeconds ?? 1800,
    };
  }, [strategies, projectID]);
  const isWeighted = strategyInfo.type === 'weighted_random';

  const createRoute = useCreateRoute();
  const toggleRoute = useToggleRoute();
  const deleteRoute = useDeleteRoute();
  const bulkDeleteRoutes = useBulkDeleteRoutes();
  const updatePositions = useUpdateRoutePositions();

  const loading = routesLoading || providersLoading;

  // Get routes for this clientType and projectID
  const clientRoutes = useMemo(() => {
    return allRoutes?.filter((r) => r.clientType === clientType && r.projectID === projectID) || [];
  }, [allRoutes, clientType, projectID]);

  const normalizedQuery = useMemo(() => searchQuery.trim().toLowerCase(), [searchQuery]);

  const providerById = useMemo(() => {
    const map = new Map<number, Provider>();
    for (const provider of providers) {
      map.set(Number(provider.id), provider);
    }
    return map;
  }, [providers]);

  const routeByProviderId = useMemo(() => {
    const map = new Map<number, (typeof clientRoutes)[number]>();
    for (const route of clientRoutes) {
      map.set(Number(route.providerID), route);
    }
    return map;
  }, [clientRoutes]);

  // Build provider config items
  const items = useMemo((): ProviderConfigItem[] => {
    const allItems: ProviderConfigItem[] = [];

    for (const route of clientRoutes) {
      const provider = providerById.get(Number(route.providerID));
      if (!provider) continue;
      const isNative = (provider.supportedClientTypes || []).includes(clientType);
      allItems.push({
        id: `${clientType}-provider-${provider.id}`,
        provider,
        route,
        enabled: route.isEnabled ?? false,
        isNative,
      });
    }

    let filteredItems = allItems;

    // Apply search filter
    if (normalizedQuery) {
      filteredItems = filteredItems.filter(
        (item) =>
          item.provider.name.toLowerCase().includes(normalizedQuery) ||
          item.provider.type.toLowerCase().includes(normalizedQuery),
      );
    }

    return filteredItems.sort((a, b) => {
      const posDiff = (a.route?.position ?? 0) - (b.route?.position ?? 0);
      if (posDiff !== 0) return posDiff;
      if (a.isNative !== b.isNative) return a.isNative ? -1 : 1;
      return a.provider.name.localeCompare(b.provider.name);
    });
  }, [clientRoutes, clientType, normalizedQuery, providerById]);

  const streamingThrottleMs = items.length > 200 ? 1000 : 0;
  const { countsByProviderAndClient } = useStreamingRequests({ throttleMs: streamingThrottleMs });

  const visibleRouteIds = useMemo(
    () => items.flatMap((item) => (item.route ? [item.route.id] : [])),
    [items],
  );

  const visibleRouteIdSet = useMemo(() => new Set(visibleRouteIds), [visibleRouteIds]);

  useEffect(() => {
    setSelectedRouteIds((prev) => {
      const next = new Set<number>();
      for (const id of prev) {
        if (visibleRouteIdSet.has(id)) {
          next.add(id);
        }
      }
      return next.size === prev.size ? prev : next;
    });
  }, [visibleRouteIdSet]);

  const selectedItems = useMemo(
    () => items.filter((item) => item.route && selectedRouteIds.has(item.route.id)),
    [items, selectedRouteIds],
  );

  const selectedProviderNames = useMemo(
    () => selectedItems.map((item) => item.provider.name),
    [selectedItems],
  );

  const selectedStreamingCount = useMemo(() => {
    return selectedItems.reduce((total, item) => {
      return total + (countsByProviderAndClient.get(`${item.provider.id}:${clientType}`) || 0);
    }, 0);
  }, [clientType, countsByProviderAndClient, selectedItems]);

  const scopeName = useMemo(() => {
    if (projectID === 0) return t('routes.globalScope');
    return projects.find((project) => project.id === projectID)?.name ?? t('routes.projectScope');
  }, [projectID, projects, t]);

  const allVisibleSelected =
    visibleRouteIds.length > 0 && selectedRouteIds.size === visibleRouteIds.length;
  const someVisibleSelected = selectedRouteIds.size > 0 && !allVisibleSelected;

  const availableProviders = useMemo(() => {
    return providers.filter((p) => !routeByProviderId.has(Number(p.id)));
  }, [providers, routeByProviderId]);

  const availableProviderIdSet = useMemo(() => {
    return new Set(availableProviders.map((provider) => Number(provider.id)));
  }, [availableProviders]);

  useEffect(() => {
    setSelectedAvailableProviderIds((prev) => {
      const next = new Set<number>();
      for (const id of prev) {
        if (availableProviderIdSet.has(id)) {
          next.add(id);
        }
      }
      return next.size === prev.size ? prev : next;
    });
  }, [availableProviderIdSet]);

  useEffect(() => {
    setSelectedAvailableProviderIds(new Set());
  }, [clientType, projectID]);

  // Get available providers (without routes yet), grouped by type and sorted alphabetically
  const groupedAvailableProviders = useMemo((): Record<ProviderTypeKey, Provider[]> => {
    let available = availableProviders;

    // Apply search filter
    if (normalizedQuery) {
      available = available.filter(
        (p) =>
          p.name.toLowerCase().includes(normalizedQuery) ||
          p.type.toLowerCase().includes(normalizedQuery),
      );
    }

    return createProviderTypeGroups(available);
  }, [availableProviders, normalizedQuery]);

  const visibleAvailableProviderIds = useMemo(() => {
    return PROVIDER_TYPE_ORDER.flatMap((type) =>
      groupedAvailableProviders[type].map((provider) => Number(provider.id)),
    );
  }, [groupedAvailableProviders]);

  const selectedAvailableProviders = useMemo(() => {
    return availableProviders.filter((provider) =>
      selectedAvailableProviderIds.has(Number(provider.id)),
    );
  }, [availableProviders, selectedAvailableProviderIds]);

  const allVisibleAvailableSelected =
    visibleAvailableProviderIds.length > 0 &&
    visibleAvailableProviderIds.every((id) => selectedAvailableProviderIds.has(id));
  const someVisibleAvailableSelected =
    visibleAvailableProviderIds.some((id) => selectedAvailableProviderIds.has(id)) &&
    !allVisibleAvailableSelected;

  // Check if there are any available providers
  const hasAvailableProviders = useMemo(() => {
    return PROVIDER_TYPE_ORDER.some((type) => groupedAvailableProviders[type].length > 0);
  }, [groupedAvailableProviders]);

  const itemsById = useMemo(() => {
    const map = new Map<string, ProviderConfigItem>();
    for (const item of items) {
      map.set(item.id, item);
    }
    return map;
  }, [items]);

  const itemIds = useMemo(() => items.map((item) => item.id), [items]);

  const itemIndexById = useMemo(() => {
    const map = new Map<string, number>();
    items.forEach((item, index) => {
      map.set(item.id, index);
    });
    return map;
  }, [items]);

  const activeItem = activeId ? (itemsById.get(activeId) ?? null) : null;

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
        weight: 1,
        retryConfigID: 0,
      });
    }
  };

  const handleToggleAvailableProviderSelection = (providerId: number, checked: boolean) => {
    setBulkAddError(null);
    setSelectedAvailableProviderIds((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(providerId);
      } else {
        next.delete(providerId);
      }
      return next;
    });
  };

  const handleToggleAllVisibleAvailableProviders = (checked: boolean) => {
    setBulkAddError(null);
    setSelectedAvailableProviderIds((prev) => {
      if (!checked) {
        const next = new Set(prev);
        for (const providerId of visibleAvailableProviderIds) {
          next.delete(providerId);
        }
        return next;
      }

      return new Set([...prev, ...visibleAvailableProviderIds]);
    });
  };

  const handleInvertVisibleAvailableProviders = () => {
    setBulkAddError(null);
    setSelectedAvailableProviderIds((prev) =>
      invertVisibleProviderSelection(prev, visibleAvailableProviderIds),
    );
  };

  const handleClearAvailableProviderSelection = () => {
    setBulkAddError(null);
    setSelectedAvailableProviderIds(new Set());
  };

  const handleBulkAddRoutes = async () => {
    if (selectedAvailableProviders.length === 0 || createRoute.isPending) return;

    setBulkAddError(null);
    const startingPosition = items.length + 1;
    const results = await Promise.allSettled(
      selectedAvailableProviders.map((provider, index) =>
        createRoute.mutateAsync({
          isEnabled: true,
          isNative: (provider.supportedClientTypes || []).includes(clientType),
          projectID,
          clientType,
          providerID: provider.id,
          position: startingPosition + index,
          weight: 1,
          retryConfigID: 0,
        }),
      ),
    );

    const failedProviderIds = new Set<number>();
    results.forEach((result, index) => {
      if (result.status === 'rejected') {
        const provider = selectedAvailableProviders[index];
        failedProviderIds.add(Number(provider.id));
        console.error('Failed to bulk add provider route', {
          providerID: provider.id,
          providerName: provider.name,
          error: result.reason,
        });
      }
    });

    setSelectedAvailableProviderIds(failedProviderIds);
    if (failedProviderIds.size > 0) {
      setBulkAddError(t('routes.bulkAddProvidersFailed', { count: failedProviderIds.size }));
    }
  };

  const handleDeleteRoute = (routeId: number) => {
    deleteRoute.mutate(routeId);
  };

  const handleToggleRouteSelection = (routeId: number, checked: boolean) => {
    setSelectedRouteIds((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(routeId);
      } else {
        next.delete(routeId);
      }
      return next;
    });
  };

  const handleToggleAllVisible = (checked: boolean) => {
    setSelectedRouteIds(checked ? new Set(visibleRouteIds) : new Set());
  };

  const handleConfirmBulkDelete = () => {
    const ids = selectedItems.flatMap((item) => (item.route ? [item.route.id] : []));
    if (ids.length === 0) return;

    bulkDeleteRoutes.mutate(
      { ids, clientType, projectID },
      {
        onSuccess: () => {
          setSelectedRouteIds(new Set());
          setBulkDeleteOpen(false);
        },
      },
    );
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

    const oldIndex = itemIndexById.get(active.id as string);
    const newIndex = itemIndexById.get(over.id as string);

    if (oldIndex === undefined || newIndex === undefined) return;
    if (oldIndex === newIndex) return;

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
          {/* Effective routing strategy for this scope */}
          <div className="flex flex-col gap-3 xl:flex-row xl:items-center">
            <div className="min-w-0 flex-1">
              <RoutingStrategyBanner
                type={strategyInfo.type}
                inherited={strategyInfo.inherited}
                isDefault={strategyInfo.isDefault}
                stickyEnabled={strategyInfo.stickyEnabled}
                stickyScope={strategyInfo.stickyScope}
                stickyTTLSeconds={strategyInfo.stickyTTLSeconds}
              />
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-2">
              {clientType === 'claude' && (
                <ClaudeProviderBatchTestDialog
                  providers={providers}
                  routes={allRoutes ?? []}
                  projectID={projectID}
                />
              )}
              <SyncRoutesDialog
                clientType={clientType}
                projectID={projectID}
                routes={allRoutes}
                projects={projects}
              />
            </div>
          </div>

          {items.length > 0 && (
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/60 bg-background/80 px-4 py-3 shadow-sm">
              <label className="flex items-center gap-3 text-sm text-muted-foreground">
                <input
                  type="checkbox"
                  checked={allVisibleSelected}
                  aria-checked={someVisibleSelected ? 'mixed' : allVisibleSelected}
                  onChange={(event) => handleToggleAllVisible(event.target.checked)}
                  className="h-4 w-4 rounded border-border accent-destructive"
                />
                <span>
                  {t('routes.bulkSelectVisible', { count: visibleRouteIds.length })}
                  {normalizedQuery && (
                    <span className="ml-1 text-muted-foreground/70">
                      {t('routes.bulkFilteredSelection')}
                    </span>
                  )}
                </span>
              </label>
              <div className="flex items-center gap-2">
                {selectedRouteIds.size > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {t('routes.bulkSelectedCount', { count: selectedRouteIds.size })}
                  </span>
                )}
                <Button
                  variant="destructive"
                  size="sm"
                  disabled={selectedRouteIds.size === 0 || bulkDeleteRoutes.isPending}
                  onClick={() => setBulkDeleteOpen(true)}
                >
                  <Trash2 className="h-4 w-4" />
                  {t('routes.bulkDelete')}
                </Button>
              </div>
            </div>
          )}

          {/* Routes List */}
          {items.length > 0 ? (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragStart={handleDragStart}
              onDragEnd={handleDragEnd}
            >
              <SortableContext items={itemIds} strategy={verticalListSortingStrategy}>
                <div className="space-y-2">
                  {items.map((item, index) => (
                    <div
                      key={item.id}
                      className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-3"
                    >
                      <input
                        type="checkbox"
                        checked={item.route ? selectedRouteIds.has(item.route.id) : false}
                        disabled={!item.route || bulkDeleteRoutes.isPending}
                        aria-label={t('routes.selectRoute', { name: item.provider.name })}
                        onPointerDown={(event) => event.stopPropagation()}
                        onClick={(event) => event.stopPropagation()}
                        onChange={(event) => {
                          if (item.route) {
                            handleToggleRouteSelection(item.route.id, event.target.checked);
                          }
                        }}
                        className="h-4 w-4 rounded border-border accent-destructive"
                      />
                      <SortableProviderRow
                        item={item}
                        index={index}
                        clientType={clientType}
                        streamingCount={
                          countsByProviderAndClient.get(`${item.provider.id}:${clientType}`) || 0
                        }
                        stats={stableProviderStats[item.provider.id]}
                        isToggling={toggleRoute.isPending || createRoute.isPending}
                        showWeight={isWeighted}
                        onToggle={() => handleToggle(item)}
                        onDelete={item.route ? () => handleDeleteRoute(item.route!.id) : undefined}
                      />
                    </div>
                  ))}
                </div>
              </SortableContext>

              <DragOverlay dropAnimation={null}>
                {activeItem && (
                  <ProviderRowContent
                    item={activeItem}
                    index={itemIndexById.get(activeItem.id) ?? 0}
                    clientType={clientType}
                    streamingCount={
                      countsByProviderAndClient.get(`${activeItem.provider.id}:${clientType}`) || 0
                    }
                    stats={stableProviderStats[activeItem.provider.id]}
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
              <div className="mb-6 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="flex items-center gap-2">
                  <Settings2 size={14} style={{ color }} />
                  <span className="text-caption font-medium text-muted-foreground">
                    {t('routes.availableProviders')}
                  </span>
                </div>
                <div className="flex flex-wrap items-center gap-2 rounded-xl border border-border/60 bg-background/80 px-3 py-2 shadow-sm">
                  <label className="flex items-center gap-2 text-sm text-muted-foreground">
                    <input
                      type="checkbox"
                      checked={allVisibleAvailableSelected}
                      aria-checked={
                        someVisibleAvailableSelected ? 'mixed' : allVisibleAvailableSelected
                      }
                      onChange={(event) =>
                        handleToggleAllVisibleAvailableProviders(event.target.checked)
                      }
                      className="h-4 w-4 rounded border-border accent-primary"
                    />
                    <span>
                      {t('routes.bulkSelectVisibleProviders', {
                        count: visibleAvailableProviderIds.length,
                      })}
                      {normalizedQuery && (
                        <span className="ml-1 text-muted-foreground/70">
                          {t('routes.bulkFilteredSelection')}
                        </span>
                      )}
                    </span>
                  </label>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={visibleAvailableProviderIds.length === 0 || createRoute.isPending}
                    onClick={handleInvertVisibleAvailableProviders}
                  >
                    {t('routes.bulkInvertVisibleProviders')}
                  </Button>
                  {selectedAvailableProviders.length > 0 && (
                    <span className="text-xs text-muted-foreground">
                      {t('routes.bulkSelectedProvidersCount', {
                        count: selectedAvailableProviders.length,
                      })}
                    </span>
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={selectedAvailableProviders.length === 0 || createRoute.isPending}
                    onClick={handleClearAvailableProviderSelection}
                  >
                    {t('common.clear')}
                  </Button>
                  <Button
                    size="sm"
                    disabled={selectedAvailableProviders.length === 0 || createRoute.isPending}
                    onClick={handleBulkAddRoutes}
                  >
                    {createRoute.isPending
                      ? t('routes.bulkAddingProviders')
                      : t('routes.bulkAddSelectedProviders', {
                          count: selectedAvailableProviders.length,
                        })}
                  </Button>
                </div>
              </div>
              {bulkAddError && (
                <div className="mb-4 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                  {bulkAddError}
                </div>
              )}
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
                            : PROVIDER_TYPE_CONFIGS[typeKey].label}
                        </span>
                        <div className="h-px flex-1 bg-border/50" />
                      </div>
                      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                        {typeProviders.map((provider) => {
                          const providerId = Number(provider.id);
                          const isSelected = selectedAvailableProviderIds.has(providerId);
                          const isNative = (provider.supportedClientTypes || []).includes(
                            clientType,
                          );
                          const providerColor = getProviderColor(provider.type as ProviderType);
                          return (
                            <div
                              key={provider.id}
                              onClick={() => {
                                if (!createRoute.isPending) {
                                  handleToggleAvailableProviderSelection(providerId, !isSelected);
                                }
                              }}
                              className={cn(
                                'h-auto group relative flex cursor-pointer items-center justify-between gap-4 p-4 rounded-xl border border-border/40 bg-background hover:bg-secondary/50 hover:border-border shadow-sm hover:shadow transition-all duration-300 text-left overflow-hidden',
                                isSelected && 'border-primary/70 bg-primary/10 shadow-primary/10',
                                createRoute.isPending && 'cursor-not-allowed opacity-50',
                              )}
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

                              {/* Right: Bulk add selection */}
                              <input
                                type="checkbox"
                                checked={isSelected}
                                disabled={createRoute.isPending}
                                aria-label={t('routes.selectAvailableProvider', {
                                  name: provider.name,
                                })}
                                onClick={(event) => event.stopPropagation()}
                                onChange={(event) =>
                                  handleToggleAvailableProviderSelection(
                                    providerId,
                                    event.target.checked,
                                  )
                                }
                                className="h-5 w-5 shrink-0 rounded border-border accent-primary"
                              />
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          <AlertDialog open={bulkDeleteOpen} onOpenChange={setBulkDeleteOpen}>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>{t('routes.bulkDeleteDialog.title')}</AlertDialogTitle>
                <AlertDialogDescription>
                  {t('routes.bulkDeleteDialog.description', {
                    count: selectedItems.length,
                    client: getClientName(clientType),
                    scope: scopeName,
                  })}
                </AlertDialogDescription>
              </AlertDialogHeader>

              <div className="space-y-3 text-sm">
                <div className="rounded-lg border border-border/60 bg-muted/30 p-3">
                  <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    {t('routes.bulkDeleteDialog.providersPreview')}
                  </div>
                  <ul className="space-y-1 text-foreground">
                    {selectedProviderNames.slice(0, 5).map((name, index) => (
                      <li key={`${name}-${index}`} className="truncate">
                        {name}
                      </li>
                    ))}
                  </ul>
                  {selectedProviderNames.length > 5 && (
                    <div className="mt-2 text-xs text-muted-foreground">
                      {t('routes.bulkDeleteDialog.moreProviders', {
                        count: selectedProviderNames.length - 5,
                      })}
                    </div>
                  )}
                </div>

                {selectedStreamingCount > 0 && (
                  <div className="flex gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-amber-700 dark:text-amber-300">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                    <span>
                      {t('routes.bulkDeleteDialog.streamingWarning', {
                        count: selectedStreamingCount,
                      })}
                    </span>
                  </div>
                )}
              </div>

              <AlertDialogFooter>
                <AlertDialogCancel disabled={bulkDeleteRoutes.isPending}>
                  {t('common.cancel')}
                </AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  disabled={selectedItems.length === 0 || bulkDeleteRoutes.isPending}
                  onClick={handleConfirmBulkDelete}
                >
                  {bulkDeleteRoutes.isPending
                    ? t('routes.bulkDeleteDialog.deleting')
                    : t('routes.bulkDeleteDialog.confirm')}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>
    </div>
  );
}
