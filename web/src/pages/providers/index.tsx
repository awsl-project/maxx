import { useMemo, useRef, useState, type ComponentProps, type MouseEvent } from 'react';
import { Plus, Layers, Download, Upload, Search, RefreshCw, Terminal } from 'lucide-react';
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
} from '@/hooks/queries';
import { useStreamingRequests } from '@/hooks/use-streaming';
import type { Provider, ImportResult } from '@/lib/transport';
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

export function ProvidersPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user } = useAuth();
  const { data: providers, isLoading } = useProviders();
  const { data: providerStats = {} } = useAllProviderStats();
  const { countsByProvider } = useStreamingRequests();
  const [importStatus, setImportStatus] = useState<ImportResult | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
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
  const canManageProviderSettings = user?.role === 'admin';
  const providerReadOnlyHint = t('providers.readOnlyHint');

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
                          {typeProviders.map((provider) => (
                            <ProviderRow
                              key={provider.id}
                              provider={provider}
                              stats={providerStats[provider.id]}
                              streamingCount={countsByProvider.get(provider.id) || 0}
                              onClick={
                                canManageProviderSettings
                                  ? () => navigate(`/providers/${provider.id}/edit`)
                                  : undefined
                              }
                              title={!canManageProviderSettings ? providerReadOnlyHint : undefined}
                            />
                          ))}
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
    </div>
  );
}
