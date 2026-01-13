import { useState, useMemo, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import * as Diff from 'diff'
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Badge,
} from '@/components/ui'
import {
  useProxyRequest,
  useProxyUpstreamAttempts,
  useProxyRequestUpdates,
  useProviders,
  useProjects,
  useSessions,
  useRoutes,
} from '@/hooks/queries'
import {
  ArrowLeft,
  Clock,
  Zap,
  AlertCircle,
  Server,
  CheckCircle,
  XCircle,
  Loader2,
  Ban,
  Code,
  Database,
  Info,
  FileInput,
  Copy,
  Check,
  GitCompare,
  X,
} from 'lucide-react'
import { statusVariant } from './index'
import type {
  ProxyUpstreamAttempt,
  ProxyRequest,
  ClientType,
} from '@/lib/transport'
import { cn } from '@/lib/utils'
import {
  ClientIcon,
  getClientName,
  getClientColor,
} from '@/components/icons/client-icons'

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

// Copy Button Component
function CopyButton({ content, label = 'Copy' }: { content: string; label?: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  return (
    <Button
      variant="outline"
      size="sm"
      onClick={handleCopy}
      className="h-6 px-2 text-[10px] gap-1"
    >
      {copied ? (
        <>
          <Check className="h-3 w-3" />
          Copied
        </>
      ) : (
        <>
          <Copy className="h-3 w-3" />
          {label}
        </>
      )}
    </Button>
  )
}

// Diff Modal Component - Shows line-by-line diff
function DiffModal({
  isOpen,
  onClose,
  title,
  leftContent,
  rightContent,
}: {
  isOpen: boolean
  onClose: () => void
  title: string
  leftContent: string
  rightContent: string
}) {
  // Compute diff (must be before any conditional returns)
  const diffResult = useMemo(() => {
    return Diff.diffLines(leftContent, rightContent)
  }, [leftContent, rightContent])

  // Handle ESC key press
  useEffect(() => {
    if (!isOpen) return

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation() // Prevent event from bubbling to parent
        onClose()
      }
    }

    window.addEventListener('keydown', handleKeyDown, { capture: true })
    return () => window.removeEventListener('keydown', handleKeyDown, { capture: true })
  }, [isOpen, onClose])

  // Early return AFTER all hooks
  if (!isOpen) return null

  // Handle backdrop click
  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      e.stopPropagation()
      onClose()
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      onClick={handleBackdropClick}
      onKeyDown={(e) => e.stopPropagation()} // Prevent any key events from bubbling
    >
      <div
        className="bg-surface-primary border border-border rounded-lg shadow-2xl w-[90vw] h-[85vh] flex flex-col"
        onClick={(e) => e.stopPropagation()} // Prevent clicks inside modal from closing it
      >
        {/* Modal Header */}
        <div className="h-14 border-b border-border px-6 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-3">
            <GitCompare className="h-5 w-5 text-accent" />
            <h3 className="text-sm font-semibold text-text-primary">{title}</h3>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            className="h-8 w-8 text-text-secondary hover:text-text-primary"
          >
            <X className="h-5 w-5" />
          </Button>
        </div>

        {/* Legend */}
        <div className="h-10 border-b border-border px-6 flex items-center gap-6 shrink-0 bg-surface-secondary/20">
          <div className="flex items-center gap-2 text-xs">
            <div className="w-3 h-3 bg-green-500/20 border border-green-500/50 rounded"></div>
            <span className="text-text-muted">Added in Upstream</span>
          </div>
          <div className="flex items-center gap-2 text-xs">
            <div className="w-3 h-3 bg-red-500/20 border border-red-500/50 rounded"></div>
            <span className="text-text-muted">Removed from Client</span>
          </div>
          <div className="flex items-center gap-2 text-xs">
            <div className="w-3 h-3 bg-surface-secondary border border-border rounded"></div>
            <span className="text-text-muted">Unchanged</span>
          </div>
        </div>

        {/* Unified Diff View */}
        <div className="flex-1 overflow-auto p-4 bg-[#1a1a1a]">
          <div className="font-mono text-xs">
            {diffResult.map((part, index) => {
              const lines = part.value.split('\n').filter((line, idx, arr) => {
                // Remove the last empty line if it exists
                return idx < arr.length - 1 || line !== ''
              })

              return lines.map((line, lineIndex) => {
                let bgColor = ''
                let textColor = 'text-text-primary'
                let prefix = ' '
                let borderColor = 'border-border/30'

                if (part.added) {
                  bgColor = 'bg-green-500/10'
                  textColor = 'text-green-400'
                  prefix = '+'
                  borderColor = 'border-green-500/30'
                } else if (part.removed) {
                  bgColor = 'bg-red-500/10'
                  textColor = 'text-red-400'
                  prefix = '-'
                  borderColor = 'border-red-500/30'
                } else {
                  bgColor = 'bg-surface-primary/20'
                  textColor = 'text-text-secondary'
                }

                return (
                  <div
                    key={`${index}-${lineIndex}`}
                    className={`${bgColor} ${textColor} px-4 py-1 border-l-2 ${borderColor} whitespace-pre-wrap break-all leading-relaxed hover:bg-opacity-80 transition-colors`}
                  >
                    <span className="inline-block w-4 opacity-60 select-none">
                      {prefix}
                    </span>
                    {line || ' '}
                  </div>
                )
              })
            })}
          </div>
        </div>

        {/* Modal Footer */}
        <div className="h-14 border-t border-border px-6 flex items-center justify-between shrink-0">
          <div className="text-xs text-text-muted">
            {diffResult.filter(p => p.added).length > 0 && (
              <span className="text-green-400 mr-4">
                +{diffResult.filter(p => p.added).reduce((acc, p) => acc + p.value.split('\n').length - 1, 0)} lines
              </span>
            )}
            {diffResult.filter(p => p.removed).length > 0 && (
              <span className="text-red-400">
                -{diffResult.filter(p => p.removed).reduce((acc, p) => acc + p.value.split('\n').length - 1, 0)} lines
              </span>
            )}
          </div>
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </div>
  )
}

// Diff Button Component
function DiffButton({
  clientContent,
  upstreamContent,
  title,
}: {
  clientContent: string
  upstreamContent: string
  title: string
}) {
  const [isOpen, setIsOpen] = useState(false)

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setIsOpen(true)}
        className="h-6 px-2 text-[10px] gap-1"
      >
        <GitCompare className="h-3 w-3" />
        Diff
      </Button>
      <DiffModal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        title={title}
        leftContent={clientContent}
        rightContent={upstreamContent}
      />
    </>
  )
}

export function RequestDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: request, isLoading, error } = useProxyRequest(Number(id))
  const { data: attempts } = useProxyUpstreamAttempts(Number(id))
  const { data: providers } = useProviders()
  const { data: projects } = useProjects()
  const { data: sessions } = useSessions()
  const { data: routes } = useRoutes()
  const [selection, setSelection] = useState<SelectionType>({ type: 'request' })
  const [activeTab, setActiveTab] = useState<
    'request' | 'response' | 'metadata'
  >('request')

  useProxyRequestUpdates()

  // ESC 键返回列表
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        navigate('/requests')
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [navigate])

  const selectedAttempt = useMemo(() => {
    if (selection.type === 'attempt') {
      return attempts?.find(a => a.id === selection.attemptId)
    }
    return null
  }, [attempts, selection])

  // Create lookup map for provider names
  const providerMap = useMemo(() => {
    const map = new Map<number, string>()
    providers?.forEach(p => {
      map.set(p.id, p.name)
    })
    return map
  }, [providers])

  // Create lookup map for project names
  const projectMap = useMemo(() => {
    const map = new Map<number, string>()
    projects?.forEach(p => {
      map.set(p.id, p.name)
    })
    return map
  }, [projects])

  // Create lookup map for sessions by sessionID
  const sessionMap = useMemo(() => {
    const map = new Map<string, { clientType: string; projectID: number }>()
    sessions?.forEach(s => {
      map.set(s.sessionID, { clientType: s.clientType, projectID: s.projectID })
    })
    return map
  }, [sessions])

  // Create lookup map for routes by routeID
  const routeMap = useMemo(() => {
    const map = new Map<number, { projectID: number }>()
    routes?.forEach(r => {
      map.set(r.id, { projectID: r.projectID })
    })
    return map
  }, [routes])

  const formatDuration = (ns: number) => {
    const ms = ns / 1_000_000
    if (ms < 1000) return `${ms.toFixed(0)}ms`
    return `${(ms / 1000).toFixed(2)}s`
  }

  const formatTime = (dateStr: string) => {
    if (!dateStr || dateStr === '0001-01-01T00:00:00Z') return '-'
    return new Date(dateStr).toLocaleString()
  }

  const formatJSON = (obj: unknown): string => {
    if (!obj) return '-'
    try {
      return JSON.stringify(obj, null, 2)
    } catch {
      return String(obj)
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'COMPLETED':
        return <CheckCircle className="h-4 w-4 text-blue-400" />
      case 'FAILED':
        return <XCircle className="h-4 w-4 text-red-400" />
      case 'CANCELLED':
        return <Ban className="h-4 w-4 text-warning" />
      case 'IN_PROGRESS':
        return <Loader2 className="h-4 w-4 text-info animate-spin" />
      default:
        return <Clock className="h-4 w-4 text-text-muted" />
    }
  }

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-accent" />
      </div>
    )
  }

  if (error || !request) {
    return (
      <div className="flex flex-col items-center justify-center h-full space-y-4 bg-background">
        <div className="p-4 bg-red-400/10 rounded-full">
          <AlertCircle className="h-12 w-12 text-red-400" />
        </div>
        <h3 className="text-lg font-semibold text-text-primary">
          Request Not Found
        </h3>
        <p className="text-text-secondary">
          The request doesn't exist or has been deleted.
        </p>
        <Button variant="outline" onClick={() => navigate('/requests')}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Requests
        </Button>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full overflow-hidden bg-background">
      {/* Header */}
      <div className="h-[73px] border-b border-border bg-surface-primary shrink-0 px-6 flex items-center">
        <div className="flex items-center justify-between gap-6 w-full">
          {/* Left: Back + Main Info */}
          <div className="flex items-center gap-3 min-w-0">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => navigate('/requests')}
              className="h-8 w-8 -ml-2 text-text-secondary hover:text-text-primary shrink-0"
            >
              <ArrowLeft className="h-5 w-5" />
            </Button>
            <div
              className="w-10 h-10 rounded-lg flex items-center justify-center shrink-0"
              style={
                {
                  backgroundColor: `${getClientColor(request.clientType as ClientType)}15`,
                } as React.CSSProperties
              }
            >
              <ClientIcon type={request.clientType as ClientType} size={24} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <h2 className="text-lg font-semibold text-text-primary tracking-tight leading-none">
                  {request.requestModel || 'Unknown Model'}
                </h2>
                <Badge
                  variant={statusVariant[request.status]}
                  className="capitalize"
                >
                  {request.status.toLowerCase().replace('_', ' ')}
                </Badge>
              </div>
              <div className="flex items-center gap-3 mt-1.5 text-xs text-text-muted leading-none">
                <span className="font-mono bg-surface-secondary px-1.5 py-0.5 rounded">
                  #{request.id}
                </span>
                <span>{getClientName(request.clientType as ClientType)}</span>
                <span>·</span>
                <span>{formatTime(request.startTime)}</span>
                {request.responseModel &&
                  request.responseModel !== request.requestModel && (
                    <>
                      <span>·</span>
                      <span className="text-text-secondary">
                        Response:{' '}
                        <span className="text-text-primary">
                          {request.responseModel}
                        </span>
                      </span>
                    </>
                  )}
              </div>
            </div>
          </div>

          {/* Right: Stats Grid */}
          <div className="flex items-center gap-4 shrink-0">
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">
                Duration
              </div>
              <div className="text-sm font-mono font-medium text-text-primary">
                {request.duration ? formatDuration(request.duration) : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">
                Input
              </div>
              <div className="text-sm font-mono font-medium text-text-secondary">
                {request.inputTokenCount > 0
                  ? request.inputTokenCount.toLocaleString()
                  : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">
                Output
              </div>
              <div className="text-sm font-mono font-medium text-text-primary">
                {request.outputTokenCount > 0
                  ? request.outputTokenCount.toLocaleString()
                  : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">
                Cache Read
              </div>
              <div className="text-sm font-mono font-medium text-violet-400">
                {request.cacheReadCount > 0
                  ? request.cacheReadCount.toLocaleString()
                  : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">
                Cache Write
              </div>
              <div className="text-sm font-mono font-medium text-amber-400">
                {request.cacheWriteCount > 0
                  ? request.cacheWriteCount.toLocaleString()
                  : '-'}
              </div>
            </div>
            <div className="w-px h-8 bg-border" />
            <div className="text-center px-3">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-0.5">
                Cost
              </div>
              <div className="text-sm font-mono font-medium text-blue-400">
                {formatCost(request.cost)}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Error Banner */}
      {request.error && (
        <div className="shrink-0 bg-red-400/10 border-b border-red-400/20 px-6 py-3 flex items-start gap-3">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-400" />
          <div className="flex-1">
            <h4 className="text-sm font-medium text-red-400 mb-1">
              Request Failed
            </h4>
            <pre className="whitespace-pre-wrap break-words font-mono text-xs text-red-400/90 max-h-24 overflow-auto">
              {request.error}
            </pre>
          </div>
        </div>
      )}

      {/* Main Content - Split View */}
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar: Request & Attempts List */}
        <div className="w-80 flex flex-col border-r border-border bg-surface-primary shrink-0">
          {/* Request Section */}
          <div className="shrink-0">
            <div className="h-10 px-4 border-b border-border bg-surface-secondary/50 flex items-center">
              <span className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                <FileInput size={12} /> Client Request
              </span>
            </div>
            <button
              type="button"
              onClick={() => setSelection({ type: 'request' })}
              className={cn(
                'w-full text-left p-3.5 transition-all outline-none border-l-[3px] border-b border-border',
                selection.type === 'request'
                  ? 'bg-accent/5 border-l-accent'
                  : 'border-l-transparent hover:bg-surface-secondary/50'
              )}
            >
              <div className="flex items-center justify-between mb-1.5">
                <div className="flex items-center gap-2">
                  {getStatusIcon(request.status)}
                  <span
                    className={cn(
                      'text-sm font-medium transition-colors',
                      selection.type === 'request'
                        ? 'text-text-primary'
                        : 'text-text-secondary'
                    )}
                  >
                    {getClientName(request.clientType as ClientType)} Request
                  </span>
                </div>
                {request.responseInfo && (
                  <span
                    className={cn(
                      'text-[10px] font-mono px-1.5 py-0.5 rounded font-medium',
                      request.responseInfo.status >= 400
                        ? 'text-red-400 bg-red-400/10'
                        : 'text-blue-400 bg-blue-400/10'
                    )}
                  >
                    {request.responseInfo.status}
                  </span>
                )}
              </div>
              <div className="flex items-center justify-between text-xs text-text-muted">
                <span className="truncate max-w-[180px]">
                  {request.requestModel}
                </span>
                <span className="font-mono opacity-70">
                  {request.duration ? formatDuration(request.duration) : '-'}
                </span>
              </div>
            </button>
          </div>

          {/* Attempts Section */}
          <div className="h-10 px-4 border-b border-border bg-surface-secondary/50 flex items-center justify-between shrink-0">
            <span className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
              <Server size={12} /> Upstream Attempts
            </span>
            <Badge variant="outline" className="text-[10px] h-5 px-1.5">
              {attempts?.length || 0}
            </Badge>
          </div>

          <div className="flex-1 overflow-y-auto">
            {attempts && attempts.length > 0 ? (
              <div className="divide-y divide-border">
                {attempts.map(
                  (attempt: ProxyUpstreamAttempt, index: number) => (
                    <button
                      type="button"
                      key={attempt.id}
                      onClick={() =>
                        setSelection({ type: 'attempt', attemptId: attempt.id })
                      }
                      className={cn(
                        'w-full text-left p-3.5 transition-all outline-none border-l-[3px]',
                        selection.type === 'attempt' &&
                          selection.attemptId === attempt.id
                          ? 'bg-accent/5 border-l-accent'
                          : 'border-l-transparent hover:bg-surface-secondary/50'
                      )}
                    >
                      <div className="flex items-center justify-between mb-1.5">
                        <div className="flex items-center gap-2">
                          {getStatusIcon(attempt.status)}
                          <span
                            className={cn(
                              'text-sm font-medium transition-colors',
                              selection.type === 'attempt' &&
                                selection.attemptId === attempt.id
                                ? 'text-text-primary'
                                : 'text-text-secondary group-hover:text-text-primary'
                            )}
                          >
                            Attempt {index + 1}
                          </span>
                        </div>
                        {attempt.responseInfo && (
                          <span
                            className={cn(
                              'text-[10px] font-mono px-1.5 py-0.5 rounded font-medium',
                              attempt.responseInfo.status >= 400
                                ? 'text-red-400 bg-red-400/10'
                                : 'text-blue-400 bg-blue-400/10'
                            )}
                          >
                            {attempt.responseInfo.status}
                          </span>
                        )}
                      </div>
                      <div className="flex items-center justify-between text-xs text-text-muted">
                        <span
                          className="flex items-center gap-1.5 truncate max-w-[140px]"
                          title={
                            providerMap.get(attempt.providerID) ||
                            `Provider #${attempt.providerID}`
                          }
                        >
                          {providerMap.get(attempt.providerID) ||
                            `Provider #${attempt.providerID}`}
                          {(() => {
                            const route = routeMap.get(attempt.routeID)
                            if (route?.projectID === 0) {
                              return (
                                <Badge
                                  variant="outline"
                                  className="text-[9px] h-4 px-1 ml-1"
                                >
                                  Global
                                </Badge>
                              )
                            } else if (route?.projectID) {
                              return (
                                <Badge
                                  variant="info"
                                  className="text-[9px] h-4 px-1 ml-1"
                                >
                                  {projectMap.get(route.projectID) ||
                                    `#${route.projectID}`}
                                </Badge>
                              )
                            }
                            return null
                          })()}
                        </span>
                        <span className="font-mono opacity-70">
                          {attempt.duration > 0
                            ? formatDuration(attempt.duration)
                            : '-'}
                        </span>
                      </div>
                    </button>
                  )
                )}
              </div>
            ) : (
              <EmptyState message="No attempts available" />
            )}
          </div>
        </div>

        {/* Main Panel: Selected Detail */}
        <div className="flex-1 flex flex-col bg-background min-w-0">
          {selection.type === 'request' ? (
            /* Request Detail View */
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
          ) : selectedAttempt ? (
            <>
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
                <div className="flex bg-surface-secondary/50 p-1 rounded-lg border border-border">
                  <button
                    type="button"
                    onClick={() => setActiveTab('request')}
                    className={cn(
                      'px-4 py-1.5 text-xs font-medium rounded-md transition-all',
                      activeTab === 'request'
                        ? 'bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50'
                        : 'text-text-secondary hover:text-text-primary hover:bg-surface-hover/50'
                    )}
                  >
                    Request
                  </button>
                  <button
                    type="button"
                    onClick={() => setActiveTab('response')}
                    className={cn(
                      'px-4 py-1.5 text-xs font-medium rounded-md transition-all',
                      activeTab === 'response'
                        ? 'bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50'
                        : 'text-text-secondary hover:text-text-primary hover:bg-surface-hover/50'
                    )}
                  >
                    Response
                  </button>
                  <button
                    type="button"
                    onClick={() => setActiveTab('metadata')}
                    className={cn(
                      'px-4 py-1.5 text-xs font-medium rounded-md transition-all',
                      activeTab === 'metadata'
                        ? 'bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50'
                        : 'text-text-secondary hover:text-text-primary hover:bg-surface-hover/50'
                    )}
                  >
                    Metadata
                  </button>
                </div>
              </div>

              {/* Detail Content */}
              <div className="flex-1 overflow-hidden flex flex-col bg-background relative">
                {activeTab === 'request' &&
                  (selectedAttempt.requestInfo ? (
                    <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in">
                      <div className="flex items-center gap-3 p-3 bg-surface-secondary/30 rounded-lg border border-border shrink-0">
                        <Badge variant="info" className="font-mono text-xs">
                          {selectedAttempt.requestInfo.method}
                        </Badge>
                        <code className="flex-1 font-mono text-xs text-text-primary break-all">
                          {selectedAttempt.requestInfo.url}
                        </code>
                      </div>

                      <div className="flex flex-col min-h-0 flex-1 gap-6">
                        <div className="flex flex-col min-h-0 gap-3" style={{ maxHeight: '300px' }}>
                          <div className="flex items-center justify-between shrink-0">
                            <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                              <Code size={14} /> Headers
                            </h5>
                            <div className="flex items-center gap-2">
                              <DiffButton
                                clientContent={formatJSON(request.requestInfo?.headers || {})}
                                upstreamContent={formatJSON(selectedAttempt.requestInfo.headers)}
                                title="Compare Headers - Client vs Upstream"
                              />
                              <CopyButton
                                content={formatJSON(selectedAttempt.requestInfo.headers)}
                                label="Copy"
                              />
                            </div>
                          </div>
                          <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group min-h-0">
                            <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                              <Badge
                                variant="outline"
                                className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                              >
                                JSON
                              </Badge>
                            </div>
                            <pre className="text-xs font-mono text-text-secondary leading-relaxed whitespace-pre-wrap break-words">
                              {formatJSON(selectedAttempt.requestInfo.headers)}
                            </pre>
                          </div>
                        </div>

                        {selectedAttempt.requestInfo.body && (
                          <div className="flex flex-col min-h-0 gap-3" style={{ maxHeight: '400px' }}>
                            <div className="flex items-center justify-between shrink-0">
                              <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                                <Database size={14} /> Body
                              </h5>
                              <div className="flex items-center gap-2">
                                <DiffButton
                                  clientContent={(() => {
                                    try {
                                      return formatJSON(JSON.parse(request.requestInfo?.body || '{}'))
                                    } catch {
                                      return request.requestInfo?.body || ''
                                    }
                                  })()}
                                  upstreamContent={(() => {
                                    try {
                                      return formatJSON(JSON.parse(selectedAttempt.requestInfo.body))
                                    } catch {
                                      return selectedAttempt.requestInfo.body
                                    }
                                  })()}
                                  title="Compare Body - Client vs Upstream"
                                />
                                <CopyButton
                                  content={(() => {
                                    try {
                                      return formatJSON(JSON.parse(selectedAttempt.requestInfo.body))
                                    } catch {
                                      return selectedAttempt.requestInfo.body
                                    }
                                  })()}
                                  label="Copy"
                                />
                              </div>
                            </div>
                            <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group min-h-0">
                              <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                <Badge
                                  variant="outline"
                                  className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                                >
                                  JSON
                                </Badge>
                              </div>
                              <pre className="text-xs font-mono text-text-primary whitespace-pre-wrap break-words leading-relaxed">
                                {(() => {
                                  try {
                                    return formatJSON(
                                      JSON.parse(
                                        selectedAttempt.requestInfo.body
                                      )
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
                  ))}

                {activeTab === 'response' &&
                  (selectedAttempt.responseInfo ? (
                    <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in">
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
                        <div className="flex flex-col min-h-0 gap-3" style={{ maxHeight: '300px' }}>
                          <div className="flex items-center justify-between shrink-0">
                            <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                              <Code size={14} /> Headers
                            </h5>
                            <CopyButton
                              content={formatJSON(selectedAttempt.responseInfo.headers)}
                              label="Copy"
                            />
                          </div>
                          <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group min-h-0">
                            <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                              <Badge
                                variant="outline"
                                className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                              >
                                JSON
                              </Badge>
                            </div>
                            <pre className="text-xs font-mono text-text-secondary leading-relaxed whitespace-pre-wrap break-words">
                              {formatJSON(selectedAttempt.responseInfo.headers)}
                            </pre>
                          </div>
                        </div>

                        {selectedAttempt.responseInfo.body && (
                          <div className="flex flex-col min-h-0 gap-3" style={{ maxHeight: '400px' }}>
                            <div className="flex items-center justify-between shrink-0">
                              <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                                <Database size={14} /> Body
                              </h5>
                              <CopyButton
                                content={(() => {
                                  try {
                                    return formatJSON(JSON.parse(selectedAttempt.responseInfo.body))
                                  } catch {
                                    return selectedAttempt.responseInfo.body
                                  }
                                })()}
                                label="Copy"
                              />
                            </div>
                            <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group min-h-0">
                              <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                <Badge
                                  variant="outline"
                                  className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                                >
                                  JSON
                                </Badge>
                              </div>
                              <pre className="text-xs font-mono text-text-primary whitespace-pre-wrap break-words leading-relaxed">
                                {(() => {
                                  try {
                                    return formatJSON(
                                      JSON.parse(
                                        selectedAttempt.responseInfo.body
                                      )
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
                  ))}

                {activeTab === 'metadata' && (
                  <div className="flex-1 overflow-y-auto p-6 animate-fade-in">
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
                                  <span className="text-orange-400/80">
                                    1h:
                                  </span>{' '}
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
                  </div>
                )}
              </div>
            </>
          ) : (
            <EmptyState
              message="Select an attempt to view details"
              icon={<Server className="h-12 w-12 mb-4 opacity-10" />}
            />
          )}
        </div>
      </div>
    </div>
  )
}

function EmptyState({
  message,
  icon,
}: {
  message: string
  icon?: React.ReactNode
}) {
  return (
    <div className="flex flex-col items-center justify-center h-full text-text-muted p-12 text-center select-none">
      {icon || <Server className="h-12 w-12 mb-3 opacity-10" />}
      <p className="text-sm font-medium">{message}</p>
    </div>
  )
}

// Request Detail View Component - shows client request/response (Claude format)
interface RequestDetailViewProps {
  request: ProxyRequest
  activeTab: 'request' | 'response' | 'metadata'
  setActiveTab: (tab: 'request' | 'response' | 'metadata') => void
  formatJSON: (obj: unknown) => string
  formatCost: (microUSD: number) => string
  projectName?: string
  sessionInfo?: { clientType: string; projectID: number }
  projectMap: Map<number, string>
}

function RequestDetailView({
  request,
  activeTab,
  setActiveTab,
  formatJSON,
  formatCost,
  projectName,
  sessionInfo,
  projectMap,
}: RequestDetailViewProps) {
  return (
    <>
      {/* Detail Header */}
      <div className="h-16 border-b border-border bg-surface-secondary/20 px-6 flex items-center justify-between shrink-0 backdrop-blur-sm sticky top-0 z-10">
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
            <h3 className="text-sm font-medium text-text-primary">
              {getClientName(request.clientType as ClientType)} Request
            </h3>
            <div className="flex items-center gap-3 text-xs text-text-secondary mt-0.5">
              <span>Request #{request.id}</span>
              <span className="text-text-muted">·</span>
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
        <div className="flex bg-surface-secondary/50 p-1 rounded-lg border border-border">
          <button
            type="button"
            onClick={() => setActiveTab('request')}
            className={cn(
              'px-4 py-1.5 text-xs font-medium rounded-md transition-all',
              activeTab === 'request'
                ? 'bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50'
                : 'text-text-secondary hover:text-text-primary hover:bg-surface-hover/50'
            )}
          >
            Request
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('response')}
            className={cn(
              'px-4 py-1.5 text-xs font-medium rounded-md transition-all',
              activeTab === 'response'
                ? 'bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50'
                : 'text-text-secondary hover:text-text-primary hover:bg-surface-hover/50'
            )}
          >
            Response
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('metadata')}
            className={cn(
              'px-4 py-1.5 text-xs font-medium rounded-md transition-all',
              activeTab === 'metadata'
                ? 'bg-surface-primary text-text-primary shadow-sm ring-1 ring-border/50'
                : 'text-text-secondary hover:text-text-primary hover:bg-surface-hover/50'
            )}
          >
            Metadata
          </button>
        </div>
      </div>

      {/* Detail Content */}
      <div className="flex-1 overflow-hidden flex flex-col bg-background relative">
        {activeTab === 'request' &&
          (request.requestInfo ? (
            <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in">
              <div className="flex items-center gap-3 p-3 bg-surface-secondary/30 rounded-lg border border-border shrink-0">
                <Badge variant="info" className="font-mono text-xs">
                  {request.requestInfo.method}
                </Badge>
                <code className="flex-1 font-mono text-xs text-text-primary break-all">
                  {request.requestInfo.url}
                </code>
              </div>

              <div className="flex flex-col min-h-0 flex-1 gap-6">
                <div className="flex flex-col min-h-0 gap-3" style={{ maxHeight: '300px' }}>
                  <div className="flex items-center justify-between shrink-0">
                    <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                      <Code size={14} /> Headers
                    </h5>
                    <CopyButton
                      content={formatJSON(request.requestInfo.headers)}
                      label="Copy"
                    />
                  </div>
                  <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group min-h-0">
                    <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Badge
                        variant="outline"
                        className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                      >
                        JSON
                      </Badge>
                    </div>
                    <pre className="text-xs font-mono text-text-secondary leading-relaxed whitespace-pre-wrap break-words">
                      {formatJSON(request.requestInfo.headers)}
                    </pre>
                  </div>
                </div>

                {request.requestInfo.body && (
                  <div className="flex flex-col min-h-0 gap-3" style={{ maxHeight: '400px' }}>
                    <div className="flex items-center justify-between shrink-0">
                      <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                        <Database size={14} /> Body
                      </h5>
                      <CopyButton
                        content={(() => {
                          try {
                            return formatJSON(JSON.parse(request.requestInfo.body))
                          } catch {
                            return request.requestInfo.body
                          }
                        })()}
                        label="Copy"
                      />
                    </div>
                    <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group min-h-0">
                      <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                        <Badge
                          variant="outline"
                          className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                        >
                          JSON
                        </Badge>
                      </div>
                      <pre className="text-xs font-mono text-text-primary whitespace-pre-wrap break-words leading-relaxed">
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
          ))}

        {activeTab === 'response' &&
          (request.responseInfo ? (
            <div className="flex-1 flex flex-col overflow-hidden p-6 gap-6 animate-fade-in">
              <div className="flex items-center gap-3 p-3 bg-surface-secondary/30 rounded-lg border border-border shrink-0">
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
                <span className="text-sm text-text-secondary font-medium">
                  Response Status
                </span>
              </div>

              <div className="flex flex-col min-h-0 flex-1 gap-6">
                <div className="flex flex-col min-h-0 gap-3" style={{ maxHeight: '300px' }}>
                  <div className="flex items-center justify-between shrink-0">
                    <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                      <Code size={14} /> Headers
                    </h5>
                    <CopyButton
                      content={formatJSON(request.responseInfo.headers)}
                      label="Copy"
                    />
                  </div>
                  <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group min-h-0">
                    <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Badge
                        variant="outline"
                        className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                      >
                        JSON
                      </Badge>
                    </div>
                    <pre className="text-xs font-mono text-text-secondary leading-relaxed whitespace-pre-wrap break-words">
                      {formatJSON(request.responseInfo.headers)}
                    </pre>
                  </div>
                </div>

                {request.responseInfo.body && (
                  <div className="flex flex-col min-h-0 gap-3" style={{ maxHeight: '400px' }}>
                    <div className="flex items-center justify-between shrink-0">
                      <h5 className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
                        <Database size={14} /> Body
                      </h5>
                      <CopyButton
                        content={(() => {
                          try {
                            return formatJSON(JSON.parse(request.responseInfo.body))
                          } catch {
                            return request.responseInfo.body
                          }
                        })()}
                        label="Copy"
                      />
                    </div>
                    <div className="flex-1 rounded-lg border border-border bg-[#1a1a1a] p-4 overflow-auto shadow-inner relative group min-h-0">
                      <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                        <Badge
                          variant="outline"
                          className="text-[10px] bg-surface-primary/80 backdrop-blur-sm"
                        >
                          JSON
                        </Badge>
                      </div>
                      <pre className="text-xs font-mono text-text-primary whitespace-pre-wrap break-words leading-relaxed">
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
          ))}

        {activeTab === 'metadata' && (
          <div className="flex-1 overflow-y-auto p-6 animate-fade-in">
            <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
              <Card className="bg-surface-primary border-border">
                <CardHeader className="pb-2 border-b border-border/50">
                  <CardTitle className="text-sm font-medium flex items-center gap-2">
                    <Info size={16} className="text-info" /> Request Info
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
                      <dd className="sm:col-span-2">
                        <div className="font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded select-all break-all">
                          {request.sessionID || '-'}
                        </div>
                        {sessionInfo && (
                          <div className="flex items-center gap-2 mt-1 text-[10px] text-text-muted">
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
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Project
                      </dt>
                      <dd className="sm:col-span-2 font-mono text-xs text-text-primary bg-surface-secondary px-2 py-1 rounded">
                        {projectName || '-'}
                      </dd>
                    </div>
                  </dl>
                </CardContent>
              </Card>

              <Card className="bg-surface-primary border-border">
                <CardHeader className="pb-2 border-b border-border/50">
                  <CardTitle className="text-sm font-medium flex items-center gap-2">
                    <Zap size={16} className="text-warning" /> Usage & Cache
                  </CardTitle>
                </CardHeader>
                <CardContent className="pt-4">
                  <dl className="space-y-4">
                    <div className="flex justify-between items-center border-b border-border/30 pb-2">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Input Tokens
                      </dt>
                      <dd className="text-sm text-text-primary font-mono font-medium">
                        {request.inputTokenCount.toLocaleString()}
                      </dd>
                    </div>
                    <div className="flex justify-between items-center border-b border-border/30 pb-2">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Output Tokens
                      </dt>
                      <dd className="text-sm text-text-primary font-mono font-medium">
                        {request.outputTokenCount.toLocaleString()}
                      </dd>
                    </div>
                    <div className="flex justify-between items-center border-b border-border/30 pb-2">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Cache Read
                      </dt>
                      <dd className="text-sm text-violet-400 font-mono font-medium">
                        {request.cacheReadCount.toLocaleString()}
                      </dd>
                    </div>
                    <div className="flex justify-between items-center border-b border-border/30 pb-2">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                        Cache Write
                      </dt>
                      <dd className="text-sm text-amber-400 font-mono font-medium">
                        {request.cacheWriteCount.toLocaleString()}
                      </dd>
                    </div>
                    {(request.cache5mWriteCount > 0 ||
                      request.cache1hWriteCount > 0) && (
                      <div className="flex justify-between items-center border-b border-border/30 pb-2 pl-4">
                        <dt className="text-xs font-medium text-text-secondary/70 tracking-wider">
                          <span className="text-cyan-400/80">5m:</span>{' '}
                          {request.cache5mWriteCount}
                          <span className="mx-2">|</span>
                          <span className="text-orange-400/80">1h:</span>{' '}
                          {request.cache1hWriteCount}
                        </dt>
                      </div>
                    )}
                    <div className="flex justify-between items-center">
                      <dt className="text-xs font-medium text-text-secondary uppercase tracking-wider">
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
          </div>
        )}
      </div>
    </>
  )
}
