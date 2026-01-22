import { Badge, Button } from '@/components/ui';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { ArrowLeft, RefreshCw } from 'lucide-react';
import { statusVariant } from '../index';
import type { ProxyRequest, ClientType } from '@/lib/transport';
import { ClientIcon, getClientName, getClientColor } from '@/components/icons/client-icons';
import { formatDuration } from '@/lib/utils';

function formatCost(nanoUSD: number): string {
  if (nanoUSD === 0) return '-';
  // 向下取整到 6 位小数 (microUSD 精度)
  const usd = Math.floor(nanoUSD / 1000) / 1_000_000;
  return `$${usd.toFixed(6)}`;
}

function formatTime(timestamp: string): string {
  const date = new Date(timestamp);
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

interface RequestHeaderProps {
  request: ProxyRequest;
  onBack: () => void;
  onRecalculateCost?: () => void;
  isRecalculating?: boolean;
}

export function RequestHeader({
  request,
  onBack,
  onRecalculateCost,
  isRecalculating,
}: RequestHeaderProps) {
  return (
    <div className="h-[73px] border-b border-border bg-card px-6 flex items-center">
      <div className="flex items-center justify-between gap-6 w-full">
        {/* Left: Back + Main Info */}
        <div className="flex items-center gap-3 min-w-0">
          <Button
            variant="ghost"
            size="icon"
            onClick={onBack}
            className="h-8 w-8 -ml-2 text-muted-foreground hover:text-foreground shrink-0"
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
              <h2 className="text-lg font-semibold text-foreground tracking-tight leading-none">
                {request.requestModel || 'Unknown Model'}
              </h2>
              <Badge variant={statusVariant[request.status]} className="capitalize">
                {request.status.toLowerCase().replace('_', ' ')}
              </Badge>
            </div>
            <div className="flex items-center gap-3 mt-1.5 text-xs text-muted-foreground leading-none">
              <span className="font-mono bg-muted px-1.5 py-0.5 rounded">#{request.id}</span>
              <span>{getClientName(request.clientType as ClientType)}</span>
              <span>·</span>
              <span>{formatTime(request.startTime)}</span>
              {request.responseModel && request.responseModel !== request.requestModel && (
                <>
                  <span>·</span>
                  <span className="text-muted-foreground">
                    Response: <span className="text-foreground">{request.responseModel}</span>
                  </span>
                </>
              )}
            </div>
          </div>
        </div>

        {/* Right: Stats Grid */}
        <div className="flex items-center gap-4 shrink-0">
          <div className="text-center px-3">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-0.5">
              TTFT
            </div>
            <div className="text-sm font-mono font-medium text-muted-foreground">
              {request.ttft && request.ttft > 0 ? formatDuration(request.ttft) : '-'}
            </div>
          </div>
          <div className="w-px h-8 bg-border" />
          <div className="text-center px-3">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-0.5">
              Duration
            </div>
            <div className="text-sm font-mono font-medium text-foreground">
              {request.duration ? formatDuration(request.duration) : '-'}
            </div>
          </div>
          <div className="w-px h-8 bg-border" />
          <div className="text-center px-3">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-0.5">
              Input
            </div>
            <div className="text-sm font-mono font-medium text-muted-foreground">
              {request.inputTokenCount > 0 ? request.inputTokenCount.toLocaleString() : '-'}
            </div>
          </div>
          <div className="w-px h-8 bg-border" />
          <div className="text-center px-3">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-0.5">
              Output
            </div>
            <div className="text-sm font-mono font-medium text-foreground">
              {request.outputTokenCount > 0 ? request.outputTokenCount.toLocaleString() : '-'}
            </div>
          </div>
          <div className="w-px h-8 bg-border" />
          <div className="text-center px-3">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-0.5">
              Cache Read
            </div>
            <div className="text-sm font-mono font-medium text-violet-400">
              {request.cacheReadCount > 0 ? request.cacheReadCount.toLocaleString() : '-'}
            </div>
          </div>
          <div className="w-px h-8 bg-border" />
          <div className="text-center px-3">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-0.5">
              Cache Write
            </div>
            <div className="text-sm font-mono font-medium text-amber-400">
              {request.cacheWriteCount > 0 ? request.cacheWriteCount.toLocaleString() : '-'}
            </div>
          </div>
          <div className="w-px h-8 bg-border" />
          <div className="text-center px-3">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-0.5">
              Cost
            </div>
            <div className="text-sm font-mono font-medium text-blue-400 flex items-center gap-1">
              {formatCost(request.cost)}
              {onRecalculateCost && (
                <Tooltip>
                  <TooltipTrigger
                    className="inline-flex items-center justify-center h-5 w-5 rounded-md text-muted-foreground hover:text-foreground hover:bg-accent disabled:opacity-50"
                    onClick={onRecalculateCost}
                    disabled={isRecalculating}
                  >
                    <RefreshCw className={`h-3 w-3 ${isRecalculating ? 'animate-spin' : ''}`} />
                  </TooltipTrigger>
                  <TooltipContent>Recalculate Cost</TooltipContent>
                </Tooltip>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
