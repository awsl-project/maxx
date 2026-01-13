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
} from '@/components/ui'
import { Server, Code, Database, Info, Zap } from 'lucide-react'
import type { ProxyUpstreamAttempt, ProxyRequest } from '@/lib/transport'
import { cn } from '@/lib/utils'
import { CopyButton, DiffButton, EmptyState } from './components'
import { RequestDetailView } from './RequestDetailView'

// Selection type: either the main request or an attempt
type SelectionType =
  | { type: 'request' }
  | { type: 'attempt'; attemptId: number }

// 微美元转美元 (1 USD = 1,000,000 microUSD)
const MICRO_USD_PER_USD = 1_000_000
function microToUSD(microUSD: number): number {
  return microUSD / MICRO_USD_PER_USD
}

function formatCost(microUSD: number): string {
  if (microUSD === 0) return '-'
  const usd = microToUSD(microUSD)
  if (usd < 0.0001) return '<$0.0001'
  if (usd < 0.001) return `$${usd.toFixed(5)}`
  if (usd < 0.01) return `$${usd.toFixed(4)}`
  if (usd < 1) return `$${usd.toFixed(3)}`
  return `$${usd.toFixed(2)}`
}

function formatJSON(obj: unknown): string {
  if (!obj) return '-'
  try {
    return JSON.stringify(obj, null, 2)
  } catch {
    return String(obj)
  }
}

interface RequestDetailPanelProps {
  request: ProxyRequest
  selection: SelectionType
  attempts: ProxyUpstreamAttempt[] | undefined
  activeTab: 'request' | 'response' | 'metadata'
  setActiveTab: (tab: 'request' | 'response' | 'metadata') => void
  providerMap: Map<number, string>
  projectMap: Map<number, string>
  sessionMap: Map<string, { clientType: string; projectID: number }>
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
}: RequestDetailPanelProps) {
  const selectedAttempt =
    selection.type === 'attempt'
      ? attempts?.find(a => a.id === selection.attemptId)
      : null

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
      />
    )
  }

  if (!selectedAttempt) {
    return (
      <EmptyState
        message="Select an attempt to view details"
        icon={<Server className="h-12 w-12 mb-4 opacity-10" />}
      />
    )
  }

  return (
    <Tabs
      value={activeTab}
      onValueChange={(value) => setActiveTab(value as typeof activeTab)}
      className="flex flex-col h-full overflow-hidden min-w-0"
    >
      {/* Detail Header */}
      <div className="h-16 border-b border-border bg-surface-secondary/20 px-6 flex items-center justify-between shrink-0 backdrop-blur-sm sticky top-0 z-10">
        <div className="flex items-center gap-4">
          <div className="w-10 h-10 rounded-lg bg-surface-primary flex items-center justify-center text-text-primary shadow-sm border border-border">
            <Server size={20} />
          </div>
          <div>
            <h3 className="text-sm font-medium text-text-primary">
              {providerMap.get(selectedAttempt.providerID) ||
                `Provider #${selectedAttempt.providerID}`}
            </h3>
            <div className="flex items-center gap-3 text-xs text-text-secondary mt-0.5">
              <span>Attempt #{selectedAttempt.id}</span>
              {selectedAttempt.cost > 0 && (
                <span className="text-blue-400 font-medium">
                  Cost: {formatCost(selectedAttempt.cost)}
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Detail Tabs */}
        <TabsList>
          <TabsTrigger value="request" className="border-none">
            Request
          </TabsTrigger>
          <TabsTrigger value="response" className="border-none">
            Response
          </TabsTrigger>
          <TabsTrigger value="metadata" className="border-none">
            Metadata
          </TabsTrigger>
        </TabsList>
      </div>

      {/* Detail Content */}
      <TabsContent value="request" className="flex-1 overflow-hidden flex flex-col min-w-0 mt-0">
        {selectedAttempt.requestInfo ? (
            <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in min-w-0">
              <div className="flex items-center gap-3 p-3 bg-surface-secondary/30 rounded-lg border border-border shrink-0">
                <Badge variant="info" className="font-mono text-xs">
                  {selectedAttempt.requestInfo.method}
                </Badge>
                <code className="flex-1 font-mono text-xs text-text-primary break-all">
                  {selectedAttempt.requestInfo.url}
                </code>
              </div>

              <div className="flex flex-col min-h-0 flex-1 gap-6">
                <div className="flex flex-col min-h-0 gap-3 flex-1">
                  <div className="flex items-center justify-between shrink-0">
                    <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                      <Code size={14} /> Headers
                    </h5>
                    <div className="flex items-center gap-2">
                      <DiffButton
                        clientContent={formatJSON(
                          request.requestInfo?.headers || {}
                        )}
                        upstreamContent={formatJSON(
                          selectedAttempt.requestInfo.headers
                        )}
                        title="Compare Headers - Client vs Upstream"
                      />
                      <CopyButton
                        content={formatJSON(
                          selectedAttempt.requestInfo.headers
                        )}
                        label="Copy"
                      />
                    </div>
                  </div>
                  <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                    <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Badge
                        variant="outline"
                        className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                      >
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
                      <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                        <Database size={14} /> Body
                      </h5>
                      <div className="flex items-center gap-2">
                        <DiffButton
                          clientContent={(() => {
                            try {
                              return formatJSON(
                                JSON.parse(request.requestInfo?.body || '{}')
                              )
                            } catch {
                              return request.requestInfo?.body || ''
                            }
                          })()}
                          upstreamContent={(() => {
                            try {
                              return formatJSON(
                                JSON.parse(selectedAttempt.requestInfo.body)
                              )
                            } catch {
                              return selectedAttempt.requestInfo.body
                            }
                          })()}
                          title="Compare Body - Client vs Upstream"
                        />
                        <CopyButton
                          content={(() => {
                            try {
                              return formatJSON(
                                JSON.parse(selectedAttempt.requestInfo.body)
                              )
                            } catch {
                              return selectedAttempt.requestInfo.body
                            }
                          })()}
                          label="Copy"
                        />
                      </div>
                    </div>
                    <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                      <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                        <Badge
                          variant="outline"
                          className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                        >
                          JSON
                        </Badge>
                      </div>
                      <pre className="text-xs font-mono text-foreground/95 whitespace-pre overflow-x-auto leading-relaxed">
                        {(() => {
                          try {
                            return formatJSON(
                              JSON.parse(selectedAttempt.requestInfo.body)
                            )
                          } catch {
                            return selectedAttempt.requestInfo.body
                          }
                        })()}
                      </pre>
                    </div>
                  </div>
                )}
              </div>
            </div>
          ) : (
            <EmptyState message="No request data available" />
          )}
      </TabsContent>

      <TabsContent value="response" className="flex-1 overflow-hidden flex flex-col min-w-0 mt-0">
        {selectedAttempt.responseInfo ? (
            <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in min-w-0">
              <div className="flex items-center gap-3 p-3 bg-surface-secondary/30 rounded-lg border border-border shrink-0">
                <div
                  className={cn(
                    'px-2 py-1 rounded text-xs font-bold font-mono',
                    selectedAttempt.responseInfo.status >= 400
                      ? 'bg-red-400/10 text-red-400'
                      : 'bg-blue-400/10 text-blue-400'
                  )}
                >
                  {selectedAttempt.responseInfo.status}
                </div>
                <span className="text-sm text-text-secondary font-medium">
                  Response Status
                </span>
              </div>

              <div className="flex flex-col min-h-0 flex-1 gap-6">
                <div className="flex flex-col min-h-0 gap-3 flex-1">
                  <div className="flex items-center justify-between shrink-0">
                    <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                      <Code size={14} /> Headers
                    </h5>
                    <CopyButton
                      content={formatJSON(selectedAttempt.responseInfo.headers)}
                      label="Copy"
                    />
                  </div>
                  <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                    <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Badge
                        variant="outline"
                        className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                      >
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
                      <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                        <Database size={14} /> Body
                      </h5>
                      <CopyButton
                        content={(() => {
                          try {
                            return formatJSON(
                              JSON.parse(selectedAttempt.responseInfo.body)
                            )
                          } catch {
                            return selectedAttempt.responseInfo.body
                          }
                        })()}
                        label="Copy"
                      />
                    </div>
                    <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                      <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                        <Badge
                          variant="outline"
                          className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                        >
                          JSON
                        </Badge>
                      </div>
                      <pre className="text-xs font-mono text-foreground/95 whitespace-pre overflow-x-auto leading-relaxed">
                        {(() => {
                          try {
                            return formatJSON(
                              JSON.parse(selectedAttempt.responseInfo.body)
                            )
                          } catch {
                            return selectedAttempt.responseInfo.body
                          }
                        })()}
                      </pre>
                    </div>
                  </div>
                )}
              </div>
            </div>
          ) : (
            <EmptyState message="No response data available" />
          )}
      </TabsContent>

      <TabsContent value="metadata" className="flex-1 overflow-y-auto p-6 mt-0">
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
              <Card className="bg-surface-primary border-border">
                <CardHeader className="pb-2 border-b border-border/50">
                  <CardTitle className="text-sm font-medium flex items-center gap-2">
                    <Info size={16} className="text-info" />
                    Request Info
                  </CardTitle>
                </CardHeader>
                <CardContent className="pt-4">
                  <dl className="space-y-4">
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Request ID
                      </dt>
                      <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded select-all break-all">
                        {request.requestID || '-'}
                      </dd>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Session ID
                      </dt>
                      <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded select-all break-all">
                        {request.sessionID || '-'}
                      </dd>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Instance ID
                      </dt>
                      <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded select-all break-all">
                        {request.instanceID || '-'}
                      </dd>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Request Model
                      </dt>
                      <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded">
                        {request.requestModel || '-'}
                      </dd>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Response Model
                      </dt>
                      <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded">
                        {request.responseModel || '-'}
                      </dd>
                    </div>
                  </dl>
                </CardContent>
              </Card>

              <Card className="bg-surface-primary border-border">
                <CardHeader className="pb-2 border-b border-border/50">
                  <CardTitle className="text-sm font-medium flex items-center gap-2">
                    <Zap size={16} className="text-warning" />
                    Attempt Usage & Cache
                  </CardTitle>
                </CardHeader>
                <CardContent className="pt-4">
                  <dl className="space-y-4">
                    <div className="flex justify-between items-center border-b border-border/30 pb-2">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Input Tokens
                      </dt>
                      <dd className="text-sm text-text-primary font-mono font-medium">
                        {selectedAttempt.inputTokenCount.toLocaleString()}
                      </dd>
                    </div>
                    <div className="flex justify-between items-center border-b border-border/30 pb-2">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Output Tokens
                      </dt>
                      <dd className="text-sm text-text-primary font-mono font-medium">
                        {selectedAttempt.outputTokenCount.toLocaleString()}
                      </dd>
                    </div>
                    <div className="flex justify-between items-center border-b border-border/30 pb-2">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Cache Read
                      </dt>
                      <dd className="text-sm text-violet-400 font-mono font-medium">
                        {selectedAttempt.cacheReadCount.toLocaleString()}
                      </dd>
                    </div>
                    <div className="flex justify-between items-center border-b border-border/30 pb-2">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Cache Write
                      </dt>
                      <dd className="text-sm text-amber-400 font-mono font-medium">
                        {selectedAttempt.cacheWriteCount.toLocaleString()}
                      </dd>
                    </div>
                    {(selectedAttempt.cache5mWriteCount > 0 ||
                      selectedAttempt.cache1hWriteCount > 0) && (
                      <div className="flex justify-between items-center border-b border-border/30 pb-2 pl-4">
                        <dt className="text-xs font-medium text-text-secondary/70 tracking-wider">
                          <span className="text-cyan-400/80">5m:</span>{' '}
                          {selectedAttempt.cache5mWriteCount}
                          <span className="mx-2">|</span>
                          <span className="text-orange-400/80">1h:</span>{' '}
                          {selectedAttempt.cache1hWriteCount}
                        </dt>
                      </div>
                    )}
                    <div className="flex justify-between items-center">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Cost
                      </dt>
                      <dd className="text-sm text-blue-400 font-mono font-medium">
                        {formatCost(selectedAttempt.cost)}
                      </dd>
                    </div>
                  </dl>
                </CardContent>
              </Card>
            </div>
        </TabsContent>
      </Tabs>
  )
}
