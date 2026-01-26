import { useState, useMemo, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  BarChart3,
  RefreshCw,
  Calculator,
  PanelLeft,
  Activity,
  Cpu,
  Coins,
  CheckCircle,
  X,
} from 'lucide-react';
import { PageHeader } from '@/components/layout/page-header';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Tabs,
  TabsList,
  TabsTrigger,
  Button,
  Progress,
} from '@/components/ui';
import { cn } from '@/lib/utils';
import {
  useUsageStats,
  useProviders,
  useProjects,
  useAPITokens,
  useRecalculateUsageStats,
  useRecalculateCosts,
  useResponseModels,
} from '@/hooks/queries';
import type {
  UsageStatsFilter,
  UsageStats,
  StatsGranularity,
  RecalculateCostsProgress,
  RecalculateStatsProgress,
} from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import {
  ComposedChart,
  Bar,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';

type TimeRange = '1h' | '24h' | '7d' | '30d' | '90d' | 'today' | 'yesterday' | 'thisWeek' | 'lastWeek' | 'thisMonth' | 'lastMonth' | 'all';

interface TimeRangeConfig {
  start: Date | null; // null means all time
  end: Date;
  granularity: StatsGranularity;
  durationMinutes: number; // Total duration in minutes for RPM/TPM calculation
}

/**
 * 获取时间范围配置，包括合适的粒度
 */
function getTimeRangeConfig(range: TimeRange): TimeRangeConfig {
  const now = new Date();
  let start: Date | null;
  let end: Date = now;
  let granularity: StatsGranularity;
  let durationMinutes: number;

  // 获取今天的开始时间（00:00:00）
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());

  switch (range) {
    case '1h':
      start = new Date(now.getTime() - 60 * 60 * 1000);
      granularity = 'minute';
      durationMinutes = 60;
      break;
    case '24h':
      start = new Date(now.getTime() - 24 * 60 * 60 * 1000);
      granularity = 'hour';
      durationMinutes = 24 * 60;
      break;
    case '7d':
      start = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
      granularity = 'hour';
      durationMinutes = 7 * 24 * 60;
      break;
    case '30d':
      start = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
      granularity = 'day';
      durationMinutes = 30 * 24 * 60;
      break;
    case '90d':
      start = new Date(now.getTime() - 90 * 24 * 60 * 60 * 1000);
      granularity = 'day';
      durationMinutes = 90 * 24 * 60;
      break;
    case 'today':
      start = todayStart;
      granularity = 'hour';
      durationMinutes = Math.floor((now.getTime() - todayStart.getTime()) / 60000) || 1;
      break;
    case 'yesterday': {
      const yesterdayStart = new Date(todayStart);
      yesterdayStart.setDate(yesterdayStart.getDate() - 1);
      start = yesterdayStart;
      // 减去 1 毫秒，确保不包含今天
      end = new Date(todayStart.getTime() - 1);
      granularity = 'hour';
      durationMinutes = 24 * 60;
      break;
    }
    case 'thisWeek': {
      // 本周一开始
      const dayOfWeek = now.getDay();
      const diff = dayOfWeek === 0 ? 6 : dayOfWeek - 1; // 周日为0，需要回退6天
      const weekStart = new Date(todayStart);
      weekStart.setDate(weekStart.getDate() - diff);
      start = weekStart;
      granularity = 'day';
      durationMinutes = Math.floor((now.getTime() - weekStart.getTime()) / 60000) || 1;
      break;
    }
    case 'lastWeek': {
      // 上周一到上周日（不包含本周一）
      const dayOfWeek = now.getDay();
      const diff = dayOfWeek === 0 ? 6 : dayOfWeek - 1;
      const thisWeekStart = new Date(todayStart);
      thisWeekStart.setDate(thisWeekStart.getDate() - diff);
      const lastWeekStart = new Date(thisWeekStart);
      lastWeekStart.setDate(lastWeekStart.getDate() - 7);
      start = lastWeekStart;
      // 减去 1 毫秒，确保不包含本周一
      end = new Date(thisWeekStart.getTime() - 1);
      granularity = 'day';
      durationMinutes = 7 * 24 * 60;
      break;
    }
    case 'thisMonth': {
      const monthStart = new Date(now.getFullYear(), now.getMonth(), 1);
      start = monthStart;
      granularity = 'day';
      durationMinutes = Math.floor((now.getTime() - monthStart.getTime()) / 60000) || 1;
      break;
    }
    case 'lastMonth': {
      const thisMonthStart = new Date(now.getFullYear(), now.getMonth(), 1);
      const lastMonthStart = new Date(now.getFullYear(), now.getMonth() - 1, 1);
      start = lastMonthStart;
      // 减去 1 毫秒，确保不包含本月第一天
      end = new Date(thisMonthStart.getTime() - 1);
      granularity = 'day';
      // 计算上月天数
      const lastMonthDays = Math.floor((thisMonthStart.getTime() - lastMonthStart.getTime()) / (24 * 60 * 60 * 1000));
      durationMinutes = lastMonthDays * 24 * 60;
      break;
    }
    case 'all':
      // 全部时间，使用 year 粒度
      start = new Date(now.getFullYear() - 4, 0, 1); // 5年前的1月1日
      granularity = 'year';
      durationMinutes = 5 * 365 * 24 * 60;
      break;
  }

  return { start, end, granularity, durationMinutes };
}

interface ChartDataPoint {
  label: string;
  totalRequests: number;
  successful: number;
  failed: number;
  successRate: number; // 成功率 0-100
  inputTokens: number;
  outputTokens: number;
  cacheRead: number;
  cacheWrite: number;
  cost: number;
}

/**
 * 生成时间轴上的所有时间点
 */
function generateTimeAxis(start: Date | null, end: Date, granularity: StatsGranularity): string[] {
  const keys: string[] = [];
  if (!start) return keys;

  const current = new Date(start);
  while (current <= end) {
    keys.push(getAggregationKey(current, granularity));
    // 根据粒度增加时间
    switch (granularity) {
      case 'minute':
        current.setMinutes(current.getMinutes() + 1);
        break;
      case 'hour':
        current.setHours(current.getHours() + 1);
        break;
      case 'day':
        current.setDate(current.getDate() + 1);
        break;
      case 'week':
        current.setDate(current.getDate() + 7);
        break;
      case 'month':
        current.setMonth(current.getMonth() + 1);
        break;
      case 'year':
        current.setFullYear(current.getFullYear() + 1);
        break;
    }
  }
  return keys;
}

/**
 * 聚合数据用于图表，根据粒度自动调整聚合方式，并补全空的时间点
 */
function aggregateForChart(
  stats: UsageStats[] | undefined,
  granularity: StatsGranularity,
  timeRange: TimeRange,
  timeConfig: TimeRangeConfig,
): ChartDataPoint[] {
  const dataMap = new Map<string, ChartDataPoint>();

  const emptyDataPoint = (): Omit<ChartDataPoint, 'label'> => ({
    totalRequests: 0,
    successful: 0,
    failed: 0,
    successRate: 0,
    inputTokens: 0,
    outputTokens: 0,
    cacheRead: 0,
    cacheWrite: 0,
    cost: 0,
  });

  // 先生成完整的时间轴（对于非 'all' 的时间范围）
  if (timeConfig.start) {
    const timeAxis = generateTimeAxis(timeConfig.start, timeConfig.end, granularity);
    timeAxis.forEach((key) => {
      dataMap.set(key, { label: key, ...emptyDataPoint() });
    });
  }

  // 填充实际数据
  if (stats && stats.length > 0) {
    stats.forEach((s) => {
      const bucketDate = new Date(s.timeBucket);
      const key = getAggregationKey(bucketDate, granularity);

      const existing = dataMap.get(key) || { label: key, ...emptyDataPoint() };

      existing.successful += s.successfulRequests;
      existing.failed += s.failedRequests;
      existing.inputTokens += s.inputTokens;
      existing.outputTokens += s.outputTokens;
      existing.cacheRead += s.cacheRead;
      existing.cacheWrite += s.cacheWrite;
      existing.cost += s.cost;
      dataMap.set(key, existing);
    });
  }

  if (dataMap.size === 0) return [];

  // 排序、计算成功率并格式化标签
  return Array.from(dataMap.values())
    .sort((a, b) => a.label.localeCompare(b.label))
    .map((item) => {
      const totalRequests = item.successful + item.failed;
      return {
        ...item,
        label: formatLabel(item.label, granularity, timeRange),
        totalRequests,
        successRate: totalRequests > 0 ? (item.successful / totalRequests) * 100 : 0,
        // 转换 cost 从纳美元到美元 (1 USD = 1,000,000,000 nanoUSD)
        cost: item.cost / 1_000_000_000,
      };
    });
}

/**
 * 获取聚合键（用于分组）
 */
function getAggregationKey(date: Date, granularity: StatsGranularity): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hour = String(date.getHours()).padStart(2, '0');
  const minute = String(date.getMinutes()).padStart(2, '0');

  switch (granularity) {
    case 'minute':
      return `${year}-${month}-${day}T${hour}:${minute}`;
    case 'hour':
      return `${year}-${month}-${day}T${hour}`;
    case 'day':
      return `${year}-${month}-${day}`;
    case 'week': {
      // 使用周一的日期作为键
      const dayOfWeek = date.getDay();
      const diff = dayOfWeek === 0 ? 6 : dayOfWeek - 1;
      const monday = new Date(date);
      monday.setDate(date.getDate() - diff);
      return `${monday.getFullYear()}-${String(monday.getMonth() + 1).padStart(2, '0')}-${String(monday.getDate()).padStart(2, '0')}`;
    }
    case 'month':
      return `${year}-${month}`;
    case 'year':
      return `${year}`;
    default:
      return `${year}-${month}-${day}T${hour}`;
  }
}

/**
 * 格式化显示标签
 */
function formatLabel(key: string, granularity: StatsGranularity, timeRange: TimeRange): string {
  // 根据键的格式解析日期
  let date: Date;

  if (key.includes('T')) {
    // 包含时间部分
    const [datePart, timePart] = key.split('T');
    const [year, month, day] = datePart.split('-').map(Number);
    const [hour, minute] = timePart.split(':').map(Number);
    date = new Date(year, month - 1, day, hour || 0, minute || 0);
  } else if (key.length === 4) {
    // 年份格式: YYYY
    const year = Number(key);
    date = new Date(year, 0, 1);
  } else if (key.length === 7) {
    // 月份格式: YYYY-MM
    const [year, month] = key.split('-').map(Number);
    date = new Date(year, month - 1, 1);
  } else {
    // 日期格式: YYYY-MM-DD
    const [year, month, day] = key.split('-').map(Number);
    date = new Date(year, month - 1, day);
  }

  switch (granularity) {
    case 'minute':
      return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    case 'hour':
      if (timeRange === '24h') {
        return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      }
      return date.toLocaleDateString([], { month: 'short', day: 'numeric', hour: '2-digit' });
    case 'day':
      return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
    case 'week':
      return `Week of ${date.toLocaleDateString([], { month: 'short', day: 'numeric' })}`;
    case 'month':
      return date.toLocaleDateString([], { year: 'numeric', month: 'short' });
    case 'year':
      return String(date.getFullYear());
    default:
      return key;
  }
}

/**
 * 格式化数字（K, M, B）
 */
function formatNumber(num: number): string {
  if (num >= 1000000000) {
    return (num / 1000000000).toFixed(1) + 'B';
  }
  if (num >= 1000000) {
    return (num / 1000000).toFixed(1) + 'M';
  }
  if (num >= 1000) {
    return (num / 1000).toFixed(1) + 'K';
  }
  return num.toFixed(1);
}

type ChartView = 'requests' | 'tokens';

export function StatsPage() {
  const { t } = useTranslation();
  const [timeRange, setTimeRange] = useState<TimeRange>('24h');
  const [providerId, setProviderId] = useState<string>('all');
  const [projectId, setProjectId] = useState<string>('all');
  const [clientType, setClientType] = useState<string>('all');
  const [apiTokenId, setApiTokenId] = useState<string>('all');
  const [model, setModel] = useState<string>('all');
  const [chartView, setChartView] = useState<ChartView>('requests');
  const [costsProgress, setCostsProgress] = useState<RecalculateCostsProgress | null>(null);
  const [statsProgress, setStatsProgress] = useState<RecalculateStatsProgress | null>(null);

  // Reset all filters to 'all'
  const handleResetFilters = () => {
    setProviderId('all');
    setProjectId('all');
    setClientType('all');
    setApiTokenId('all');
    setModel('all');
  };

  // Subscribe to cost recalculation progress updates via WebSocket
  useEffect(() => {
    const transport = getTransport();
    const unsubscribe = transport.subscribe<RecalculateCostsProgress>(
      'recalculate_costs_progress',
      (data) => {
        setCostsProgress(data);
        // Clear progress after completion (with a delay to show final message)
        if (data.phase === 'completed') {
          setTimeout(() => setCostsProgress(null), 3000);
        }
      },
    );
    return unsubscribe;
  }, []);

  // Subscribe to stats recalculation progress updates via WebSocket
  useEffect(() => {
    const transport = getTransport();
    const unsubscribe = transport.subscribe<RecalculateStatsProgress>(
      'recalculate_stats_progress',
      (data) => {
        setStatsProgress(data);
        // Clear progress after completion (with a delay to show final message)
        if (data.phase === 'completed') {
          setTimeout(() => setStatsProgress(null), 3000);
        }
      },
    );
    return unsubscribe;
  }, []);

  const { data: providers } = useProviders();
  const { data: projects } = useProjects();
  const { data: apiTokens } = useAPITokens();
  const { data: responseModels } = useResponseModels();

  const timeConfig = useMemo(() => getTimeRangeConfig(timeRange), [timeRange]);

  const filter = useMemo<UsageStatsFilter>(() => {
    const f: UsageStatsFilter = {
      granularity: timeConfig.granularity,
      end: timeConfig.end.toISOString(),
    };
    if (timeConfig.start) {
      f.start = timeConfig.start.toISOString();
    }
    if (providerId !== 'all') f.providerId = Number(providerId);
    if (projectId !== 'all') f.projectId = Number(projectId);
    if (clientType !== 'all') f.clientType = clientType;
    if (apiTokenId !== 'all') f.apiTokenId = Number(apiTokenId);
    if (model !== 'all') f.model = model;
    return f;
  }, [timeConfig, providerId, projectId, clientType, apiTokenId, model]);

  const { data: stats, isLoading } = useUsageStats(filter);
  const chartData = useMemo(
    () => aggregateForChart(stats, timeConfig.granularity, timeRange, timeConfig),
    [stats, timeConfig, timeRange],
  );
  const recalculateStatsMutation = useRecalculateUsageStats();
  const recalculateCostsMutation = useRecalculateCosts();

  // 计算汇总数据和 RPM/TPM
  const summary = useMemo(() => {
    if (!stats || stats.length === 0) {
      return {
        totalRequests: 0,
        successfulRequests: 0,
        failedRequests: 0,
        totalTokens: 0,
        totalCacheRead: 0,
        totalCacheWrite: 0,
        cacheHitRate: 0,
        totalCost: 0,
        avgRpm: 0,
        avgTpm: 0,
        avgTtft: 0,
      };
    }

    const totals = stats.reduce(
      (acc, s) => ({
        totalRequests: acc.totalRequests + s.totalRequests,
        successfulRequests: acc.successfulRequests + s.successfulRequests,
        failedRequests: acc.failedRequests + s.failedRequests,
        // Total tokens = input + output + cache read + cache write
        totalTokens: acc.totalTokens + s.inputTokens + s.outputTokens + s.cacheRead + s.cacheWrite,
        totalInputTokens: acc.totalInputTokens + s.inputTokens,
        totalCacheRead: acc.totalCacheRead + s.cacheRead,
        totalCacheWrite: acc.totalCacheWrite + s.cacheWrite,
        totalCost: acc.totalCost + s.cost,
        totalDurationMs: acc.totalDurationMs + s.totalDurationMs,
        totalTtftMs: acc.totalTtftMs + (s.totalTtftMs || 0),
      }),
      {
        totalRequests: 0,
        successfulRequests: 0,
        failedRequests: 0,
        totalTokens: 0,
        totalInputTokens: 0,
        totalCacheRead: 0,
        totalCacheWrite: 0,
        totalCost: 0,
        totalDurationMs: 0,
        totalTtftMs: 0,
      },
    );

    // 计算缓存命中率 = cacheRead / (inputTokens + cacheRead)
    // 即：从缓存读取的 token 占实际输入 token 的比例
    const totalInputWithCache = totals.totalInputTokens + totals.totalCacheRead;
    const cacheHitRate = totalInputWithCache > 0
      ? (totals.totalCacheRead / totalInputWithCache) * 100
      : 0;

    // 计算平均 TTFT (毫秒转秒)
    const avgTtft = totals.successfulRequests > 0
      ? totals.totalTtftMs / totals.successfulRequests / 1000
      : 0;

    // 基于 totalDurationMs 计算 RPM 和 TPM
    // RPM = (totalRequests / totalDurationMs) * 60000
    // TPM = (totalTokens / totalDurationMs) * 60000
    const avgRpm =
      totals.totalDurationMs > 0 ? (totals.totalRequests / totals.totalDurationMs) * 60000 : 0;
    const avgTpm =
      totals.totalDurationMs > 0 ? (totals.totalTokens / totals.totalDurationMs) * 60000 : 0;

    return {
      ...totals,
      cacheHitRate,
      avgRpm,
      avgTpm,
      avgTtft,
    };
  }, [stats]);

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={BarChart3}
        iconClassName="text-emerald-500"
        title={t('stats.title')}
        description={t('stats.description')}
        actions={
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => recalculateCostsMutation.mutate()}
              disabled={recalculateCostsMutation.isPending || recalculateStatsMutation.isPending || !!costsProgress || !!statsProgress}
            >
              <Calculator
                className={`h-4 w-4 mr-2 ${recalculateCostsMutation.isPending || costsProgress ? 'animate-spin' : ''}`}
              />
              {t('stats.recalculateCosts')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => recalculateStatsMutation.mutate()}
              disabled={recalculateStatsMutation.isPending || recalculateCostsMutation.isPending || !!costsProgress || !!statsProgress}
            >
              <RefreshCw
                className={`h-4 w-4 mr-2 ${recalculateStatsMutation.isPending || statsProgress ? 'animate-spin' : ''}`}
              />
              {t('stats.recalculateStats')}
            </Button>
          </div>
        }
      />

      {/* Cost recalculation progress bar */}
      {costsProgress && (
        <div className="px-6 pt-4">
          <div className="bg-muted/50 rounded-lg p-4 space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">{costsProgress.message}</span>
              {costsProgress.phase !== 'completed' && costsProgress.total > 0 && (
                <span className="text-muted-foreground">
                  {costsProgress.percentage}%
                </span>
              )}
            </div>
            <Progress
              value={costsProgress.phase === 'completed' ? 100 : costsProgress.percentage}
              className="h-2"
            />
          </div>
        </div>
      )}

      {/* Stats recalculation progress bar */}
      {statsProgress && (
        <div className="px-6 pt-4">
          <div className="bg-muted/50 rounded-lg p-4 space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">{statsProgress.message}</span>
              {statsProgress.phase !== 'completed' && statsProgress.total > 0 && (
                <span className="text-muted-foreground">
                  {statsProgress.percentage}%
                </span>
              )}
            </div>
            <Progress
              value={statsProgress.phase === 'completed' ? 100 : statsProgress.percentage}
              className="h-2"
            />
          </div>
        </div>
      )}

      <div className="flex-1 flex overflow-hidden">
        {/* 左侧筛选栏 */}
        <div className="w-72 border-r border-border/40 flex-shrink-0">
          <div className="h-full overflow-y-auto p-4 space-y-6">
            {/* 标题 */}
            <div className="flex items-center pb-2 border-b border-border/40">
              <span className="text-xs font-semibold text-foreground uppercase tracking-wider flex items-center gap-2">
                <PanelLeft className="h-3.5 w-3.5" />
                {t('stats.filterConditions')}
              </span>
            </div>

            {/* 时间范围 */}
            <FilterSection
              label={t('stats.timeRange')}
              showClear={timeRange !== 'all'}
              onClear={() => setTimeRange('all')}
            >
              {[
                { value: 'today', label: t('stats.today') },
                { value: 'yesterday', label: t('stats.yesterday') },
                { value: 'thisWeek', label: t('stats.thisWeek') },
                { value: 'lastWeek', label: t('stats.lastWeek') },
                { value: 'thisMonth', label: t('stats.thisMonth') },
                { value: 'lastMonth', label: t('stats.lastMonth') },
                { value: '1h', label: t('stats.last1h') },
                { value: '24h', label: t('stats.last24h') },
                { value: '7d', label: t('stats.last7d') },
                { value: '30d', label: t('stats.last30d') },
                { value: '90d', label: t('stats.last90d') },
              ].map((item) => (
                <FilterChip
                  key={item.value}
                  selected={timeRange === item.value}
                  onClick={() => setTimeRange(item.value as TimeRange)}
                >
                  {item.label}
                </FilterChip>
              ))}
            </FilterSection>

            {/* Provider - 按类型分组，按名称排序 */}
            {providers && providers.length > 0 && (
              <FilterSection
                label={t('stats.provider')}
                showClear={providerId !== 'all'}
                onClear={() => setProviderId('all')}
              >
                {(() => {
                  // 按类型分组
                  const grouped = providers.reduce(
                    (acc, p) => {
                      const type = p.type || 'other';
                      if (!acc[type]) acc[type] = [];
                      acc[type].push(p);
                      return acc;
                    },
                    {} as Record<string, typeof providers>,
                  );
                  // 类型排序优先级
                  const typeOrder = ['antigravity', 'kiro', 'codex', 'custom', 'other'];
                  const sortedTypes = Object.keys(grouped).sort((a, b) => {
                    const aIndex = typeOrder.indexOf(a);
                    const bIndex = typeOrder.indexOf(b);
                    if (aIndex === -1 && bIndex === -1) return a.localeCompare(b);
                    if (aIndex === -1) return 1;
                    if (bIndex === -1) return -1;
                    return aIndex - bIndex;
                  });
                  return sortedTypes.map((type) => (
                    <div key={type} className="w-full">
                      <div className="text-[10px] font-medium text-muted-foreground/60 uppercase tracking-wide mb-1.5">
                        {type}
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {grouped[type]
                          .sort((a, b) => a.name.localeCompare(b.name))
                          .map((p) => (
                            <FilterChip
                              key={p.id}
                              selected={providerId === String(p.id)}
                              onClick={() => setProviderId(String(p.id))}
                            >
                              {p.name}
                            </FilterChip>
                          ))}
                      </div>
                    </div>
                  ));
                })()}
              </FilterSection>
            )}

            {/* Project */}
            {projects && projects.length > 0 && (
              <FilterSection
                label={t('stats.project')}
                showClear={projectId !== 'all'}
                onClear={() => setProjectId('all')}
              >
                {projects.map((p) => (
                  <FilterChip
                    key={p.id}
                    selected={projectId === String(p.id)}
                    onClick={() => setProjectId(String(p.id))}
                  >
                    {p.name}
                  </FilterChip>
                ))}
              </FilterSection>
            )}

            {/* Client Type */}
            <FilterSection
              label={t('stats.clientType')}
              showClear={clientType !== 'all'}
              onClear={() => setClientType('all')}
            >
              {[
                { value: 'claude', label: 'Claude' },
                { value: 'openai', label: 'OpenAI' },
                { value: 'codex', label: 'Codex' },
                { value: 'gemini', label: 'Gemini' },
              ].map((item) => (
                <FilterChip
                  key={item.value}
                  selected={clientType === item.value}
                  onClick={() => setClientType(item.value)}
                >
                  {item.label}
                </FilterChip>
              ))}
            </FilterSection>

            {/* API Token */}
            {apiTokens && apiTokens.length > 0 && (
              <FilterSection
                label={t('stats.apiToken')}
                showClear={apiTokenId !== 'all'}
                onClear={() => setApiTokenId('all')}
              >
                {apiTokens.map((token) => (
                  <FilterChip
                    key={token.id}
                    selected={apiTokenId === String(token.id)}
                    onClick={() => setApiTokenId(String(token.id))}
                  >
                    {token.name}
                  </FilterChip>
                ))}
              </FilterSection>
            )}

            {/* Model */}
            {responseModels && responseModels.length > 0 && (
              <FilterSection
                label={t('stats.model')}
                showClear={model !== 'all'}
                onClear={() => setModel('all')}
              >
                {responseModels.map((m) => (
                  <FilterChip
                    key={m}
                    selected={model === m}
                    onClick={() => setModel(m)}
                    title={m}
                  >
                    {m}
                  </FilterChip>
                ))}
              </FilterSection>
            )}

            {/* 重置按钮 */}
            <Button
              variant="outline"
              className="w-full text-xs h-8"
              onClick={handleResetFilters}
            >
              <RefreshCw className="h-3 w-3 mr-2" />
              {t('common.reset')}
            </Button>
          </div>
        </div>

        {/* 右侧内容区 */}
        <div className="flex-1 flex flex-col overflow-hidden p-4 md:p-6">
          <div className="max-w-7xl mx-auto w-full flex flex-col gap-6 flex-1 min-h-0">
            {/* 当前筛选条件摘要 */}
            <div className="flex items-center gap-2 text-sm text-muted-foreground flex-wrap">
              <span className="font-medium text-foreground">{t('stats.filterSummary')}:</span>
              <span className="bg-muted/50 px-2 py-0.5 rounded text-xs">
                {timeConfig.start
                  ? `${timeConfig.start.toLocaleString()} - ${timeConfig.end.toLocaleString()}`
                  : t('stats.allTime')}
              </span>
              {providerId !== 'all' && (
                <span className="bg-muted/50 px-2 py-0.5 rounded text-xs">
                  {t('stats.provider')}: {providers?.find((p) => String(p.id) === providerId)?.name || providerId}
                </span>
              )}
              {projectId !== 'all' && (
                <span className="bg-muted/50 px-2 py-0.5 rounded text-xs">
                  {t('stats.project')}: {projects?.find((p) => String(p.id) === projectId)?.name || projectId}
                </span>
              )}
              {clientType !== 'all' && (
                <span className="bg-muted/50 px-2 py-0.5 rounded text-xs">
                  {t('stats.clientType')}: {clientType}
                </span>
              )}
              {apiTokenId !== 'all' && (
                <span className="bg-muted/50 px-2 py-0.5 rounded text-xs">
                  {t('stats.apiToken')}: {apiTokens?.find((t) => String(t.id) === apiTokenId)?.name || apiTokenId}
                </span>
              )}
              {model !== 'all' && (
                <span className="bg-muted/50 px-2 py-0.5 rounded text-xs">
                  {t('stats.model')}: {model}
                </span>
              )}
            </div>

            {/* 汇总卡片 - 与 Dashboard 一致的排列顺序 */}
            <div className="grid gap-4 grid-cols-2 lg:grid-cols-4">
              <StatCard
                title={t('stats.requests')}
                value={summary.totalRequests.toLocaleString()}
                subtitle={`${formatNumber(summary.avgRpm)} RPM · ${summary.avgTtft.toFixed(2)}s TTFT`}
                icon={Activity}
                iconClassName="text-blue-600 dark:text-blue-400"
              />
              <StatCard
                title={t('stats.tokens')}
                value={formatNumber(summary.totalTokens)}
                subtitle={`${formatNumber(summary.avgTpm)} TPM · ${summary.cacheHitRate.toFixed(1)}% ${t('stats.cacheHit')}`}
                icon={Cpu}
                iconClassName="text-violet-600 dark:text-violet-400"
              />
              <StatCard
                title={t('stats.totalCost')}
                value={`$${(summary.totalCost / 1_000_000_000).toFixed(4)}`}
                icon={Coins}
                iconClassName="text-amber-600 dark:text-amber-400"
              />
              <StatCard
                title={t('stats.successRate')}
                value={`${summary.totalRequests > 0 ? ((summary.successfulRequests / summary.totalRequests) * 100).toFixed(1) : 0}%`}
                icon={CheckCircle}
                iconClassName={cn(
                  (summary.successfulRequests / summary.totalRequests) >= 0.95
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : (summary.successfulRequests / summary.totalRequests) >= 0.8
                      ? 'text-amber-600 dark:text-amber-400'
                      : 'text-red-600 dark:text-red-400'
                )}
              />
            </div>

            {isLoading ? (
              <div className="text-center text-muted-foreground py-8">{t('common.loading')}</div>
            ) : chartData.length === 0 ? (
              <div className="text-center text-muted-foreground py-8">{t('common.noData')}</div>
            ) : (
              <Card className="flex flex-col flex-1 min-h-0 border-border/50 bg-card/50 backdrop-blur-sm">
                <CardHeader className="flex flex-row items-center justify-between shrink-0 pb-2">
                  <CardTitle className="text-base font-semibold flex items-center gap-2">
                    <BarChart3 className="h-4 w-4 text-emerald-500" />
                    {t('stats.chart')}
                  </CardTitle>
                  <Tabs value={chartView} onValueChange={(v) => setChartView(v as ChartView)}>
                    <TabsList>
                      <TabsTrigger value="requests">{t('stats.requests')}</TabsTrigger>
                      <TabsTrigger value="tokens">{t('stats.tokens')}</TabsTrigger>
                    </TabsList>
                  </Tabs>
                </CardHeader>
                                <CardContent className="flex-1 min-h-0">
                                  <div className="w-full h-full" style={{ minHeight: '300px' }}>
                                    <ResponsiveContainer width="100%" height="100%">
                                      <ComposedChart data={chartData}>
                                        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                                        <XAxis
                                          dataKey="label"
                                          tick={{ fontSize: 11 }}
                                          tickLine={false}
                                          axisLine={false}
                                          interval="preserveStartEnd"
                                        />
                                        <YAxis
                                          yAxisId="left"
                                          tick={{ fontSize: 11 }}
                                          tickLine={false}
                                          axisLine={false}
                                          tickFormatter={(v) => formatNumber(v)}
                                        />
                                        <YAxis
                                          yAxisId="right"
                                          orientation="right"
                                          tick={{ fontSize: 11 }}
                                          tickLine={false}
                                          axisLine={false}
                                          tickFormatter={(v) => `${v.toFixed(2)}`}
                                        />
                                        <Tooltip
                                          contentStyle={{
                                            backgroundColor: 'var(--card)',
                                            border: '1px solid var(--border)',
                                            borderRadius: '8px',
                                            fontSize: '12px',
                                            boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1)',
                                          }}
                                          itemSorter={(a) => (a.name === t('stats.costUSD') ? -1 : 0)}
                                          formatter={(value, name) => {
                                            const numValue = typeof value === 'number' ? value : 0;
                                            const nameStr = name ?? '';
                                            if (nameStr === t('stats.costUSD'))
                                              return [`$${numValue.toFixed(4)}`, nameStr];
                                            return [numValue.toLocaleString(), nameStr];
                                          }}
                                        />
                                        <Legend
                                          wrapperStyle={{ fontSize: '12px', paddingTop: '8px' }}
                                          itemSorter={(a) => (a.value === t('stats.costUSD') ? -1 : 0)}
                                        />
                                        {chartView === 'requests' && (
                                          <>
                                            <Line
                                              yAxisId="right"
                                              type="monotone"
                                              dataKey="cost"
                                              name={t('stats.costUSD')}
                                              stroke="var(--color-chart-3)"
                                              strokeWidth={2}
                                              dot={false}
                                            />
                                            <Bar
                                              yAxisId="left"
                                              dataKey="successful"
                                              name={t('stats.successful')}
                                              stackId="a"
                                              fill="var(--color-chart-2)"
                                              radius={[0, 0, 0, 0]}
                                            />
                                            <Bar
                                              yAxisId="left"
                                              dataKey="failed"
                                              name={t('stats.failed')}
                                              stackId="a"
                                              fill="var(--color-destructive)"
                                              radius={[4, 4, 0, 0]}
                                            />
                                          </>
                                        )}
                                        {chartView === 'tokens' && (
                                          <>
                                            <Line
                                              yAxisId="right"
                                              type="monotone"
                                              dataKey="cost"
                                              name={t('stats.costUSD')}
                                              stroke="var(--color-chart-3)"
                                              strokeWidth={2}
                                              dot={false}
                                            />
                                            <Bar
                                              yAxisId="left"
                                              dataKey="inputTokens"
                                              name={t('stats.inputTokens')}
                                              stackId="a"
                                              fill="var(--color-chart-1)"
                                              radius={[0, 0, 0, 0]}
                                            />
                                            <Bar
                                              yAxisId="left"
                                              dataKey="outputTokens"
                                              name={t('stats.outputTokens')}
                                              stackId="a"
                                              fill="var(--color-chart-2)"
                                              radius={[0, 0, 0, 0]}
                                            />
                                            <Bar
                                              yAxisId="left"
                                              dataKey="cacheRead"
                                              name={t('stats.cacheRead')}
                                              stackId="a"
                                              fill="var(--color-chart-4)"
                                              radius={[0, 0, 0, 0]}
                                            />
                                            <Bar
                                              yAxisId="left"
                                              dataKey="cacheWrite"
                                              name={t('stats.cacheWrite')}
                                              stackId="a"
                                              fill="var(--color-chart-5)"
                                              radius={[4, 4, 0, 0]}
                                            />
                                          </>
                                        )}
                                      </ComposedChart>                    </ResponsiveContainer>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function StatCard({
  title,
  value,
  subtitle,
  icon: Icon,
  iconClassName,
}: {
  title: string;
  value: string;
  subtitle?: string;
  icon: React.ElementType;
  iconClassName?: string;
}) {
  return (
    <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
      <CardContent className="p-4">
        <div className="flex items-start justify-between">
          <div className="space-y-1">
            <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
              {title}
            </p>
            <p className="text-2xl font-bold text-foreground font-mono tracking-tight">
              {value}
            </p>
            {subtitle && (
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">{subtitle}</span>
              </div>
            )}
          </div>
          <div
            className={cn(
              'w-10 h-10 rounded-xl bg-muted flex items-center justify-center box-border border-2 border-transparent transition-shadow duration-300',
              iconClassName
            )}
          >
            <Icon className="h-5 w-5" />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function FilterSection({
  label,
  children,
  onClear,
  showClear,
}: {
  label: string;
  children: React.ReactNode;
  onClear?: () => void;
  showClear?: boolean;
}) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <label className="text-xs font-bold text-muted-foreground uppercase tracking-widest pl-1 opacity-80">
          {label}
        </label>
        {onClear && (
          <button
            type="button"
            onClick={onClear}
            className={cn(
              "p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors",
              showClear ? "opacity-100" : "opacity-0 pointer-events-none"
            )}
            title="Clear"
          >
            <X className="h-3 w-3" />
          </button>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        {children}
      </div>
    </div>
  );
}

function FilterChip({
  selected,
  onClick,
  children,
  title,
}: {
  selected: boolean;
  onClick: () => void;
  children: React.ReactNode;
  title?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      className={cn(
        "h-8 px-3 text-sm rounded-full transition-all truncate max-w-full border flex items-center",
        selected
          ? "bg-primary text-primary-foreground border-primary hover:bg-primary/90"
          : "bg-background text-foreground border-input hover:bg-accent hover:text-accent-foreground"
      )}
    >
      {children}
    </button>
  );
}
