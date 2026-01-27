import {
  Badge,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from '@/components/ui';
import { Server, Code, Database, Info, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { ProxyUpstreamAttempt, ProxyRequest, ModelPricing } from '@/lib/transport';
import { cn, formatDuration } from '@/lib/utils';
import { CopyButton, CopyAsCurlButton, DiffButton, EmptyState } from './components';
import { RequestDetailView } from './RequestDetailView';
import { usePricing } from '@/hooks/queries';

// Selection type: either the main request or an attempt
type SelectionType = { type: 'request' } | { type: 'attempt'; attemptId: number };

function formatCost(nanoUSD: number): string {
  if (nanoUSD === 0) return '-';
  // 向下取整到 6 位小数 (microUSD 精度)
  const usd = Math.floor(nanoUSD / 1000) / 1_000_000;
  return `$${usd.toFixed(6)}`;
}

// Cost breakdown item
export interface CostBreakdownItem {
  label: string;
  tokens: number;
  pricePerM: number; // microUSD/M tokens
  cost: number; // nanoUSD
}

// Cost breakdown result
export interface CostBreakdown {
  model: string;
  pricing?: ModelPricing;
  items: CostBreakdownItem[];
  totalCost: number; // nanoUSD
}

// MicroToNano conversion factor
const MICRO_TO_NANO = 1000;

// Calculate linear cost (same as backend CalculateLinearCost)
// Returns nanoUSD
function calculateLinearCost(tokens: number, priceMicro: number): number {
  // Use BigInt to prevent overflow for large token counts
  const t = BigInt(tokens);
  const p = BigInt(priceMicro);
  const microToNano = BigInt(MICRO_TO_NANO);
  const tokensPerMillion = BigInt(1_000_000);

  const result = (t * p * microToNano) / tokensPerMillion;
  return Number(result);
}

// Calculate tiered cost for 1M context models (same as backend CalculateTieredCost)
// Returns nanoUSD
function calculateTieredCost(
  tokens: number,
  basePriceMicro: number,
  premiumNum: number,
  premiumDenom: number,
  threshold: number,
): number {
  if (tokens <= threshold) {
    return calculateLinearCost(tokens, basePriceMicro);
  }

  const baseCostNano = calculateLinearCost(threshold, basePriceMicro);
  const premiumTokens = tokens - threshold;

  // Use BigInt for premium calculation
  const t = BigInt(premiumTokens);
  const p = BigInt(basePriceMicro);
  const microToNano = BigInt(MICRO_TO_NANO);
  const tokensPerMillion = BigInt(1_000_000);
  const num = BigInt(premiumNum);
  const denom = BigInt(premiumDenom);

  const premiumCostNano = (t * p * microToNano * num) / tokensPerMillion / denom;
  return baseCostNano + Number(premiumCostNano);
}

// Calculate cost breakdown from request/attempt data and pricing table
function calculateCostBreakdown(
  model: string,
  inputTokens: number,
  outputTokens: number,
  cacheReadTokens: number,
  cacheWriteTokens: number,
  cache5mWriteTokens: number,
  cache1hWriteTokens: number,
  priceTable?: Record<string, ModelPricing>,
): CostBreakdown {
  const items: CostBreakdownItem[] = [];
  let pricing: ModelPricing | undefined;

  // Find pricing for model (exact match first, then prefix match)
  if (priceTable) {
    pricing = priceTable[model];
    if (!pricing) {
      // Try prefix match (find longest matching prefix)
      let bestMatch: ModelPricing | undefined;
      let bestLen = 0;
      for (const [key, p] of Object.entries(priceTable)) {
        if (model.startsWith(key) && key.length > bestLen) {
          bestMatch = p;
          bestLen = key.length;
        }
      }
      pricing = bestMatch;
    }
  }

  if (pricing) {
    // Get 1M context settings
    const has1MContext = pricing.has1mContext || false;
    const threshold = pricing.context1mThreshold || 200_000;
    const inputPremiumNum = pricing.inputPremiumNum || 2;
    const inputPremiumDenom = pricing.inputPremiumDenom || 1;
    const outputPremiumNum = pricing.outputPremiumNum || 3;
    const outputPremiumDenom = pricing.outputPremiumDenom || 2;

    // Input tokens
    if (inputTokens > 0) {
      const cost = has1MContext
        ? calculateTieredCost(
            inputTokens,
            pricing.inputPriceMicro,
            inputPremiumNum,
            inputPremiumDenom,
            threshold,
          )
        : calculateLinearCost(inputTokens, pricing.inputPriceMicro);
      items.push({
        label: 'Input',
        tokens: inputTokens,
        pricePerM: pricing.inputPriceMicro,
        cost,
      });
    }

    // Output tokens
    if (outputTokens > 0) {
      const cost = has1MContext
        ? calculateTieredCost(
            outputTokens,
            pricing.outputPriceMicro,
            outputPremiumNum,
            outputPremiumDenom,
            threshold,
          )
        : calculateLinearCost(outputTokens, pricing.outputPriceMicro);
      items.push({
        label: 'Output',
        tokens: outputTokens,
        pricePerM: pricing.outputPriceMicro,
        cost,
      });
    }

    // Cache read
    if (cacheReadTokens > 0) {
      const cacheReadPrice =
        pricing.cacheReadPriceMicro || Math.floor(pricing.inputPriceMicro / 10);
      items.push({
        label: 'Cache Read',
        tokens: cacheReadTokens,
        pricePerM: cacheReadPrice,
        cost: calculateLinearCost(cacheReadTokens, cacheReadPrice),
      });
    }

    // Cache write (5m or 1h)
    if (cache5mWriteTokens > 0) {
      const cache5mPrice =
        pricing.cache5mWritePriceMicro || Math.floor((pricing.inputPriceMicro * 5) / 4);
      items.push({
        label: 'Cache Write (5m)',
        tokens: cache5mWriteTokens,
        pricePerM: cache5mPrice,
        cost: calculateLinearCost(cache5mWriteTokens, cache5mPrice),
      });
    }
    if (cache1hWriteTokens > 0) {
      const cache1hPrice =
        pricing.cache1hWritePriceMicro || Math.floor(pricing.inputPriceMicro * 2);
      items.push({
        label: 'Cache Write (1h)',
        tokens: cache1hWriteTokens,
        pricePerM: cache1hPrice,
        cost: calculateLinearCost(cache1hWriteTokens, cache1hPrice),
      });
    }
    // Fallback: if no 5m/1h breakdown but has cacheWrite
    if (cache5mWriteTokens === 0 && cache1hWriteTokens === 0 && cacheWriteTokens > 0) {
      const cacheWritePrice =
        pricing.cache5mWritePriceMicro || Math.floor((pricing.inputPriceMicro * 5) / 4);
      items.push({
        label: 'Cache Write',
        tokens: cacheWriteTokens,
        pricePerM: cacheWritePrice,
        cost: calculateLinearCost(cacheWriteTokens, cacheWritePrice),
      });
    }
  }

  const totalCost = items.reduce((sum, item) => sum + item.cost, 0);

  return { model, pricing, items, totalCost };
}

function formatJSON(obj: unknown): string {
  if (!obj) return '-';
  try {
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(obj);
  }
}

interface RequestDetailPanelProps {
  request: ProxyRequest;
  selection: SelectionType;
  attempts: ProxyUpstreamAttempt[] | undefined;
  activeTab: 'request' | 'response' | 'metadata';
  setActiveTab: (tab: 'request' | 'response' | 'metadata') => void;
  providerMap: Map<number, string>;
  projectMap: Map<number, string>;
  sessionMap: Map<string, { clientType: string; projectID: number }>;
  tokenMap: Map<number, string>;
}

export function RequestDetailPanel({
  request,
  selection,
  attempts,
  activeTab,
  setActiveTab,
  providerMap,
  projectMap,
  sessionMap,
  tokenMap,
}: RequestDetailPanelProps) {
  const { t } = useTranslation();
  const { data: priceTable } = usePricing();
  const selectedAttempt =
    selection.type === 'attempt' ? attempts?.find((a) => a.id === selection.attemptId) : null;

  // Calculate cost breakdown for request
  const requestCostBreakdown = priceTable
    ? calculateCostBreakdown(
        request.responseModel || request.requestModel,
        request.inputTokenCount,
        request.outputTokenCount,
        request.cacheReadCount,
        request.cacheWriteCount,
        request.cache5mWriteCount || 0,
        request.cache1hWriteCount || 0,
        priceTable.models,
      )
    : undefined;

  // Calculate cost breakdown for selected attempt
  const attemptCostBreakdown =
    selectedAttempt && priceTable
      ? calculateCostBreakdown(
          selectedAttempt.responseModel || selectedAttempt.mappedModel || selectedAttempt.requestModel,
          selectedAttempt.inputTokenCount,
          selectedAttempt.outputTokenCount,
          selectedAttempt.cacheReadCount,
          selectedAttempt.cacheWriteCount,
          selectedAttempt.cache5mWriteCount || 0,
          selectedAttempt.cache1hWriteCount || 0,
          priceTable.models,
        )
      : undefined;

  // Helper to format price per million tokens
  const formatPricePerM = (priceMicro: number): string => {
    const usd = priceMicro / 1_000_000;
    if (usd < 0.01) return `$${usd.toFixed(4)}/M`;
    if (usd < 1) return `$${usd.toFixed(2)}/M`;
    return `$${usd.toFixed(2)}/M`;
  };

  if (selection.type === 'request') {
    return (
      <RequestDetailView
        request={request}
        activeTab={activeTab}
        setActiveTab={setActiveTab}
        formatJSON={formatJSON}
        formatCost={formatCost}
        projectName={projectMap.get(request.projectID)}
        sessionInfo={sessionMap.get(request.sessionID)}
        projectMap={projectMap}
        tokenName={tokenMap.get(request.apiTokenID)}
        costBreakdown={requestCostBreakdown}
      />
    );
  }

  if (!selectedAttempt) {
    return (
      <EmptyState
        message={t('requests.selectAttempt')}
        icon={<Server className="h-12 w-12 mb-4 opacity-10" />}
      />
    );
  }

  return (
    <Tabs
      value={activeTab}
      onValueChange={(value) => setActiveTab(value as typeof activeTab)}
      className="flex flex-col h-full overflow-hidden min-w-0"
    >
      {/* Detail Header */}
      <div className="h-16 border-b border-border bg-muted/20 px-6 flex items-center justify-between shrink-0 backdrop-blur-sm sticky top-0 z-10">
        <div className="flex items-center gap-4">
          <div className="w-10 h-10 rounded-lg bg-card flex items-center justify-center text-foreground shadow-sm border border-border">
            <Server size={20} />
          </div>
          <div>
            <h3 className="text-sm font-medium text-foreground">
              {providerMap.get(selectedAttempt.providerID) ||
                `Provider #${selectedAttempt.providerID}`}
            </h3>
            <div className="flex items-center gap-3 text-xs text-text-secondary mt-0.5">
              <span>{t('requests.attemptId', { id: selectedAttempt.id })}</span>
              {selectedAttempt.mappedModel &&
                selectedAttempt.requestModel !== selectedAttempt.mappedModel && (
                  <span className="text-muted-foreground">
                    <span className="text-muted-foreground">{selectedAttempt.requestModel}</span>
                    <span className="mx-1">→</span>
                    <span className="text-foreground">{selectedAttempt.mappedModel}</span>
                  </span>
                )}
              {selectedAttempt.cost > 0 && (
                <span className="text-blue-400 font-medium">
                  {t('requests.cost')}: {formatCost(selectedAttempt.cost)}
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Detail Tabs */}
        <TabsList>
          <TabsTrigger value="request" className="border-none">
            {t('requests.tabs.request')}
          </TabsTrigger>
          <TabsTrigger value="response" className="border-none">
            {t('requests.tabs.response')}
          </TabsTrigger>
          <TabsTrigger value="metadata" className="border-none">
            {t('requests.tabs.metadata')}
          </TabsTrigger>
        </TabsList>
      </div>

      {/* Detail Content */}
      <TabsContent value="request" className="flex-1 overflow-hidden flex flex-col min-w-0 mt-0">
        {selectedAttempt.requestInfo ? (
          <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in min-w-0">
            <div className="flex items-center gap-3 p-3 bg-muted/30 rounded-lg border border-border shrink-0">
              <Badge variant="info" className="font-mono text-xs">
                {selectedAttempt.requestInfo.method}
              </Badge>
              <code className="flex-1 font-mono text-xs text-foreground break-all">
                {selectedAttempt.requestInfo.url}
              </code>
              <CopyAsCurlButton requestInfo={selectedAttempt.requestInfo} />
            </div>

            <div className="flex flex-col min-h-0 flex-1 gap-6">
              <div className="flex flex-col min-h-0 gap-3 flex-1">
                <div className="flex items-center justify-between shrink-0">
                  <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                    <Code size={14} /> {t('requests.headers')}
                  </h5>
                  <div className="flex items-center gap-2">
                    <DiffButton
                      clientContent={formatJSON(request.requestInfo?.headers || {})}
                      upstreamContent={formatJSON(selectedAttempt.requestInfo.headers)}
                      title={t('requests.compareHeaders')}
                    />
                    <CopyButton content={formatJSON(selectedAttempt.requestInfo.headers)} />
                  </div>
                </div>
                <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                  <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Badge variant="outline" className="text-[10px] bg-card/80 backdrop-blur-sm">
                      JSON
                    </Badge>
                  </div>
                  <pre className="text-xs font-mono text-foreground/90 leading-relaxed whitespace-pre overflow-x-auto">
                    {formatJSON(selectedAttempt.requestInfo.headers)}
                  </pre>
                </div>
              </div>

              {selectedAttempt.requestInfo.body && (
                <div className="flex flex-col min-h-0 gap-3 flex-1">
                <div className="flex items-center justify-between shrink-0">
                  <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                      <Database size={14} /> {t('requests.body')}
                    </h5>
                    <div className="flex items-center gap-2">
                      <DiffButton
                        clientContent={(() => {
                          try {
                            return formatJSON(JSON.parse(request.requestInfo?.body || '{}'));
                          } catch {
                            return request.requestInfo?.body || '';
                          }
                        })()}
                        upstreamContent={(() => {
                          try {
                            return formatJSON(JSON.parse(selectedAttempt.requestInfo.body));
                          } catch {
                            return selectedAttempt.requestInfo.body;
                          }
                        })()}
                        title={t('requests.compareBody')}
                      />
                      <CopyButton
                        content={(() => {
                          try {
                            return formatJSON(JSON.parse(selectedAttempt.requestInfo.body));
                          } catch {
                            return selectedAttempt.requestInfo.body;
                          }
                        })()}
                      />
                    </div>
                  </div>
                  <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                    <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Badge variant="outline" className="text-[10px] bg-card/80 backdrop-blur-sm">
                        JSON
                      </Badge>
                    </div>
                    <pre className="text-xs font-mono text-foreground/95 whitespace-pre overflow-x-auto leading-relaxed">
                      {(() => {
                        try {
                          return formatJSON(JSON.parse(selectedAttempt.requestInfo.body));
                        } catch {
                          return selectedAttempt.requestInfo.body;
                        }
                      })()}
                    </pre>
                  </div>
                </div>
              )}
            </div>
          </div>
        ) : (
          <EmptyState message={t('requests.noRequestData')} />
        )}
      </TabsContent>

      <TabsContent value="response" className="flex-1 overflow-hidden flex flex-col min-w-0 mt-0">
        {selectedAttempt.responseInfo ? (
          <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in min-w-0">
            <div className="flex items-center gap-3 p-3 bg-muted/30 rounded-lg border border-border shrink-0">
              <div
                className={cn(
                  'px-2 py-1 rounded text-xs font-bold font-mono',
                  selectedAttempt.responseInfo.status >= 400
                    ? 'bg-red-400/10 text-red-400'
                    : 'bg-blue-400/10 text-blue-400',
                )}
              >
                {selectedAttempt.responseInfo.status}
              </div>
              <span className="text-sm text-muted-foreground font-medium">
                {t('requests.responseStatus')}
              </span>
            </div>

            <div className="flex flex-col min-h-0 flex-1 gap-6">
              <div className="flex flex-col min-h-0 gap-3 flex-1">
                <div className="flex items-center justify-between shrink-0">
                  <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                    <Code size={14} /> {t('requests.headers')}
                  </h5>
                  <CopyButton content={formatJSON(selectedAttempt.responseInfo.headers)} />
                </div>
                <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                  <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Badge variant="outline" className="text-[10px] bg-card/80 backdrop-blur-sm">
                      JSON
                    </Badge>
                  </div>
                  <pre className="text-xs font-mono text-foreground/90 leading-relaxed whitespace-pre overflow-x-auto">
                    {formatJSON(selectedAttempt.responseInfo.headers)}
                  </pre>
                </div>
              </div>

              {selectedAttempt.responseInfo.body && (
                <div className="flex flex-col min-h-0 gap-3 flex-1">
                <div className="flex items-center justify-between shrink-0">
                  <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                      <Database size={14} /> {t('requests.body')}
                    </h5>
                    <CopyButton
                      content={(() => {
                        try {
                          return formatJSON(JSON.parse(selectedAttempt.responseInfo.body));
                        } catch {
                          return selectedAttempt.responseInfo.body;
                        }
                      })()}
                    />
                  </div>
                  <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                    <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Badge variant="outline" className="text-[10px] bg-card/80 backdrop-blur-sm">
                        JSON
                      </Badge>
                    </div>
                    <pre className="text-xs font-mono text-foreground/95 whitespace-pre overflow-x-auto leading-relaxed">
                      {(() => {
                        try {
                          return formatJSON(JSON.parse(selectedAttempt.responseInfo.body));
                        } catch {
                          return selectedAttempt.responseInfo.body;
                        }
                      })()}
                    </pre>
                  </div>
                </div>
              )}
            </div>
          </div>
        ) : (
          <EmptyState message={t('requests.noResponseData')} />
        )}
      </TabsContent>

      <TabsContent value="metadata" className="flex-1 overflow-y-auto p-6 mt-0">
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
          <Card className="bg-card border-border">
            <CardHeader className="pb-2 border-b border-border/50">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <Info size={16} className="text-info" />
                {t('requests.attemptInfo')}
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-4">
              <dl className="space-y-4">
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.attemptIdLabel')}
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded select-all break-all">
                    #{selectedAttempt.id}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.provider')}
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded">
                    {providerMap.get(selectedAttempt.providerID) ||
                      t('requests.providerFallback', { id: selectedAttempt.providerID })}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.requestModel')}
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded">
                    {selectedAttempt.requestModel || '-'}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.mappedModel')}
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded">
                    {selectedAttempt.mappedModel || '-'}
                    {selectedAttempt.mappedModel &&
                      selectedAttempt.requestModel !== selectedAttempt.mappedModel && (
                        <span className="ml-2 text-muted-foreground text-[10px]">
                          {t('requests.converted')}
                        </span>
                      )}
                  </dd>
                </div>
                {selectedAttempt.responseModel && (
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                    <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                      {t('requests.responseModel')}
                    </dt>
                    <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded">
                      {selectedAttempt.responseModel}
                      {selectedAttempt.responseModel !== selectedAttempt.mappedModel && (
                        <span className="ml-2 text-muted-foreground text-[10px]">
                          {t('requests.upstream')}
                        </span>
                      )}
                    </dd>
                  </div>
                )}
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('common.status')}
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded">
                    {selectedAttempt.status}
                  </dd>
                </div>
              </dl>
            </CardContent>
          </Card>

          <Card className="bg-card border-border">
            <CardHeader className="pb-2 border-b border-border/50">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <Zap size={16} className="text-warning" />
                {t('requests.attemptUsageCache')}
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-4">
              <dl className="space-y-4">
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    TTFT
                  </dt>
                  <dd className="text-sm text-foreground font-mono font-medium">
                    {selectedAttempt.ttft && selectedAttempt.ttft > 0 ? formatDuration(selectedAttempt.ttft) : '-'}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.inputTokens')}
                  </dt>
                  <dd className="text-sm text-foreground font-mono font-medium flex items-center gap-2">
                    <span>{selectedAttempt.inputTokenCount.toLocaleString()}</span>
                    {attemptCostBreakdown?.items.find((i) => i.label === 'Input') && (
                      <span className="text-xs text-muted-foreground">
                        × {formatPricePerM(attemptCostBreakdown.items.find((i) => i.label === 'Input')!.pricePerM)} ={' '}
                        <span className="text-blue-400">
                          {formatCost(attemptCostBreakdown.items.find((i) => i.label === 'Input')!.cost)}
                        </span>
                      </span>
                    )}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.outputTokens')}
                  </dt>
                  <dd className="text-sm text-foreground font-mono font-medium flex items-center gap-2">
                    <span>{selectedAttempt.outputTokenCount.toLocaleString()}</span>
                    {attemptCostBreakdown?.items.find((i) => i.label === 'Output') && (
                      <span className="text-xs text-muted-foreground">
                        × {formatPricePerM(attemptCostBreakdown.items.find((i) => i.label === 'Output')!.pricePerM)} ={' '}
                        <span className="text-blue-400">
                          {formatCost(attemptCostBreakdown.items.find((i) => i.label === 'Output')!.cost)}
                        </span>
                      </span>
                    )}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.cacheRead')}
                  </dt>
                  <dd className="text-sm text-violet-400 font-mono font-medium flex items-center gap-2">
                    <span>{selectedAttempt.cacheReadCount.toLocaleString()}</span>
                    {attemptCostBreakdown?.items.find((i) => i.label === 'Cache Read') && (
                      <span className="text-xs text-muted-foreground">
                        × {formatPricePerM(attemptCostBreakdown.items.find((i) => i.label === 'Cache Read')!.pricePerM)} ={' '}
                        <span className="text-blue-400">
                          {formatCost(attemptCostBreakdown.items.find((i) => i.label === 'Cache Read')!.cost)}
                        </span>
                      </span>
                    )}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.cacheWrite')}
                  </dt>
                  <dd className="text-sm text-amber-400 font-mono font-medium flex items-center gap-2">
                    <span>{selectedAttempt.cacheWriteCount.toLocaleString()}</span>
                    {(() => {
                      const cache5m = attemptCostBreakdown?.items.find((i) => i.label === 'Cache Write (5m)');
                      const cache1h = attemptCostBreakdown?.items.find((i) => i.label === 'Cache Write (1h)');
                      const cacheWrite = attemptCostBreakdown?.items.find((i) => i.label === 'Cache Write');
                      const item = cache5m || cache1h || cacheWrite;
                      if (!item) return null;
                      return (
                        <span className="text-xs text-muted-foreground">
                          × {formatPricePerM(item.pricePerM)} ={' '}
                          <span className="text-blue-400">{formatCost(item.cost)}</span>
                        </span>
                      );
                    })()}
                  </dd>
                </div>
                {(selectedAttempt.cache5mWriteCount > 0 ||
                  selectedAttempt.cache1hWriteCount > 0) && (
                  <div className="flex justify-between items-center border-b border-border/30 pb-2 pl-4">
                    <dt className="text-xs font-medium text-muted-foreground/70 tracking-wider">
                      <span className="text-cyan-400/80">5m:</span>{' '}
                      {selectedAttempt.cache5mWriteCount}
                      <span className="mx-2">|</span>
                      <span className="text-orange-400/80">1h:</span>{' '}
                      {selectedAttempt.cache1hWriteCount}
                    </dt>
                    <dd className="text-xs text-muted-foreground font-mono">
                      {(() => {
                        const cache5m = attemptCostBreakdown?.items.find((i) => i.label === 'Cache Write (5m)');
                        const cache1h = attemptCostBreakdown?.items.find((i) => i.label === 'Cache Write (1h)');
                        const parts: string[] = [];
                        if (cache5m) parts.push(`5m: ${formatCost(cache5m.cost)}`);
                        if (cache1h) parts.push(`1h: ${formatCost(cache1h.cost)}`);
                        return parts.length > 0 ? parts.join(' | ') : null;
                      })()}
                    </dd>
                  </div>
                )}
                {/* Subtotal before multiplier - only show when multiplier != 1.0x */}
                {attemptCostBreakdown && selectedAttempt.multiplier > 0 && selectedAttempt.multiplier !== 10000 && (
                  <div className="flex justify-between items-center border-b border-border/30 pb-2">
                    <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                      {t('requests.subtotal')}
                    </dt>
                    <dd className="text-sm text-muted-foreground font-mono font-medium">
                      {formatCost(attemptCostBreakdown.totalCost)}
                    </dd>
                  </div>
                )}
                {/* Multiplier row - always show */}
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.multiplier')}
                  </dt>
                  <dd className={`text-sm font-mono font-medium ${selectedAttempt.multiplier > 0 && selectedAttempt.multiplier !== 10000 ? 'text-yellow-400' : 'text-foreground'}`}>
                    ×{((selectedAttempt.multiplier > 0 ? selectedAttempt.multiplier : 10000) / 10000).toFixed(2)}
                  </dd>
                </div>
                <div className="flex justify-between items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('requests.cost')}
                  </dt>
                  <dd className="text-sm font-mono font-medium flex items-center gap-2">
                    <span className="text-blue-400">{formatCost(selectedAttempt.cost)}</span>
                    {attemptCostBreakdown && (() => {
                      // Calculate expected cost with multiplier applied
                      const multiplier = selectedAttempt.multiplier > 0 ? selectedAttempt.multiplier : 10000;
                      const expectedCost = Math.floor(attemptCostBreakdown.totalCost * multiplier / 10000);
                      if (expectedCost !== selectedAttempt.cost) {
                        return (
                          <span
                            className="text-xs text-amber-400"
                            title={t('requests.costMismatchTitle')}
                          >
                            {t('requests.calculatedCost', { cost: formatCost(expectedCost) })}
                          </span>
                        );
                      }
                      return null;
                    })()}
                  </dd>
                </div>
              </dl>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
    </Tabs>
  );
}
