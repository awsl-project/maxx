import { useState, useMemo, useRef, useEffect } from 'react';
import { useCallback, memo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  useInfiniteProxyRequests,
  useProxyRequestUpdates,
  useProxyRequestsCount,
  useProviders,
  useProjects,
  useAPITokens,
  useSettings,
} from '@/hooks/queries';
import { Activity, RefreshCw, Loader2, CheckCircle, AlertTriangle, Ban } from 'lucide-react';
import type { APIToken, Project, ProxyRequest, ProxyRequestStatus, Provider } from '@/lib/transport';
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
import { useIsMobile } from '@/hooks/use-mobile';

type ProviderTypeKey = 'antigravity' | 'kiro' | 'codex' | 'custom';

const PROVIDER_TYPE_ORDER: ProviderTypeKey[] = ['antigravity', 'kiro', 'codex', 'custom'];

const PROVIDER_TYPE_LABELS: Record<ProviderTypeKey, string> = {
  antigravity: 'Antigravity',
  kiro: 'Kiro',
  codex: 'Codex',
  custom: 'Custom',
};

type RequestFilterMode = 'token' | 'provider' | 'project';

const REQUEST_FILTER_MODE_STORAGE_KEY = 'maxx-requests-filter-mode';
const REQUEST_PROVIDER_FILTER_STORAGE_KEY = 'maxx-requests-provider-filter';
const REQUEST_TOKEN_FILTER_STORAGE_KEY = 'maxx-requests-token-filter';
const REQUEST_PROJECT_FILTER_STORAGE_KEY = 'maxx-requests-project-filter';

function readStoredNumber(key: string): number | undefined {
  if (typeof window === 'undefined') {
    return undefined;
  }
  const raw = window.localStorage.getItem(key);
  if (!raw) {
    return undefined;
  }
  const parsed = Number(raw);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return undefined;
  }
  return parsed;
}

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
  const isMobile = useIsMobile();

  // 过滤维度（默认令牌）
  const [filterMode, setFilterMode] = useState<RequestFilterMode>(() => {
    if (typeof window === 'undefined') {
      return 'token';
    }
    const stored = window.localStorage.getItem(REQUEST_FILTER_MODE_STORAGE_KEY);
    if (stored === 'provider' || stored === 'project') return stored;
    return 'token';
  });
  // Provider 过滤器
  const [selectedProviderId, setSelectedProviderId] = useState<number | undefined>(() =>
    readStoredNumber(REQUEST_PROVIDER_FILTER_STORAGE_KEY),
  );
  // Token 过滤器
  const [selectedTokenId, setSelectedTokenId] = useState<number | undefined>(() =>
    readStoredNumber(REQUEST_TOKEN_FILTER_STORAGE_KEY),
  );
  // Project 过滤器
  const [selectedProjectId, setSelectedProjectId] = useState<number | undefined>(() =>
    readStoredNumber(REQUEST_PROJECT_FILTER_STORAGE_KEY),
  );
  // Status 过滤器
  const [selectedStatus, setSelectedStatus] = useState<string | undefined>(undefined);

  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const loadMoreRef = useRef<HTMLDivElement | null>(null);

  const activeProviderId = filterMode === 'provider' ? selectedProviderId : undefined;
  const activeTokenId = filterMode === 'token' ? selectedTokenId : undefined;
  const activeProjectId = filterMode === 'project' ? selectedProjectId : undefined;

  const { data: providers = [], isSuccess: providersIsSuccess } = useProviders();
  const { data: projects = [], isSuccess: projectsIsSuccess } = useProjects();
  const { data: apiTokens = [], isSuccess: apiTokensIsSuccess } = useAPITokens();
  const { data: settings } = useSettings();

  const waitingProviderFilterValidation =
    filterMode === 'provider' && selectedProviderId !== undefined && !providersIsSuccess;
  const waitingTokenFilterValidation =
    filterMode === 'token' && selectedTokenId !== undefined && !apiTokensIsSuccess;
  const waitingProjectFilterValidation =
    filterMode === 'project' && selectedProjectId !== undefined && !projectsIsSuccess;
  const requestsQueryEnabled = !waitingProviderFilterValidation && !waitingTokenFilterValidation && !waitingProjectFilterValidation;

  // 使用 Infinite Query
  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isFetching,
    refetch,
  } = useInfiniteProxyRequests(activeProviderId, selectedStatus, activeTokenId, activeProjectId, requestsQueryEnabled);

  const { data: totalCount, refetch: refetchCount } = useProxyRequestsCount(
    activeProviderId,
    selectedStatus,
    activeTokenId,
    activeProjectId,
    requestsQueryEnabled,
  );

  // Check if API Token auth is enabled
  const apiTokenAuthEnabled = settings?.api_token_auth_enabled === 'true';

  // Check if force project binding is enabled
  const forceProjectBinding = settings?.force_project_binding === 'true';

  // Check if there are any projects
  const hasProjects = projects.length > 0;

  // Subscribe to real-time updates
  useProxyRequestUpdates();

  // Create provider ID to name mapping
  const providerMap = useMemo(() => new Map(providers.map((p) => [p.id, p.name])), [providers]);
  // Create project ID to name mapping
  const projectMap = useMemo(() => new Map(projects.map((p) => [p.id, p.name])), [projects]);
  // Create API Token ID to name mapping
  const tokenMap = useMemo(() => new Map(apiTokens.map((t) => [t.id, t.name])), [apiTokens]);

  // 使用 totalCount
  const total = typeof totalCount === 'number' ? totalCount : 0;

  // 合并所有页的数据
  const allRequests = useMemo(() => {
    return data?.pages.flatMap((page) => page.items) ?? [];
  }, [data]);
  const showLoadingState = (isLoading || isFetching || !requestsQueryEnabled) && allRequests.length === 0;

  const activeCount = useMemo(() => {
    return allRequests.reduce((count, req) => {
      return req.status === 'PENDING' || req.status === 'IN_PROGRESS' ? count + 1 : count;
    }, 0);
  }, [allRequests]);
  const hasActiveRequests = activeCount > 0;
  const enableMarquee = activeCount <= 10;

  const [nowMs, setNowMs] = useState(() => Date.now());

  // 全局 tick：仅在有“传输中”请求时更新，避免每行一个定时器导致重渲染风暴
  useEffect(() => {
    if (!hasActiveRequests) {
      return;
    }

    setNowMs(Date.now());
    const interval = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [hasActiveRequests]);

  // IntersectionObserver 触底检测
  useEffect(() => {
    const loadMoreEl = loadMoreRef.current;
    if (!loadMoreEl) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage();
        }
      },
      { rootMargin: '200px' }, // 提前 200px 触发
    );

    observer.observe(loadMoreEl);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }
    window.localStorage.setItem(REQUEST_FILTER_MODE_STORAGE_KEY, filterMode);
  }, [filterMode]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }
    if (selectedProviderId === undefined) {
      window.localStorage.removeItem(REQUEST_PROVIDER_FILTER_STORAGE_KEY);
      return;
    }
    window.localStorage.setItem(REQUEST_PROVIDER_FILTER_STORAGE_KEY, String(selectedProviderId));
  }, [selectedProviderId]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }
    if (selectedTokenId === undefined) {
      window.localStorage.removeItem(REQUEST_TOKEN_FILTER_STORAGE_KEY);
      return;
    }
    window.localStorage.setItem(REQUEST_TOKEN_FILTER_STORAGE_KEY, String(selectedTokenId));
  }, [selectedTokenId]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }
    if (selectedProjectId === undefined) {
      window.localStorage.removeItem(REQUEST_PROJECT_FILTER_STORAGE_KEY);
      return;
    }
    window.localStorage.setItem(REQUEST_PROJECT_FILTER_STORAGE_KEY, String(selectedProjectId));
  }, [selectedProjectId]);

  useEffect(() => {
    if (!providersIsSuccess || selectedProviderId === undefined) {
      return;
    }
    if (!providers.some((provider) => provider.id === selectedProviderId)) {
      setSelectedProviderId(undefined);
    }
  }, [providers, providersIsSuccess, selectedProviderId]);

  useEffect(() => {
    if (!apiTokensIsSuccess || selectedTokenId === undefined) {
      return;
    }
    if (!apiTokens.some((token) => token.id === selectedTokenId)) {
      setSelectedTokenId(undefined);
    }
  }, [apiTokens, apiTokensIsSuccess, selectedTokenId]);

  useEffect(() => {
    if (!projectsIsSuccess || selectedProjectId === undefined) {
      return;
    }
    if (!projects.some((project) => project.id === selectedProjectId)) {
      setSelectedProjectId(undefined);
    }
  }, [projects, projectsIsSuccess, selectedProjectId]);

  // 当所有项目被删除时，自动重置过滤模式
  useEffect(() => {
    if (projectsIsSuccess && !hasProjects && filterMode === 'project') {
      setFilterMode('token');
    }
  }, [projectsIsSuccess, hasProjects, filterMode]);

  // 刷新
  const handleRefresh = () => {
    if (!requestsQueryEnabled) {
      return;
    }
    scrollContainerRef.current?.scrollTo({ top: 0 });
    refetch();
    refetchCount();
  };

  // 过滤模式变化时重置滚动
  const handleFilterModeChange = (mode: RequestFilterMode) => {
    setFilterMode(mode);
    scrollContainerRef.current?.scrollTo({ top: 0 });
  };

  // Provider 过滤器变化时重置
  const handleProviderFilterChange = (providerId: number | undefined) => {
    setSelectedProviderId(providerId);
    scrollContainerRef.current?.scrollTo({ top: 0 });
  };

  // Token 过滤器变化时重置
  const handleTokenFilterChange = (tokenId: number | undefined) => {
    setSelectedTokenId(tokenId);
    scrollContainerRef.current?.scrollTo({ top: 0 });
  };

  // Project 过滤器变化时重置
  const handleProjectFilterChange = (projectId: number | undefined) => {
    setSelectedProjectId(projectId);
    scrollContainerRef.current?.scrollTo({ top: 0 });
  };

  // Status 过滤器变化时重置
  const handleStatusFilterChange = (status: string | undefined) => {
    setSelectedStatus(status);
    scrollContainerRef.current?.scrollTo({ top: 0 });
  };

  const handleOpenRequest = useCallback(
    (id: number) => {
      navigate(`/requests/${id}`);
    },
    [navigate],
  );

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Activity}
        iconClassName="text-emerald-500"
        title={t('requests.title')}
        description={t('requests.description', { count: total })}
      >
        {/* Filter Mode + Dynamic Target Filter */}
        <FilterModeSelect mode={filterMode} hasProjects={hasProjects} onSelect={handleFilterModeChange} />
        {filterMode === 'provider' ? (
          <ProviderFilter
            providers={providers}
            selectedProviderId={selectedProviderId}
            onSelect={handleProviderFilterChange}
          />
        ) : filterMode === 'project' ? (
          <ProjectFilter
            projects={projects}
            selectedProjectId={selectedProjectId}
            onSelect={handleProjectFilterChange}
          />
        ) : (
          <TokenFilter
            tokens={apiTokens}
            selectedTokenId={selectedTokenId}
            onSelect={handleTokenFilterChange}
          />
        )}
        {/* Status Filter */}
        <StatusFilter selectedStatus={selectedStatus} onSelect={handleStatusFilterChange} />
        <button
          onClick={handleRefresh}
          disabled={isFetching || !requestsQueryEnabled}
          className={cn(
            'flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all',
            'bg-muted/50 hover:bg-muted border border-border/50 hover:border-border',
            'text-muted-foreground hover:text-foreground',
            (isFetching || !requestsQueryEnabled) && 'opacity-50 cursor-not-allowed',
          )}
        >
          <RefreshCw size={14} className={isFetching ? 'animate-spin' : ''} />
          <span>{t('requests.refresh')}</span>
        </button>
      </PageHeader>

      {/* Content */}
      <div className="flex-1 min-h-0 flex flex-col">
        {showLoadingState ? (
          <div className="flex-1 flex items-center justify-center">
            <Loader2 className="w-8 h-8 animate-spin text-accent" />
          </div>
        ) : allRequests.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center text-text-muted">
            <div className="p-4 bg-muted rounded-full mb-4">
              <Activity size={32} className="opacity-50" />
            </div>
            <p className="text-body font-medium">{t('requests.noRequests')}</p>
            <p className="text-caption mt-1">{t('requests.noRequestsHint')}</p>
          </div>
        ) : (
          <div className="flex-1 min-h-0 overflow-auto" ref={scrollContainerRef}>
            {isMobile ? (
              <div>
                {allRequests.map((req) => (
                  <MemoMobileRequestCard
                    key={req.id}
                    request={req}
                    providerName={providerMap.get(req.providerID)}
                    onOpenRequest={handleOpenRequest}
                  />
                ))}
                {/* 触底加载指示器 */}
                <div ref={loadMoreRef} className="py-4 flex justify-center">
                  {isFetchingNextPage && (
                    <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
                  )}
                  {!hasNextPage && allRequests.length > 0 && (
                    <span className="text-xs text-muted-foreground">
                      {t('requests.noMoreData')}
                    </span>
                  )}
                </div>
              </div>
            ) : (
              <>
                <Table>
                  <TableHeader className="bg-card/80 backdrop-blur-md sticky top-0 z-10 shadow-sm border-b border-border">
                    <TableRow className="hover:bg-transparent border-none text-sm">
                      <TableHead className="w-[180px] font-medium">{t('requests.time')}</TableHead>
                      <TableHead className="w-[120px] pr-4 font-medium">
                        {t('requests.client')}
                      </TableHead>
                      <TableHead className="min-w-[250px] font-medium">
                        {t('requests.model')}
                      </TableHead>
                      {hasProjects && (
                        <TableHead className="w-[100px] font-medium">
                          {t('requests.project')}
                        </TableHead>
                      )}
                      {apiTokenAuthEnabled && (
                        <TableHead className="w-[100px] font-medium">
                          {t('requests.token')}
                        </TableHead>
                      )}
                      <TableHead className="min-w-[100px] font-medium">
                        {t('requests.provider')}
                      </TableHead>
                      <TableHead className="w-[100px] font-medium">{t('common.status')}</TableHead>
                      <TableHead className="w-[60px] text-center font-medium">
                        {t('requests.code')}
                      </TableHead>
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
                    {allRequests.map((req) => (
                      <MemoLogRow
                        key={req.id}
                        request={req}
                        providerName={providerMap.get(req.providerID)}
                        projectName={projectMap.get(req.projectID)}
                        tokenName={tokenMap.get(req.apiTokenID)}
                        showProjectColumn={hasProjects}
                        showTokenColumn={apiTokenAuthEnabled}
                        forceProjectBinding={forceProjectBinding}
                        nowMs={nowMs}
                        enableMarquee={enableMarquee}
                        onOpenRequest={handleOpenRequest}
                      />
                    ))}
                  </TableBody>
                </Table>
                {/* 触底加载指示器 */}
                <div ref={loadMoreRef} className="py-4 flex justify-center">
                  {isFetchingNextPage && (
                    <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
                  )}
                  {!hasNextPage && allRequests.length > 0 && (
                    <span className="text-xs text-muted-foreground">
                      {t('requests.noMoreData')}
                    </span>
                  )}
                </div>
              </>
            )}
          </div>
        )}
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
      default:
        return {
          variant: 'default' as const,
          label: status || 'Unknown',
          icon: null,
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
    // >= 5 位数 (10000+) 使用 K/M 格式
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 10_000) return `${(n / 1000).toFixed(1)}K`;
    // 4 位数及以下使用千分位分隔符
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
type LogRowProps = {
  request: ProxyRequest;
  providerName?: string;
  projectName?: string;
  tokenName?: string;
  showProjectColumn?: boolean;
  showTokenColumn?: boolean;
  forceProjectBinding?: boolean;
  nowMs: number;
  enableMarquee: boolean;
  onOpenRequest: (id: number) => void;
};

function LogRow({
  request,
  providerName,
  projectName,
  tokenName,
  showProjectColumn,
  showTokenColumn,
  forceProjectBinding,
  nowMs,
  enableMarquee,
  onOpenRequest,
}: LogRowProps) {
  const isPending = request.status === 'PENDING' || request.status === 'IN_PROGRESS';
  const isFailed = request.status === 'FAILED';
  const isPendingBinding =
    request.status === 'PENDING' &&
    forceProjectBinding &&
    (!request.projectID || request.projectID === 0);
  const [isRecent, setIsRecent] = useState(false);

  useEffect(() => {
    // Check if request is new (less than 5 seconds old)
    const startTime = new Date(request.startTime).getTime();
    if (Date.now() - startTime < 5000) {
      setIsRecent(true);
      const timer = setTimeout(() => setIsRecent(false), 2000);
      return () => clearTimeout(timer);
    }
  }, [request.startTime]);

  const startTimeMs = useMemo(() => new Date(request.startTime).getTime(), [request.startTime]);
  const liveDurationMs =
    isPending && Number.isFinite(startTimeMs) ? Math.max(0, nowMs - startTimeMs) : null;

  const formatDuration = (ns?: number | null) => {
    if (ns === undefined || ns === null) return '-';
    // Convert nanoseconds to seconds with 2 decimal places
    const seconds = ns / 1_000_000_000;
    return `${seconds.toFixed(2)}s`;
  };

  const formatLiveDuration = (ms: number | null) => {
    if (ms === null) return '-';
    return `${(ms / 1000).toFixed(2)}s`;
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
  const displayDuration = request.duration;

  // Duration color
  const durationColor = isPending
    ? 'text-primary font-bold'
    : displayDuration && displayDuration / 1_000_000 > 5000
      ? 'text-amber-400'
      : 'text-muted-foreground';

  // Get HTTP status code (use denormalized field for list performance)
  const statusCode = request.statusCode || request.responseInfo?.status;

  const handleClick = () => onOpenRequest(request.id);

  return (
    <TableRow
      onClick={handleClick}
      className={cn(
        'cursor-pointer group transition-colors',
        // Zebra striping - use CSS selector to avoid passing index (inserts won't invalidate memo)
        'even:bg-foreground/[0.03]',
        // Base hover effect (stronger background change)
        !isRecent && !isFailed && !isPending && !isPendingBinding && 'hover:bg-accent/50',

        // Failed state - Red background only (testing without border)
        isFailed && cn('bg-red-500/20 even:bg-red-500/25', 'hover:bg-red-500/40'),

        // Pending binding state - Amber background with left border
        isPendingBinding &&
          cn(
            'bg-amber-500/10 even:bg-amber-500/15',
            'hover:bg-amber-500/25',
            'border-l-2 border-l-amber-500',
          ),

        // Active/Pending state - Blue left border + (optional) Marquee animation
        isPending &&
          !isPendingBinding &&
          (enableMarquee
            ? 'animate-marquee-row'
            : cn(
                'bg-blue-500/5 even:bg-blue-500/10',
                'border-l-2 border-l-blue-500/50',
              )),

        // New Item Flash Animation
        isRecent && !isPending && !isPendingBinding && 'bg-accent/20',
      )}
    >
      {/* Time - 显示结束时间，如果没有结束时间则显示开始时间（更浅样式） */}
      <TableCell className="w-[180px] px-2 py-1 font-mono text-sm whitespace-nowrap">
        {request.endTime && new Date(request.endTime).getTime() > 0 ? (
          <span className="text-foreground font-medium">{formatTime(request.endTime)}</span>
        ) : (
          <span className="text-muted-foreground">
            {formatTime(request.startTime || request.createdAt)}
          </span>
        )}
      </TableCell>

      {/* Client */}
      <TableCell className="w-[120px] px-2 pr-4 py-1">
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
          <span className="text-sm text-foreground font-medium" title={request.requestModel}>
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
        <span className="text-sm text-muted-foreground" title={providerName}>
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
          {request.ttft && request.ttft > 0 ? `${(request.ttft / 1_000_000_000).toFixed(2)}s` : '-'}
        </span>
      </TableCell>

      {/* Duration */}
      <TableCell className="w-[80px] px-2 py-1 text-center">
        <span
          className={`text-xs font-mono ${durationColor}`}
          title={`${formatTime(request.startTime || request.createdAt)} → ${request.endTime && new Date(request.endTime).getTime() > 0 ? formatTime(request.endTime) : '...'}`}
        >
          {isPending ? formatLiveDuration(liveDurationMs) : formatDuration(displayDuration)}
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

const MemoLogRow = memo(
  LogRow,
  (prev: Readonly<LogRowProps>, next: Readonly<LogRowProps>) => {
    if (prev.request !== next.request) return false;
    if (prev.providerName !== next.providerName) return false;
    if (prev.projectName !== next.projectName) return false;
    if (prev.tokenName !== next.tokenName) return false;
    if (prev.showProjectColumn !== next.showProjectColumn) return false;
    if (prev.showTokenColumn !== next.showTokenColumn) return false;
    if (prev.forceProjectBinding !== next.forceProjectBinding) return false;
    if (prev.enableMarquee !== next.enableMarquee) return false;
    if (prev.onOpenRequest !== next.onOpenRequest) return false;

    const prevPending = prev.request.status === 'PENDING' || prev.request.status === 'IN_PROGRESS';
    const nextPending = next.request.status === 'PENDING' || next.request.status === 'IN_PROGRESS';
    if (prevPending || nextPending) {
      return prev.nowMs === next.nowMs;
    }

    return true;
  },
);

// Mobile Request Card Component
type MobileRequestCardProps = {
  request: ProxyRequest;
  providerName?: string;
  onOpenRequest: (id: number) => void;
};

function MobileRequestCard({ request, providerName, onOpenRequest }: MobileRequestCardProps) {
  const isPending = request.status === 'PENDING' || request.status === 'IN_PROGRESS';
  const isFailed = request.status === 'FAILED';
  const handleClick = useCallback(() => onOpenRequest(request.id), [onOpenRequest, request.id]);

  const formatTime = (dateStr: string) => {
    const date = new Date(dateStr);
    const HH = String(date.getHours()).padStart(2, '0');
    const MM = String(date.getMinutes()).padStart(2, '0');
    const SS = String(date.getSeconds()).padStart(2, '0');
    return `${HH}:${MM}:${SS}`;
  };

  const formatDurationMs = (ns?: number | null) => {
    if (!ns) return '-';
    const seconds = ns / 1_000_000_000;
    return `${seconds.toFixed(2)}s`;
  };

  const formatCostShort = (nanoUSD: number) => {
    if (nanoUSD === 0) return '-';
    const usd = Math.floor(nanoUSD / 1000) / 1_000_000;
    return `$${usd.toFixed(4)}`;
  };

  const timeStr =
    request.endTime && new Date(request.endTime).getTime() > 0
      ? formatTime(request.endTime)
      : formatTime(request.startTime || request.createdAt);

  return (
    <div
      onClick={handleClick}
      className={cn(
        'px-4 py-2.5 border-b border-border cursor-pointer active:bg-accent/50 transition-colors',
        isFailed && 'bg-red-500/10',
        isPending && 'bg-blue-500/5',
      )}
    >
      {/* Row 1: Client + Model + Status */}
      <div className="flex items-center gap-2 mb-1">
        <ClientIcon type={request.clientType} size={14} className="shrink-0" />
        <span className="text-sm font-medium text-foreground truncate flex-1">
          {request.requestModel || '-'}
        </span>
        <RequestStatusBadge status={request.status} />
      </div>
      {/* Row 2: Time + Duration + Cost */}
      <div className="flex items-center gap-3 text-xs text-muted-foreground font-mono">
        <span>{timeStr}</span>
        <span>{formatDurationMs(request.duration)}</span>
        <span className="ml-auto">{formatCostShort(request.cost)}</span>
      </div>
      {/* Row 3: Provider */}
      {providerName && (
        <div className="text-xs text-muted-foreground mt-1 truncate">{providerName}</div>
      )}
    </div>
  );
}

const MemoMobileRequestCard = memo(
  MobileRequestCard,
  (prev: Readonly<MobileRequestCardProps>, next: Readonly<MobileRequestCardProps>) => {
    if (prev.request !== next.request) return false;
    if (prev.providerName !== next.providerName) return false;
    if (prev.onOpenRequest !== next.onOpenRequest) return false;
    return true;
  },
);

function FilterModeSelect({
  mode,
  hasProjects,
  onSelect,
}: {
  mode: RequestFilterMode;
  hasProjects: boolean;
  onSelect: (mode: RequestFilterMode) => void;
}) {
  const { t } = useTranslation();

  const displayText =
    mode === 'project'
      ? t('requests.filterByProject')
      : mode === 'provider'
        ? t('requests.filterByProvider')
        : t('requests.filterByToken');

  return (
    <Select
      value={mode}
      onValueChange={(value) => {
        if (value === 'token' || value === 'provider' || value === 'project') {
          onSelect(value);
        }
      }}
    >
      <SelectTrigger className="w-24 md:w-28 h-8" size="sm">
        <SelectValue>{displayText}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="token">{t('requests.filterByToken')}</SelectItem>
        <SelectItem value="provider">{t('requests.filterByProvider')}</SelectItem>
        {hasProjects && (
          <SelectItem value="project">{t('requests.filterByProject')}</SelectItem>
        )}
      </SelectContent>
    </Select>
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
      <SelectTrigger className="w-32 md:w-48 h-8" size="sm">
        <SelectValue>{displayText}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t('requests.allProviders')}</SelectItem>
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

function TokenFilter({
  tokens,
  selectedTokenId,
  onSelect,
}: {
  tokens: APIToken[];
  selectedTokenId: number | undefined;
  onSelect: (tokenId: number | undefined) => void;
}) {
  const { t } = useTranslation();

  const selectedToken = tokens.find((token) => token.id === selectedTokenId);
  const displayText = selectedToken?.name ?? t('requests.allTokens');

  return (
    <Select
      value={selectedTokenId !== undefined ? String(selectedTokenId) : 'all'}
      onValueChange={(value) => {
        if (value === 'all') {
          onSelect(undefined);
        } else {
          onSelect(Number(value));
        }
      }}
    >
      <SelectTrigger className="w-32 md:w-48 h-8" size="sm">
        <SelectValue>{displayText}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t('requests.allTokens')}</SelectItem>
        {tokens.map((token) => (
          <SelectItem key={token.id} value={String(token.id)}>
            {token.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function ProjectFilter({
  projects,
  selectedProjectId,
  onSelect,
}: {
  projects: Project[];
  selectedProjectId: number | undefined;
  onSelect: (projectId: number | undefined) => void;
}) {
  const { t } = useTranslation();

  const selectedProject = projects.find((p) => p.id === selectedProjectId);
  const displayText = selectedProject?.name ?? t('requests.allProjects');

  return (
    <Select
      value={selectedProjectId !== undefined ? String(selectedProjectId) : 'all'}
      onValueChange={(value) => {
        if (value === 'all') {
          onSelect(undefined);
        } else {
          onSelect(Number(value));
        }
      }}
    >
      <SelectTrigger className="w-32 md:w-48 h-8" size="sm">
        <SelectValue>{displayText}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t('requests.allProjects')}</SelectItem>
        {projects.map((project) => (
          <SelectItem key={project.id} value={String(project.id)}>
            {project.name}
          </SelectItem>
        ))}
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
      <SelectTrigger className="w-24 md:w-32 h-8" size="sm">
        <SelectValue>{displayText}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t('requests.allStatuses')}</SelectItem>
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
