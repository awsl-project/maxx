import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BarChart3, RefreshCw } from 'lucide-react';
import { PageHeader } from '@/components/layout/page-header';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Tabs,
  TabsList,
  TabsTrigger,
  Button,
} from '@/components/ui';
import { useUsageStats, useProviders, useProjects, useAPITokens, useRecalculateUsageStats, useResponseModels } from '@/hooks/queries';
import type { UsageStatsFilter, UsageStats, StatsGranularity } from '@/lib/transport';
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

type TimeRange = '1h' | '24h' | '7d' | '30d' | '90d' | 'all';

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
  let granularity: StatsGranularity;
  let durationMinutes: number;

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
    case 'all':
      // 最近 12 个月
      start = new Date(now.getFullYear(), now.getMonth() - 11, 1); // 12个月前的月初
      granularity = 'month';
      durationMinutes = 365 * 24 * 60; // 约一年
      break;
  }

  return { start, end: now, granularity, durationMinutes };
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
  timeConfig: TimeRangeConfig
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
        // 转换 cost 从微美元到美元
        cost: item.cost / 1000000,
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
    [stats, timeConfig, timeRange]
  );
  const recalculateMutation = useRecalculateUsageStats();

  // 计算汇总数据和 RPM/TPM
  const summary = useMemo(() => {
    if (!stats || stats.length === 0) {
      return {
        totalRequests: 0,
        successfulRequests: 0,
        failedRequests: 0,
        totalTokens: 0,
        totalCost: 0,
        avgRpm: 0,
        avgTpm: 0,
      };
    }

    const totals = stats.reduce(
      (acc, s) => ({
        totalRequests: acc.totalRequests + s.totalRequests,
        successfulRequests: acc.successfulRequests + s.successfulRequests,
        failedRequests: acc.failedRequests + s.failedRequests,
        totalTokens: acc.totalTokens + s.inputTokens + s.outputTokens,
        totalCost: acc.totalCost + s.cost,
        totalDurationMs: acc.totalDurationMs + s.totalDurationMs,
      }),
      { totalRequests: 0, successfulRequests: 0, failedRequests: 0, totalTokens: 0, totalCost: 0, totalDurationMs: 0 }
    );

    // 基于 totalDurationMs 计算 RPM 和 TPM
    // RPM = (totalRequests / totalDurationMs) * 60000
    // TPM = (totalTokens / totalDurationMs) * 60000
    const avgRpm = totals.totalDurationMs > 0 ? (totals.totalRequests / totals.totalDurationMs) * 60000 : 0;
    const avgTpm = totals.totalDurationMs > 0 ? (totals.totalTokens / totals.totalDurationMs) * 60000 : 0;

    return {
      ...totals,
      avgRpm,
      avgTpm,
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
          <Button
            variant="outline"
            size="sm"
            onClick={() => recalculateMutation.mutate()}
            disabled={recalculateMutation.isPending}
          >
            <RefreshCw className={`h-4 w-4 mr-2 ${recalculateMutation.isPending ? 'animate-spin' : ''}`} />
            {t('stats.recalculate')}
          </Button>
        }
      />

      <div className="flex-1 overflow-auto p-6 flex flex-col gap-6">
        {/* 过滤器 */}
        <div className="flex flex-wrap items-center gap-4">
          <FilterSelect
            label={t('stats.timeRange')}
            value={timeRange}
            onChange={(v) => setTimeRange(v as TimeRange)}
            options={[
              { value: '1h', label: t('stats.last1h') },
              { value: '24h', label: t('stats.last24h') },
              { value: '7d', label: t('stats.last7d') },
              { value: '30d', label: t('stats.last30d') },
              { value: '90d', label: t('stats.last90d') },
              { value: 'all', label: t('stats.allTime') },
            ]}
          />
          <FilterSelect
            label={t('stats.provider')}
            value={providerId}
            onChange={setProviderId}
            options={[
              { value: 'all', label: t('stats.allProviders') },
              ...(providers?.map((p) => ({
                value: String(p.id),
                label: p.name,
              })) || []),
            ]}
          />
          <FilterSelect
            label={t('stats.project')}
            value={projectId}
            onChange={setProjectId}
            options={[
              { value: 'all', label: t('stats.allProjects') },
              ...(projects?.map((p) => ({
                value: String(p.id),
                label: p.name,
              })) || []),
            ]}
          />
          <FilterSelect
            label={t('stats.clientType')}
            value={clientType}
            onChange={setClientType}
            options={[
              { value: 'all', label: t('stats.allClients') },
              { value: 'claude', label: 'Claude' },
              { value: 'openai', label: 'OpenAI' },
              { value: 'codex', label: 'Codex' },
              { value: 'gemini', label: 'Gemini' },
            ]}
          />
          <FilterSelect
            label={t('stats.apiToken')}
            value={apiTokenId}
            onChange={setApiTokenId}
            options={[
              { value: 'all', label: t('stats.allTokens') },
              ...(apiTokens?.map((t) => ({
                value: String(t.id),
                label: t.name,
              })) || []),
            ]}
          />
          <FilterSelect
            label={t('stats.model')}
            value={model}
            onChange={setModel}
            options={[
              { value: 'all', label: t('stats.allModels') },
              ...(responseModels?.map((m) => ({ value: m, label: m })) || []),
            ]}
          />
        </div>

        {/* 汇总卡片 */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <SummaryCard
            title={t('stats.requests')}
            value={summary.totalRequests.toLocaleString()}
            subtitle={`${formatNumber(summary.avgRpm)} RPM`}
          />
          <SummaryCard
            title={t('stats.tokens')}
            value={summary.totalTokens.toLocaleString()}
            subtitle={`${formatNumber(summary.avgTpm)} TPM`}
          />
          <SummaryCard
            title={t('stats.successRate')}
            value={`${summary.totalRequests > 0 ? ((summary.successfulRequests / summary.totalRequests) * 100).toFixed(1) : 0}%`}
            className={summary.totalRequests > 0 && summary.successfulRequests / summary.totalRequests >= 0.95 ? 'text-green-600' : summary.totalRequests > 0 && summary.successfulRequests / summary.totalRequests < 0.8 ? 'text-red-600' : 'text-yellow-600'}
          />
          <SummaryCard
            title={t('stats.totalCost')}
            value={`$${(summary.totalCost / 1000000).toFixed(4)}`}
          />
        </div>

        {isLoading ? (
          <div className="text-center text-muted-foreground py-8">{t('common.loading')}</div>
        ) : chartData.length === 0 ? (
          <div className="text-center text-muted-foreground py-8">{t('common.noData')}</div>
        ) : (
          <Card className="flex flex-col flex-1 min-h-0">
            <CardHeader className="flex flex-row items-center justify-between flex-shrink-0">
              <CardTitle>{t('stats.chart')}</CardTitle>
              <Tabs value={chartView} onValueChange={(v) => setChartView(v as ChartView)}>
                <TabsList>
                  <TabsTrigger value="requests">{t('stats.requests')}</TabsTrigger>
                  <TabsTrigger value="tokens">{t('stats.tokens')}</TabsTrigger>
                </TabsList>
              </Tabs>
            </CardHeader>
            <CardContent className="flex-1 min-h-0 overflow-x-auto">
              <div
                style={{
                  minWidth: `${Math.max(chartData.length * 60, 600)}px`,
                  height: '100%',
                }}
              >
                <ResponsiveContainer width="100%" height="100%">
                  <ComposedChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                    <XAxis dataKey="label" className="text-xs" />
                    <YAxis yAxisId="left" className="text-xs" />
                    <YAxis yAxisId="right" orientation="right" className="text-xs" tickFormatter={(v) => `$${v.toFixed(2)}`} />
                    <Tooltip
                      formatter={(value, name) => {
                        const numValue = typeof value === 'number' ? value : 0;
                        const nameStr = name ?? '';
                        if (nameStr === t('stats.costUSD')) return [`$${numValue.toFixed(4)}`, nameStr];
                        return [numValue.toLocaleString(), nameStr];
                      }}
                    />
                    <Legend />
                    {chartView === 'requests' && (
                      <>
                        <Bar yAxisId="left" dataKey="successful" name={t('stats.successful')} stackId="a" fill="#22c55e" />
                        <Bar yAxisId="left" dataKey="failed" name={t('stats.failed')} stackId="a" fill="#ef4444" />
                        <Line yAxisId="right" type="monotone" dataKey="cost" name={t('stats.costUSD')} stroke="#f59e0b" strokeWidth={2} dot={false} />
                      </>
                    )}
                    {chartView === 'tokens' && (
                      <>
                        <Bar yAxisId="left" dataKey="inputTokens" name={t('stats.inputTokens')} stackId="a" fill="#3b82f6" />
                        <Bar yAxisId="left" dataKey="outputTokens" name={t('stats.outputTokens')} stackId="a" fill="#8b5cf6" />
                        <Bar yAxisId="left" dataKey="cacheRead" name={t('stats.cacheRead')} stackId="a" fill="#22c55e" />
                        <Bar yAxisId="left" dataKey="cacheWrite" name={t('stats.cacheWrite')} stackId="a" fill="#f59e0b" />
                        <Line yAxisId="right" type="monotone" dataKey="cost" name={t('stats.costUSD')} stroke="#ef4444" strokeWidth={2} dot={false} />
                      </>
                    )}
                  </ComposedChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}

function FilterSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
}) {
  const selectedLabel = options.find((opt) => opt.value === value)?.label;
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-xs text-muted-foreground">{label}</label>
      <Select value={value} onValueChange={(v) => v && onChange(v)}>
        <SelectTrigger className="w-40">
          <SelectValue>{selectedLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          {options.map((opt) => (
            <SelectItem key={opt.value} value={opt.value}>
              {opt.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function SummaryCard({
  title,
  value,
  subtitle,
  className,
}: {
  title: string;
  value: string;
  subtitle?: string;
  className?: string;
}) {
  return (
    <Card>
      <CardContent className="pt-6">
        <div className="text-sm text-muted-foreground">{title}</div>
        <div className={`text-2xl font-bold ${className || ''}`}>
          {value}
          {subtitle && <span className="text-xs font-normal text-muted-foreground ml-1">{subtitle}</span>}
        </div>
      </CardContent>
    </Card>
  );
}
