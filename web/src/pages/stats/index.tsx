import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BarChart3 } from 'lucide-react';
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
  const now = new Date();
  const end = now.toISOString();
  let start: Date;

  switch (range) {
    case '24h':
      start = new Date(now.getTime() - 24 * 60 * 60 * 1000);
      break;
    case '7d':
      start = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
      break;
    case '30d':
      start = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
      break;
  }

  return { start: start.toISOString(), end };
}

// 按小时聚合数据用于图表
function aggregateByHour(stats: UsageStats[] | undefined, timeRange: TimeRange) {
  if (!stats || stats.length === 0) return [];

  const hourMap = new Map<string, {
    hour: string;
    successful: number;
    failed: number;
    inputTokens: number;
    outputTokens: number;
    cacheRead: number;
    cacheWrite: number;
    cost: number;
  }>();

  stats.forEach((s) => {
    const hourKey = s.hour.slice(0, 13); // YYYY-MM-DDTHH
    const existing = hourMap.get(hourKey) || {
      hour: hourKey,
      successful: 0,
      failed: 0,
      inputTokens: 0,
      outputTokens: 0,
      cacheRead: 0,
      cacheWrite: 0,
      cost: 0,
    };
    existing.successful += s.successfulRequests;
    existing.failed += s.failedRequests;
    existing.inputTokens += s.inputTokens;
    existing.outputTokens += s.outputTokens;
    existing.cacheRead += s.cacheRead;
    existing.cacheWrite += s.cacheWrite;
    existing.cost += s.cost;
    hourMap.set(hourKey, existing);
  });

  // 排序并格式化
  return Array.from(hourMap.values())
    .sort((a, b) => a.hour.localeCompare(b.hour))
    .map((item) => ({
      ...item,
      hour: formatHourLabel(item.hour, timeRange),
      // 转换 cost 从微美元到美元
      cost: item.cost / 1000000,
    }));
}

function formatHourLabel(hour: string, timeRange: TimeRange): string {
  // hour 格式是 YYYY-MM-DDTHH，需要补全为有效的 ISO 格式
  const date = new Date(hour + ':00:00');
  if (timeRange === '24h') {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  // 7d/30d 显示日期+小时
  return date.toLocaleDateString([], { month: 'short', day: 'numeric', hour: '2-digit' });
}

type ChartView = 'requests' | 'tokens' | 'cost';

export function StatsPage() {
  const { t } = useTranslation();
  const [timeRange, setTimeRange] = useState<TimeRange>('7d');
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

      <div className="flex-1 overflow-auto p-6 space-y-6">
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
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle>{t('stats.chart')}</CardTitle>
              <Tabs value={chartView} onValueChange={(v) => setChartView(v as ChartView)}>
                <TabsList>
                  <TabsTrigger value="requests">{t('stats.requests')}</TabsTrigger>
                  <TabsTrigger value="tokens">{t('stats.tokens')}</TabsTrigger>
                  <TabsTrigger value="cost">{t('stats.cost')}</TabsTrigger>
                </TabsList>
              </Tabs>
            </CardHeader>
            <CardContent>
              <div className="h-80">
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                    <XAxis dataKey="hour" className="text-xs" />
                    <YAxis className="text-xs" />
                    <Tooltip />
                    <Legend />
                    {chartView === 'requests' && (
                      <>
                        <Bar dataKey="successful" name={t('stats.successful')} stackId="a" fill="#22c55e" />
                        <Bar dataKey="failed" name={t('stats.failed')} stackId="a" fill="#ef4444" />
                      </>
                    )}
                    {chartView === 'tokens' && (
                      <>
                        <Bar dataKey="inputTokens" name={t('stats.inputTokens')} stackId="a" fill="#3b82f6" />
                        <Bar dataKey="outputTokens" name={t('stats.outputTokens')} stackId="a" fill="#8b5cf6" />
                        <Bar dataKey="cacheRead" name={t('stats.cacheRead')} stackId="a" fill="#22c55e" />
                        <Bar dataKey="cacheWrite" name={t('stats.cacheWrite')} stackId="a" fill="#f59e0b" />
                      </>
                    )}
                    {chartView === 'cost' && (
                      <Bar dataKey="cost" name={t('stats.costUSD')} fill="#10b981" />
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
