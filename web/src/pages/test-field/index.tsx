import { useEffect, useMemo, useRef, useState } from 'react';
import { FlaskConical, Plus, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { PageHeader } from '@/components/layout/page-header';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui';
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox';
import { Textarea } from '@/components/ui/textarea';
import { useProviders } from '@/hooks/queries';
import { getTransport, type Provider, type TestFieldModelBenchmarkResponse } from '@/lib/transport';

const SUPPORTED_PROVIDER_TYPES = new Set(['custom', 'newapi', 'openrouter']);

function formatDuration(durationMs: number) {
  if (!Number.isFinite(durationMs) || durationMs < 0) return '-';
  if (durationMs < 1000) return `${durationMs}ms`;
  return `${(durationMs / 1000).toFixed(2)}s`;
}

function providerLabel(provider: Provider) {
  return `${provider.name} · ${provider.type} · #${provider.id}`;
}

type TestFieldBenchmarkCache = {
  providerToAdd: string;
  selectedProviderIDs: number[];
  prompt: string;
  concurrency: number;
  timeoutMs: number;
  minModelsPerProvider: number;
  result: TestFieldModelBenchmarkResponse | null;
  activeJobID: string | null;
};

const testFieldBenchmarkCacheKey = ['test-field', 'benchmark-state'] as const;

export function TestFieldPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const cachedBenchmarkState = queryClient.getQueryData<TestFieldBenchmarkCache>(
    testFieldBenchmarkCacheKey,
  );
  const providersQuery = useProviders();
  const providers = useMemo(() => providersQuery.data ?? [], [providersQuery.data]);
  const [providerToAdd, setProviderToAdd] = useState<string>(
    cachedBenchmarkState?.providerToAdd ?? '',
  );
  const [providerSearch, setProviderSearch] = useState('');
  const [selectedProviderIDs, setSelectedProviderIDs] = useState<number[]>(
    cachedBenchmarkState?.selectedProviderIDs ?? [],
  );
  const [prompt, setPrompt] = useState(
    cachedBenchmarkState?.prompt ?? '请用一句话回答：你现在可用吗？',
  );
  const [concurrency, setConcurrency] = useState(cachedBenchmarkState?.concurrency ?? 4);
  const [timeoutMs, setTimeoutMs] = useState(cachedBenchmarkState?.timeoutMs ?? 30000);
  const [minModelsPerProvider, setMinModelsPerProvider] = useState(
    cachedBenchmarkState?.minModelsPerProvider ?? 20,
  );
  const [result, setResult] = useState<TestFieldModelBenchmarkResponse | null>(
    cachedBenchmarkState?.result ?? null,
  );
  const [activeJobID, setActiveJobID] = useState<string | null>(
    cachedBenchmarkState?.activeJobID ?? null,
  );
  const [error, setError] = useState<string | null>(null);
  const isRunning = result?.status === 'running' || activeJobID !== null;
  const activeJobIDRef = useRef<string | null>(activeJobID);
  const resultsCardRef = useRef<HTMLDivElement | null>(null);

  const selectedProviders = useMemo(
    () =>
      selectedProviderIDs
        .map((id) => providers.find((provider) => provider.id === id))
        .filter(Boolean) as Provider[],
    [providers, selectedProviderIDs],
  );
  const availableProviders = providers.filter(
    (provider) => !selectedProviderIDs.includes(provider.id),
  );
  const filteredAvailableProviders = useMemo(() => {
    const query = providerSearch.trim().toLowerCase();
    if (!query) return availableProviders;
    return availableProviders.filter((provider) =>
      providerLabel(provider).toLowerCase().includes(query),
    );
  }, [availableProviders, providerSearch]);
  const providerToAddLabel = providerToAdd
    ? providerLabel(
        providers.find((provider) => provider.id === Number(providerToAdd)) ??
          ({ id: Number(providerToAdd), name: `#${providerToAdd}`, type: '-' } as Provider),
      )
    : t('testField.benchmark.providerPlaceholder');
  const canRun = selectedProviderIDs.length > 0 && prompt.trim().length > 0 && !isRunning;

  useEffect(() => {
    queryClient.setQueryData<TestFieldBenchmarkCache>(testFieldBenchmarkCacheKey, {
      providerToAdd,
      selectedProviderIDs,
      prompt,
      concurrency,
      timeoutMs,
      minModelsPerProvider,
      result,
      activeJobID,
    });
  }, [
    activeJobID,
    concurrency,
    minModelsPerProvider,
    prompt,
    providerToAdd,
    queryClient,
    result,
    selectedProviderIDs,
    timeoutMs,
  ]);

  useEffect(() => {
    activeJobIDRef.current = activeJobID;
  }, [activeJobID]);

  useEffect(() => {
    if (!result) return;
    requestAnimationFrame(() => {
      resultsCardRef.current?.scrollIntoView({ block: 'start', behavior: 'smooth' });
    });
  }, [result]);

  const addProvider = () => {
    const id = Number(providerToAdd);
    if (!id || selectedProviderIDs.includes(id)) return;
    setSelectedProviderIDs((ids) => [...ids, id]);
    setProviderToAdd('');
    setProviderSearch('');
  };

  const removeProvider = (id: number) => {
    if (isRunning) return;
    setSelectedProviderIDs((ids) => ids.filter((providerID) => providerID !== id));
  };

  const runBenchmark = async () => {
    if (!canRun) return;
    setError(null);
    setResult({
      status: 'running',
      prompt: prompt.trim(),
      concurrency,
      timeoutMs,
      minModelsPerProvider,
      startedAt: new Date().toISOString(),
      finishedAt: '',
      providers: [],
      results: [],
      totalTargets: 0,
      completedTargets: 0,
      cachedResultCount: 0,
    });
    try {
      const response = await getTransport().startTestFieldModelBenchmark({
        providerIDs: selectedProviderIDs,
        prompt: prompt.trim(),
        concurrency,
        timeoutMs,
        minModelsPerProvider,
        reuseCachedModelLists: true,
        reuseCachedResults: true,
      });
      setActiveJobID(response.jobID);
    } catch (err) {
      setActiveJobID(null);
      setResult(null);
      setError(err instanceof Error ? err.message : t('testField.benchmark.failed'));
    }
  };

  const cancelBenchmark = async () => {
    const jobID = activeJobIDRef.current;
    if (!jobID) return;
    setError(t('testField.benchmark.cancelling'));
    try {
      const response = await getTransport().cancelTestFieldModelBenchmarkJob(jobID);
      setResult(response);
      setActiveJobID(null);
      setError(t('testField.benchmark.cancelled'));
    } catch (err) {
      setError(err instanceof Error ? err.message : t('testField.benchmark.failed'));
    }
  };

  useEffect(() => {
    if (!activeJobID) return;
    let stopped = false;
    const poll = async () => {
      try {
        const response = await getTransport().getTestFieldModelBenchmarkJob(activeJobID);
        if (stopped || activeJobIDRef.current !== activeJobID) return;
        setResult(response);
        if (response.status && response.status !== 'running') {
          setActiveJobID(null);
          if (response.status === 'failed' && response.error) setError(response.error);
          if (response.status === 'cancelled') setError(t('testField.benchmark.cancelled'));
        }
      } catch (err) {
        if (stopped) return;
        setActiveJobID(null);
        setError(err instanceof Error ? err.message : t('testField.benchmark.failed'));
      }
    };
    void poll();
    const timer = window.setInterval(() => void poll(), 1500);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, [activeJobID, t]);

  return (
    <div className="flex h-full flex-col bg-background">
      <PageHeader title={t('testField.title')} description={t('testField.description')} />

      <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
        <div className="mx-auto max-w-7xl space-y-4">
          <Card className="border-border bg-card">
            <CardHeader className="border-b border-border py-4">
              <CardTitle className="text-base font-medium flex items-center gap-2">
                <FlaskConical className="h-4 w-4 text-muted-foreground" />
                {t('testField.benchmark.title')}
              </CardTitle>
              <CardDescription>{t('testField.benchmark.description')}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5 p-6">
              <div className="grid gap-3 md:grid-cols-[1fr_auto]">
                <div className="space-y-2">
                  <Label>{t('testField.benchmark.providerSelect')}</Label>
                  <Combobox
                    value={providerToAdd || null}
                    onValueChange={(value) => setProviderToAdd(value ?? '')}
                    onInputValueChange={(value) => setProviderSearch(value)}
                    itemToStringLabel={(value) =>
                      providerLabel(
                        providers.find((provider) => provider.id === Number(value)) ??
                          ({ id: Number(value), name: `#${value}`, type: '-' } as Provider),
                      )
                    }
                  >
                    <ComboboxInput
                      className="w-full"
                      disabled={isRunning}
                      placeholder={providerToAddLabel}
                      showClear
                    />
                    <ComboboxContent>
                      <ComboboxEmpty>{t('testField.benchmark.providerSearchEmpty')}</ComboboxEmpty>
                      <ComboboxList>
                        {filteredAvailableProviders.map((provider) => (
                          <ComboboxItem key={provider.id} value={String(provider.id)}>
                            {providerLabel(provider)}
                          </ComboboxItem>
                        ))}
                      </ComboboxList>
                    </ComboboxContent>
                  </Combobox>
                </div>
                <Button
                  className="self-end"
                  type="button"
                  onClick={addProvider}
                  disabled={!providerToAdd || isRunning}
                >
                  <Plus className="mr-2 h-4 w-4" />
                  {t('testField.benchmark.addProvider')}
                </Button>
              </div>

              <div className="flex flex-wrap gap-2">
                {selectedProviders.length === 0 ? (
                  <span className="text-sm text-muted-foreground">
                    {t('testField.benchmark.noSelectedProviders')}
                  </span>
                ) : (
                  selectedProviders.map((provider) => {
                    const supported = SUPPORTED_PROVIDER_TYPES.has(provider.type);
                    return (
                      <Badge
                        key={provider.id}
                        variant={supported ? 'secondary' : 'outline'}
                        className="gap-2 py-1"
                      >
                        <span>{providerLabel(provider)}</span>
                        {!supported && (
                          <span className="text-muted-foreground">
                            {t('testField.benchmark.unsupportedBadge')}
                          </span>
                        )}
                        <button
                          type="button"
                          className="rounded-sm text-muted-foreground hover:text-foreground disabled:opacity-50"
                          aria-label={t('testField.benchmark.removeProvider')}
                          onClick={() => removeProvider(provider.id)}
                          disabled={isRunning}
                        >
                          <X className="h-3 w-3" />
                        </button>
                      </Badge>
                    );
                  })
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="test-field-prompt">{t('testField.benchmark.prompt')}</Label>
                <Textarea
                  id="test-field-prompt"
                  value={prompt}
                  onChange={(event) => setPrompt(event.target.value)}
                  disabled={isRunning}
                  rows={3}
                />
              </div>

              <div className="grid gap-3 md:grid-cols-3">
                <div className="space-y-2">
                  <Label htmlFor="test-field-concurrency">
                    {t('testField.benchmark.concurrency')}
                  </Label>
                  <Input
                    id="test-field-concurrency"
                    type="number"
                    min={1}
                    max={10}
                    value={concurrency}
                    onChange={(event) => setConcurrency(Number(event.target.value) || 1)}
                    disabled={isRunning}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="test-field-timeout">{t('testField.benchmark.timeoutMs')}</Label>
                  <Input
                    id="test-field-timeout"
                    type="number"
                    min={1000}
                    max={30000}
                    value={timeoutMs}
                    onChange={(event) => setTimeoutMs(Number(event.target.value) || 30000)}
                    disabled={isRunning}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="test-field-max-models">
                    {t('testField.benchmark.minModelsPerProvider')}
                  </Label>
                  <Input
                    id="test-field-max-models"
                    type="number"
                    min={1}
                    max={50}
                    value={minModelsPerProvider}
                    onChange={(event) => setMinModelsPerProvider(Number(event.target.value) || 20)}
                    disabled={isRunning}
                  />
                </div>
              </div>

              <div className="flex items-center gap-2">
                <Button type="button" onClick={runBenchmark} disabled={!canRun}>
                  {isRunning ? t('testField.benchmark.running') : t('testField.benchmark.run')}
                </Button>
                {isRunning && (
                  <Button type="button" variant="outline" onClick={cancelBenchmark}>
                    {t('testField.benchmark.cancel')}
                  </Button>
                )}
                <span className="text-xs text-muted-foreground">
                  {t('testField.benchmark.manualOnlyHint')}
                </span>
              </div>

              {error && (
                <div className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
                  {error}
                </div>
              )}
            </CardContent>
          </Card>

          {result && (
            <Card ref={resultsCardRef} className="border-border bg-card">
              <CardHeader className="border-b border-border py-4">
                <CardTitle className="text-base font-medium">
                  {t('testField.benchmark.resultsTitle')}
                </CardTitle>
                <CardDescription>
                  {t('testField.benchmark.resultsDescription', {
                    count: result.results.length,
                    concurrency: result.concurrency,
                    completed: result.completedTargets,
                    total: result.totalTargets,
                    cached: result.cachedResultCount,
                    status: result.status ?? 'completed',
                  })}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4 p-6">
                <div className="grid gap-2 md:grid-cols-2">
                  {result.providers.map((provider) => (
                    <div
                      key={provider.providerID}
                      className="rounded-md border border-border p-3 text-sm"
                    >
                      <div className="font-medium">
                        {provider.providerName || `#${provider.providerID}`}
                      </div>
                      <div className="text-muted-foreground">
                        {provider.providerType || '-'} ·{' '}
                        {t('testField.benchmark.modelsSummary', {
                          tested: provider.testedCount,
                          total: provider.modelCount,
                        })}
                        {provider.cachedModels ? ` · ${t('testField.benchmark.cachedModels')}` : ''}
                      </div>
                      {provider.error && (
                        <div className="mt-1 text-destructive">{provider.error}</div>
                      )}
                    </div>
                  ))}
                </div>

                <div className="overflow-x-auto rounded-md border border-border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('testField.benchmark.rank')}</TableHead>
                        <TableHead>{t('testField.benchmark.provider')}</TableHead>
                        <TableHead>{t('testField.benchmark.model')}</TableHead>
                        <TableHead>{t('testField.benchmark.duration')}</TableHead>
                        <TableHead>{t('testField.benchmark.status')}</TableHead>
                        <TableHead>{t('testField.benchmark.response')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {result.results.map((item, index) => (
                        <TableRow key={`${item.providerID}:${item.model}`}>
                          <TableCell>{index + 1}</TableCell>
                          <TableCell>{item.providerName}</TableCell>
                          <TableCell className="font-mono text-xs">{item.model}</TableCell>
                          <TableCell>{formatDuration(item.durationMs)}</TableCell>
                          <TableCell>
                            {item.available ? (
                              <Badge variant="secondary">
                                {t('testField.benchmark.available')}
                              </Badge>
                            ) : (
                              <Badge variant="destructive">
                                {t('testField.benchmark.unavailable')}
                              </Badge>
                            )}
                            {item.cached && (
                              <Badge variant="outline" className="ml-2">
                                {t('testField.benchmark.cached')}
                              </Badge>
                            )}
                          </TableCell>
                          <TableCell className="max-w-xl truncate text-sm text-muted-foreground">
                            {item.available ? item.response : item.error}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
