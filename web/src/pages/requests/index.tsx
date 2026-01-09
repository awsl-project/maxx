import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useProxyRequests, useProxyRequestUpdates } from '@/hooks/queries';
import {
  Activity,
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  Loader2,
  CheckCircle,
  AlertTriangle,
  Ban
} from 'lucide-react';
import type { ProxyRequest, ProxyRequestStatus } from '@/lib/transport';
import { ClientIcon } from '@/components/icons/client-icons';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Badge,
} from '@/components/ui';
import { cn } from '@/lib/utils';

const PAGE_SIZE = 50;

export const statusVariant: Record<ProxyRequestStatus, 'default' | 'success' | 'warning' | 'danger' | 'info'> = {
  PENDING: 'default',
  IN_PROGRESS: 'info',
  COMPLETED: 'success',
  FAILED: 'danger',
  CANCELLED: 'warning',
};

export function RequestsPage() {
  const navigate = useNavigate();
  const [page, setPage] = useState(0);
  const { data: requests = [], isLoading, refetch } = useProxyRequests({ limit: PAGE_SIZE, offset: page * PAGE_SIZE });

  // Subscribe to real-time updates
  useProxyRequestUpdates();

  const total = requests.length;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary flex-shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent/10 rounded-lg">
            <Activity size={20} className="text-accent" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-text-primary leading-tight">Requests</h2>
            <p className="text-xs text-text-secondary">
              {total} total requests
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => refetch()}
            disabled={isLoading}
            className="btn btn-secondary flex items-center gap-2 h-9 px-3"
          >
            <RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-hidden flex flex-col">
        {isLoading && requests.length === 0 ? (
          <div className="flex-1 flex items-center justify-center">
            <Loader2 className="w-8 h-8 animate-spin text-accent" />
          </div>
        ) : requests.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center text-text-muted">
            <div className="p-4 bg-surface-secondary rounded-full mb-4">
              <Activity size={32} className="opacity-50" />
            </div>
            <p className="text-body font-medium">No requests recorded</p>
            <p className="text-caption mt-1">Requests will appear here automatically</p>
          </div>
        ) : (
          <div className="flex-1 overflow-auto">
            <Table>
              <TableHeader className="bg-surface-primary/80 backdrop-blur-md sticky top-0 z-10 shadow-sm border-b border-border">
                <TableRow className="hover:bg-transparent border-none">
                  <TableHead className="w-[90px] font-medium">Time</TableHead>
                  <TableHead className="w-[100px] font-medium">Status</TableHead>
                  <TableHead className="w-[50px] font-medium">Code</TableHead>
                  <TableHead className="w-[100px] font-medium">Client</TableHead>
                  <TableHead className="w-[160px] font-medium">Model</TableHead>
                  <TableHead className="w-[70px] text-right font-medium">Duration</TableHead>
                  <TableHead className="w-[70px] text-right font-medium">Cost</TableHead>
                  <TableHead className="w-[40px] text-center font-medium" title="Attempts">Att.</TableHead>
                  <TableHead className="w-[55px] text-right font-medium" title="Input Tokens">In</TableHead>
                  <TableHead className="w-[55px] text-right font-medium" title="Output Tokens">Out</TableHead>
                  <TableHead className="w-[55px] text-right font-medium" title="Cache Read">CacheR</TableHead>
                  <TableHead className="w-[55px] text-right font-medium" title="Cache Write">CacheW</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {requests.map((req) => (
                  <LogRow
                    key={req.id}
                    request={req}
                    onClick={() => navigate(`/requests/${req.id}`)}
                  />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between px-6 py-3 border-t border-border bg-surface-primary flex-shrink-0">
        <span className="text-xs text-text-secondary">
          {total > 0 ? (
            <>
              Showing <span className="font-medium text-text-primary">{page * PAGE_SIZE + 1}</span> to{' '}
              <span className="font-medium text-text-primary">{Math.min((page + 1) * PAGE_SIZE, total)}</span> of{' '}
              <span className="font-medium text-text-primary">{total}</span>
            </>
          ) : (
            'No items'
          )}
        </span>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={page === 0}
            className="p-1.5 rounded-md hover:bg-surface-hover text-text-secondary disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronLeft size={16} />
          </button>
          <span className="text-xs text-text-secondary min-w-[60px] text-center font-medium">
            Page {page + 1} of {totalPages}
          </span>
          <button
            onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
            disabled={page >= totalPages - 1}
            className="p-1.5 rounded-md hover:bg-surface-hover text-text-secondary disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronRight size={16} />
          </button>
        </div>
      </div>
    </div>
  );
}

// Request Status Badge Component
function RequestStatusBadge({ status }: { status: ProxyRequestStatus }) {
  const getStatusConfig = () => {
    switch (status) {
      case 'PENDING':
      case 'IN_PROGRESS':
        return {
          variant: 'info' as const,
          label: status === 'IN_PROGRESS' ? 'Streaming' : 'Pending',
          icon: (
            <span className="relative flex h-2 w-2 mr-1.5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-info opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2 w-2 bg-info"></span>
            </span>
          ),
        };
      case 'COMPLETED':
        return {
          variant: 'success' as const,
          label: 'Completed',
          icon: <CheckCircle size={12} className="mr-1.5" />,
        };
      case 'FAILED':
        return {
          variant: 'danger' as const,
          label: 'Failed',
          icon: <AlertTriangle size={12} className="mr-1.5" />,
        };
      case 'CANCELLED':
        return {
          variant: 'warning' as const,
          label: 'Cancelled',
          icon: <Ban size={12} className="mr-1.5" />,
        };
    }
  };

  const config = getStatusConfig();

  return (
    <Badge variant={config.variant} className="pl-1.5 pr-2 py-0.5 font-medium">
      {config.icon}
      {config.label}
    </Badge>
  );
}

// Token Cell Component - single value with color
function TokenCell({ count, color }: { count: number; color: string }) {
  if (count === 0) {
    return <span className="text-caption text-text-muted font-mono">-</span>;
  }

  const formatTokens = (n: number) => {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
    return n.toString();
  };

  return <span className={`text-xs font-mono ${color}`}>{formatTokens(count)}</span>;
}

// Cost Cell Component
function CostCell({ cost }: { cost: number }) {
  if (cost === 0) {
    return <span className="text-caption text-text-muted font-mono">-</span>;
  }

  const formatCost = (c: number) => {
    if (c < 0.001) return '<$0.001';
    if (c < 0.01) return `$${c.toFixed(4)}`;
    if (c < 1) return `$${c.toFixed(3)}`;
    return `$${c.toFixed(2)}`;
  };

  const getCostColor = (c: number) => {
    if (c >= 0.10) return 'text-rose-400 font-medium';
    if (c >= 0.01) return 'text-amber-400';
    return 'text-text-primary';
  };

  return <span className={`text-xs font-mono ${getCostColor(cost)}`}>{formatCost(cost)}</span>;
}

// Log Row Component
function LogRow({
  request,
  onClick,
}: {
  request: ProxyRequest;
  onClick: () => void;
}) {
  const isPending = request.status === 'PENDING' || request.status === 'IN_PROGRESS';
  const isFailed = request.status === 'FAILED';

  // Live duration calculation for pending requests
  const [liveDuration, setLiveDuration] = useState<number | null>(null);

  useEffect(() => {
    if (!isPending) {
      setLiveDuration(null);
      return;
    }

    const startTime = new Date(request.startTime).getTime();
    const updateDuration = () => {
      const now = Date.now();
      setLiveDuration(now - startTime);
    };

    updateDuration();
    const interval = setInterval(updateDuration, 100);

    return () => clearInterval(interval);
  }, [isPending, request.startTime]);

  const formatDuration = (ns?: number | null) => {
    if (ns === undefined || ns === null) return '-';
    // If it's live duration (ms), convert directly
    if (isPending && liveDuration !== null) {
      return `${(liveDuration / 1000).toFixed(1)}s`;
    }
    // If it's stored duration (nanoseconds), convert
    const ms = ns / 1_000_000;
    if (ms < 1000) return `${ms.toFixed(0)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const formatTime = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
  };

  // Display duration
  const displayDuration = isPending ? liveDuration : request.duration;

  // Duration color
  const durationColor = isPending
    ? 'text-accent font-bold'
    : (displayDuration && displayDuration / 1_000_000 > 5000)
      ? 'text-amber-400'
      : 'text-text-secondary';

  // Get HTTP status code from responseInfo
  const statusCode = request.responseInfo?.status;

  return (
    <TableRow
      onClick={onClick}
      className={cn(
        "cursor-pointer group border-b border-border/40 transition-colors",
        !isPending && "hover:bg-surface-secondary/40",
        isPending && "bg-accent/5 hover:bg-accent/10",
        isFailed && "bg-error/5 hover:bg-error/10"
      )}
    >
      {/* Time */}
      <TableCell className="font-mono text-xs text-text-muted whitespace-nowrap">
        {formatTime(request.startTime || request.createdAt)}
      </TableCell>
      
      {/* Status */}
      <TableCell>
        <RequestStatusBadge status={request.status} />
      </TableCell>
      
      {/* Code */}
      <TableCell>
        <span className={cn(
          "font-mono text-xs font-medium px-1.5 py-0.5 rounded",
          isFailed ? "bg-error/10 text-error" : 
          statusCode && statusCode >= 200 && statusCode < 300 ? "bg-success/10 text-success" : 
          "bg-surface-secondary text-text-muted"
        )}>
          {statusCode && statusCode > 0 ? statusCode : '-'}
        </span>
      </TableCell>
      
      {/* Client */}
      <TableCell>
        <div className="flex items-center gap-2">
          <div className="p-1 rounded bg-surface-secondary text-text-secondary">
            <ClientIcon type={request.clientType} size={14} />
          </div>
          <span className="text-xs text-text-primary capitalize font-medium truncate max-w-[100px]">
            {request.clientType}
          </span>
        </div>
      </TableCell>
      
      {/* Model */}
      <TableCell>
        <div className="flex flex-col max-w-[200px]">
          <span className="text-xs text-text-primary truncate font-medium" title={request.requestModel}>
            {request.requestModel || '-'}
          </span>
          {request.responseModel && request.responseModel !== request.requestModel && (
            <span className="text-[10px] text-text-muted truncate flex items-center gap-1">
              <span className="opacity-50">→</span> {request.responseModel}
            </span>
          )}
        </div>
      </TableCell>
      
      {/* Duration */}
      <TableCell className="text-right">
        <span className={`text-xs font-mono ${durationColor}`}>
          {formatDuration(displayDuration)}
        </span>
      </TableCell>
      
      {/* Cost */}
      <TableCell className="text-right">
        <CostCell cost={request.cost} />
      </TableCell>
      
      {/* Attempts */}
      <TableCell className="text-center">
        {request.proxyUpstreamAttemptCount > 1 ? (
           <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-warning/10 text-warning text-[10px] font-bold">
             {request.proxyUpstreamAttemptCount}
           </span>
        ) : (
          <span className="text-xs text-text-muted opacity-30">1</span>
        )}
      </TableCell>

      {/* Input Tokens - sky blue */}
      <TableCell className="text-right">
        <TokenCell count={request.inputTokenCount} color="text-sky-400" />
      </TableCell>

      {/* Output Tokens - emerald green */}
      <TableCell className="text-right">
        <TokenCell count={request.outputTokenCount} color="text-emerald-400" />
      </TableCell>

      {/* Cache Read - violet */}
      <TableCell className="text-right">
        <TokenCell count={request.cacheReadCount} color="text-violet-400" />
      </TableCell>

      {/* Cache Write - amber */}
      <TableCell className="text-right">
        <TokenCell count={request.cacheWriteCount} color="text-amber-400" />
      </TableCell>
    </TableRow>
  );
}

export default RequestsPage;
