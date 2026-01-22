import { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import {
  Snowflake,
  Clock,
  AlertCircle,
  Server,
  Wifi,
  Zap,
  Ban,
  HelpCircle,
  X,
  Thermometer,
  Activity,
  Info,
  TrendingUp,
  DollarSign,
  Hash,
  CheckCircle2,
  XCircle,
  Trash2,
} from 'lucide-react';
import type { Cooldown, ProviderStats, ClientType } from '@/lib/transport/types';
import type { ProviderConfigItem } from '@/pages/client-routes/types';
import { useCooldownsContext } from '@/contexts/cooldowns-context';
import { Button, Switch } from '@/components/ui';
import { getProviderColor, type ProviderType } from '@/lib/theme';
import { cn } from '@/lib/utils';
import { Dialog, DialogContent } from '@/components/ui/dialog';

interface ProviderDetailsDialogProps {
  item: ProviderConfigItem | null;
  clientType: ClientType;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  stats?: ProviderStats;
  cooldown?: Cooldown | null;
  streamingCount: number;
  onToggle: () => void;
  isToggling: boolean;
  onDelete?: () => void;
  onClearCooldown?: () => void;
  isClearingCooldown?: boolean;
}

// Reason 信息和图标 - 使用翻译
const getReasonInfo = (t: TFunction) => ({
  server_error: {
    label: t('provider.reasons.serverError'),
    description: t(
      'provider.reasons.serverErrorDesc',
      '上游服务器返回 5xx 错误，系统自动进入冷却保护',
    ),
    icon: Server,
    color: 'text-rose-500 dark:text-rose-400',
    bgColor: 'bg-rose-500/10 dark:bg-rose-500/15 border-rose-500/30 dark:border-rose-500/25',
  },
  network_error: {
    label: t('provider.reasons.networkError'),
    description: t(
      'provider.reasons.networkErrorDesc',
      '无法连接到上游服务器，可能是网络故障或服务器宕机',
    ),
    icon: Wifi,
    color: 'text-amber-600 dark:text-amber-400',
    bgColor: 'bg-amber-500/10 dark:bg-amber-500/15 border-amber-500/30 dark:border-amber-500/25',
  },
  quota_exhausted: {
    label: t('provider.reasons.quotaExhausted'),
    description: t('provider.reasons.quotaExhaustedDesc', 'API 配额已用完，等待配额重置'),
    icon: AlertCircle,
    color: 'text-rose-500 dark:text-rose-400',
    bgColor: 'bg-rose-500/10 dark:bg-rose-500/15 border-rose-500/30 dark:border-rose-500/25',
  },
  rate_limit_exceeded: {
    label: t('provider.reasons.rateLimitExceeded'),
    description: t('provider.reasons.rateLimitExceededDesc', '请求速率超过限制，触发了速率保护'),
    icon: Zap,
    color: 'text-yellow-600 dark:text-yellow-400',
    bgColor:
      'bg-yellow-500/10 dark:bg-yellow-500/15 border-yellow-500/30 dark:border-yellow-500/25',
  },
  concurrent_limit: {
    label: t('provider.reasons.concurrentLimit'),
    description: t('provider.reasons.concurrentLimitDesc', '并发请求数超过限制'),
    icon: Ban,
    color: 'text-orange-600 dark:text-orange-400',
    bgColor:
      'bg-orange-500/10 dark:bg-orange-500/15 border-orange-500/30 dark:border-orange-500/25',
  },
  unknown: {
    label: t('provider.reasons.unknown'),
    description: t('provider.reasons.unknownDesc', '因未知原因进入冷却状态'),
    icon: HelpCircle,
    color: 'text-muted-foreground',
    bgColor: 'bg-muted/50 border-border',
  },
});

// 格式化 Token 数量
function formatTokens(count: number): string {
  if (count >= 1_000_000) {
    return `${(count / 1_000_000).toFixed(1)}M`;
  }
  if (count >= 1_000) {
    return `${(count / 1_000).toFixed(1)}K`;
  }
  return count.toString();
}

// 格式化成本 (纳美元 → 美元，向下取整到 6 位)
function formatCost(nanoUsd: number): string {
  // 向下取整到 6 位小数 (microUSD 精度)
  const usd = Math.floor(nanoUsd / 1000) / 1_000_000;
  if (usd >= 1) {
    return `$${usd.toFixed(2)}`;
  }
  if (usd >= 0.01) {
    return `$${usd.toFixed(3)}`;
  }
  return `$${usd.toFixed(6).replace(/\.?0+$/, '')}`;
}

// 计算缓存利用率
function calcCacheRate(stats: ProviderStats): number {
  const cacheTotal = stats.totalCacheRead + stats.totalCacheWrite;
  const total = stats.totalInputTokens + stats.totalOutputTokens + cacheTotal;
  if (total === 0) return 0;
  return (cacheTotal / total) * 100;
}

export function ProviderDetailsDialog({
  item,
  clientType,
  open,
  onOpenChange,
  stats,
  cooldown,
  streamingCount,
  onToggle,
  isToggling,
  onDelete,
  onClearCooldown,
  isClearingCooldown,
}: ProviderDetailsDialogProps) {
  const { t, i18n } = useTranslation();
  const REASON_INFO = getReasonInfo(t);
  const { formatRemaining } = useCooldownsContext();

  // 计算初始倒计时值
  const getInitialCountdown = useCallback(() => {
    return cooldown ? formatRemaining(cooldown) : '';
  }, [cooldown, formatRemaining]);

  const [liveCountdown, setLiveCountdown] = useState<string>(getInitialCountdown);

  // 每秒更新倒计时
  useEffect(() => {
    if (!cooldown) return;

    const interval = setInterval(() => {
      setLiveCountdown(formatRemaining(cooldown));
    }, 1000);

    return () => clearInterval(interval);
  }, [cooldown, formatRemaining]);

  if (!item) return null;

  const { provider, enabled, route, isNative } = item;
  const color = getProviderColor(provider.type as ProviderType);
  const isInCooldown = !!cooldown;

  const formatUntilTime = (until: string) => {
    const date = new Date(until);
    return date.toLocaleString(i18n.resolvedLanguage ?? i18n.language, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
  };

  const endpoint =
    provider.config?.custom?.clientBaseURL?.[clientType] ||
    provider.config?.custom?.baseURL ||
    provider.config?.antigravity?.endpoint ||
    t('provider.defaultEndpoint');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="overflow-hidden w-full max-w-[95vw] md:max-w-4xl lg:max-w-5xl xl:max-w-6xl p-0"
        showCloseButton={false}
      >
        {/* Header with Provider Color Gradient */}
        <div
          className="relative p-4 lg:p-6"
          style={{
            background: `linear-gradient(to bottom, ${color}15, transparent)`,
          }}
        >
          {/* 右上角：开关 + 关闭按钮 */}
          <div className="absolute top-3 right-3 lg:top-4 lg:right-4 flex items-center gap-3">
            {/* Toggle Switch */}
            <div className="flex items-center gap-2">
              <span
                className={cn(
                  'text-xs font-bold',
                  enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground',
                )}
              >
                {enabled ? t('provider.status.on') : t('provider.status.off')}
              </span>
              <Switch checked={enabled} onCheckedChange={onToggle} disabled={isToggling} />
            </div>
            <div className="w-px h-6 bg-border" />
            <Button
              type="button"
              onClick={() => onOpenChange(false)}
              variant={'ghost'}
              className={'border-none rounded-full'}
            >
              <X size={18} />
            </Button>
          </div>

          <div className="flex items-center gap-4 pr-32 lg:pr-40">
            {/* Provider Icon */}
            <div
              className={cn(
                'relative w-14 h-14 lg:w-16 lg:h-16 rounded-2xl flex items-center justify-center border shadow-lg',
                isInCooldown
                  ? 'bg-cyan-500/10 dark:bg-cyan-950/40 border-cyan-500/40 dark:border-cyan-500/30'
                  : 'bg-muted border-border',
              )}
              style={!isInCooldown ? { color } : {}}
            >
              <span
                className={cn(
                  'text-2xl lg:text-3xl font-black',
                  isInCooldown
                    ? 'text-cyan-400 dark:text-cyan-300 opacity-20 scale-150 blur-[1px]'
                    : '',
                )}
              >
                {provider.name.charAt(0).toUpperCase()}
              </span>
              {isInCooldown && (
                <Snowflake
                  size={24}
                  className="absolute text-cyan-400 dark:text-cyan-300 animate-pulse drop-shadow-[0_0_8px_rgba(34,211,238,0.5)]"
                />
              )}
            </div>

            {/* Provider Info */}
            <div className="flex-1 min-w-0">
              <h2 className="text-lg lg:text-xl font-bold text-foreground truncate mb-1">
                {provider.name}
              </h2>
              <div className="flex flex-wrap items-center gap-2">
                {isNative ? (
                  <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/10 dark:bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30 dark:border-emerald-500/25">
                    <Zap size={10} className="fill-emerald-500/20" /> NATIVE
                  </span>
                ) : (
                  <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold bg-amber-500/10 dark:bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30 dark:border-amber-500/25">
                    <Activity size={10} /> CONVERTED
                  </span>
                )}
                <span className="px-2 py-0.5 rounded-full text-[10px] font-mono bg-accent text-muted-foreground">
                  {provider.type}
                </span>
                {streamingCount > 0 && (
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-bold bg-blue-500/10 dark:bg-blue-500/15 text-blue-600 dark:text-blue-400 border border-blue-500/30 dark:border-blue-500/25 animate-pulse">
                    {streamingCount} Streaming
                  </span>
                )}
              </div>
            </div>
          </div>
        </div>
        {/* Body Content - 双栏布局 */}
        <div className="px-4 pb-4 lg:px-6 lg:pb-6">
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-4 lg:gap-6">
            {/* 左侧：Provider 信息 + 操作 */}
            <div className="lg:col-span-5 xl:col-span-4 space-y-4">
              {/* Provider Basic Info Card */}
              <div className="rounded-xl border border-border bg-muted-background p-4 space-y-3">
                <div className="flex items-start gap-2">
                  <Info size={14} className="text-muted-foreground mt-0.5 shrink-0" />
                  <div className="flex-1 min-w-0">
                    <div className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider mb-1">
                      Endpoint
                    </div>
                    <div className="text-xs text-muted-foreground font-mono break-all">
                      {endpoint}
                    </div>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <div className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider mb-1">
                      Client Type
                    </div>
                    <div className="text-xs text-foreground font-semibold">{clientType}</div>
                  </div>
                  {route && (
                    <div>
                      <div className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider mb-1">
                        Priority
                      </div>
                      <div className="text-xs text-foreground font-semibold">
                        #{route.position + 1}
                      </div>
                    </div>
                  )}
                </div>
              </div>

              {/* Actions Section */}
              <div className="space-y-3">
                {/* Cooldown Actions (if in cooldown) */}
                {isInCooldown && (
                  <Button
                    onClick={onClearCooldown}
                    disabled={isClearingCooldown || isToggling}
                    className="w-full relative overflow-hidden rounded-xl p-px group disabled:opacity-50 disabled:cursor-not-allowed transition-all hover:scale-[1.01] active:scale-[0.99] shadow-lg shadow-teal-500/20 dark:shadow-teal-500/30 hover:shadow-teal-500/40 dark:hover:shadow-teal-500/50"
                  >
                    <span className="absolute inset-0 bg-linear-to-r from-teal-500 via-cyan-500 to-blue-500 rounded-xl" />
                    <div className="relative flex items-center justify-center gap-2 rounded-lg px-4 py-3 transition-colors">
                      {isClearingCooldown ? (
                        <>
                          <div className="h-4 w-4 animate-spin rounded-full border-2 border-teal-400/30 border-t-teal-400" />
                          <span className="text-sm font-bold ">{t('provider.thawing')}</span>
                        </>
                      ) : (
                        <>
                          <Zap size={16} />
                          {t('provider.forceThaw')}
                        </>
                      )}
                    </div>
                  </Button>
                )}

                {/* Delete Button */}
                {onDelete && (
                  <Button
                    onClick={onDelete}
                    className="w-full flex items-center justify-center gap-2 rounded-xl border border-rose-500/30 dark:border-rose-500/25 bg-rose-500/5 dark:bg-rose-500/10 hover:bg-rose-500/10 dark:hover:bg-rose-500/15 px-4 py-2.5 text-sm font-medium text-rose-600 dark:text-rose-400 transition-colors"
                  >
                    <Trash2 size={14} />
                    {t('provider.deleteRoute')}
                  </Button>
                )}

                {/* Warning Note */}
                {isInCooldown && (
                  <div className="flex items-start gap-2 rounded-lg bg-muted/50 p-2.5 text-[11px] text-muted-foreground">
                    <Activity size={12} className="mt-0.5 shrink-0" />
                    <p>{t('provider.forceThawWarning')}</p>
                  </div>
                )}
              </div>
            </div>

            {/* 右侧：Cooldown + Statistics */}
            <div className="lg:col-span-7 xl:col-span-8 space-y-4">
              {/* Cooldown Warning (if in cooldown) */}
              {isInCooldown && cooldown && (
                <div className="rounded-xl border p-4 space-y-3">
                  <div className="flex items-center gap-2 text-teal-600 dark:text-teal-400">
                    <Snowflake size={16} className="animate-spin-slow" />
                    <span className="text-sm font-bold">{t('provider.cooldownActive')}</span>
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    {/* Reason Section */}
                    <div
                      className={`rounded-xl border p-3 backdrop-blur-sm ${REASON_INFO[cooldown.reason]?.bgColor || REASON_INFO.unknown.bgColor}`}
                    >
                      <div className="flex gap-3">
                        <div
                          className={`mt-0.5 shrink-0 ${REASON_INFO[cooldown.reason]?.color || REASON_INFO.unknown.color}`}
                        >
                          {(() => {
                            const Icon =
                              REASON_INFO[cooldown.reason]?.icon || REASON_INFO.unknown.icon;
                            return <Icon size={18} />;
                          })()}
                        </div>
                        <div>
                          <h3
                            className={`text-sm font-bold ${REASON_INFO[cooldown.reason]?.color || REASON_INFO.unknown.color} mb-1`}
                          >
                            {REASON_INFO[cooldown.reason]?.label || REASON_INFO.unknown.label}
                          </h3>
                          <p className="text-xs text-muted-foreground leading-relaxed">
                            {REASON_INFO[cooldown.reason]?.description ||
                              REASON_INFO.unknown.description}
                          </p>
                        </div>
                      </div>
                    </div>

                    {/* Timer Section */}
                    <div className="relative overflow-hidden rounded-xl bg-gradient-to-br from-teal-500/10 via-cyan-500/5 to-transparent dark:from-teal-950/40 dark:via-cyan-950/20 dark:to-transparent border border-teal-400/30 dark:border-teal-500/20 p-4 flex flex-col items-center justify-center group shadow-inner">
                      <div className="absolute inset-0 bg-gradient-to-br from-teal-400/5 to-cyan-400/5 dark:from-teal-400/5 dark:to-cyan-400/5 opacity-50 group-hover:opacity-100 transition-opacity" />
                      <div className="relative flex items-center gap-1.5 text-teal-600 dark:text-teal-400 mb-1">
                        <Thermometer size={12} />
                        <span className="text-[9px] font-bold uppercase tracking-widest">
                          Remaining
                        </span>
                      </div>
                      <div className="relative font-mono text-2xl lg:text-3xl font-bold text-teal-600 dark:text-teal-400 tracking-widest tabular-nums drop-shadow-[0_0_12px_rgba(20,184,166,0.4)]">
                        {liveCountdown}
                      </div>
                      {(() => {
                        const untilDateStr = formatUntilTime(cooldown.untilTime);
                        return (
                          <div className="relative mt-2 text-[10px] text-teal-600/70 dark:text-teal-400/70 font-mono flex items-center gap-2">
                            <Clock size={10} />
                            {untilDateStr}
                          </div>
                        );
                      })()}
                    </div>
                  </div>
                </div>
              )}

              {/* Statistics Section */}
              <div className="space-y-3">
                <div className="flex items-center gap-2 text-muted-foreground">
                  <TrendingUp size={14} />
                  <span className="text-xs font-bold uppercase tracking-wider">Statistics</span>
                </div>

                {stats && stats.totalRequests > 0 ? (
                  <div className="grid grid-cols-2 lg:grid-cols-4 gap-2 lg:gap-3">
                    {/* Requests */}
                    <div className="p-3 rounded-lg bg-linear-to-br from-slate-50 to-slate-100/50 dark:from-slate-900/50 dark:to-slate-800/30 border border-slate-200 dark:border-slate-700/50 shadow-sm hover:shadow-md transition-shadow">
                      <div className="flex items-center gap-1.5 mb-2">
                        <Hash size={12} className="text-slate-500 dark:text-slate-400" />
                        <span className="text-[9px] font-bold text-slate-600 dark:text-slate-400 uppercase tracking-wider">
                          Requests
                        </span>
                      </div>
                      <div className="space-y-1">
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-slate-600 dark:text-slate-400">Total</span>
                          <span className="font-mono font-bold text-slate-900 dark:text-slate-100">
                            {stats.totalRequests}
                          </span>
                        </div>
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-emerald-600 dark:text-emerald-400 flex items-center gap-1">
                            <CheckCircle2 size={10} /> OK
                          </span>
                          <span className="font-mono font-bold text-emerald-600 dark:text-emerald-400">
                            {stats.successfulRequests}
                          </span>
                        </div>
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-rose-600 dark:text-rose-400 flex items-center gap-1">
                            <XCircle size={10} /> Fail
                          </span>
                          <span className="font-mono font-bold text-rose-600 dark:text-rose-400">
                            {stats.failedRequests}
                          </span>
                        </div>
                      </div>
                    </div>

                    {/* Success Rate */}
                    <div className="p-3 rounded-lg bg-linear-to-br from-emerald-50 to-emerald-100/50 dark:from-emerald-950/30 dark:to-emerald-900/20 border border-emerald-200 dark:border-emerald-800/50 shadow-sm hover:shadow-md transition-shadow">
                      <div className="flex items-center gap-1.5 mb-2">
                        <Activity size={12} className="text-emerald-600 dark:text-emerald-400" />
                        <span className="text-[9px] font-bold text-emerald-700 dark:text-emerald-400 uppercase tracking-wider">
                          Success Rate
                        </span>
                      </div>
                      <div className="flex flex-col items-center justify-center h-[52px]">
                        <div
                          className={cn(
                            'text-2xl lg:text-3xl font-black font-mono drop-shadow-sm',
                            stats.successRate >= 95
                              ? 'text-emerald-600 dark:text-emerald-400'
                              : stats.successRate >= 90
                                ? 'text-blue-600 dark:text-blue-400'
                                : 'text-amber-600 dark:text-amber-400',
                          )}
                        >
                          {Math.round(stats.successRate)}%
                        </div>
                      </div>
                    </div>

                    {/* Tokens */}
                    <div className="p-3 rounded-lg bg-linear-to-br from-blue-50 to-blue-100/50 dark:from-blue-950/30 dark:to-blue-900/20 border border-blue-200 dark:border-blue-800/50 shadow-sm hover:shadow-md transition-shadow">
                      <div className="flex items-center gap-1.5 mb-2">
                        <Zap size={12} className="text-blue-600 dark:text-blue-400" />
                        <span className="text-[9px] font-bold text-blue-700 dark:text-blue-400 uppercase tracking-wider">
                          Tokens
                        </span>
                      </div>
                      <div className="space-y-1">
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-blue-600 dark:text-blue-400">In</span>
                          <span className="font-mono font-bold text-blue-700 dark:text-blue-300">
                            {formatTokens(stats.totalInputTokens)}
                          </span>
                        </div>
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-purple-600 dark:text-purple-400">Out</span>
                          <span className="font-mono font-bold text-purple-700 dark:text-purple-300">
                            {formatTokens(stats.totalOutputTokens)}
                          </span>
                        </div>
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-cyan-600 dark:text-cyan-400">Cache</span>
                          <span className="font-mono font-bold text-cyan-700 dark:text-cyan-300">
                            {formatTokens(stats.totalCacheRead + stats.totalCacheWrite)}
                          </span>
                        </div>
                      </div>
                    </div>

                    {/* Cost */}
                    <div className="p-3 rounded-lg bg-linear-to-br from-purple-50 to-purple-100/50 dark:from-purple-950/30 dark:to-purple-900/20 border border-purple-200 dark:border-purple-800/50 shadow-sm hover:shadow-md transition-shadow">
                      <div className="flex items-center gap-1.5 mb-2">
                        <DollarSign size={12} className="text-purple-600 dark:text-purple-400" />
                        <span className="text-[9px] font-bold text-purple-700 dark:text-purple-400 uppercase tracking-wider">
                          Cost
                        </span>
                      </div>
                      <div className="flex flex-col items-center justify-center h-[52px]">
                        <div className="text-xl lg:text-2xl font-black font-mono text-purple-700 dark:text-purple-300 drop-shadow-sm">
                          {formatCost(stats.totalCost)}
                        </div>
                        <div className="text-[9px] text-purple-600/70 dark:text-purple-400/70 mt-0.5 font-medium">
                          Cache: {calcCacheRate(stats).toFixed(1)}%
                        </div>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="p-6 lg:p-8 flex flex-col items-center gap-2 text-slate-400 dark:text-slate-500 rounded-lg bg-gradient-to-br from-slate-50 to-slate-100/50 dark:from-slate-900/30 dark:to-slate-800/20 border border-slate-200 dark:border-slate-700/50">
                    <Activity size={24} />
                    <span className="text-xs font-bold uppercase tracking-widest">
                      No Statistics Available
                    </span>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
