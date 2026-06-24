import { useEffect, useMemo, useRef, useState, type ComponentProps, type MouseEvent } from 'react';
import {
  Plus,
  Layers,
  Download,
  Upload,
  Search,
  RefreshCw,
  Terminal,
  Trash2,
  AlertTriangle,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import {
  useProviders,
  useAllProviderStats,
  usePublicSettings,
  useSettings,
  useUpdateSetting,
  useProxyRequestUpdates,
  useCreateProvider,
  useCreateModelMapping,
  useDeleteProvider,
  useRoutes,
  useModelMappings,
} from '@/hooks/queries';
import { useStreamingRequests } from '@/hooks/use-streaming';
import type { Provider, ImportResult, Route, ModelMapping } from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import { ProviderRow } from './components/provider-row';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Switch } from '@/components/ui/switch';
import { PageHeader } from '@/components/layout/page-header';
import { PROVIDER_TYPE_CONFIGS, type ProviderTypeKey } from './types';
import { AntigravityQuotasProvider } from '@/contexts/antigravity-quotas-context';
import { CodexQuotasProvider } from '@/contexts/codex-quotas-context';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useAuth } from '@/lib/auth-context';
import { cn } from '@/lib/utils';
import {
  parseBulkCustomProviderCommands,
  toCreateProviderData,
  type BulkCustomProviderParseError,
} from './utils/bulk-custom-provider-import';

type ManageProvidersButtonProps = Omit<ComponentProps<typeof Button>, 'disabled'> & {
  canManage: boolean;
  blockedReason: string;
};

function ManageProvidersButton({
  canManage,
  blockedReason,
  className,
  onClick,
  children,
  ...props
}: ManageProvidersButtonProps) {
  if (canManage) {
    return (
      <Button className={className} onClick={onClick} {...props}>
        {children}
      </Button>
    );
  }

  const handleBlockedClick = (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
  };

  return (
    <Tooltip>
      <TooltipTrigger
        render={(triggerProps) => (
          <Button
            {...props}
            {...triggerProps}
            aria-disabled="true"
            className={cn(
              className,
              triggerProps.className,
              'aria-disabled:cursor-not-allowed aria-disabled:opacity-50',
            )}
            onClick={handleBlockedClick}
          >
            {children}
          </Button>
        )}
      />
      <TooltipContent>{blockedReason}</TooltipContent>
    </Tooltip>
  );
}

type ProviderBulkDeletePreviewItem = {
  provider: Provider;
  routes: Route[];
  modelMappings: ModelMapping[];
  streamingCount: number;
};

type ProviderBulkDeleteStatus = {
  deleted: number;
  failed: Array<{ id: number; name: string; message: string }>;
} | null;

export function ProvidersPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user } = useAuth();
  const { data: providers, isLoading } = useProviders();
  const { data: routes } = useRoutes();
  const { data: modelMappings } = useModelMappings();
  const { data: providerStats = {} } = useAllProviderStats();
  const { countsByProvider } = useStreamingRequests();
  const [importStatus, setImportStatus] = useState<ImportResult | null>(null);
  const [bulkDeleteStatus, setBulkDeleteStatus] = useState<ProviderBulkDeleteStatus>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedProviderIds, setSelectedProviderIds] = useState<Set<number>>(new Set());
  const [isBulkDeleteOpen, setIsBulkDeleteOpen] = useState(false);
  const [isRefreshingQuotas, setIsRefreshingQuotas] = useState(false);
  const [isRefreshingCodex, setIsRefreshingCodex] = useState(false);
  const [isBulkImportOpen, setIsBulkImportOpen] = useState(false);
  const [bulkImportCommands, setBulkImportCommands] = useState('');
  const [bulkImportStatus, setBulkImportStatus] = useState<{
    imported: number;
    mappings: number;
    errors: BulkCustomProviderParseError[];
  } | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();
  const createProvider = useCreateProvider();
  const createModelMapping = useCreateModelMapping();
  const deleteProvider = useDeleteProvider();
  const canManageProviderSettings = user?.role === 'admin';
  const providerReadOnlyHint = t('providers.readOnlyHint');
  const isBulkDeleting = deleteProvider.isPending;

  // 订阅请求更新事件，确保 providerStats 实时刷新
  useProxyRequestUpdates();

  // Settings for auto-sort
  const { data: adminSettings } = useSettings(canManageProviderSettings);
  const { data: publicSettings } = usePublicSettings(!canManageProviderSettings);
  const settings = canManageProviderSettings ? adminSettings : publicSettings;
  const updateSetting = useUpdateSetting();
  const autoSortAntigravity = settings?.auto_sort_antigravity === 'true';
  const autoSortCodex = settings?.auto_sort_codex === 'true';
  const bulkImportPreview = useMemo(
    () => parseBulkCustomProviderCommands(bulkImportCommands),
    [bulkImportCommands],
  );
  const isBulkImporting = createProvider.isPending || createModelMapping.isPending;

  const handleToggleAutoSortAntigravity = (checked: boolean) => {
    updateSetting.mutate({
      key: 'auto_sort_antigravity',
      value: checked ? 'true' : 'false',
    });
  };

  const handleToggleAutoSortCodex = (checked: boolean) => {
    updateSetting.mutate({
      key: 'auto_sort_codex',
      value: checked ? 'true' : 'false',
    });
  };

  const groupedProviders = useMemo(() => {
    // 按类型分组，使用配置系统中定义的类型
    const groups: Record<ProviderTypeKey, Provider[]> = {
      antigravity: [],
      bedrock: [],
      kiro: [],
      codex: [],
      claude: [],
      custom: [],
    };

    // Filter providers by search query
    const filteredProviders = providers?.filter((p) => {
      if (!searchQuery.trim()) return true;
      const query = searchQuery.toLowerCase();
      const config = PROVIDER_TYPE_CONFIGS[p.type as ProviderTypeKey];
      const displayInfo = config?.getDisplayInfo(p) || '';
      return p.name.toLowerCase().includes(query) || displayInfo.toLowerCase().includes(query);
    });

    filteredProviders?.forEach((p) => {
      const type = p.type as ProviderTypeKey;
      if (groups[type]) {
        groups[type].push(p);
      } else {
        // 未知类型归入 custom
        groups.custom.push(p);
      }
    });

    // 按名称字母顺序排列
    for (const key of Object.keys(groups) as ProviderTypeKey[]) {
      groups[key].sort((a, b) => a.name.localeCompare(b.name));
    }

    return groups;
  }, [providers, searchQuery]);

  const visibleProviderIds = useMemo(
    () =>
      (Object.keys(PROVIDER_TYPE_CONFIGS) as ProviderTypeKey[]).flatMap((typeKey) =>
        groupedProviders[typeKey].map((provider) => provider.id),
      ),
    [groupedProviders],
  );

  const selectedProviders = useMemo(
    () => providers?.filter((provider) => selectedProviderIds.has(provider.id)) ?? [],
    [providers, selectedProviderIds],
  );

  const selectedVisibleProviderCount = visibleProviderIds.filter((id) =>
    selectedProviderIds.has(id),
  ).length;
  const allVisibleProvidersSelected =
    visibleProviderIds.length > 0 && selectedVisibleProviderCount === visibleProviderIds.length;

  const bulkDeletePreview = useMemo<ProviderBulkDeletePreviewItem[]>(
    () =>
      selectedProviders.map((provider) => ({
        provider,
        routes: (routes ?? []).filter((route) => route.providerID === provider.id),
        modelMappings: (modelMappings ?? []).filter(
          (mapping) => mapping.scope === 'provider' && mapping.providerID === provider.id,
        ),
        streamingCount: countsByProvider.get(provider.id) || 0,
      })),
    [countsByProvider, modelMappings, routes, selectedProviders],
  );

  const bulkDeleteRouteCount = bulkDeletePreview.reduce((sum, item) => sum + item.routes.length, 0);
  const bulkDeleteMappingCount = bulkDeletePreview.reduce(
    (sum, item) => sum + item.modelMappings.length,
    0,
  );
  const bulkDeleteStreamingCount = bulkDeletePreview.reduce(
    (sum, item) => sum + item.streamingCount,
    0,
  );

  useEffect(() => {
    if (!providers) return;
    const providerIds = new Set(providers.map((provider) => provider.id));
    setSelectedProviderIds((previous) => {
      const next = new Set(Array.from(previous).filter((id) => providerIds.has(id)));
      return next.size === previous.size ? previous : next;
    });
  }, [providers]);

  // Export providers as JSON file
  const handleExport = async () => {
    if (!canManageProviderSettings || !providers?.length) return;

    try {
      const transport = getTransport();
      const data = await transport.exportProviders();
      const blob = new Blob([JSON.stringify(data, null, 2)], {
        type: 'application/json',
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `providers-${new Date().toISOString().split('T')[0]}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Export failed:', error);
    }
  };

  // Import providers from JSON file
  const handleImport = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    try {
      const text = await file.text();
      const data = JSON.parse(text) as Provider[];
      const transport = getTransport();
      const result = await transport.importProviders(data);
      setImportStatus(result);
      queryClient.invalidateQueries({ queryKey: ['providers'] });
      queryClient.invalidateQueries({ queryKey: ['routes'] });
      // Clear file input
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
      // Auto-hide status after 5 seconds
      setTimeout(() => setImportStatus(null), 5000);
    } catch (error) {
      console.error('Import failed:', error);
      setImportStatus({
        imported: 0,
        skipped: 0,
        errors: [`Import failed: ${error}`],
      });
      setTimeout(() => setImportStatus(null), 5000);
    }
  };

  const handleBulkImportOpenChange = (open: boolean) => {
    if (isBulkImporting && !open) return;
    setIsBulkImportOpen(open);
  };

  const handleBulkImport = async () => {
    if (
      !canManageProviderSettings ||
      bulkImportPreview.errors.length > 0 ||
      bulkImportPreview.commands.length === 0 ||
      isBulkImporting
    ) {
      return;
    }

    const importErrors: BulkCustomProviderParseError[] = [];
    let imported = 0;
    let mappings = 0;

    for (const command of bulkImportPreview.commands) {
      try {
        const provider = await createProvider.mutateAsync(toCreateProviderData(command));
        imported += 1;

        for (const [pattern, target] of Object.entries(command.modelMapping)) {
          try {
            await createModelMapping.mutateAsync({
              scope: 'provider',
              providerID: provider.id,
              pattern,
              target,
              isEnabled: true,
            });
            mappings += 1;
          } catch (error) {
            importErrors.push({
              lineNumber: command.lineNumber,
              message: `${pattern} -> ${target}: ${error instanceof Error ? error.message : String(error)}`,
            });
          }
        }
      } catch (error) {
        importErrors.push({
          lineNumber: command.lineNumber,
          message: error instanceof Error ? error.message : String(error),
        });
      }
    }

    setBulkImportStatus({ imported, mappings, errors: importErrors });
    queryClient.invalidateQueries({ queryKey: ['providers'] });
    queryClient.invalidateQueries({ queryKey: ['routes'] });

    if (importErrors.length === 0) {
      setBulkImportCommands('');
      setTimeout(() => {
        setIsBulkImportOpen(false);
        setBulkImportStatus(null);
      }, 800);
    }
  };

  const handleToggleProviderSelection = (providerId: number, checked: boolean) => {
    setSelectedProviderIds((previous) => {
      const next = new Set(previous);
      if (checked) {
        next.add(providerId);
      } else {
        next.delete(providerId);
      }
      return next;
    });
    setBulkDeleteStatus(null);
  };

  const handleToggleVisibleProviderSelection = () => {
    setSelectedProviderIds((previous) => {
      const next = new Set(previous);
      if (allVisibleProvidersSelected) {
        visibleProviderIds.forEach((id) => next.delete(id));
      } else {
        visibleProviderIds.forEach((id) => next.add(id));
      }
      return next;
    });
    setBulkDeleteStatus(null);
  };

  const handleClearProviderSelection = () => {
    setSelectedProviderIds(new Set());
    setBulkDeleteStatus(null);
  };

  const handleBulkDeleteProviders = async () => {
    if (!canManageProviderSettings || selectedProviders.length === 0 || isBulkDeleting) return;

    const failed: Array<{ id: number; name: string; message: string }> = [];
    let deleted = 0;

    for (const provider of selectedProviders) {
      try {
        await deleteProvider.mutateAsync(provider.id);
        deleted += 1;
      } catch (error) {
        failed.push({
          id: provider.id,
          name: provider.name,
          message: error instanceof Error ? error.message : String(error),
        });
      }
    }

    queryClient.invalidateQueries({ queryKey: ['providers'] });
    queryClient.invalidateQueries({ queryKey: ['routes'] });
    queryClient.invalidateQueries({ queryKey: ['model-mappings'] });
    setBulkDeleteStatus({ deleted, failed });

    if (failed.length === 0) {
      setSelectedProviderIds(new Set());
      setIsBulkDeleteOpen(false);
      setTimeout(() => setBulkDeleteStatus(null), 5000);
    } else {
      setSelectedProviderIds(new Set(failed.map((item) => item.id)));
    }
  };

  // Refresh Antigravity quotas
  const handleRefreshQuotas = async () => {
    if (!canManageProviderSettings || isRefreshingQuotas) return;

    setIsRefreshingQuotas(true);
    try {
      const transport = getTransport();
      await transport.refreshAntigravityQuotas();
      // Invalidate quota cache - key matches useAntigravityBatchQuotas
      queryClient.invalidateQueries({ queryKey: ['providers', 'antigravity-batch-quotas'] });
    } catch (error) {
      console.error('Refresh quotas failed:', error);
    } finally {
      setIsRefreshingQuotas(false);
    }
  };

  // Refresh all Codex providers quotas
  const handleRefreshCodex = async () => {
    if (!canManageProviderSettings || isRefreshingCodex) return;

    setIsRefreshingCodex(true);
    try {
      const transport = getTransport();
      // Force refresh all Codex quotas and save to database
      await transport.refreshCodexQuotas();
      // Invalidate quota cache - key matches useCodexBatchQuotas
      queryClient.invalidateQueries({ queryKey: ['providers', 'codex-batch-quotas'] });
    } catch (error) {
      console.error('Refresh Codex quotas failed:', error);
    } finally {
      setIsRefreshingCodex(false);
    }
  };

  // Provider list
  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Layers}
        iconClassName="text-blue-500"
        title={t('providers.title')}
        description={t('providers.description', {
          count: providers?.length || 0,
        })}
      >
        <div className="relative">
          <Search
            size={14}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            placeholder={t('common.search')}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9 w-32 md:w-48"
          />
        </div>
        <input
          type="file"
          ref={fileInputRef}
          onChange={handleImport}
          accept=".json"
          className="hidden"
        />
        <ManageProvidersButton
          canManage={canManageProviderSettings}
          blockedReason={t('providers.importProvidersAdminOnly')}
          onClick={() => fileInputRef.current?.click()}
          className="flex items-center gap-2"
          title={canManageProviderSettings ? t('providers.importProviders') : undefined}
          variant="outline"
        >
          <Upload size={14} />
          <span>{t('common.import')}</span>
        </ManageProvidersButton>
        {canManageProviderSettings ? (
          <Button
            onClick={handleExport}
            className="flex items-center gap-2"
            disabled={!providers?.length}
            title={t('providers.exportProviders')}
            variant="outline"
          >
            <Download size={14} />
            <span>{t('common.export')}</span>
          </Button>
        ) : (
          <ManageProvidersButton
            canManage={false}
            blockedReason={providerReadOnlyHint}
            className="flex items-center gap-2"
            title={t('providers.exportProviders')}
            variant="outline"
          >
            <Download size={14} />
            <span>{t('common.export')}</span>
          </ManageProvidersButton>
        )}
        <ManageProvidersButton
          canManage={canManageProviderSettings}
          blockedReason={t('providers.addProviderAdminOnly')}
          onClick={() => setIsBulkImportOpen(true)}
          className="flex items-center gap-2"
          title={canManageProviderSettings ? t('providers.bulkImport.open') : undefined}
          variant="outline"
        >
          <Terminal size={14} />
          <span>{t('providers.bulkImport.open')}</span>
        </ManageProvidersButton>
        <ManageProvidersButton
          canManage={canManageProviderSettings}
          blockedReason={t('providers.addProviderAdminOnly')}
          onClick={() => navigate('/providers/create')}
          title={canManageProviderSettings ? t('providers.addProvider') : undefined}
        >
          <Plus size={14} />
          <span>{t('providers.addProvider')}</span>
        </ManageProvidersButton>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="mx-auto max-w-7xl">
          {canManageProviderSettings && providers && providers.length > 0 && (
            <div className="mb-4 flex flex-wrap items-center gap-3 rounded-lg border border-border bg-card/80 p-3 shadow-sm">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleToggleVisibleProviderSelection}
                disabled={visibleProviderIds.length === 0 || isBulkDeleting}
              >
                {allVisibleProvidersSelected
                  ? t('providers.bulkDelete.clearVisible')
                  : t('providers.bulkDelete.selectVisible', { count: visibleProviderIds.length })}
              </Button>
              <div className="text-sm text-muted-foreground">
                {t('providers.bulkDelete.selectedCount', {
                  count: selectedProviderIds.size,
                })}
              </div>
              {selectedProviderIds.size > 0 && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={handleClearProviderSelection}
                  disabled={isBulkDeleting}
                >
                  {t('common.cancel')}
                </Button>
              )}
              <Button
                type="button"
                variant="destructive"
                size="sm"
                className="ml-auto gap-2"
                onClick={() => setIsBulkDeleteOpen(true)}
                disabled={selectedProviderIds.size === 0 || isBulkDeleting}
              >
                <Trash2 size={14} />
                {t('providers.bulkDelete.open')}
              </Button>
            </div>
          )}

          {isLoading ? (
            <div className="flex items-center justify-center h-full">
              <div className="text-text-muted">{t('common.loading')}</div>
            </div>
          ) : providers?.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
              <Layers size={48} className="mb-4 opacity-50" />
              <p className="text-body">{t('providers.noProviders')}</p>
              <p className="text-caption mt-2">{t('providers.noProvidersHint')}</p>
              <ManageProvidersButton
                canManage={canManageProviderSettings}
                blockedReason={t('providers.addProviderAdminOnly')}
                onClick={() => navigate('/providers/create')}
                className="mt-6 flex items-center gap-2"
                title={canManageProviderSettings ? t('providers.addProvider') : undefined}
              >
                <Plus size={14} />
                <span>{t('providers.addProvider')}</span>
              </ManageProvidersButton>
            </div>
          ) : (
            <AntigravityQuotasProvider>
              <CodexQuotasProvider>
                <div className="space-y-8">
                  {/* 动态渲染各类型分组 */}
                  {(Object.keys(PROVIDER_TYPE_CONFIGS) as ProviderTypeKey[]).map((typeKey) => {
                    const typeProviders = groupedProviders[typeKey];
                    if (typeProviders.length === 0) return null;

                    const config = PROVIDER_TYPE_CONFIGS[typeKey];
                    const TypeIcon = config.icon;

                    return (
                      <section key={typeKey} className="space-y-3">
                        <div className="flex items-center gap-2 px-1">
                          <TypeIcon size={16} style={{ color: config.color }} />
                          <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
                            {config.label}
                          </h3>
                          <div className="h-px flex-1 bg-border/50 ml-2" />
                          {/* Refresh Quotas Button - Only for Antigravity */}
                          {typeKey === 'antigravity' && (
                            <>
                              {canManageProviderSettings && (
                                <div className="flex items-center gap-1.5">
                                  <span className="text-xs text-muted-foreground">
                                    {t('settings.autoSortAntigravity')}
                                  </span>
                                  <Switch
                                    checked={autoSortAntigravity}
                                    onCheckedChange={handleToggleAutoSortAntigravity}
                                    disabled={updateSetting.isPending}
                                  />
                                </div>
                              )}
                              {canManageProviderSettings ? (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={handleRefreshQuotas}
                                  disabled={isRefreshingQuotas}
                                  className="h-7 px-2 gap-1.5 text-xs text-muted-foreground hover:text-foreground shrink-0"
                                  title={t('providers.refreshQuotas')}
                                >
                                  <RefreshCw
                                    size={12}
                                    className={isRefreshingQuotas ? 'animate-spin' : ''}
                                  />
                                  <span>{t('common.refresh')}</span>
                                </Button>
                              ) : (
                                <ManageProvidersButton
                                  canManage={false}
                                  blockedReason={providerReadOnlyHint}
                                  variant="ghost"
                                  size="sm"
                                  className="h-7 px-2 gap-1.5 text-xs text-muted-foreground hover:text-foreground shrink-0"
                                  title={t('providers.refreshQuotas')}
                                >
                                  <RefreshCw size={12} />
                                  <span>{t('common.refresh')}</span>
                                </ManageProvidersButton>
                              )}
                            </>
                          )}
                          {/* Refresh Button - Only for Codex */}
                          {typeKey === 'codex' && (
                            <>
                              {canManageProviderSettings && (
                                <div className="flex items-center gap-1.5">
                                  <span className="text-xs text-muted-foreground">
                                    {t('settings.autoSortCodex')}
                                  </span>
                                  <Switch
                                    checked={autoSortCodex}
                                    onCheckedChange={handleToggleAutoSortCodex}
                                    disabled={updateSetting.isPending}
                                  />
                                </div>
                              )}
                              {canManageProviderSettings ? (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={handleRefreshCodex}
                                  disabled={isRefreshingCodex}
                                  className="h-7 px-2 gap-1.5 text-xs text-muted-foreground hover:text-foreground shrink-0"
                                  title={t('providers.refreshCodex')}
                                >
                                  <RefreshCw
                                    size={12}
                                    className={isRefreshingCodex ? 'animate-spin' : ''}
                                  />
                                  <span>{t('common.refresh')}</span>
                                </Button>
                              ) : (
                                <ManageProvidersButton
                                  canManage={false}
                                  blockedReason={providerReadOnlyHint}
                                  variant="ghost"
                                  size="sm"
                                  className="h-7 px-2 gap-1.5 text-xs text-muted-foreground hover:text-foreground shrink-0"
                                  title={t('providers.refreshCodex')}
                                >
                                  <RefreshCw size={12} />
                                  <span>{t('common.refresh')}</span>
                                </ManageProvidersButton>
                              )}
                            </>
                          )}
                        </div>
                        <div className="space-y-3">
                          {typeProviders.map((provider) => {
                            const isSelected = selectedProviderIds.has(provider.id);
                            return (
                              <div key={provider.id} className="relative">
                                {canManageProviderSettings && (
                                  <label
                                    className={cn(
                                      'absolute left-3 top-1/2 z-30 flex h-7 w-7 -translate-y-1/2 cursor-pointer items-center justify-center rounded-full border bg-background/90 shadow-sm backdrop-blur transition-all',
                                      isSelected
                                        ? 'border-primary bg-primary/10 ring-2 ring-primary/15'
                                        : 'border-border/80 opacity-70 hover:opacity-100',
                                    )}
                                    onClick={(event) => event.stopPropagation()}
                                  >
                                    <input
                                      type="checkbox"
                                      className="h-3.5 w-3.5 accent-primary"
                                      aria-label={t('providers.bulkDelete.selectProvider', {
                                        name: provider.name,
                                      })}
                                      checked={isSelected}
                                      disabled={isBulkDeleting}
                                      onChange={(event) =>
                                        handleToggleProviderSelection(
                                          provider.id,
                                          event.target.checked,
                                        )
                                      }
                                    />
                                  </label>
                                )}
                                <ProviderRow
                                  provider={provider}
                                  stats={providerStats[provider.id]}
                                  streamingCount={countsByProvider.get(provider.id) || 0}
                                  className={cn(
                                    canManageProviderSettings && 'pl-12',
                                    isSelected &&
                                      'border-primary/50 bg-primary/5 ring-1 ring-primary/20',
                                  )}
                                  onClick={
                                    canManageProviderSettings
                                      ? () => navigate(`/providers/${provider.id}/edit`)
                                      : undefined
                                  }
                                  title={
                                    !canManageProviderSettings ? providerReadOnlyHint : undefined
                                  }
                                />
                              </div>
                            );
                          })}
                        </div>
                      </section>
                    );
                  })}
                </div>
              </CodexQuotasProvider>
            </AntigravityQuotasProvider>
          )}
        </div>
      </div>

      <Dialog open={isBulkImportOpen} onOpenChange={handleBulkImportOpenChange}>
        <DialogContent className="max-w-3xl" showCloseButton={!isBulkImporting}>
          <DialogHeader>
            <DialogTitle>{t('providers.bulkImport.title')}</DialogTitle>
            <DialogDescription>{t('providers.bulkImport.description')}</DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
              <div className="mb-2 font-medium text-foreground">
                {t('providers.bulkImport.exampleTitle')}
              </div>
              <code className="block whitespace-pre-wrap break-all">
                provider add --name "Mimo" --base-url "https://api.example.com" --api-key "sk-..."
                --clients claude,openai --models claude-sonnet-4,gpt-5 --map "*=mimo-v2.5-pro"
                --response-map "mimo-v2.5-pro=claude-sonnet-4"
              </code>
            </div>

            <Textarea
              value={bulkImportCommands}
              onChange={(event) => {
                setBulkImportCommands(event.target.value);
                setBulkImportStatus(null);
              }}
              placeholder={t('providers.bulkImport.placeholder')}
              className="min-h-52 font-mono text-xs"
            />

            <div className="rounded-lg border border-border p-3">
              <div className="mb-3 flex flex-wrap items-center gap-2 text-sm">
                <span className="font-medium">{t('providers.bulkImport.preview')}</span>
                <Badge variant="secondary">
                  {t('providers.bulkImport.providersCount', {
                    count: bulkImportPreview.commands.length,
                  })}
                </Badge>
                <Badge variant={bulkImportPreview.errors.length > 0 ? 'danger' : 'success'}>
                  {t('providers.bulkImport.errorsCount', {
                    count: bulkImportPreview.errors.length,
                  })}
                </Badge>
              </div>

              {bulkImportPreview.commands.length > 0 && (
                <div className="max-h-40 space-y-2 overflow-y-auto text-xs">
                  {bulkImportPreview.commands.map((command) => (
                    <div key={command.lineNumber} className="rounded-md bg-muted/40 p-2">
                      <div className="font-medium text-foreground">
                        {t('providers.bulkImport.lineProvider', {
                          line: command.lineNumber,
                          name: command.name,
                        })}
                      </div>
                      <div className="mt-1 text-muted-foreground">
                        {t('providers.bulkImport.previewDetails', {
                          clients: command.clients.join(', '),
                          models: command.supportModels.length,
                          mappings: Object.keys(command.modelMapping).length,
                          responseMappings: Object.keys(command.responseModelMapping).length,
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {bulkImportPreview.errors.length > 0 && (
                <div className="mt-3 max-h-32 space-y-1 overflow-y-auto text-xs text-red-400">
                  {bulkImportPreview.errors.map((error, index) => (
                    <div key={`${error.lineNumber}-${index}`}>
                      {t('providers.bulkImport.lineError', {
                        line: error.lineNumber,
                        message: error.message,
                      })}
                    </div>
                  ))}
                </div>
              )}

              {bulkImportStatus && (
                <div
                  className={cn(
                    'mt-3 rounded-md p-2 text-xs',
                    bulkImportStatus.errors.length > 0
                      ? 'bg-red-500/10 text-red-400'
                      : 'bg-emerald-500/10 text-emerald-400',
                  )}
                >
                  <div>
                    {t('providers.bulkImport.importResult', {
                      providers: bulkImportStatus.imported,
                      mappings: bulkImportStatus.mappings,
                    })}
                  </div>
                  {bulkImportStatus.errors.map((error, index) => (
                    <div key={`${error.lineNumber}-${index}`}>
                      {t('providers.bulkImport.lineError', {
                        line: error.lineNumber,
                        message: error.message,
                      })}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="secondary"
              onClick={() => handleBulkImportOpenChange(false)}
              disabled={isBulkImporting}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleBulkImport}
              disabled={
                !canManageProviderSettings ||
                isBulkImporting ||
                bulkImportPreview.errors.length > 0 ||
                bulkImportPreview.commands.length === 0
              }
            >
              {isBulkImporting ? t('common.saving') : t('providers.bulkImport.submit')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={isBulkDeleteOpen}
        onOpenChange={(open) => {
          if (!isBulkDeleting) setIsBulkDeleteOpen(open);
        }}
      >
        <AlertDialogContent className="max-w-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{t('providers.bulkDelete.dialogTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('providers.bulkDelete.dialogDescription', {
                count: selectedProviders.length,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>

          <div className="space-y-4">
            {(bulkDeleteRouteCount > 0 || bulkDeleteStreamingCount > 0) && (
              <div className="flex gap-3 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
                <AlertTriangle size={18} className="mt-0.5 shrink-0" />
                <div className="space-y-1">
                  {bulkDeleteRouteCount > 0 && (
                    <p>
                      {t('providers.bulkDelete.routeWarning', {
                        count: bulkDeleteRouteCount,
                      })}
                    </p>
                  )}
                  {bulkDeleteStreamingCount > 0 && (
                    <p>
                      {t('providers.bulkDelete.streamingWarning', {
                        count: bulkDeleteStreamingCount,
                      })}
                    </p>
                  )}
                </div>
              </div>
            )}

            <div className="grid grid-cols-3 gap-3 text-sm">
              <div className="rounded-lg border border-border bg-muted/30 p-3">
                <div className="text-xs uppercase tracking-wider text-muted-foreground">
                  {t('providers.bulkDelete.providers')}
                </div>
                <div className="mt-1 text-lg font-semibold">{selectedProviders.length}</div>
              </div>
              <div className="rounded-lg border border-border bg-muted/30 p-3">
                <div className="text-xs uppercase tracking-wider text-muted-foreground">
                  {t('providers.bulkDelete.routes')}
                </div>
                <div className="mt-1 text-lg font-semibold">{bulkDeleteRouteCount}</div>
              </div>
              <div className="rounded-lg border border-border bg-muted/30 p-3">
                <div className="text-xs uppercase tracking-wider text-muted-foreground">
                  {t('providers.bulkDelete.modelMappings')}
                </div>
                <div className="mt-1 text-lg font-semibold">{bulkDeleteMappingCount}</div>
              </div>
            </div>

            <div className="max-h-56 space-y-2 overflow-y-auto rounded-lg border border-border p-3 text-sm">
              {bulkDeletePreview.map((item) => (
                <div key={item.provider.id} className="rounded-md bg-muted/40 p-2">
                  <div className="font-medium text-foreground">{item.provider.name}</div>
                  <div className="mt-1 text-xs text-muted-foreground">
                    {t('providers.bulkDelete.itemDetails', {
                      routes: item.routes.length,
                      mappings: item.modelMappings.length,
                    })}
                  </div>
                </div>
              ))}
            </div>

            {bulkDeleteStatus?.failed.length ? (
              <div className="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">
                {bulkDeleteStatus.failed.map((item) => (
                  <div key={item.id}>
                    {item.name}: {item.message}
                  </div>
                ))}
              </div>
            ) : null}
          </div>

          <AlertDialogFooter>
            <AlertDialogCancel disabled={isBulkDeleting}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={selectedProviders.length === 0 || isBulkDeleting}
              onClick={(event) => {
                event.preventDefault();
                handleBulkDeleteProviders();
              }}
            >
              {isBulkDeleting
                ? t('providers.bulkDelete.deleting')
                : t('providers.bulkDelete.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Import Status Toast */}
      {importStatus && (
        <div className="fixed bottom-6 right-6 bg-card border border-border rounded-lg shadow-lg p-4">
          <div className="space-y-2">
            <div className="text-sm font-medium text-text-primary">
              {t('providers.importCompleted', {
                imported: importStatus.imported,
                skipped: importStatus.skipped,
              })}
            </div>
            {importStatus.errors.length > 0 && (
              <div className="text-xs text-red-400 space-y-1">
                {importStatus.errors.map((error, i) => (
                  <div key={i}>• {error}</div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {bulkDeleteStatus && bulkDeleteStatus.failed.length === 0 && (
        <div className="fixed bottom-6 right-6 bg-card border border-border rounded-lg shadow-lg p-4">
          <div className="text-sm font-medium text-text-primary">
            {t('providers.bulkDelete.completed', { count: bulkDeleteStatus.deleted })}
          </div>
        </div>
      )}
    </div>
  );
}
