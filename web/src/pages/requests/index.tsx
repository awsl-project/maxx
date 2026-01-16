import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  useProxyRequests,
  useProxyRequestUpdates,
  useProxyRequestsCount,
  useProviders,
  useProjects,
  useAPITokens,
} from '@/hooks/queries'
import {
  Activity,
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  Loader2,
  CheckCircle,
  AlertTriangle,
  Ban,
} from 'lucide-react'
import type { ProxyRequest, ProxyRequestStatus } from '@/lib/transport'
import { ClientIcon } from '@/components/icons/client-icons'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Badge,
  Button,
} from '@/components/ui'
import { cn } from '@/lib/utils'
import { PageHeader } from '@/components/layout/page-header'

const PAGE_SIZE = 50

export const statusVariant: Record<
  ProxyRequestStatus,
  'default' | 'success' | 'warning' | 'danger' | 'info'
> = {
  PENDING: 'default',
  IN_PROGRESS: 'info',
  COMPLETED: 'success',
  FAILED: 'danger',
  CANCELLED: 'warning',
  REJECTED: 'danger',
}

export function RequestsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  // 使用游标分页：存储每页的 lastId 用于向后翻页
  const [cursors, setCursors] = useState<(number | undefined)[]>([undefined])
  const [pageIndex, setPageIndex] = useState(0)

  const currentCursor = cursors[pageIndex]
  const { data, isLoading, refetch } = useProxyRequests({
    limit: PAGE_SIZE,
    before: currentCursor,
  })
  const { data: totalCount, refetch: refetchCount } = useProxyRequestsCount()
  const { data: providers = [] } = useProviders()
  const { data: projects = [] } = useProjects()
  const { data: apiTokens = [] } = useAPITokens()

  // Subscribe to real-time updates
  useProxyRequestUpdates()

  const requests = data?.items ?? []
  const hasMore = data?.hasMore ?? false

  // Create provider ID to name mapping
  const providerMap = new Map(providers.map(p => [p.id, p.name]))
  // Create project ID to name mapping
  const projectMap = new Map(projects.map(p => [p.id, p.name]))
  // Create API Token ID to name mapping
  const tokenMap = new Map(apiTokens.map(t => [t.id, t.name]))

  // 使用 totalCount
  const total = typeof totalCount === 'number' ? totalCount : 0

  // 下一页
  const goToNextPage = () => {
    if (hasMore && data?.lastId) {
      const nextCursors = [...cursors]
      if (pageIndex + 1 >= nextCursors.length) {
        nextCursors.push(data.lastId)
      }
      setCursors(nextCursors)
      setPageIndex(pageIndex + 1)
    }
  }

  // 上一页
  const goToPrevPage = () => {
    if (pageIndex > 0) {
      setPageIndex(pageIndex - 1)
    }
  }

  // 刷新时重置到第一页
  const handleRefresh = () => {
    setCursors([undefined])
    setPageIndex(0)
    refetch()
    refetchCount()
  }

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Activity}
        iconClassName="text-emerald-500"
        title={t('requests.title')}
        description={t('requests.description', { count: total })}
      >
        <Button onClick={handleRefresh} disabled={isLoading}>
          <RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} />
          <span>{t('requests.refresh')}</span>
        </Button>
      </PageHeader>

      {/* Content */}
      <div className="flex-1 overflow-hidden flex flex-col">
        {isLoading && requests.length === 0 ? (
          <div className="flex-1 flex items-center justify-center">
            <Loader2 className="w-8 h-8 animate-spin text-accent" />
          </div>
        ) : requests.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center text-muted-foreground">
            <div className="p-4 bg-muted rounded-full mb-4">
              <Activity size={32} className="opacity-50" />
            </div>
            <p className="text-body font-medium">{t('requests.noRequests')}</p>
            <p className="text-caption mt-1">{t('requests.noRequestsHint')}</p>
          </div>
        ) : (
          <div className="flex-1 overflow-auto">
            <Table>
              <TableHeader className="bg-card/80 backdrop-blur-md sticky top-0 z-10 shadow-sm border-b border-border">
                <TableRow className="hover:bg-transparent border-none text-sm">
                  <TableHead className="w-[180px] font-medium">
                    {t('requests.time')}
                  </TableHead>
                  <TableHead className="w-[120px] font-medium">
                    {t('requests.client')}
                  </TableHead>
                  <TableHead className="w-[180px] font-medium">
                    {t('requests.model')}
                  </TableHead>
                  <TableHead className="w-[100px] font-medium">
                    {t('requests.project')}
                  </TableHead>
                  <TableHead className="w-[100px] font-medium">Token</TableHead>
                  <TableHead className="w-[120px] font-medium">
                    {t('requests.provider')}
                  </TableHead>
                  <TableHead className="w-[100px] font-medium">
                    {t('common.status')}
                  </TableHead>
                  <TableHead className="w-[60px] font-medium">
                    {t('requests.code')}
                  </TableHead>
                  <TableHead className="w-[80px] text-right font-medium">
                    {t('requests.duration')}
                  </TableHead>
                  <TableHead className="w-[80px] text-right font-medium">
                    {t('requests.cost')}
                  </TableHead>
                  <TableHead
                    className="w-[45px] text-center font-medium"
                    title={t('requests.attempts')}
                  >
                    {t('requests.attShort')}
                  </TableHead>
                  <TableHead
                    className="w-[65px] text-right font-medium"
                    title={t('requests.inputTokens')}
                  >
                    {t('requests.inShort')}
                  </TableHead>
                  <TableHead
                    className="w-[65px] text-right font-medium"
                    title={t('requests.outputTokens')}
                  >
                    {t('requests.outShort')}
                  </TableHead>
                  <TableHead
                    className="w-[65px] text-right font-medium"
                    title={t('requests.cacheRead')}
                  >
                    {t('requests.cacheRShort')}
                  </TableHead>
                  <TableHead
                    className="w-[65px] text-right font-medium"
                    title={t('requests.cacheWrite')}
                  >
                    {t('requests.cacheWShort')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {requests.map(req => (
                  <LogRow
                    key={req.id}
                    request={req}
                    providerName={providerMap.get(req.providerID)}
                    projectName={projectMap.get(req.projectID)}
                    tokenName={tokenMap.get(req.apiTokenID)}
                    onClick={() => navigate(`/requests/${req.id}`)}
                  />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      {/* Pagination */}
      <div className="h-[53px] flex items-center justify-between px-6 border-t border-border bg-card shrink-0">
        <span className="text-xs text-muted-foreground">
          {total > 0
            ? t('requests.pageInfo', {
                page: pageIndex + 1,
                count: requests.length,
                total,
              })
            : t('requests.noItems')}
        </span>
        <div className="flex items-center gap-1">
          <Button
            onClick={goToPrevPage}
            disabled={pageIndex === 0}
            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronLeft size={16} />
          </Button>
          <span className="text-xs text-muted-foreground min-w-[60px] text-center font-medium">
            {t('requests.page', { current: pageIndex + 1 })}
          </span>
          <Button
            onClick={goToNextPage}
            disabled={!hasMore}
            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronRight size={16} />
          </Button>
        </div>
      </div>
    </div>
  )
}

// Request Status Badge Component
function RequestStatusBadge({ status }: { status: ProxyRequestStatus }) {
  const { t } = useTranslation()
  const getStatusConfig = () => {
    switch (status) {
      case 'PENDING':
        return {
          variant: 'default' as const,
          label: t('requests.status.pending'),
          icon: <Loader2 size={10} className="mr-1 shrink-0" />,
        }
      case 'IN_PROGRESS':
        return {
          variant: 'info' as const,
          label: t('requests.status.streaming'),
          icon: <Loader2 size={10} className="mr-1 shrink-0 animate-spin" />,
        }
      case 'COMPLETED':
        return {
          variant: 'success' as const,
          label: t('requests.status.completed'),
          icon: <CheckCircle size={10} className="mr-1 shrink-0" />,
        }
      case 'FAILED':
        return {
          variant: 'danger' as const,
          label: t('requests.status.failed'),
          icon: <AlertTriangle size={10} className="mr-1 shrink-0" />,
        }
      case 'CANCELLED':
        return {
          variant: 'warning' as const,
          label: t('requests.status.cancelled'),
          icon: <Ban size={10} className="mr-1 shrink-0" />,
        }
      case 'REJECTED':
        return {
          variant: 'danger' as const,
          label: t('requests.status.rejected'),
          icon: <Ban size={10} className="mr-1 flex-shrink-0" />,
        }
    }
  }

  const config = getStatusConfig()

  return (
    <Badge
      variant={config.variant}
      className="inline-flex items-center pl-1 pr-1.5 py-0 text-[10px] font-medium h-4"
    >
      {config.icon}
      {config.label}
    </Badge>
  )
}

// Token Cell Component - single value with color
function TokenCell({ count, color }: { count: number; color: string }) {
  if (count === 0) {
    return (
      <span className="text-caption text-muted-foreground font-mono">-</span>
    )
  }

  const formatTokens = (n: number) => {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
    if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
    return n.toString()
  }

  return (
    <span className={`text-xs font-mono ${color}`}>{formatTokens(count)}</span>
  )
}

// 微美元转美元 (1 USD = 1,000,000 microUSD)
const MICRO_USD_PER_USD = 1_000_000
function microToUSD(microUSD: number): number {
  return microUSD / MICRO_USD_PER_USD
}

// Cost Cell Component (接收 microUSD)
function CostCell({ cost }: { cost: number }) {
  if (cost === 0) {
    return (
      <span className="text-caption text-muted-foreground font-mono">-</span>
    )
  }

  const usd = microToUSD(cost)

  const formatCost = (c: number) => {
    if (c < 0.001) return '<$0.001'
    if (c < 0.01) return `$${c.toFixed(4)}`
    if (c < 1) return `$${c.toFixed(3)}`
    return `$${c.toFixed(2)}`
  }

  const getCostColor = (c: number) => {
    if (c >= 0.1) return 'text-rose-400 font-medium'
    if (c >= 0.01) return 'text-amber-400'
    return 'text-foreground'
  }

  return (
    <span className={`text-xs font-mono ${getCostColor(usd)}`}>
      {formatCost(usd)}
    </span>
  )
}

// Log Row Component
function LogRow({
  request,
  providerName,
  projectName,
  tokenName,
  onClick,
}: {
  request: ProxyRequest
  providerName?: string
  projectName?: string
  tokenName?: string
  onClick: () => void
}) {
  const isPending =
    request.status === 'PENDING' || request.status === 'IN_PROGRESS'
  const isFailed = request.status === 'FAILED'
  const [isRecent, setIsRecent] = useState(false)

  // Live duration calculation for pending requests
  const [liveDuration, setLiveDuration] = useState<number | null>(null)

  useEffect(() => {
    // Check if request is new (less than 5 seconds old)
    const startTime = new Date(request.startTime).getTime()
    if (Date.now() - startTime < 5000) {
      setIsRecent(true)
      const timer = setTimeout(() => setIsRecent(false), 2000)
      return () => clearTimeout(timer)
    }
  }, [request.startTime])

  useEffect(() => {
    if (!isPending) {
      setLiveDuration(null)
      return
    }

    const startTime = new Date(request.startTime).getTime()
    const updateDuration = () => {
      const now = Date.now()
      setLiveDuration(now - startTime)
    }

    updateDuration()
    const interval = setInterval(updateDuration, 100)

    return () => clearInterval(interval)
  }, [isPending, request.startTime])

  const formatDuration = (ns?: number | null) => {
    if (ns === undefined || ns === null) return '-'
    // If it's live duration (ms), convert directly to seconds
    if (isPending && liveDuration !== null) {
      return `${(liveDuration / 1000).toFixed(1)}s`
    }
    // If it's stored duration (nanoseconds), use base formatter
    // But simplify to only show ms or seconds for list view
    const ms = ns / 1_000_000
    if (ms < 1000) return `${ms.toFixed(0)}ms`
    return `${(ms / 1000).toFixed(2)}s`
  }

  const formatTime = (dateStr: string) => {
    const date = new Date(dateStr)
    const yyyy = date.getFullYear()
    const mm = String(date.getMonth() + 1).padStart(2, '0')
    const dd = String(date.getDate()).padStart(2, '0')
    const HH = String(date.getHours()).padStart(2, '0')
    const MM = String(date.getMinutes()).padStart(2, '0')
    const SS = String(date.getSeconds()).padStart(2, '0')
    return `${yyyy}-${mm}-${dd} ${HH}:${MM}:${SS}`
  }

  // Display duration
  const displayDuration = isPending ? liveDuration : request.duration

  // Duration color
  const durationColor = isPending
    ? 'text-primary font-bold'
    : displayDuration && displayDuration / 1_000_000 > 5000
      ? 'text-amber-400'
      : 'text-muted-foreground'

  // Get HTTP status code (use denormalized field for list performance)
  const statusCode = request.statusCode || request.responseInfo?.status

  return (
    <TableRow
      onClick={onClick}
      className={cn(
        'cursor-pointer group border-none transition-none',
        // Base hover
        !isRecent && 'hover:bg-accent/50',

        // Failed state - Red left border (via shadow) and subtle red bg
        isFailed &&
          'bg-error/5 hover:bg-error/10 shadow-[inset_3px_0_0_0_rgba(239,68,68,0.4)]',

        // Active/Pending state - Blue left border + Marquee animation
        isPending && 'shadow-[inset_3px_0_0_0_#0078D4] animate-marquee-row',

        // New Item Flash Animation
        isRecent &&
          !isPending &&
          'bg-accent/20 shadow-[inset_3px_0_0_0_#0078D4]'
      )}
    >
      {/* Time */}
      <TableCell className="py-1 font-mono text-sm text-foreground font-medium whitespace-nowrap">
        {formatTime(request.startTime || request.createdAt)}
      </TableCell>

      {/* Client */}
      <TableCell className="py-1">
        <div className="flex items-center gap-2">
          <div className="p-1 rounded bg-muted text-muted-foreground">
            <ClientIcon type={request.clientType} size={16} />
          </div>
          <span className="text-sm text-foreground capitalize font-medium truncate max-w-[100px]">
            {request.clientType}
          </span>
        </div>
      </TableCell>

      {/* Model */}
      <TableCell className="py-1">
        <div className="flex flex-col max-w-[200px]">
          <span
            className="text-sm text-foreground truncate font-medium"
            title={request.requestModel}
          >
            {request.requestModel || '-'}
          </span>
          {request.responseModel &&
            request.responseModel !== request.requestModel && (
              <span className="text-[10px] text-muted-foreground truncate flex items-center gap-1">
                <span className="opacity-50">→</span> {request.responseModel}
              </span>
            )}
        </div>
      </TableCell>

      {/* Project */}
      <TableCell className="py-1">
        <span
          className="text-sm text-muted-foreground truncate max-w-[100px] block"
          title={projectName}
        >
          {projectName || '-'}
        </span>
      </TableCell>

      {/* Token */}
      <TableCell className="py-1">
        <span
          className="text-sm text-muted-foreground truncate max-w-[100px] block"
          title={tokenName}
        >
          {tokenName || '-'}
        </span>
      </TableCell>

      {/* Provider */}
      <TableCell className="py-1">
        <span
          className="text-sm text-muted-foreground truncate max-w-[120px] block"
          title={providerName}
        >
          {providerName || '-'}
        </span>
      </TableCell>

      {/* Status */}
      <TableCell className="py-1">
        <RequestStatusBadge status={request.status} />
      </TableCell>

      {/* Code */}
      <TableCell className="py-1">
        <span
          className={cn(
            'font-mono text-xs font-medium px-1.5 py-0.5 rounded',
            isFailed
              ? 'bg-red-400/10 text-red-400'
              : statusCode && statusCode >= 200 && statusCode < 300
                ? 'bg-blue-400/10 text-blue-400'
                : 'bg-muted text-muted-foreground'
          )}
        >
          {statusCode && statusCode > 0 ? statusCode : '-'}
        </span>
      </TableCell>

      {/* Duration */}
      <TableCell className="py-1 text-right">
        <span className={`text-sm font-mono ${durationColor}`}>
          {formatDuration(displayDuration)}
        </span>
      </TableCell>

      {/* Cost */}
      <TableCell className="py-1 text-right">
        <CostCell cost={request.cost} />
      </TableCell>

      {/* Attempts */}
      <TableCell className="py-1 text-center">
        {request.proxyUpstreamAttemptCount > 1 ? (
          <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-warning/10 text-warning text-[10px] font-bold">
            {request.proxyUpstreamAttemptCount}
          </span>
        ) : request.proxyUpstreamAttemptCount === 1 ? (
          <span className="text-sm text-muted-foreground">1</span>
        ) : (
          <span className="text-sm text-muted-foreground opacity-30">-</span>
        )}
      </TableCell>

      {/* Input Tokens - sky blue */}
      <TableCell className="py-1 text-right">
        <TokenCell count={request.inputTokenCount} color="text-sky-400" />
      </TableCell>

      {/* Output Tokens - emerald green */}
      <TableCell className="py-1 text-right">
        <TokenCell count={request.outputTokenCount} color="text-emerald-400" />
      </TableCell>

      {/* Cache Read - violet */}
      <TableCell className="py-1 text-right">
        <TokenCell count={request.cacheReadCount} color="text-violet-400" />
      </TableCell>

      {/* Cache Write - amber */}
      <TableCell className="py-1 text-right">
        <TokenCell count={request.cacheWriteCount} color="text-amber-400" />
      </TableCell>
    </TableRow>
  )
}

export default RequestsPage
