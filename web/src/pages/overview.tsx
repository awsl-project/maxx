import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import {
  LayoutDashboard,
  TrendingUp,
  TrendingDown,
  Minus,
  Activity,
  Coins,
  CheckCircle,
  XCircle,
  Clock,
  Zap,
  ArrowRight,
  Server,
  Calendar,
  Hash,
  Cpu,
  AlertTriangle,
} from 'lucide-react';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ActivityHeatmap,
} from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import {
  useDashboardSummary,
  useAllTimeStats,
  useActivityHeatmap,
  useTopModels,
  use24HourTrend,
  useFirstUseDate,
  useDashboardProviderStats,
  useProviders,
  useProxyRequests,
  useProxyRequestUpdates,
  useSessions,
} from '@/hooks/queries';
import { useCooldowns } from '@/hooks/use-cooldowns';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';
import { cn } from '@/lib/utils';

// 格式化数字（K, M, B）
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
  return num.toLocaleString();
}

// 格式化成本
function formatCost(microUsd: number): string {
  const usd = microUsd / 1000000;
  if (usd >= 1000) {
    return '$' + (usd / 1000).toFixed(1) + 'K';
  }
  if (usd >= 1) {
    return '$' + usd.toFixed(2);
  }
  return '$' + usd.toFixed(4);
}

// 格式化相对时间
function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);

  if (diffSec < 60) return `${diffSec}s ago`;
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHour < 24) return `${diffHour}h ago`;
  return date.toLocaleDateString();
}

// 变化趋势指示器
function TrendIndicator({ value, suffix = '%' }: { value: number; suffix?: string }) {
  if (Math.abs(value) < 0.1) {
    return (
      <span className="flex items-center gap-1 text-xs text-muted-foreground">
        <Minus className="h-3 w-3" />
        <span>0{suffix}</span>
      </span>
    );
  }

  const isPositive = value > 0;
  return (
    <span
      className={cn(
        'flex items-center gap-1 text-xs',
        isPositive ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'
      )}
    >
      {isPositive ? <TrendingUp className="h-3 w-3" /> : <TrendingDown className="h-3 w-3" />}
      <span>
        {isPositive ? '+' : ''}
        {value.toFixed(1)}
        {suffix}
      </span>
    </span>
  );
}

// 统计卡片组件
function StatCard({
  title,
  value,
  subtitle,
  trend,
  icon: Icon,
  iconClassName,
}: {
  title: string;
  value: string;
  subtitle?: string;
  trend?: number;
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
            <div className="flex items-center gap-2">
              {subtitle && (
                <span className="text-xs text-muted-foreground">{subtitle}</span>
              )}
              {trend !== undefined && <TrendIndicator value={trend} />}
            </div>
          </div>
          <div className={cn('p-2.5 rounded-xl bg-muted', iconClassName)}>
            <Icon className="h-5 w-5" />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function OverviewPage() {
  const { t } = useTranslation();

  // 数据 hooks
  const { data: summary } = useDashboardSummary();
  const { data: allTimeStats } = useAllTimeStats();
  const { data: heatmapData, isLoading: heatmapLoading, timezone: heatmapTimezone } = useActivityHeatmap();
  const { data: topModels, isLoading: modelsLoading } = useTopModels();
  const { data: trendData, isLoading: trendLoading } = use24HourTrend();
  const { data: firstUseInfo } = useFirstUseDate();
  const { data: providers } = useProviders();
  const { data: providerStats } = useDashboardProviderStats();
  const { data: requestsData } = useProxyRequests({ limit: 10 });
  const { data: sessions } = useSessions();
  const { cooldowns, formatRemaining } = useCooldowns();

  // 启用请求实时更新
  useProxyRequestUpdates();

  const recentRequests = requestsData?.items ?? [];

  // 计算 Provider 使用分布
  const providerDistribution = useMemo(() => {
    if (!providers || !providerStats) return [];

    const totalRequests = Object.values(providerStats).reduce(
      (acc, s) => acc + s.totalRequests,
      0
    );

    return providers
      .map((p) => {
        const stats = providerStats[p.id];
        const requests = stats?.totalRequests || 0;
        const percentage = totalRequests > 0 ? (requests / totalRequests) * 100 : 0;
        return {
          id: p.id,
          name: p.name,
          requests,
          percentage,
          successRate: stats?.successRate || 0,
          rpm: stats?.rpm,
          tpm: stats?.tpm,
        };
      })
      .filter((p) => p.requests > 0)
      .sort((a, b) => b.requests - a.requests)
      .slice(0, 5);
  }, [providers, providerStats]);

  // 活跃冷却中的 Provider
  const activeCooldowns = useMemo(() => {
    return cooldowns.filter((cd) => new Date(cd.untilTime) > new Date());
  }, [cooldowns]);

  // 活跃 Sessions
  const activeSessions = useMemo(() => {
    if (!sessions) return [];
    // 只显示最近有活动的 sessions
    return sessions.slice(0, 5);
  }, [sessions]);

  const hasProviders = (providers?.length ?? 0) > 0;

  // 欢迎页面（无 Provider 时）
  if (!hasProviders) {
    return (
      <div className="flex flex-col h-full">
        <PageHeader
          icon={LayoutDashboard}
          iconClassName="text-indigo-500"
          title={t('dashboard.title')}
          description={t('dashboard.description')}
        />
        <div className="flex-1 flex items-center justify-center p-4">
          <div className="text-center max-w-md">
            <div className="w-20 h-20 rounded-2xl bg-gradient-to-br from-violet-500 to-indigo-600 flex items-center justify-center mx-auto mb-6 shadow-xl">
              <Zap size={36} className="text-white" />
            </div>
            <h1 className="text-2xl font-bold text-foreground mb-4">
              {t('dashboard.welcome')}
            </h1>
            <p className="text-muted-foreground mb-6">
              {t('dashboard.welcomeDescription')}
            </p>
            <Link
              to="/providers"
              className="inline-flex items-center gap-2 bg-gradient-to-r from-violet-600 to-indigo-600 text-white px-6 py-2.5 rounded-lg hover:opacity-90 transition-opacity font-medium text-sm"
            >
              {t('dashboard.getStarted')}
              <ArrowRight className="h-4 w-4" />
            </Link>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={LayoutDashboard}
        iconClassName="text-indigo-500"
        title={t('dashboard.title')}
        description={t('dashboard.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="space-y-6 max-w-7xl mx-auto">
          {/* 核心统计卡片 */}
          <div className="grid gap-4 grid-cols-2 lg:grid-cols-4">
            <StatCard
              title={t('dashboard.todayRequests')}
              value={formatNumber(summary?.todayRequests || 0)}
              subtitle={summary?.rpm ? `${summary.rpm.toFixed(1)} RPM` : undefined}
              trend={summary?.requestsChange}
              icon={Activity}
              iconClassName="text-blue-600 dark:text-blue-400"
            />
            <StatCard
              title={t('dashboard.todayTokens')}
              value={formatNumber(summary?.todayTokens || 0)}
              subtitle={summary?.tpm ? `${formatNumber(summary.tpm)} TPM` : undefined}
              trend={summary?.tokensChange}
              icon={Cpu}
              iconClassName="text-violet-600 dark:text-violet-400"
            />
            <StatCard
              title={t('dashboard.todayCost')}
              value={formatCost(summary?.todayCost || 0)}
              trend={summary?.costChange}
              icon={Coins}
              iconClassName="text-amber-600 dark:text-amber-400"
            />
            <StatCard
              title={t('dashboard.successRate')}
              value={`${(summary?.todaySuccessRate || 0).toFixed(1)}%`}
              subtitle={
                (summary?.todaySuccessRate || 0) >= 95
                  ? t('dashboard.healthy')
                  : (summary?.todaySuccessRate || 0) >= 80
                    ? t('dashboard.warning')
                    : t('dashboard.critical')
              }
              icon={CheckCircle}
              iconClassName={cn(
                (summary?.todaySuccessRate || 0) >= 95
                  ? 'text-emerald-600 dark:text-emerald-400'
                  : (summary?.todaySuccessRate || 0) >= 80
                    ? 'text-amber-600 dark:text-amber-400'
                    : 'text-red-600 dark:text-red-400'
              )}
            />
          </div>

          {/* 第二行：趋势图 + 使用统计 */}
          <div className="grid gap-4 grid-cols-1 lg:grid-cols-3">
            {/* 24小时趋势 */}
            <Card className="lg:col-span-2 border-border/50 bg-card/50 backdrop-blur-sm">
              <CardHeader className="pb-2">
                <CardTitle className="text-base font-semibold flex items-center gap-2">
                  <Activity className="h-4 w-4 text-blue-500" />
                  {t('dashboard.trend24h')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="h-[180px]">
                  {trendLoading ? (
                    <div className="h-full flex items-center justify-center text-muted-foreground">
                      {t('common.loading')}
                    </div>
                  ) : trendData && trendData.length > 0 ? (
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={trendData}>
                        <defs>
                          <linearGradient id="colorRequests" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3} />
                            <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
                          </linearGradient>
                        </defs>
                        <XAxis
                          dataKey="hour"
                          tick={{ fontSize: 10 }}
                          tickLine={false}
                          axisLine={false}
                          interval="preserveStartEnd"
                        />
                        <YAxis hide />
                        <Tooltip
                          contentStyle={{
                            backgroundColor: 'hsl(var(--card))',
                            border: '1px solid hsl(var(--border))',
                            borderRadius: '8px',
                            fontSize: '12px',
                          }}
                          formatter={(value, name) => {
                            const numValue = typeof value === 'number' ? value : 0;
                            return [
                              name === 'requests' ? numValue.toLocaleString() : `$${numValue.toFixed(4)}`,
                              name === 'requests' ? t('dashboard.requests') : t('dashboard.cost'),
                            ];
                          }}
                        />
                        <Area
                          type="monotone"
                          dataKey="requests"
                          stroke="#3b82f6"
                          fillOpacity={1}
                          fill="url(#colorRequests)"
                          strokeWidth={2}
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  ) : (
                    <div className="h-full flex items-center justify-center text-muted-foreground">
                      {t('common.noData')}
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>

            {/* 使用统计 (Cursor 风格) */}
            <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
              <CardHeader className="pb-2">
                <CardTitle className="text-base font-semibold flex items-center gap-2">
                  <Hash className="h-4 w-4 text-violet-500" />
                  {t('dashboard.usageStats')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground flex items-center gap-2">
                    <Calendar className="h-4 w-4" />
                    {t('dashboard.firstUse')}
                  </span>
                  <span className="text-sm font-medium">
                    {firstUseInfo?.daysSinceFirstUse || 0} {t('dashboard.daysAgo')}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground flex items-center gap-2">
                    <Activity className="h-4 w-4" />
                    {t('dashboard.totalRequests')}
                  </span>
                  <span className="text-sm font-medium font-mono">
                    {formatNumber(allTimeStats?.totalRequests || 0)}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground flex items-center gap-2">
                    <Cpu className="h-4 w-4" />
                    {t('dashboard.totalTokens')}
                  </span>
                  <span className="text-sm font-medium font-mono">
                    {formatNumber(allTimeStats?.totalTokens || 0)}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground flex items-center gap-2">
                    <Coins className="h-4 w-4" />
                    {t('dashboard.totalCost')}
                  </span>
                  <span className="text-sm font-medium font-mono">
                    {formatCost(allTimeStats?.totalCost || 0)}
                  </span>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* 第三行：热力图 + Top 模型 */}
          <div className="grid gap-4 grid-cols-1 lg:grid-cols-2">
            {/* 活动热力图 */}
            <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
              <CardHeader className="pb-2">
                <CardTitle className="text-base font-semibold flex items-center gap-2">
                  <Activity className="h-4 w-4 text-emerald-500" />
                  {t('dashboard.activityHeatmap')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                {heatmapLoading ? (
                  <div className="h-24 flex items-center justify-center text-muted-foreground">
                    {t('common.loading')}
                  </div>
                ) : (
                  <ActivityHeatmap data={heatmapData || []} colorScheme="green" timezone={heatmapTimezone} />
                )}
              </CardContent>
            </Card>

            {/* Top 模型 */}
            <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
              <CardHeader className="pb-2">
                <CardTitle className="text-base font-semibold flex items-center gap-2">
                  <Cpu className="h-4 w-4 text-violet-500" />
                  {t('dashboard.topModels')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                {modelsLoading ? (
                  <div className="h-24 flex items-center justify-center text-muted-foreground">
                    {t('common.loading')}
                  </div>
                ) : topModels && topModels.length > 0 ? (
                  <div className="space-y-2">
                    {topModels.map((model, index) => (
                      <div
                        key={model.model}
                        className="flex items-center justify-between py-1.5 gap-4"
                      >
                        <div className="flex items-center gap-3 min-w-0 flex-1">
                          <span className="text-sm font-bold text-muted-foreground flex-shrink-0">
                            {index + 1}
                          </span>
                          <span className="text-sm font-medium truncate">
                            {model.model}
                          </span>
                        </div>
                        <div className="flex items-center gap-3 text-xs text-muted-foreground font-mono flex-shrink-0">
                          <span>{formatNumber(model.tokens)} tok</span>
                          <span>{formatNumber(model.requests)} req</span>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="h-24 flex items-center justify-center text-muted-foreground">
                    {t('common.noData')}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          {/* 第四行：Provider 状态 + 最近请求 */}
          <div className="grid gap-4 grid-cols-1 lg:grid-cols-2">
            {/* Provider 状态 */}
            <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
              <CardHeader className="pb-2">
                <CardTitle className="text-base font-semibold flex items-center gap-2">
                  <Server className="h-4 w-4 text-blue-500" />
                  {t('dashboard.providerStatus')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                {providerDistribution.length > 0 ? (
                  <div className="space-y-3">
                    {providerDistribution.map((provider) => (
                      <div key={provider.id} className="space-y-1">
                        <div className="flex items-center justify-between text-sm gap-4">
                          <span className="font-medium truncate min-w-0 flex-1">
                            {provider.name}
                          </span>
                          <div className="flex items-center gap-2 text-xs text-muted-foreground flex-shrink-0">
                            {provider.rpm !== undefined && provider.rpm > 0 && (
                              <span className="font-mono">{formatNumber(provider.rpm)} RPM</span>
                            )}
                            <span>{provider.percentage.toFixed(1)}%</span>
                          </div>
                        </div>
                        <div className="h-2 bg-muted rounded-full overflow-hidden">
                          <div
                            className="h-full bg-blue-500 rounded-full transition-all"
                            style={{ width: `${Math.max(provider.percentage, 2)}%` }}
                          />
                        </div>
                      </div>
                    ))}

                    {/* 冷却中的 Provider */}
                    {activeCooldowns.length > 0 && (
                      <div className="pt-2 border-t border-border/50 mt-3">
                        <div className="flex items-center gap-2 text-xs text-amber-600 dark:text-amber-400">
                          <AlertTriangle className="h-3 w-3" />
                          <span>{t('dashboard.cooldownActive')}</span>
                        </div>
                        {activeCooldowns.slice(0, 2).map((cd) => {
                          const provider = providers?.find((p) => p.id === cd.providerID);
                          return (
                            <div
                              key={cd.id}
                              className="flex items-center justify-between text-xs mt-1"
                            >
                              <span className="text-muted-foreground">
                                {provider?.name || `Provider #${cd.providerID}`}
                              </span>
                              <span className="font-mono text-amber-600 dark:text-amber-400">
                                {formatRemaining(cd)}
                              </span>
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="h-24 flex items-center justify-center text-muted-foreground">
                    {t('common.noData')}
                  </div>
                )}
              </CardContent>
            </Card>

            {/* 最近请求 */}
            <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-base font-semibold flex items-center gap-2">
                    <Clock className="h-4 w-4 text-emerald-500" />
                    {t('dashboard.recentRequests')}
                  </CardTitle>
                  <Link
                    to="/requests"
                    className="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1"
                  >
                    {t('dashboard.viewAll')}
                    <ArrowRight className="h-3 w-3" />
                  </Link>
                </div>
              </CardHeader>
              <CardContent>
                {recentRequests.length > 0 ? (
                  <div className="space-y-2">
                    {recentRequests.map((req) => {
                      const totalTokens = (req.inputTokenCount || 0) + (req.outputTokenCount || 0);
                      return (
                        <Link
                          key={req.id}
                          to={`/requests/${req.id}`}
                          className="flex items-center justify-between py-1.5 hover:bg-muted/50 rounded px-2 -mx-2 transition-colors gap-4"
                        >
                          <div className="flex items-center gap-2 min-w-0 flex-1">
                            {req.status === 'COMPLETED' ? (
                              <CheckCircle className="h-3.5 w-3.5 text-emerald-500 flex-shrink-0" />
                            ) : req.status === 'FAILED' ? (
                              <XCircle className="h-3.5 w-3.5 text-red-500 flex-shrink-0" />
                            ) : (
                              <Clock className="h-3.5 w-3.5 text-amber-500 flex-shrink-0" />
                            )}
                            <span className="text-sm font-medium truncate">
                              {req.responseModel || req.requestModel || 'Unknown'}
                            </span>
                          </div>
                          <div className="flex items-center gap-2 text-xs text-muted-foreground flex-shrink-0">
                            {totalTokens > 0 && (
                              <span className="font-mono">{formatNumber(totalTokens)} tok</span>
                            )}
                            <span>{formatRelativeTime(req.createdAt)}</span>
                          </div>
                        </Link>
                      );
                    })}
                  </div>
                ) : (
                  <div className="h-24 flex items-center justify-center text-muted-foreground">
                    {t('common.noData')}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          {/* 第五行：活跃 Sessions */}
          {activeSessions.length > 0 && (
            <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-base font-semibold flex items-center gap-2">
                    <Zap className="h-4 w-4 text-amber-500" />
                    {t('dashboard.activeSessions')}
                  </CardTitle>
                  <Link
                    to="/sessions"
                    className="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1"
                  >
                    {t('dashboard.viewAll')}
                    <ArrowRight className="h-3 w-3" />
                  </Link>
                </div>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap gap-2">
                  {activeSessions.map((session) => (
                    <div
                      key={session.id}
                      className="inline-flex items-center gap-2 px-3 py-1.5 bg-muted/50 rounded-full text-sm"
                    >
                      <span className="font-medium">{session.clientType}</span>
                      <span className="text-muted-foreground">•</span>
                      <span className="text-muted-foreground text-xs truncate max-w-[100px]">
                        {session.sessionID.slice(0, 8)}...
                      </span>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
