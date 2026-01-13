import { Button, Badge } from '@/components/ui'
import { ArrowLeft } from 'lucide-react'
import { statusVariant } from '../index'
import type { ProxyRequest, ClientType } from '@/lib/transport'
import {
  ClientIcon,
  getClientName,
  getClientColor,
} from '@/components/icons/client-icons'

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

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(2)}s`
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = Math.floor(seconds % 60)
  return `${minutes}m ${remainingSeconds}s`
}

function formatTime(timestamp: string): string {
  const date = new Date(timestamp)
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

interface RequestHeaderProps {
  request: ProxyRequest
  onBack: () => void
}

export function RequestHeader({ request, onBack }: RequestHeaderProps) {
  return (
    <div className="h-[73px] border-b border-border bg-surface-primary px-6 flex items-center">
      <div className="flex items-center justify-between gap-6 w-full">
        {/* Left: Back + Main Info */}
        <div className="flex items-center gap-3 min-w-0">
          <Button
            variant="ghost"
            size="icon"
            onClick={onBack}
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
  )
}
