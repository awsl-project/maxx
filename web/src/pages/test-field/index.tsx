import { useMemo, useRef, useState } from 'react';
import { FlaskConical, Plus, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui';
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

export function TestFieldPage() {
  const { t } = useTranslation();
  const providersQuery = useProviders();
  const providers = useMemo(() => providersQuery.data ?? [], [providersQuery.data]);
  const [providerToAdd, setProviderToAdd] = useState<string>('');
  const [selectedProviderIDs, setSelectedProviderIDs] = useState<number[]>([]);
  const [prompt, setPrompt] = useState('请用一句话回答：你现在可用吗？');
  const [concurrency, setConcurrency] = useState(4);
  const [timeoutMs, setTimeoutMs] = useState(30000);
  const [maxModelsPerProvider, setMaxModelsPerProvider] = useState(20);
  const [result, setResult] = useState<TestFieldModelBenchmarkResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

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
  const providerToAddLabel = providerToAdd
    ? providerLabel(
        providers.find((provider) => provider.id === Number(providerToAdd)) ??
          ({ id: Number(providerToAdd), name: `#${providerToAdd}`, type: '-' } as Provider),
      )
    : t('testField.benchmark.providerPlaceholder');
  const canRun = selectedProviderIDs.length > 0 && prompt.trim().length > 0 && !isRunning;

  const addProvider = () => {
    const id = Number(providerToAdd);
    if (!id || selectedProviderIDs.includes(id)) return;
    setSelectedProviderIDs((ids) => [...ids, id]);
    setProviderToAdd('');
  };

  const removeProvider = (id: number) => {
    if (isRunning) return;
    setSelectedProviderIDs((ids) => ids.filter((providerID) => providerID !== id));
  };

  const runBenchmark = async () => {
    if (!canRun) return;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setIsRunning(true);
    setError(null);
    setResult(null);
    try {
      const response = await getTransport().runTestFieldModelBenchmark(
        {
          providerIDs: selectedProviderIDs,
          prompt: prompt.trim(),
          concurrency,
          timeoutMs,
          maxModelsPerProvider,
        },
        controller.signal,
      );
      setResult(response);
    } catch (err) {
      setError(
        controller.signal.aborted
          ? t('testField.benchmark.cancelled')
          : err instanceof Error
            ? err.message
            : t('testField.benchmark.failed'),
      );
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      setIsRunning(false);
    }
  };

  const cancelBenchmark = () => {
    abortRef.current?.abort();
    setError(t('testField.benchmark.cancelling'));
  };

  return (
    <div className="flex flex-1 flex-col gap-4 p-4 pt-0">
      <PageHeader title={t('testField.title')} description={t('testField.description')} />

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
              <Select
                value={providerToAdd}
                onValueChange={(value) => setProviderToAdd(value ?? '')}
                disabled={isRunning}
              >
                <SelectTrigger>
                  <SelectValue>{providerToAddLabel}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {availableProviders.map((provider) => (
                    <SelectItem key={provider.id} value={String(provider.id)}>
                      {providerLabel(provider)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
              <Label htmlFor="test-field-concurrency">{t('testField.benchmark.concurrency')}</Label>
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
                {t('testField.benchmark.maxModelsPerProvider')}
              </Label>
              <Input
                id="test-field-max-models"
                type="number"
                min={1}
                max={50}
                value={maxModelsPerProvider}
                onChange={(event) => setMaxModelsPerProvider(Number(event.target.value) || 20)}
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
        <Card className="border-border bg-card">
          <CardHeader className="border-b border-border py-4">
            <CardTitle className="text-base font-medium">
              {t('testField.benchmark.resultsTitle')}
            </CardTitle>
            <CardDescription>
              {t('testField.benchmark.resultsDescription', {
                count: result.results.length,
                concurrency: result.concurrency,
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
                  </div>
                  {provider.error && <div className="mt-1 text-destructive">{provider.error}</div>}
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
                          <Badge variant="secondary">{t('testField.benchmark.available')}</Badge>
                        ) : (
                          <Badge variant="destructive">
                            {t('testField.benchmark.unavailable')}
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
  );
}
