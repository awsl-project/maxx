import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  useProxyRequests,
  useProxyRequestUpdates,
  useProxyRequestsCount,
  useProviders,
  useProjects,
  useAPITokens,
  useSettings,
} from '@/hooks/queries';
import {
  Activity,
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  Loader2,
  CheckCircle,
  AlertTriangle,
  Ban,
} from 'lucide-react';
import type { ProxyRequest, ProxyRequestStatus, Provider } from '@/lib/transport';
import { ClientIcon } from '@/components/icons/client-icons';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Badge,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  SelectGroup,
  SelectLabel,
} from '@/components/ui';
import { cn } from '@/lib/utils';
import { PageHeader } from '@/components/layout/page-header';

type ProviderTypeKey = 'antigravity' | 'kiro' | 'codex' | 'custom';

const PROVIDER_TYPE_ORDER: ProviderTypeKey[] = ['antigravity', 'kiro', 'codex', 'custom'];

const PROVIDER_TYPE_LABELS: Record<ProviderTypeKey, string> = {
  antigravity: 'Antigravity',
  kiro: 'Kiro',
  codex: 'Codex',
  custom: 'Custom',
};

const PAGE_SIZE = 50;

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
};

export function RequestsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  // 使用游标分页：存储每页的 lastId 用于向后翻页
  const [cursors, setCursors] = useState<(number | undefined)[]>([undefined]);
  const [pageIndex, setPageIndex] = useState(0);
  // Provider 过滤器
  const [selectedProviderId, setSelectedProviderId] = useState<number | undefined>(undefined);
  // Status 过滤器
  const [selectedStatus, setSelectedStatus] = useState<string | undefined>(undefined);

  const currentCursor = cursors[pageIndex];
  const { data, isLoading, refetch } = useProxyRequests({
    limit: PAGE_SIZE,
    before: currentCursor,
    providerId: selectedProviderId,
    status: selectedStatus,
  });
  const { data: totalCount, refetch: refetchCount } = useProxyRequestsCount(selectedProviderId, selectedStatus);
  const { data: providers = [] } = useProviders();
  const { data: projects = [] } = useProjects();
  const { data: apiTokens = [] } = useAPITokens();
  const { data: settings } = useSettings();

  // Check if API Token auth is enabled
  const apiTokenAuthEnabled = settings?.api_token_auth_enabled === 'true';

  // Check if force project binding is enabled
  const forceProjectBinding = settings?.force_project_binding === 'true';

  // Check if there are any projects
  const hasProjects = projects.length > 0;

  // Subscribe to real-time updates
  useProxyRequestUpdates();

  const requests = data?.items ?? [];
  const hasMore = data?.hasMore ?? false;

  // Create provider ID to name mapping
  const providerMap = new Map(providers.map((p) => [p.id, p.name]));
  // Create project ID to name mapping
  const projectMap = new Map(projects.map((p) => [p.id, p.name]));
  // Create API Token ID to name mapping
  const tokenMap = new Map(apiTokens.map((t) => [t.id, t.name]));

  // 使用 totalCount
  const total = typeof totalCount === 'number' ? totalCount : 0;

  // 下一页
  const goToNextPage = () => {
    if (hasMore && data?.lastId) {
      const nextCursors = [...cursors];
      if (pageIndex + 1 >= nextCursors.length) {
        nextCursors.push(data.lastId);
      }
      setCursors(nextCursors);
      setPageIndex(pageIndex + 1);
    }
  };

  // 上一页
  const goToPrevPage = () => {
    if (pageIndex > 0) {
      setPageIndex(pageIndex - 1);
    }
  };

  // 刷新时重置到第一页
  const handleRefresh = () => {
    setCursors([undefined]);
    setPageIndex(0);
    refetch();
    refetchCount();
  };

  // Provider 过滤器变化时重置分页
  const handleProviderFilterChange = (providerId: number | undefined) => {
    setSelectedProviderId(providerId);
    setCursors([undefined]);
    setPageIndex(0);
  };

  // Status 过滤器变化时重置分页
  const handleStatusFilterChange = (status: string | undefined) => {
    setSelectedStatus(status);
    setCursors([undefined]);
    setPageIndex(0);
  };

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Activity}
        iconClassName="text-emerald-500"
        title={t('requests.title')}
        description={t('requests.description', { count: total })}
      >
        {/* Provider Filter */}
        {providers.length > 0 && (
          <ProviderFilter
            providers={providers}
            selectedProviderId={selectedProviderId}
            onSelect={handleProviderFilterChange}
          />
        )}
        {/* Status Filter */}
        <StatusFilter
          selectedStatus={selectedStatus}
          onSelect={handleStatusFilterChange}
        />
        <button
          onClick={handleRefresh}
          disabled={isLoading}
          className={cn(
            'flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all',
            'bg-muted/50 hover:bg-muted border border-border/50 hover:border-border',
            'text-muted-foreground hover:text-foreground',
            isLoading && 'opacity-50 cursor-not-allowed',
          )}
        >
          <RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} />
          <span>{t('requests.refresh')}</span>
        </button>
      </PageHeader>

      {/* Content */}
      <div className="flex-1 min-h-0 flex flex-col">
        {isLoading && requests.length === 0 ? (
          <div className="flex-1 flex items-center justify-center">
            <Loader2 className="w-8 h-8 animate-spin text-accent" />
          </div>
        ) : requests.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center text-text-muted">
            <div className="p-4 bg-muted rounded-full mb-4">
              <Activity size={32} className="opacity-50" />
            </div>
            <p className="text-body font-medium">{t('requests.noRequests')}</p>
            <p className="text-caption mt-1">{t('requests.noRequestsHint')}</p>
          </div>
        ) : (
          <div className="flex-1 min-h-0 overflow-auto">
            <Table>
              <TableHeader className="bg-card/80 backdrop-blur-md sticky top-0 z-10 shadow-sm border-b border-border">
                <TableRow className="hover:bg-transparent border-none text-sm">
                  <TableHead className="w-[180px] font-medium">{t('requests.time')}</TableHead>
                  <TableHead className="w-[120px] font-medium">{t('requests.client')}</TableHead>
                  <TableHead className="min-w-[250px] font-medium">{t('requests.model')}</TableHead>
                  {hasProjects && (
                    <TableHead className="w-[100px] font-medium">{t('requests.project')}</TableHead>
                  )}
                  {apiTokenAuthEnabled && (
                    <TableHead className="w-[100px] font-medium">{t('requests.token')}</TableHead>
                  )}
                  <TableHead className="min-w-[100px] font-medium">{t('requests.provider')}</TableHead>
                  <TableHead className="w-[100px] font-medium">{t('common.status')}</TableHead>
                  <TableHead className="w-[60px] text-center font-medium">{t('requests.code')}</TableHead>
                  <TableHead
                    className="w-[60px] text-center font-medium"
                    title={t('requests.ttft')}
                  >
                    TTFT
                  </TableHead>
                  <TableHead className="w-[80px] text-center font-medium">
                    {t('requests.duration')}
                  </TableHead>
                  <TableHead
                    className="w-[45px] text-center font-medium"
                    title={t('requests.attempts')}
                  >
                    {t('requests.attShort')}
                  </TableHead>
                  <TableHead
                    className="w-[65px] text-center font-medium"
                    title={t('requests.inputTokens')}
                  >
                    {t('requests.inShort')}
                  </TableHead>
                  <TableHead
                    className="w-[65px] text-center font-medium"
                    title={t('requests.outputTokens')}
                  >
                    {t('requests.outShort')}
                  </TableHead>
                  <TableHead
                    className="w-[65px] text-center font-medium"
                    title={t('requests.cacheRead')}
                  >
                    {t('requests.cacheRShort')}
                  </TableHead>
                  <TableHead
                    className="w-[65px] text-center font-medium"
                    title={t('requests.cacheWrite')}
                  >
                    {t('requests.cacheWShort')}
                  </TableHead>
                  <TableHead className="w-[80px] text-center font-medium">
                    {t('requests.cost')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {requests.map((req, index) => (
                  <LogRow
                    key={req.id}
                    request={req}
                    index={index}
                    providerName={providerMap.get(req.providerID)}
                    projectName={projectMap.get(req.projectID)}
                    tokenName={tokenMap.get(req.apiTokenID)}
                    showProjectColumn={hasProjects}
                    showTokenColumn={apiTokenAuthEnabled}
                    forceProjectBinding={forceProjectBinding}
                    onClick={() => navigate(`/requests/${req.id}`)}
                  />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      {/* Pagination */}
      <div className="h-12 flex items-center justify-between px-6 border-t border-border bg-card/50 backdrop-blur-sm shrink-0">
        <span className="text-xs text-muted-foreground">
          {total > 0
            ? t('requests.pageInfo', {
                page: pageIndex + 1,
                count: requests.length,
                total,
              })
            : t('requests.noItems')}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={goToPrevPage}
            disabled={pageIndex === 0}
            className={cn(
              'w-8 h-8 rounded-lg flex items-center justify-center transition-all',
              pageIndex === 0
                ? 'text-muted-foreground/30 cursor-not-allowed'
                : 'text-muted-foreground hover:text-foreground hover:bg-accent',
            )}
          >
            <ChevronLeft size={18} />
          </button>
          <div className="flex items-center justify-center min-w-[48px] h-8 px-3 rounded-lg bg-muted/50 border border-border/50">
            <span className="text-sm font-bold text-foreground tabular-nums">
              {pageIndex + 1}
            </span>
            <span className="text-sm text-muted-foreground mx-1">/</span>
            <span className="text-sm text-muted-foreground tabular-nums">
              {Math.ceil(total / PAGE_SIZE) || 1}
            </span>
          </div>
          <button
            onClick={goToNextPage}
            disabled={!hasMore}
            className={cn(
              'w-8 h-8 rounded-lg flex items-center justify-center transition-all',
              !hasMore
                ? 'text-muted-foreground/30 cursor-not-allowed'
                : 'text-muted-foreground hover:text-foreground hover:bg-accent',
            )}
          >
            <ChevronRight size={18} />
          </button>
        </div>
      </div>
    </div>
  );
}

// Request Status Badge Component
function RequestStatusBadge({
  status,
  projectID,
  forceProjectBinding,
}: {
  status: ProxyRequestStatus;
  projectID?: number;
  forceProjectBinding?: boolean;
}) {
  const { t } = useTranslation();

  // Check if pending and waiting for project binding
  const isPendingBinding =
    status === 'PENDING' && forceProjectBinding && (!projectID || projectID === 0);

  const getStatusConfig = () => {
    if (isPendingBinding) {
      return {
        variant: 'warning' as const,
        label: t('requests.status.pendingBinding'),
        icon: <Loader2 size={10} className="mr-1 shrink-0 animate-spin" />,
      };
    }

    switch (status) {
      case 'PENDING':
        return {
          variant: 'default' as const,
          label: t('requests.status.pending'),
          icon: <Loader2 size={10} className="mr-1 shrink-0" />,
        };
      case 'IN_PROGRESS':
        return {
          variant: 'info' as const,
          label: t('requests.status.streaming'),
          icon: <Loader2 size={10} className="mr-1 shrink-0 animate-spin" />,
        };
      case 'COMPLETED':
        return {
          variant: 'success' as const,
          label: t('requests.status.completed'),
          icon: <CheckCircle size={10} className="mr-1 shrink-0" />,
        };
      case 'FAILED':
        return {
          variant: 'danger' as const,
          label: t('requests.status.failed'),
          icon: <AlertTriangle size={10} className="mr-1 shrink-0" />,
        };
      case 'CANCELLED':
        return {
          variant: 'warning' as const,
          label: t('requests.status.cancelled'),
          icon: <Ban size={10} className="mr-1 shrink-0" />,
        };
      case 'REJECTED':
        return {
          variant: 'danger' as const,
          label: t('requests.status.rejected'),
          icon: <Ban size={10} className="mr-1 flex-shrink-0" />,
        };
    }
  };

  const config = getStatusConfig();

  return (
    <Badge
      variant={config.variant}
      className="inline-flex items-center pl-1 pr-1.5 py-0 text-[10px] font-medium h-4"
    >
      {config.icon}
      {config.label}
    </Badge>
  );
}

// Token Cell Component - single value with color
function TokenCell({ count, color }: { count: number; color: string }) {
  if (count === 0) {
    return <span className="text-xs text-muted-foreground font-mono">-</span>;
  }

  const formatTokens = (n: number) => {
    // >= 5位数 (10000+) 使用 K/M 格式
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 10_000) return `${(n / 1000).toFixed(1)}K`;
    // 4位数及以下使用千分位分隔符
    return n.toLocaleString();
  };

  return <span className={`text-xs font-mono ${color}`}>{formatTokens(count)}</span>;
}

// 纳美元转美元 (1 USD = 1,000,000,000 nanoUSD)
// 向下取整到 6 位小数 (microUSD 精度)
function nanoToUSD(nanoUSD: number): number {
  return Math.floor(nanoUSD / 1000) / 1_000_000;
}

// Cost Cell Component (接收 nanoUSD)
function CostCell({ cost }: { cost: number }) {
  if (cost === 0) {
    return <span className="text-xs text-muted-foreground font-mono">-</span>;
  }

  const usd = nanoToUSD(cost);

  // 完整显示 6 位小数
  const formatCost = (c: number) => {
    return `$${c.toFixed(6)}`;
  };

  const getCostColor = (c: number) => {
    if (c >= 0.1) return 'text-rose-400 font-medium';
    if (c >= 0.01) return 'text-amber-400';
    return 'text-foreground';
  };

  return <span className={`text-xs font-mono ${getCostColor(usd)}`}>{formatCost(usd)}</span>;
}

// Log Row Component
function LogRow({
  request,
  index,
  providerName,
  projectName,
  tokenName,
  showProjectColumn,
  showTokenColumn,
  forceProjectBinding,
  onClick,
}: {
  request: ProxyRequest;
  index: number;
  providerName?: string;
  projectName?: string;
  tokenName?: string;
  showProjectColumn?: boolean;
  showTokenColumn?: boolean;
  forceProjectBinding?: boolean;
  onClick: () => void;
}) {
  const isPending = request.status === 'PENDING' || request.status === 'IN_PROGRESS';
  const isFailed = request.status === 'FAILED';
  const isPendingBinding =
    request.status === 'PENDING' &&
    forceProjectBinding &&
    (!request.projectID || request.projectID === 0);
  const [isRecent, setIsRecent] = useState(false);

  // Live duration calculation for pending requests
  const [liveDuration, setLiveDuration] = useState<number | null>(null);

  useEffect(() => {
    // Check if request is new (less than 5 seconds old)
    const startTime = new Date(request.startTime).getTime();
    if (Date.now() - startTime < 5000) {
      setIsRecent(true);
      const timer = setTimeout(() => setIsRecent(false), 2000);
      return () => clearTimeout(timer);
    }
  }, [request.startTime]);

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
    // If it's live duration (ms), convert directly to seconds
    if (isPending && liveDuration !== null) {
      return `${(liveDuration / 1000).toFixed(2)}s`;
    }
    // Convert nanoseconds to seconds with 2 decimal places
    const seconds = ns / 1_000_000_000;
    return `${seconds.toFixed(2)}s`;
  };

  const formatTime = (dateStr: string) => {
    const date = new Date(dateStr);
    const yyyy = date.getFullYear();
    const mm = String(date.getMonth() + 1).padStart(2, '0');
    const dd = String(date.getDate()).padStart(2, '0');
    const HH = String(date.getHours()).padStart(2, '0');
    const MM = String(date.getMinutes()).padStart(2, '0');
    const SS = String(date.getSeconds()).padStart(2, '0');
    return `${yyyy}-${mm}-${dd} ${HH}:${MM}:${SS}`;
  };

  // Display duration
  const displayDuration = isPending ? liveDuration : request.duration;

  // Duration color
  const durationColor = isPending
    ? 'text-primary font-bold'
    : displayDuration && displayDuration / 1_000_000 > 5000
      ? 'text-amber-400'
      : 'text-muted-foreground';

  // Get HTTP status code (use denormalized field for list performance)
  const statusCode = request.statusCode || request.responseInfo?.status;

  // Zebra striping base class
  const zebraClass = index % 2 === 1 ? 'bg-foreground/[0.03]' : '';

  return (
    <TableRow
      onClick={onClick}
      className={cn(
        'cursor-pointer group transition-colors',
        // Zebra striping - applies to all rows as base layer
        zebraClass,
        // Base hover effect (stronger background change)
        !isRecent && !isFailed && !isPending && !isPendingBinding && 'hover:bg-accent/50',

        // Failed state - Red background only (testing without border)
        isFailed && cn(
          index % 2 === 1 ? 'bg-red-500/25' : 'bg-red-500/20',
          'hover:bg-red-500/40'
        ),

        // Pending binding state - Amber background with left border
        isPendingBinding && cn(
          index % 2 === 1 ? 'bg-amber-500/15' : 'bg-amber-500/10',
          'hover:bg-amber-500/25',
          'border-l-2 border-l-amber-500'
        ),

        // Active/Pending state - Blue left border + Marquee animation
        isPending && !isPendingBinding && 'animate-marquee-row',

        // New Item Flash Animation
        isRecent && !isPending && !isPendingBinding && 'bg-accent/20',
      )}
    >
      {/* Time - 显示结束时间，如果没有结束时间则显示开始时间（更浅样式） */}
      <TableCell className="w-[180px] px-2 py-1 font-mono text-sm whitespace-nowrap">
        {request.endTime && new Date(request.endTime).getTime() > 0 ? (
          <span className="text-foreground font-medium">{formatTime(request.endTime)}</span>
        ) : (
          <span className="text-muted-foreground">{formatTime(request.startTime || request.createdAt)}</span>
        )}
      </TableCell>

      {/* Client */}
      <TableCell className="w-[120px] px-2 py-1">
        <div className="flex items-center gap-1.5">
          <ClientIcon type={request.clientType} size={16} className="shrink-0" />
          <span className="text-sm text-foreground capitalize font-medium">
            {request.clientType}
          </span>
        </div>
      </TableCell>

      {/* Model */}
      <TableCell className="min-w-[250px] px-2 py-1">
        <div className="flex items-center gap-2">
          <span
            className="text-sm text-foreground font-medium"
            title={request.requestModel}
          >
            {request.requestModel || '-'}
          </span>
          {request.responseModel && request.responseModel !== request.requestModel && (
            <span className="text-[10px] text-muted-foreground shrink-0">
              → {request.responseModel}
            </span>
          )}
        </div>
      </TableCell>

      {/* Project */}
      {showProjectColumn && (
        <TableCell className="w-[100px] px-2 py-1">
          <span
            className="text-sm text-muted-foreground truncate max-w-[100px] block"
            title={projectName}
          >
            {projectName || '-'}
          </span>
        </TableCell>
      )}

      {/* Token */}
      {showTokenColumn && (
        <TableCell className="w-[100px] px-2 py-1">
          <span
            className="text-sm text-muted-foreground truncate max-w-[100px] block"
            title={tokenName}
          >
            {tokenName || '-'}
          </span>
        </TableCell>
      )}

      {/* Provider */}
      <TableCell className="min-w-[100px] px-2 py-1">
        <span
          className="text-sm text-muted-foreground"
          title={providerName}
        >
          {providerName || '-'}
        </span>
      </TableCell>

      {/* Status */}
      <TableCell className="w-[100px] px-2 py-1">
        <RequestStatusBadge
          status={request.status}
          projectID={request.projectID}
          forceProjectBinding={forceProjectBinding}
        />
      </TableCell>

      {/* Code */}
      <TableCell className="w-[60px] px-2 py-1 text-center">
        <span
          className={cn(
            'font-mono text-xs font-medium px-1.5 py-0.5 rounded',
            isFailed
              ? 'bg-red-400/10 text-red-400'
              : statusCode && statusCode >= 200 && statusCode < 300
                ? 'bg-blue-400/10 text-blue-400'
                : 'bg-muted text-muted-foreground',
          )}
        >
          {statusCode && statusCode > 0 ? statusCode : '-'}
        </span>
      </TableCell>

      {/* TTFT (Time To First Token) */}
      <TableCell className="w-[60px] px-2 py-1 text-center">
        <span className="text-xs font-mono text-muted-foreground">
          {request.ttft && request.ttft > 0
            ? `${(request.ttft / 1_000_000_000).toFixed(2)}s`
            : '-'}
        </span>
      </TableCell>

      {/* Duration */}
      <TableCell className="w-[80px] px-2 py-1 text-center">
        <span
          className={`text-xs font-mono ${durationColor}`}
          title={`${formatTime(request.startTime || request.createdAt)} → ${request.endTime && new Date(request.endTime).getTime() > 0 ? formatTime(request.endTime) : '...'}`}
        >
          {formatDuration(displayDuration)}
        </span>
      </TableCell>

      {/* Attempts */}
      <TableCell className="w-[45px] px-2 py-1 text-center">
        {request.proxyUpstreamAttemptCount > 1 ? (
          <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-warning/10 text-warning text-[10px] font-bold">
            {request.proxyUpstreamAttemptCount}
          </span>
        ) : request.proxyUpstreamAttemptCount === 1 ? (
          <span className="text-xs text-muted-foreground/30">1</span>
        ) : (
          <span className="text-xs text-muted-foreground/30">-</span>
        )}
      </TableCell>

      {/* Input Tokens - sky blue */}
      <TableCell className="w-[65px] px-2 py-1 text-center">
        <TokenCell count={request.inputTokenCount} color="text-sky-400" />
      </TableCell>

      {/* Output Tokens - emerald green */}
      <TableCell className="w-[65px] px-2 py-1 text-center">
        <TokenCell count={request.outputTokenCount} color="text-emerald-400" />
      </TableCell>

      {/* Cache Read - violet */}
      <TableCell className="w-[65px] px-2 py-1 text-center">
        <TokenCell count={request.cacheReadCount} color="text-violet-400" />
      </TableCell>

      {/* Cache Write - amber */}
      <TableCell className="w-[65px] px-2 py-1 text-center">
        <TokenCell count={request.cacheWriteCount} color="text-amber-400" />
      </TableCell>

      {/* Cost */}
      <TableCell className="w-[80px] px-2 py-1 text-center">
        <CostCell cost={request.cost} />
      </TableCell>
    </TableRow>
  );
}

// Provider Filter Component using Select
function ProviderFilter({
  providers,
  selectedProviderId,
  onSelect,
}: {
  providers: Provider[];
  selectedProviderId: number | undefined;
  onSelect: (providerId: number | undefined) => void;
}) {
  const { t } = useTranslation();

  // Group providers by type and sort alphabetically
  const groupedProviders = useMemo(() => {
    const groups: Record<ProviderTypeKey, Provider[]> = {
      antigravity: [],
      kiro: [],
      codex: [],
      custom: [],
    };

    providers.forEach((p) => {
      const type = p.type as ProviderTypeKey;
      if (groups[type]) {
        groups[type].push(p);
      } else {
        groups.custom.push(p);
      }
    });

    // Sort alphabetically within each group
    for (const key of Object.keys(groups) as ProviderTypeKey[]) {
      groups[key].sort((a, b) => a.name.localeCompare(b.name));
    }

    return groups;
  }, [providers]);

  // Get selected provider name for display
  const selectedProvider = providers.find((p) => p.id === selectedProviderId);
  const displayText = selectedProvider?.name ?? t('requests.allProviders');

  return (
    <Select
      value={selectedProviderId !== undefined ? String(selectedProviderId) : 'all'}
      onValueChange={(value) => {
        if (value === 'all') {
          onSelect(undefined);
        } else {
          onSelect(Number(value));
        }
      }}
    >
      <SelectTrigger className="w-48 h-8" size="sm">
        <SelectValue>{displayText}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">
          {t('requests.allProviders')}
        </SelectItem>
        {PROVIDER_TYPE_ORDER.map((typeKey) => {
          const typeProviders = groupedProviders[typeKey];
          if (typeProviders.length === 0) return null;
          return (
            <SelectGroup key={typeKey}>
              <SelectLabel>{PROVIDER_TYPE_LABELS[typeKey]}</SelectLabel>
              {typeProviders.map((provider) => (
                <SelectItem key={provider.id} value={String(provider.id)}>
                  {provider.name}
                </SelectItem>
              ))}
            </SelectGroup>
          );
        })}
      </SelectContent>
    </Select>
  );
}

// Status Filter Component using Select
function StatusFilter({
  selectedStatus,
  onSelect,
}: {
  selectedStatus: string | undefined;
  onSelect: (status: string | undefined) => void;
}) {
  const { t } = useTranslation();

  const statuses: ProxyRequestStatus[] = [
    'COMPLETED',
    'FAILED',
    'IN_PROGRESS',
    'PENDING',
    'CANCELLED',
    'REJECTED',
  ];

  const getStatusLabel = (status: ProxyRequestStatus) => {
    switch (status) {
      case 'PENDING':
        return t('requests.status.pending');
      case 'IN_PROGRESS':
        return t('requests.status.streaming');
      case 'COMPLETED':
        return t('requests.status.completed');
      case 'FAILED':
        return t('requests.status.failed');
      case 'CANCELLED':
        return t('requests.status.cancelled');
      case 'REJECTED':
        return t('requests.status.rejected');
    }
  };

  const displayText = selectedStatus
    ? getStatusLabel(selectedStatus as ProxyRequestStatus)
    : t('requests.allStatuses');

  return (
    <Select
      value={selectedStatus ?? 'all'}
      onValueChange={(value) => {
        if (value === 'all') {
          onSelect(undefined);
        } else {
          onSelect(value ?? undefined);
        }
      }}
    >
      <SelectTrigger className="w-32 h-8" size="sm">
        <SelectValue>{displayText}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">
          {t('requests.allStatuses')}
        </SelectItem>
        {statuses.map((status) => (
          <SelectItem key={status} value={status}>
            {getStatusLabel(status)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export default RequestsPage;
