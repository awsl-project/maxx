import { Badge } from '@/components/ui'
import {
  CheckCircle,
  XCircle,
  Loader2,
  Ban,
  Clock,
  Server,
  FileInput,
} from 'lucide-react'
import type {
  ProxyUpstreamAttempt,
  ProxyRequest,
  ClientType,
} from '@/lib/transport'
import { cn } from '@/lib/utils'
import { getClientName } from '@/components/icons/client-icons'

// Selection type: either the main request or an attempt
type SelectionType =
  | { type: 'request' }
  | { type: 'attempt'; attemptId: number }

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(2)}s`
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = Math.floor(seconds % 60)
  return `${minutes}m ${remainingSeconds}s`
}

function getStatusIcon(status: string) {
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

interface RequestSidebarProps {
  request: ProxyRequest
  attempts: ProxyUpstreamAttempt[] | undefined
  selection: SelectionType
  onSelectionChange: (selection: SelectionType) => void
  providerMap: Map<number, string>
  projectMap: Map<number, string>
  routeMap: Map<number, { projectID: number }>
}

export function RequestSidebar({
  request,
  attempts,
  selection,
  onSelectionChange,
  providerMap,
  projectMap,
  routeMap,
}: RequestSidebarProps) {
  return (
    <div className="flex flex-col h-full bg-surface-primary min-w-0">
      {/* Request Section */}
      <div className="shrink-0">
        <div className="h-10 px-4 border-b border-border bg-surface-secondary/50 flex items-center">
          <span className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-2">
            <FileInput size={12} /> Client Request
          </span>
        </div>
        <button
          type="button"
          onClick={() => onSelectionChange({ type: 'request' })}
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
            {attempts.map((attempt: ProxyUpstreamAttempt, index: number) => (
              <button
                type="button"
                key={attempt.id}
                onClick={() =>
                  onSelectionChange({ type: 'attempt', attemptId: attempt.id })
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
            ))}
          </div>
        ) : (
          <EmptyState message="No attempts available" />
        )}
      </div>
    </div>
  )
}
