import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BarChart3 } from 'lucide-react';
import dayjs from 'dayjs';
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
} from '@/components/ui';
import { useUsageStats, useProviders, useProjects, useAPITokens } from '@/hooks/queries';
import type { UsageStatsFilter, UsageStats } from '@/lib/transport';
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';

type TimeRange = '24h' | '7d' | '30d';

function getTimeRange(range: TimeRange): { start: string; end: string } {
  const now = dayjs();
  const end = now.toISOString();
  let start: dayjs.Dayjs;

  switch (range) {
    case '24h':
      start = now.subtract(24, 'hour');
      break;
    case '7d':
      start = now.subtract(7, 'day');
      break;
    case '30d':
      start = now.subtract(30, 'day');
      break;
    default:
      start = now.subtract(7, 'day'); // 默认 7 天
      break;
  }

  return { start: start.toISOString(), end };
}

// 按时间范围聚合数据用于图表
function aggregateByHour(stats: UsageStats[] | undefined, timeRange: TimeRange) {
  // 生成完整的时间序列
  const now = dayjs();
  const isHourly = timeRange === '24h';
  const pointsToShow = timeRange === '24h' ? 24 : timeRange === '7d' ? 7 : 30;
  const timeMap = new Map<string, {
    hour: string;
    successful: number;
    failed: number;
    inputTokens: number;
    outputTokens: number;
    cacheRead: number;
    cacheWrite: number;
    cost: number;
  }>();

  // 生成时间键的辅助函数（使用本地时间）
  const getTimeKey = (date: dayjs.Dayjs, hourly: boolean): string => {
    if (hourly) {
      return date.format('YYYY-MM-DDTHH');
    } else {
      return date.format('YYYY-MM-DD');
    }
  };

  // 填充完整的时间序列
  for (let i = pointsToShow - 1; i >= 0; i--) {
    let date: dayjs.Dayjs;

    if (isHourly) {
      // 24h: 按小时
      date = now.subtract(i, 'hour');
    } else {
      // 7d/30d: 按天
      date = now.subtract(i, 'day');
    }

    const timeKey = getTimeKey(date, isHourly);
    timeMap.set(timeKey, {
      hour: timeKey,
      successful: 0,
      failed: 0,
      inputTokens: 0,
      outputTokens: 0,
      cacheRead: 0,
      cacheWrite: 0,
      cost: 0,
    });
  }

  // 填充实际数据
  if (stats && stats.length > 0) {
    stats.forEach((s) => {
      if (!s || !s.hour) return; // 跳过无效数据

      // 解析后端返回的时间（可能带时区）
      const date = dayjs(s.hour);
      if (!date.isValid()) return; // 跳过无效日期

      const timeKey = getTimeKey(date, isHourly);
      const existing = timeMap.get(timeKey);
      if (existing) {
        existing.successful += s.successfulRequests || 0;
        existing.failed += s.failedRequests || 0;
        existing.inputTokens += s.inputTokens || 0;
        existing.outputTokens += s.outputTokens || 0;
        existing.cacheRead += s.cacheRead || 0;
        existing.cacheWrite += s.cacheWrite || 0;
        existing.cost += s.cost || 0;
      }
    });
  }

  // 排序并格式化
  return Array.from(timeMap.values())
    .sort((a, b) => (a.hour || '').localeCompare(b.hour || ''))
    .map((item) => ({
      ...item,
      hour: formatHourLabel(item.hour, timeRange),
      // 转换 cost 从微美元到美元
      cost: (item.cost || 0) / 1000000,
    }));
}

function formatHourLabel(hour: string, timeRange: TimeRange): string {
  if (!hour) return '';

  if (timeRange === '24h') {
    // 24h: 显示时间 "14:00"
    const date = dayjs(hour + ':00:00');
    return date.isValid() ? date.format('HH:mm') : '';
  } else {
    // 7d/30d: 显示日期 "1/17"
    const date = dayjs(hour);
    return date.isValid() ? `${date.month() + 1}/${date.date()}` : '';
  }
}

// 智能单位格式化函数
function formatNumber(value: number): string {
  if (value === null || value === undefined || isNaN(value)) return '0';

  const absValue = Math.abs(value);

  if (absValue >= 1000000) {
    // >= 1M
    const formatted = value / 1000000;
    return absValue >= 10000000
      ? `${Math.round(formatted)}M`  // >= 10M: 不显示小数
      : `${formatted.toFixed(1)}M`;   // < 10M: 显示 1 位小数
  } else if (absValue >= 1000) {
    // >= 1K
    const formatted = value / 1000;
    return absValue >= 10000
      ? `${Math.round(formatted)}K`   // >= 10K: 不显示小数
      : `${formatted.toFixed(1)}K`;    // < 10K: 显示 1 位小数
  } else {
    // < 1K: 显示原数字
    return Math.round(value).toString();
  }
}

// Cost 格式化函数（添加 $ 符号）
function formatCost(value: number): string {
  if (value === null || value === undefined || isNaN(value)) return '$0';
  return `$${formatNumber(value)}`;
}

type ChartView = 'requests' | 'tokens' | 'cost';

export function StatsPage() {
  const { t } = useTranslation();
  const [timeRange, setTimeRange] = useState<TimeRange>('24h');
  const [providerId, setProviderId] = useState<string>('all');
  const [projectId, setProjectId] = useState<string>('all');
  const [clientType, setClientType] = useState<string>('all');
  const [apiTokenId, setApiTokenId] = useState<string>('all');
  const [chartView, setChartView] = useState<ChartView>('requests');

  const { data: providers } = useProviders();
  const { data: projects } = useProjects();
  const { data: apiTokens } = useAPITokens();

  const filter = useMemo<UsageStatsFilter>(() => {
    const { start, end } = getTimeRange(timeRange);
    const f: UsageStatsFilter = { start, end };
    if (providerId !== 'all') f.providerId = Number(providerId);
    if (projectId !== 'all') f.projectId = Number(projectId);
    if (clientType !== 'all') f.clientType = clientType;
    if (apiTokenId !== 'all') f.apiTokenID = Number(apiTokenId);
    return f;
  }, [timeRange, providerId, projectId, clientType, apiTokenId]);

  const { data: stats, isLoading } = useUsageStats(filter);
  const chartData = useMemo(() => aggregateByHour(stats, timeRange), [stats, timeRange]);

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={BarChart3}
        iconClassName="text-emerald-500"
        title={t('stats.title')}
        description={t('stats.description')}
      />

      <div className="flex-1 overflow-auto p-6 flex flex-col gap-6">
        {/* 过滤器 */}
        <div className="flex flex-wrap items-center gap-4">
          <FilterSelect
            label={t('stats.timeRange')}
            value={timeRange}
            onChange={(v) => setTimeRange(v as TimeRange)}
            options={[
              { value: '24h', label: t('stats.last24h') },
              { value: '7d', label: t('stats.last7d') },
              { value: '30d', label: t('stats.last30d') },
            ]}
          />
          <FilterSelect
            label={t('stats.provider')}
            value={providerId}
            onChange={setProviderId}
            options={[
              { value: 'all', label: t('stats.allProviders') },
              ...(providers?.map((p) => ({ value: String(p.id), label: p.name })) || []),
            ]}
          />
          <FilterSelect
            label={t('stats.project')}
            value={projectId}
            onChange={setProjectId}
            options={[
              { value: 'all', label: t('stats.allProjects') },
              ...(projects?.map((p) => ({ value: String(p.id), label: p.name })) || []),
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
              ...(apiTokens?.map((t) => ({ value: String(t.id), label: t.name })) || []),
            ]}
          />
        </div>

        {isLoading ? (
          <div className="text-center text-muted-foreground py-8">
            {t('common.loading')}
          </div>
        ) : chartData.length === 0 ? (
          <div className="text-center text-muted-foreground py-8">
            {t('common.noData')}
          </div>
        ) : (
          <Card className="flex flex-col flex-1 min-h-0">
            <CardHeader className="flex flex-row items-center justify-between flex-shrink-0">
              <CardTitle>{t('stats.chart')}</CardTitle>
              <Tabs value={chartView} onValueChange={(v) => setChartView(v as ChartView)}>
                <TabsList>
                  <TabsTrigger value="requests">{t('stats.requests')}</TabsTrigger>
                  <TabsTrigger value="tokens">{t('stats.tokens')}</TabsTrigger>
                  <TabsTrigger value="cost">{t('stats.cost')}</TabsTrigger>
                </TabsList>
              </Tabs>
            </CardHeader>
            <CardContent className="flex-1 min-h-0 overflow-x-auto">
              <div style={{ minWidth: `${Math.max(chartData.length * 60, 600)}px`, height: '100%' }}>
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                    <XAxis dataKey="hour" className="text-xs" />
                    <YAxis className="text-xs" tickFormatter={chartView === 'cost' ? formatCost : formatNumber} />
                    <Tooltip formatter={(value) => chartView === 'cost' ? formatCost(value as number) : formatNumber(value as number)} />
                    <Legend />
                    {chartView === 'requests' && (
                      <>
                        <Bar dataKey="successful" name={t('stats.successful')} stackId="a" fill="#22c55e" barSize={40} />
                        <Bar dataKey="failed" name={t('stats.failed')} stackId="a" fill="#ef4444" barSize={40} />
                      </>
                    )}
                    {chartView === 'tokens' && (
                      <>
                        <Bar dataKey="inputTokens" name={t('stats.inputTokens')} stackId="a" fill="#3b82f6" barSize={40} />
                        <Bar dataKey="outputTokens" name={t('stats.outputTokens')} stackId="a" fill="#8b5cf6" barSize={40} />
                        <Bar dataKey="cacheRead" name={t('stats.cacheRead')} stackId="a" fill="#22c55e" barSize={40} />
                        <Bar dataKey="cacheWrite" name={t('stats.cacheWrite')} stackId="a" fill="#f59e0b" barSize={40} />
                      </>
                    )}
                    {chartView === 'cost' && (
                      <Bar dataKey="cost" name={t('stats.costUSD')} fill="#10b981" barSize={40} />
                    )}
                  </BarChart>
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
