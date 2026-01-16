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
import { Code, Database, Info, Zap } from 'lucide-react'
import type { ProxyRequest, ClientType } from '@/lib/transport'
import { cn } from '@/lib/utils'
import {
  ClientIcon,
  getClientName,
  getClientColor,
} from '@/components/icons/client-icons'
import { CopyButton, CopyAsCurlButton, EmptyState } from './components'

interface RequestDetailViewProps {
  request: ProxyRequest
  activeTab: 'request' | 'response' | 'metadata'
  setActiveTab: (tab: 'request' | 'response' | 'metadata') => void
  formatJSON: (obj: unknown) => string
  formatCost: (microUSD: number) => string
  projectName?: string
  sessionInfo?: { clientType: string; projectID: number }
  projectMap: Map<number, string>
  tokenName?: string
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
}: RequestDetailViewProps) {
  return (
    <Tabs
      value={activeTab}
      onValueChange={value => setActiveTab(value as typeof activeTab)}
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
            <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
              <span>Request #{request.id}</span>
              <span className="text-muted-foreground">·</span>
              <span>{request.requestModel}</span>
              {request.cost > 0 && (
                <span className="text-blue-400 font-medium">
                  Cost: {formatCost(request.cost)}
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Detail Tabs */}
        <TabsList>
          <TabsTrigger value="request" className={'border-none'}>
            Request
          </TabsTrigger>
          <TabsTrigger value="response" className={'border-none'}>
            Response
          </TabsTrigger>
          <TabsTrigger value="metadata" className={'border-none'}>
            Metadata
          </TabsTrigger>
        </TabsList>
      </div>

      {/* Detail Content */}
      <TabsContent
        value="request"
        className="flex-1 overflow-hidden flex flex-col min-w-0 mt-0"
      >
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
                    label="Copy"
                  />
                </div>
                <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                  <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Badge
                      variant="outline"
                      className="text-[10px] bg-card/80 backdrop-blur-sm"
                    >
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
                          return formatJSON(
                            JSON.parse(request.requestInfo.body)
                          )
                        } catch {
                          return request.requestInfo.body
                        }
                      })()}
                      label="Copy"
                    />
                  </div>
                  <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                    <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Badge
                        variant="outline"
                        className="text-[10px] bg-card/80 backdrop-blur-sm"
                      >
                        JSON
                      </Badge>
                    </div>
                    <pre className="text-xs font-mono text-foreground/95 whitespace-pre overflow-x-auto leading-relaxed">
                      {(() => {
                        try {
                          return formatJSON(
                            JSON.parse(request.requestInfo.body)
                          )
                        } catch {
                          return request.requestInfo.body
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

      <TabsContent
        value="response"
        className="flex-1 overflow-hidden flex flex-col min-w-0 mt-0"
      >
        {request.responseInfo ? (
          <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in min-w-0">
            <div className="flex items-center gap-3 p-3 bg-muted/30 rounded-lg border border-border shrink-0">
              <div
                className={cn(
                  'px-2 py-1 rounded text-xs font-bold font-mono',
                  request.responseInfo.status >= 400
                    ? 'bg-red-400/10 text-red-400'
                    : 'bg-blue-400/10 text-blue-400'
                )}
              >
                {request.responseInfo.status}
              </div>
              <span className="text-sm text-muted-foreground font-medium">
                Response Status
              </span>
            </div>

            <div className="flex flex-col min-h-0 flex-1 gap-6">
              <div className="flex flex-col min-h-0 gap-3 flex-1">
                <div className="flex items-center justify-between shrink-0">
                  <h5 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                    <Code size={14} /> Headers
                  </h5>
                  <CopyButton
                    content={formatJSON(request.responseInfo.headers)}
                    label="Copy"
                  />
                </div>
                <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                  <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Badge
                      variant="outline"
                      className="text-[10px] bg-card/80 backdrop-blur-sm"
                    >
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
                          return formatJSON(
                            JSON.parse(request.responseInfo.body)
                          )
                        } catch {
                          return request.responseInfo.body
                        }
                      })()}
                      label="Copy"
                    />
                  </div>
                  <div className="flex-1 rounded-lg border border-border bg-muted/50 dark:bg-muted/30 p-4 overflow-auto shadow-inner relative group min-h-0">
                    <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Badge
                        variant="outline"
                        className="text-[10px] bg-card/80 backdrop-blur-sm"
                      >
                        JSON
                      </Badge>
                    </div>
                    <pre className="text-xs font-mono text-foreground/95 whitespace-pre overflow-x-auto leading-relaxed">
                      {(() => {
                        try {
                          return formatJSON(
                            JSON.parse(request.responseInfo.body)
                          )
                        } catch {
                          return request.responseInfo.body
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
                        <span className="capitalize">
                          {sessionInfo.clientType}
                        </span>
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
                    Input Tokens
                  </dt>
                  <dd className="text-sm text-foreground font-mono font-medium">
                    {request.inputTokenCount.toLocaleString()}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Output Tokens
                  </dt>
                  <dd className="text-sm text-foreground font-mono font-medium">
                    {request.outputTokenCount.toLocaleString()}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Cache Read
                  </dt>
                  <dd className="text-sm text-violet-400 font-mono font-medium">
                    {request.cacheReadCount.toLocaleString()}
                  </dd>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Cache Write
                  </dt>
                  <dd className="text-sm text-amber-400 font-mono font-medium">
                    {request.cacheWriteCount.toLocaleString()}
                  </dd>
                </div>
                {(request.cache5mWriteCount > 0 ||
                  request.cache1hWriteCount > 0) && (
                  <div className="flex justify-between items-center border-b border-border/30 pb-2 pl-4">
                    <dt className="text-xs font-medium text-muted-foreground/70 tracking-wider">
                      <span className="text-cyan-400/80">5m:</span>{' '}
                      {request.cache5mWriteCount}
                      <span className="mx-2">|</span>
                      <span className="text-orange-400/80">1h:</span>{' '}
                      {request.cache1hWriteCount}
                    </dt>
                  </div>
                )}
                <div className="flex justify-between items-center">
                  <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Cost
                  </dt>
                  <dd className="text-sm text-blue-400 font-mono font-medium">
                    {formatCost(request.cost)}
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
