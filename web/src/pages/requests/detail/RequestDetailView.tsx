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
import { Code, Database, Info, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { ProxyRequest, ClientType } from '@/lib/transport';
import { cn, formatDuration } from '@/lib/utils';
import { ClientIcon, getClientName, getClientColor } from '@/components/icons/client-icons';
import { CopyButton, CopyAsCurlButton, EmptyState } from './components';
import type { CostBreakdown } from './RequestDetailPanel';

// 微美元转美元
const MICRO_USD_PER_USD = 1_000_000;

// 格式化价格 (microUSD/M tokens -> $/M tokens)
function formatPricePerM(priceMicro: number): string {
  const usd = priceMicro / MICRO_USD_PER_USD;
  if (usd < 0.01) return `$${usd.toFixed(4)}/M`;
  if (usd < 1) return `$${usd.toFixed(2)}/M`;
  return `$${usd.toFixed(2)}/M`;
}

interface RequestDetailViewProps {
  request: ProxyRequest;
  activeTab: 'request' | 'response' | 'metadata';
  setActiveTab: (tab: 'request' | 'response' | 'metadata') => void;
  formatJSON: (obj: unknown) => string;
  formatCost: (nanoUSD: number) => string;
  projectName?: string;
  sessionInfo?: { clientType: string; projectID: number };
  projectMap: Map<number, string>;
  tokenName?: string;
  costBreakdown?: CostBreakdown;
}

export function RequestDetailView({
  request,
  activeTab,
  setActiveTab,
  formatJSON,
  formatCost,
  projectName,
  sessionInfo,
  projectMap,
  tokenName,
  costBreakdown,
}: RequestDetailViewProps) {
  const { t } = useTranslation();
  return (
    <Tabs
      value={activeTab}
      onValueChange={(value) => setActiveTab(value as typeof activeTab)}
      className="flex flex-col h-full w-full min-w-0"
    >
      {/* Detail Header */}
      <div className="h-16 border-b border-border bg-muted/20 px-6 flex items-center justify-between shrink-0 backdrop-blur-sm sticky top-0 z-10">
        <div className="flex items-center gap-4">
          <div
            className="w-10 h-10 rounded-lg flex items-center justify-center shadow-sm border border-border"
            style={
              {
                backgroundColor: `${getClientColor(request.clientType as ClientType)}15`,
              } as React.CSSProperties
            }
          >
            <ClientIcon type={request.clientType as ClientType} size={20} />
          </div>
          <div>
            <h3 className="text-sm font-medium text-foreground">
              {getClientName(request.clientType as ClientType)} Request
            </h3>
            <div className="flex items-center gap-3 text-xs text-text-secondary mt-0.5">
              <span>{t('requests.requestId', { id: request.id })}</span>
              <span className="text-text-muted">·</span>
              <span>{request.requestModel}</span>
              {request.cost > 0 && (
                <span className="text-blue-400 font-medium">Cost: {formatCost(request.cost)}</span>
              )}
            </div>
          </div>
        </div>

        {/* Detail Tabs */}
        <TabsList>
          <TabsTrigger value="request" className={'border-none'}>
            {t('requests.tabs.request')}
          </TabsTrigger>
          <TabsTrigger value="response" className={'border-none'}>
            {t('requests.tabs.response')}
          </TabsTrigger>
          <TabsTrigger value="metadata" className={'border-none'}>
            {t('requests.tabs.metadata')}
          </TabsTrigger>
        </TabsList>
      </div>

      {/* Detail Content */}
      <TabsContent value="request" className="flex-1 overflow-hidden flex flex-col min-w-0 mt-0">
        {request.requestInfo ? (
          <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in min-w-0">
            <div className="flex items-center gap-3 p-3 bg-muted/30 rounded-lg border border-border shrink-0">
              <Badge variant="info" className="font-mono text-xs">
                {request.requestInfo.method}
              </Badge>
              <code className="flex-1 font-mono text-xs text-foreground break-all">
                {request.requestInfo.url}
              </code>
              <CopyAsCurlButton requestInfo={request.requestInfo} />
            </div>

            <div className="flex flex-col min-h-0 flex-1 gap-6">
              <div className="flex flex-col min-h-0 gap-3 flex-1">
                <div className="flex items-center justify-between shrink-0">
                  <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                    <Code size={14} /> Headers
                  </h5>
                  <CopyButton
                    content={formatJSON(request.requestInfo.headers)}
                    label={t('common.copy')}
                  />
                </div>
                <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                  <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Badge variant="outline" className="text-[10px] bg-card/80 backdrop-blur-sm">
                      JSON
                    </Badge>
                  </div>
                  <pre className="text-xs font-mono text-foreground/90 leading-relaxed whitespace-pre overflow-x-auto">
                    {formatJSON(request.requestInfo.headers)}
                  </pre>
                </div>
              </div>

              {request.requestInfo.body && (
                <div className="flex flex-col min-h-0 gap-3 flex-1">
                  <div className="flex items-center justify-between shrink-0">
                    <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                      <Database size={14} /> Body
                    </h5>
                    <CopyButton
                      content={(() => {
                        try {
                          return formatJSON(JSON.parse(request.requestInfo.body));
                        } catch {
                          return request.requestInfo.body;
                        }
                      })()}
                      label={t('common.copy')}
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
                          return formatJSON(JSON.parse(request.requestInfo.body));
                        } catch {
                          return request.requestInfo.body;
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
        {request.responseInfo ? (
          <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in min-w-0">
            <div className="flex items-center gap-3 p-3 bg-muted/30 rounded-lg border border-border shrink-0">
              <div
                className={cn(
                  'px-2 py-1 rounded text-xs font-bold font-mono',
                  request.responseInfo.status >= 400
                    ? 'bg-red-400/10 text-red-400'
                    : 'bg-blue-400/10 text-blue-400',
                )}
              >
                {request.responseInfo.status}
              </div>
              <span className="text-sm text-muted-foreground font-medium">Response Status</span>
            </div>

            <div className="flex flex-col min-h-0 flex-1 gap-6">
              <div className="flex flex-col min-h-0 gap-3 flex-1">
                <div className="flex items-center justify-between shrink-0">
                  <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                    <Code size={14} /> Headers
                  </h5>
                  <CopyButton
                    content={formatJSON(request.responseInfo.headers)}
                    label={t('common.copy')}
                  />
                </div>
                <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                  <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Badge variant="outline" className="text-[10px] bg-card/80 backdrop-blur-sm">
                      JSON
                    </Badge>
                  </div>
                  <pre className="text-xs font-mono text-foreground/90 leading-relaxed whitespace-pre overflow-x-auto">
                    {formatJSON(request.responseInfo.headers)}
                  </pre>
                </div>
              </div>

              {request.responseInfo.body && (
                <div className="flex flex-col min-h-0 gap-3 flex-1">
                  <div className="flex items-center justify-between shrink-0">
                    <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                      <Database size={14} /> Body
                    </h5>
                    <CopyButton
                      content={(() => {
                        try {
                          return formatJSON(JSON.parse(request.responseInfo.body));
                        } catch {
                          return request.responseInfo.body;
                        }
                      })()}
                      label={t('common.copy')}
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
                          return formatJSON(JSON.parse(request.responseInfo.body));
                        } catch {
                          return request.responseInfo.body;
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
                <Info size={16} className="text-info" /> Request Info
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-4">
              <dl className="space-y-4">
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Request ID
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded select-all break-all">
                    {request.requestID || '-'}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Session ID
                  </dt>
                  <dd className="sm:col-span-2">
                    <div className="font-mono text-xs text-foreground bg-muted px-2 py-1 rounded select-all break-all">
                      {request.sessionID || '-'}
                    </div>
                    {sessionInfo && (
                      <div className="flex items-center gap-2 mt-1 text-[10px] text-muted-foreground">
                        <span className="capitalize">{sessionInfo.clientType}</span>
                        {sessionInfo.projectID > 0 && (
                          <>
                            <span>·</span>
                            <span>
                              {projectMap.get(sessionInfo.projectID) ||
                                `Project #${sessionInfo.projectID}`}
                            </span>
                          </>
                        )}
                      </div>
                    )}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Instance ID
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded select-all break-all">
                    {request.instanceID || '-'}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Request Model
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded">
                    {request.requestModel || '-'}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Response Model
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded">
                    {request.responseModel || '-'}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Project
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-foreground bg-muted px-2 py-1 rounded">
                    {projectName || '-'}
                  </dd>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    API Token
                  </dt>
                  <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-muted px-2 py-1 rounded">
                    {tokenName || '-'}
                  </dd>
                </div>
              </dl>
            </CardContent>
          </Card>

          <Card className="bg-card border-border">
            <CardHeader className="pb-2 border-b border-border/50">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <Zap size={16} className="text-warning" /> Usage & Cache
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-4">
              <dl className="space-y-4">
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    TTFT
                  </dt>
                  <dd className="text-sm text-foreground font-mono font-medium">
                    {request.ttft && request.ttft > 0 ? formatDuration(request.ttft) : '-'}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Input Tokens
                  </dt>
                  <dd className="text-sm text-foreground font-mono font-medium flex items-center gap-2">
                    <span>{request.inputTokenCount.toLocaleString()}</span>
                    {costBreakdown?.items.find((i) => i.label === 'Input') && (
                      <span className="text-xs text-muted-foreground">
                        × {formatPricePerM(costBreakdown.items.find((i) => i.label === 'Input')!.pricePerM)} ={' '}
                        <span className="text-blue-400">
                          {formatCost(costBreakdown.items.find((i) => i.label === 'Input')!.cost)}
                        </span>
                      </span>
                    )}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Output Tokens
                  </dt>
                  <dd className="text-sm text-foreground font-mono font-medium flex items-center gap-2">
                    <span>{request.outputTokenCount.toLocaleString()}</span>
                    {costBreakdown?.items.find((i) => i.label === 'Output') && (
                      <span className="text-xs text-muted-foreground">
                        × {formatPricePerM(costBreakdown.items.find((i) => i.label === 'Output')!.pricePerM)} ={' '}
                        <span className="text-blue-400">
                          {formatCost(costBreakdown.items.find((i) => i.label === 'Output')!.cost)}
                        </span>
                      </span>
                    )}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Cache Read
                  </dt>
                  <dd className="text-sm text-violet-400 font-mono font-medium flex items-center gap-2">
                    <span>{request.cacheReadCount.toLocaleString()}</span>
                    {costBreakdown?.items.find((i) => i.label === 'Cache Read') && (
                      <span className="text-xs text-muted-foreground">
                        × {formatPricePerM(costBreakdown.items.find((i) => i.label === 'Cache Read')!.pricePerM)} ={' '}
                        <span className="text-blue-400">
                          {formatCost(costBreakdown.items.find((i) => i.label === 'Cache Read')!.cost)}
                        </span>
                      </span>
                    )}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Cache Write
                  </dt>
                  <dd className="text-sm text-amber-400 font-mono font-medium flex items-center gap-2">
                    <span>{request.cacheWriteCount.toLocaleString()}</span>
                    {(() => {
                      const cache5m = costBreakdown?.items.find((i) => i.label === 'Cache Write (5m)');
                      const cache1h = costBreakdown?.items.find((i) => i.label === 'Cache Write (1h)');
                      const cacheWrite = costBreakdown?.items.find((i) => i.label === 'Cache Write');
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
                {(request.cache5mWriteCount > 0 || request.cache1hWriteCount > 0) && (
                  <div className="flex justify-between items-center border-b border-border/30 pb-2 pl-4">
                    <dt className="text-xs font-medium text-muted-foreground/70 tracking-wider">
                      <span className="text-cyan-400/80">5m:</span> {request.cache5mWriteCount}
                      <span className="mx-2">|</span>
                      <span className="text-orange-400/80">1h:</span> {request.cache1hWriteCount}
                    </dt>
                    <dd className="text-xs text-muted-foreground font-mono">
                      {(() => {
                        const cache5m = costBreakdown?.items.find((i) => i.label === 'Cache Write (5m)');
                        const cache1h = costBreakdown?.items.find((i) => i.label === 'Cache Write (1h)');
                        const parts: string[] = [];
                        if (cache5m) parts.push(`5m: ${formatCost(cache5m.cost)}`);
                        if (cache1h) parts.push(`1h: ${formatCost(cache1h.cost)}`);
                        return parts.length > 0 ? parts.join(' | ') : null;
                      })()}
                    </dd>
                  </div>
                )}
                {/* Subtotal before multiplier - only show when multiplier != 1.0x */}
                {costBreakdown && request.multiplier > 0 && request.multiplier !== 10000 && (
                  <div className="flex justify-between items-center border-b border-border/30 pb-2">
                    <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                      Subtotal
                    </dt>
                    <dd className="text-sm text-muted-foreground font-mono font-medium">
                      {formatCost(costBreakdown.totalCost)}
                    </dd>
                  </div>
                )}
                {/* Multiplier row - always show */}
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Multiplier
                  </dt>
                  <dd className={`text-sm font-mono font-medium ${request.multiplier > 0 && request.multiplier !== 10000 ? 'text-yellow-400' : 'text-foreground'}`}>
                    ×{((request.multiplier > 0 ? request.multiplier : 10000) / 10000).toFixed(2)}
                  </dd>
                </div>
                <div className="flex justify-between items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Cost
                  </dt>
                  <dd className="text-sm font-mono font-medium flex items-center gap-2">
                    <span className="text-blue-400">{formatCost(request.cost)}</span>
                    {costBreakdown && (() => {
                      // Calculate expected cost with multiplier applied
                      const multiplier = request.multiplier > 0 ? request.multiplier : 10000;
                      const expectedCost = Math.floor(costBreakdown.totalCost * multiplier / 10000);
                      if (expectedCost !== request.cost) {
                        return (
                          <span className="text-xs text-amber-400" title="前端计算值与后端不一致">
                            (计算: {formatCost(expectedCost)})
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
