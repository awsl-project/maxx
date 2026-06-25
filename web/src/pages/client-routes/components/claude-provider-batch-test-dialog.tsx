import { useMemo, useRef, useState } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { FlaskConical, Loader2, Play, ShieldCheck } from 'lucide-react';

import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Input,
  Switch,
} from '@/components/ui';
import { Textarea } from '@/components/ui/textarea';
import type {
  ClaudeProviderBatchPersistMode,
  ClaudeProviderBatchProviderResult,
  CreateProviderData,
  Provider,
  Route,
} from '@/lib/transport';
import { useClaudeProviderBatchTest } from '@/hooks/queries';
import { cn } from '@/lib/utils';
import {
  parseBulkCustomProviderCommands,
  toCreateProviderData,
  type BulkCustomProviderCommand,
} from '@/pages/providers/utils/bulk-custom-provider-import';

interface ClaudeProviderBatchTestDialogProps {
  providers: Provider[];
  routes: Route[];
  projectID: number;
}

type PreviewItem = {
  key: string;
  source: 'existing' | 'candidate';
  providerID?: number;
  command?: BulkCustomProviderCommand;
  provider?: Provider;
  name: string;
  baseURL: string;
  modelMapping: Record<string, string>;
  duplicate?: Provider;
  action: string;
  selected: boolean;
  error?: string;
};

const DEFAULT_TEST_MODEL = 'claude-sonnet-4';

function providerBaseURL(provider?: Provider | CreateProviderData | null) {
  return provider?.config?.custom?.baseURL?.replace(/\/+$/, '') ?? '';
}

function providerModelMapping(provider?: Provider | CreateProviderData | null) {
  return provider?.config?.custom?.modelMapping ?? {};
}

function maskURL(raw: string) {
  try {
    const url = new URL(raw);
    url.username = '';
    url.password = '';
    return url.toString();
  } catch {
    return raw;
  }
}

function mappingSummary(mapping: Record<string, string>, t: TFunction) {
  const entries = Object.entries(mapping);
  if (entries.length === 0) return t('routes.claudeBatchTest.modelMappingDefault');
  return entries.map(([from, to]) => `${from} → ${to}`).join(', ');
}

function resultBadgeVariant(
  status?: string,
): 'success' | 'warning' | 'danger' | 'outline' | 'info' {
  if (status === 'usable') return 'success';
  if (status === 'auth_failed' || status === 'model_unsupported') return 'warning';
  if (status === 'timeout' || status === 'upstream_5xx' || status === 'maxx_protocol_error')
    return 'danger';
  if (status === 'duplicate_blocked') return 'warning';
  if (!status) return 'outline';
  return 'info';
}

function resultLabel(status: string | undefined, t: TFunction) {
  switch (status) {
    case 'usable':
      return t('routes.claudeBatchTest.status.usable');
    case 'auth_failed':
      return t('routes.claudeBatchTest.status.authFailed');
    case 'model_unsupported':
      return t('routes.claudeBatchTest.status.modelUnsupported');
    case 'timeout':
      return t('routes.claudeBatchTest.status.timeout');
    case 'upstream_5xx':
      return t('routes.claudeBatchTest.status.upstream5xx');
    case 'maxx_protocol_error':
      return t('routes.claudeBatchTest.status.maxxProtocolError');
    case 'validation_failed':
      return t('routes.claudeBatchTest.status.validationFailed');
    case 'duplicate_blocked':
      return t('routes.claudeBatchTest.status.duplicateBlocked');
    case 'persistence_failed':
      return t('routes.claudeBatchTest.status.persistenceFailed');
    case 'unsupported_provider':
      return t('routes.claudeBatchTest.status.unsupportedProvider');
    default:
      return status || t('routes.claudeBatchTest.status.notTested');
  }
}

function resultByIndex(results: ClaudeProviderBatchProviderResult[] | undefined) {
  return new Map((results ?? []).map((result) => [result.index, result]));
}

export function ClaudeProviderBatchTestDialog({
  providers,
  routes,
  projectID,
}: ClaudeProviderBatchTestDialogProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [commandsText, setCommandsText] = useState('');
  const [selectedExistingIDs, setSelectedExistingIDs] = useState<number[]>([]);
  const [testModel, setTestModel] = useState(DEFAULT_TEST_MODEL);
  const [maxTokens, setMaxTokens] = useState(16);
  const [concurrency, setConcurrency] = useState(4);
  const [persistMode, setPersistMode] = useState<ClaudeProviderBatchPersistMode>('passed');
  const [createRoutes, setCreateRoutes] = useState(true);
  const [overwriteExisting, setOverwriteExisting] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const batchTest = useClaudeProviderBatchTest();

  const claudeProviders = useMemo(
    () =>
      providers.filter(
        (provider) =>
          provider.type === 'custom' &&
          (provider.supportedClientTypes?.length === 0 ||
            provider.supportedClientTypes?.includes('claude')),
      ),
    [providers],
  );

  const claudeProviderIDs = useMemo(
    () => claudeProviders.map((provider) => provider.id),
    [claudeProviders],
  );

  const selectedExistingCount = useMemo(
    () => selectedExistingIDs.filter((id) => claudeProviderIDs.includes(id)).length,
    [claudeProviderIDs, selectedExistingIDs],
  );

  const allExistingSelected =
    claudeProviderIDs.length > 0 && selectedExistingCount === claudeProviderIDs.length;

  const parsePreview = useMemo(() => parseBulkCustomProviderCommands(commandsText), [commandsText]);

  const existingByName = useMemo(() => {
    const map = new Map<string, Provider>();
    for (const provider of providers) map.set(provider.name.trim().toLowerCase(), provider);
    return map;
  }, [providers]);

  const existingByBaseURL = useMemo(() => {
    const map = new Map<string, Provider>();
    for (const provider of providers) {
      const baseURL = providerBaseURL(provider);
      if (baseURL) map.set(baseURL, provider);
    }
    return map;
  }, [providers]);

  const existingRouteProviderIDs = useMemo(
    () =>
      new Set(
        routes
          .filter((route) => route.clientType === 'claude' && route.projectID === projectID)
          .map((route) => route.providerID),
      ),
    [routes, projectID],
  );

  const previewItems = useMemo<PreviewItem[]>(() => {
    const selectedExisting = claudeProviders
      .filter((provider) => selectedExistingIDs.includes(provider.id))
      .map<PreviewItem>((provider) => ({
        key: `existing-${provider.id}`,
        source: 'existing',
        providerID: provider.id,
        provider,
        name: provider.name,
        baseURL: providerBaseURL(provider),
        modelMapping: providerModelMapping(provider),
        action: existingRouteProviderIDs.has(provider.id)
          ? t('routes.claudeBatchTest.actions.testExistingRoute')
          : t('routes.claudeBatchTest.actions.testAddRoute'),
        selected: true,
      }));

    const candidates = parsePreview.commands.map<PreviewItem>((command) => {
      const data = toCreateProviderData(command);
      const duplicate =
        existingByName.get(command.name.trim().toLowerCase()) ??
        existingByBaseURL.get(command.baseURL.replace(/\/+$/, ''));
      const supportsClaude = command.clients.includes('claude');
      return {
        key: `candidate-${command.lineNumber}`,
        source: 'candidate',
        command,
        name: command.name,
        baseURL: command.baseURL,
        modelMapping: data.config?.custom?.modelMapping ?? {},
        duplicate,
        action: duplicate
          ? overwriteExisting
            ? t('routes.claudeBatchTest.actions.updateProviderRoute')
            : t('routes.claudeBatchTest.actions.duplicateTestOnly')
          : t('routes.claudeBatchTest.actions.createProviderRoute'),
        selected: supportsClaude,
        error: supportsClaude ? undefined : t('routes.claudeBatchTest.errors.clientNotClaude'),
      };
    });

    return [...selectedExisting, ...candidates];
  }, [
    claudeProviders,
    existingByBaseURL,
    existingByName,
    existingRouteProviderIDs,
    overwriteExisting,
    parsePreview.commands,
    selectedExistingIDs,
    t,
  ]);

  const selectedCandidateData = useMemo(
    () =>
      previewItems
        .filter((item) => item.source === 'candidate' && item.command && !item.error)
        .map((item) => toCreateProviderData(item.command!)),
    [previewItems],
  );

  const resultMap = resultByIndex(batchTest.data?.results);
  const canRun =
    previewItems.some((item) => item.selected && !item.error) && parsePreview.errors.length === 0;

  const handleToggleExisting = (providerID: number, checked: boolean) => {
    setSelectedExistingIDs((current) =>
      checked ? [...new Set([...current, providerID])] : current.filter((id) => id !== providerID),
    );
  };

  const handleToggleAllExisting = () => {
    setSelectedExistingIDs((current) => {
      if (allExistingSelected) {
        const idsToClear = new Set(claudeProviderIDs);
        return current.filter((id) => !idsToClear.has(id));
      }
      return [...new Set([...current, ...claudeProviderIDs])];
    });
  };

  const handleRun = async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    await batchTest.mutateAsync({
      signal: controller.signal,
      data: {
        existingProviderIDs: selectedExistingIDs,
        candidates: selectedCandidateData,
        projectID,
        testModel: testModel.trim() || DEFAULT_TEST_MODEL,
        maxTokens,
        concurrency,
        persistMode,
        createRoutes,
        overwriteExisting,
        routeWeight: 1,
      },
    });
  };

  const handleCancel = () => {
    abortRef.current?.abort();
    abortRef.current = null;
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="outline" size="sm" className="h-8 text-xs" />}>
        <FlaskConical className="h-3.5 w-3.5 mr-1.5" />
        {t('routes.claudeBatchTest.button')}
      </DialogTrigger>
      <DialogContent
        className="grid max-w-none grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-none"
        style={{
          width: 'min(calc(100vw - 2rem), 64rem)',
          maxWidth: 'none',
          height: 'min(calc(100dvh - 4rem), 52rem)',
          maxHeight: 'calc(100dvh - 4rem)',
        }}
      >
        <DialogHeader className="min-w-0 shrink-0 border-b border-border px-6 py-5 pr-14">
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheck className="h-4 w-4" />
            {t('routes.claudeBatchTest.title')}
          </DialogTitle>
          <DialogDescription>{t('routes.claudeBatchTest.description')}</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 min-w-0 overflow-y-auto px-6 py-5 space-y-5">
          <section className="grid min-w-0 gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="min-w-0 space-y-2">
              <div className="flex items-center justify-between gap-2">
                <p className="text-sm font-medium">{t('routes.claudeBatchTest.candidateTitle')}</p>
                <Badge variant="outline">{t('routes.claudeBatchTest.parserBadge')}</Badge>
              </div>
              <Textarea
                value={commandsText}
                onChange={(event) => setCommandsText(event.target.value)}
                placeholder={
                  'provider add --name mimo-a --base-url https://example.com/anthropic --api-key sk-xxx --clients claude --map claude-sonnet-4=mimo-v2.5-pro'
                }
                className="min-h-44 field-sizing-fixed font-mono text-xs"
              />
              {parsePreview.errors.length > 0 && (
                <div className="rounded-lg border border-red-400/30 bg-red-400/10 p-3 text-xs text-red-500 space-y-1">
                  {parsePreview.errors.map((error, index) => (
                    <div key={`${error.lineNumber}-${index}`}>
                      {t('routes.claudeBatchTest.parseErrorLine', {
                        line: error.lineNumber,
                        message: error.message,
                      })}
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="min-w-0 space-y-2">
              <div className="flex items-center justify-between gap-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {t('routes.claudeBatchTest.existingTitle')}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t('routes.claudeBatchTest.existingSelectedCount', {
                      selected: selectedExistingCount,
                      total: claudeProviderIDs.length,
                    })}
                  </p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 shrink-0 text-xs"
                  disabled={claudeProviderIDs.length === 0}
                  onClick={handleToggleAllExisting}
                >
                  {allExistingSelected
                    ? t('routes.claudeBatchTest.clearExistingSelection')
                    : t('routes.claudeBatchTest.selectAllExisting')}
                </Button>
              </div>
              <div className="rounded-lg border border-border max-h-44 overflow-y-auto divide-y divide-border">
                {claudeProviders.length === 0 ? (
                  <div className="p-3 text-xs text-muted-foreground">
                    {t('routes.claudeBatchTest.noExistingProviders')}
                  </div>
                ) : (
                  claudeProviders.map((provider) => (
                    <label
                      key={provider.id}
                      className="flex items-start gap-3 p-3 text-sm hover:bg-muted/40"
                    >
                      <input
                        type="checkbox"
                        className="mt-1"
                        checked={selectedExistingIDs.includes(provider.id)}
                        onChange={(event) =>
                          handleToggleExisting(provider.id, event.target.checked)
                        }
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-medium">{provider.name}</span>
                        <span className="block truncate text-xs text-muted-foreground">
                          {maskURL(providerBaseURL(provider))}
                        </span>
                      </span>
                      {existingRouteProviderIDs.has(provider.id) && (
                        <Badge variant="secondary">
                          {t('routes.claudeBatchTest.existingRoute')}
                        </Badge>
                      )}
                    </label>
                  ))
                )}
              </div>
            </div>
          </section>

          <section className="grid min-w-0 gap-3 md:grid-cols-4">
            <label className="space-y-1 text-xs">
              <span className="text-muted-foreground">{t('routes.claudeBatchTest.testModel')}</span>
              <Input value={testModel} onChange={(event) => setTestModel(event.target.value)} />
            </label>
            <label className="space-y-1 text-xs">
              <span className="text-muted-foreground">{t('routes.claudeBatchTest.maxTokens')}</span>
              <Input
                type="number"
                min={1}
                max={128}
                value={maxTokens}
                onChange={(event) => setMaxTokens(Number(event.target.value) || 16)}
              />
            </label>
            <label className="space-y-1 text-xs">
              <span className="text-muted-foreground">
                {t('routes.claudeBatchTest.concurrency')}
              </span>
              <Input
                type="number"
                min={1}
                max={5}
                value={concurrency}
                onChange={(event) =>
                  setConcurrency(Math.min(5, Math.max(1, Number(event.target.value) || 4)))
                }
              />
            </label>
            <label className="space-y-1 text-xs">
              <span className="text-muted-foreground">
                {t('routes.claudeBatchTest.persistMode')}
              </span>
              <select
                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                value={persistMode}
                onChange={(event) =>
                  setPersistMode(event.target.value as ClaudeProviderBatchPersistMode)
                }
              >
                <option value="passed">{t('routes.claudeBatchTest.persistPassed')}</option>
                <option value="all_disabled">
                  {t('routes.claudeBatchTest.persistAllDisabled')}
                </option>
                <option value="none">{t('routes.claudeBatchTest.persistNone')}</option>
              </select>
            </label>
          </section>

          <section className="flex min-w-0 flex-wrap items-center gap-4 rounded-lg border border-border p-3 text-sm">
            <label className="flex items-center gap-2">
              <Switch checked={createRoutes} onCheckedChange={setCreateRoutes} />
              {t('routes.claudeBatchTest.createRoutes')}
            </label>
            <label className="flex items-center gap-2">
              <Switch checked={overwriteExisting} onCheckedChange={setOverwriteExisting} />
              {t('routes.claudeBatchTest.overwriteExisting')}
            </label>
          </section>

          <section className="min-w-0 space-y-2">
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm font-medium">{t('routes.claudeBatchTest.previewTitle')}</p>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span>
                  {t('routes.claudeBatchTest.countCandidates', { count: previewItems.length })}
                </span>
                <span>
                  {t('routes.claudeBatchTest.countUsable', {
                    count: batchTest.data?.usableCount ?? 0,
                  })}
                </span>
                <span>
                  {t('routes.claudeBatchTest.countSaved', {
                    count: batchTest.data?.persistedCount ?? 0,
                  })}
                </span>
                <span>
                  {t('routes.claudeBatchTest.countRoutesCreated', {
                    count: batchTest.data?.routesCreated ?? 0,
                  })}
                </span>
              </div>
            </div>
            <div className="rounded-lg border border-border overflow-hidden">
              {previewItems.length === 0 ? (
                <div className="p-4 text-sm text-muted-foreground">
                  {t('routes.claudeBatchTest.emptyPreview')}
                </div>
              ) : (
                <div className="divide-y divide-border">
                  {previewItems.map((item, visibleIndex) => {
                    const result = resultMap.get(visibleIndex);
                    return (
                      <div
                        key={item.key}
                        className={cn(
                          'p-3 grid gap-2 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto]',
                          item.error && 'bg-red-400/5',
                        )}
                      >
                        <div className="min-w-0">
                          <div className="flex items-center gap-2 min-w-0">
                            <span className="font-medium truncate">{item.name}</span>
                            <Badge variant={item.source === 'existing' ? 'secondary' : 'outline'}>
                              {item.source === 'existing'
                                ? t('routes.claudeBatchTest.sourceExisting')
                                : t('routes.claudeBatchTest.sourceCandidate')}
                            </Badge>
                            {item.duplicate && (
                              <Badge variant="warning" className="max-w-48 truncate">
                                {t('routes.claudeBatchTest.duplicateProvider', {
                                  name: item.duplicate.name,
                                })}
                              </Badge>
                            )}
                          </div>
                          <div className="text-xs text-muted-foreground truncate">
                            {maskURL(item.baseURL)}
                          </div>
                          <div className="text-xs text-muted-foreground truncate">
                            {mappingSummary(item.modelMapping, t)}
                          </div>
                        </div>
                        <div className="min-w-0 text-xs">
                          <div>
                            {t('routes.claudeBatchTest.actionLabel', { action: item.action })}
                          </div>
                          {item.error && <div className="text-red-500">{item.error}</div>}
                          {result?.error && (
                            <div className="text-red-500 truncate" title={result.error}>
                              {result.error}
                            </div>
                          )}
                          {result?.message && !result.error && (
                            <div className="text-muted-foreground truncate" title={result.message}>
                              {result.message}
                            </div>
                          )}
                        </div>
                        <div className="flex items-center justify-end gap-2">
                          {result?.httpStatus ? (
                            <Badge variant="outline">HTTP {result.httpStatus}</Badge>
                          ) : null}
                          {result?.durationMs ? (
                            <Badge variant="outline">{result.durationMs}ms</Badge>
                          ) : null}
                          <Badge
                            variant={resultBadgeVariant(
                              result?.status ?? (item.error ? 'validation_failed' : undefined),
                            )}
                          >
                            {item.error
                              ? t('routes.claudeBatchTest.status.validationFailed')
                              : resultLabel(result?.status, t)}
                          </Badge>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </section>
        </div>

        <DialogFooter className="shrink-0 border-t border-border px-6 py-4">
          {batchTest.isPending ? (
            <Button variant="outline" onClick={handleCancel}>
              {t('routes.claudeBatchTest.cancel')}
            </Button>
          ) : null}
          <Button
            onClick={handleRun}
            disabled={!canRun || batchTest.isPending}
            className="min-w-36"
          >
            {batchTest.isPending ? (
              <Loader2 className="h-4 w-4 mr-2 animate-spin" />
            ) : (
              <Play className="h-4 w-4 mr-2" />
            )}
            {batchTest.isPending
              ? t('routes.claudeBatchTest.testing')
              : t('routes.claudeBatchTest.run')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
