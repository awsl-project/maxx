import { useEffect, useCallback, useState } from 'react';
import { createPortal } from 'react-dom';
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
import type { Cooldown, CooldownReason, ProviderStats, ClientType } from '@/lib/transport/types';
import type { ProviderConfigItem } from '@/pages/client-routes/types';
import { useCooldowns } from '@/hooks/use-cooldowns';
import { Switch } from '@/components/ui';
import { getProviderColor, type ProviderType } from '@/lib/theme';
import { cn } from '@/lib/utils';

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

// Reason 中文说明和图标
const REASON_INFO: Record<CooldownReason, { label: string; description: string; icon: typeof Server; color: string; bgColor: string }> = {
  server_error: {
    label: '服务器错误',
    description: '上游服务器返回 5xx 错误，系统自动进入冷却保护',
    icon: Server,
    color: 'text-red-400',
    bgColor: 'bg-red-400/10 border-red-400/20',
  },
  network_error: {
    label: '网络错误',
    description: '无法连接到上游服务器，可能是网络故障或服务器宕机',
    icon: Wifi,
    color: 'text-amber-400',
    bgColor: 'bg-amber-400/10 border-amber-400/20',
  },
  quota_exhausted: {
    label: '配额耗尽',
    description: 'API 配额已用完，等待配额重置',
    icon: AlertCircle,
    color: 'text-red-400',
    bgColor: 'bg-red-400/10 border-red-400/20',
  },
  rate_limit_exceeded: {
    label: '速率限制',
    description: '请求速率超过限制，触发了速率保护',
    icon: Zap,
    color: 'text-yellow-400',
    bgColor: 'bg-yellow-400/10 border-yellow-400/20',
  },
  concurrent_limit: {
    label: '并发限制',
    description: '并发请求数超过限制',
    icon: Ban,
    color: 'text-orange-400',
    bgColor: 'bg-orange-400/10 border-orange-400/20',
  },
  unknown: {
    label: '未知原因',
    description: '因未知原因进入冷却状态',
    icon: HelpCircle,
    color: 'text-text-muted',
    bgColor: 'bg-surface-secondary border-border',
  },
};

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

// 格式化成本 (微美元 → 美元)
function formatCost(microUsd: number): string {
  const usd = microUsd / 1_000_000;
  if (usd >= 1) {
    return `$${usd.toFixed(2)}`;
  }
  if (usd >= 0.01) {
    return `$${usd.toFixed(3)}`;
  }
  return `$${usd.toFixed(4)}`;
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
  const { formatRemaining } = useCooldowns();
  const [liveCountdown, setLiveCountdown] = useState<string>('');

  // Handle ESC key
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      onOpenChange(false);
    }
  }, [onOpenChange]);

  useEffect(() => {
    if (open) {
      document.addEventListener('keydown', handleKeyDown);
      document.body.style.overflow = 'hidden';
      return () => {
        document.removeEventListener('keydown', handleKeyDown);
        document.body.style.overflow = '';
      };
    }
  }, [open, handleKeyDown]);

  // 每秒更新倒计时
  useEffect(() => {
    if (!cooldown) {
      setLiveCountdown('');
      return;
    }

    setLiveCountdown(formatRemaining(cooldown));
    const interval = setInterval(() => {
      setLiveCountdown(formatRemaining(cooldown));
    }, 1000);

    return () => clearInterval(interval);
  }, [cooldown, formatRemaining]);

  if (!open || !item) return null;

  const { provider, enabled, route, isNative } = item;
  const color = getProviderColor(provider.type as ProviderType);
  const isInCooldown = !!cooldown;

  const formatUntilTime = (until: string) => {
    const date = new Date(until);
    return date.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
  };

  const endpoint = provider.config?.custom?.clientBaseURL?.[clientType] ||
    provider.config?.custom?.baseURL ||
    provider.config?.antigravity?.endpoint ||
    'Default endpoint';

  return createPortal(
    <>
      {/* Overlay */}
      <div
        className="dialog-overlay backdrop-blur-[2px]"
        onClick={() => onOpenChange(false)}
        style={{ zIndex: 99998 }}
      />

      {/* Content */}
      <div
        className="dialog-content overflow-hidden w-full max-w-[95vw] md:max-w-4xl lg:max-w-5xl xl:max-w-6xl"
        style={{
          zIndex: 99999,
          padding: 0,
          background: 'var(--color-surface-primary)',
        }}
        onClick={(e) => e.stopPropagation()}
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
              <span className={cn(
                "text-xs font-bold",
                enabled ? "text-emerald-500" : "text-text-muted"
              )}>
                {enabled ? 'ON' : 'OFF'}
              </span>
              <Switch
                checked={enabled}
                onCheckedChange={onToggle}
                disabled={isToggling}
              />
            </div>
            <div className="w-px h-6 bg-border" />
            <button
              onClick={() => onOpenChange(false)}
              className="p-2 rounded-full hover:bg-surface-hover text-text-muted hover:text-text-primary transition-colors"
            >
              <X size={18} />
            </button>
          </div>

          <div className="flex items-center gap-4 pr-32 lg:pr-40">
            {/* Provider Icon */}
            <div
              className={cn(
                "relative w-14 h-14 lg:w-16 lg:h-16 rounded-2xl flex items-center justify-center border shadow-lg",
                isInCooldown ? "bg-cyan-900/40 border-cyan-500/30" : "bg-surface-secondary border-border"
              )}
              style={!isInCooldown ? { color } : {}}
            >
              <span className={cn(
                "text-2xl lg:text-3xl font-black",
                isInCooldown ? "text-cyan-400 opacity-20 scale-150 blur-[1px]" : ""
              )}>
                {provider.name.charAt(0).toUpperCase()}
              </span>
              {isInCooldown && (
                <Snowflake size={24} className="absolute text-cyan-400 animate-pulse drop-shadow-[0_0_8px_rgba(34,211,238,0.5)]" />
              )}
            </div>

            {/* Provider Info */}
            <div className="flex-1 min-w-0">
              <h2 className="text-lg lg:text-xl font-bold text-text-primary truncate mb-1">
                {provider.name}
              </h2>
              <div className="flex flex-wrap items-center gap-2">
                {isNative ? (
                  <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/10 text-emerald-500 border border-emerald-500/20">
                    <Zap size={10} className="fill-emerald-500/20" /> NATIVE
                  </span>
                ) : (
                  <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold bg-amber-500/10 text-amber-500 border border-amber-500/20">
                    <Activity size={10} /> CONVERTED
                  </span>
                )}
                <span className="px-2 py-0.5 rounded-full text-[10px] font-mono bg-surface-hover text-text-secondary">
                  {provider.type}
                </span>
                {streamingCount > 0 && (
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-bold bg-blue-500/10 text-blue-400 border border-blue-500/20 animate-pulse">
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
              <div className="rounded-xl border border-border bg-surface-secondary p-4 space-y-3">
                <div className="flex items-start gap-2">
                  <Info size={14} className="text-text-muted mt-0.5 flex-shrink-0" />
                  <div className="flex-1 min-w-0">
                    <div className="text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1">Endpoint</div>
                    <div className="text-xs text-text-secondary font-mono break-all">{endpoint}</div>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <div className="text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1">Client Type</div>
                    <div className="text-xs text-text-primary font-semibold">{clientType}</div>
                  </div>
                  {route && (
                    <div>
                      <div className="text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1">Priority</div>
                      <div className="text-xs text-text-primary font-semibold">#{route.position + 1}</div>
                    </div>
                  )}
                </div>
              </div>

              {/* Actions Section */}
              <div className="space-y-3">
                {/* Cooldown Actions (if in cooldown) */}
                {isInCooldown && (
                  <button
                    onClick={onClearCooldown}
                    disabled={isClearingCooldown || isToggling}
                    className="w-full relative overflow-hidden rounded-xl p-[1px] group disabled:opacity-50 disabled:cursor-not-allowed transition-all hover:scale-[1.01] active:scale-[0.99]"
                  >
                    <span className="absolute inset-0 bg-gradient-to-r from-cyan-500 to-blue-600 rounded-xl" />
                    <div className="relative flex items-center justify-center gap-2 rounded-[11px] bg-surface-primary group-hover:bg-transparent px-4 py-3 transition-colors">
                      {isClearingCooldown ? (
                        <>
                          <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                          <span className="text-sm font-bold text-white">Thawing...</span>
                        </>
                      ) : (
                        <>
                          <Zap size={16} className="text-cyan-400 group-hover:text-white transition-colors" />
                          <span className="text-sm font-bold text-cyan-400 group-hover:text-white transition-colors">立即解冻</span>
                        </>
                      )}
                    </div>
                  </button>
                )}

                {/* Delete Button */}
                {onDelete && (
                  <button
                    onClick={onDelete}
                    className="w-full flex items-center justify-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 hover:bg-red-500/10 px-4 py-2.5 text-sm font-medium text-red-400 transition-colors"
                  >
                    <Trash2 size={14} />
                    删除此路由
                  </button>
                )}

                {/* Warning Note */}
                {isInCooldown && (
                  <div className="flex items-start gap-2 rounded-lg bg-surface-secondary/50 p-2.5 text-[11px] text-text-muted">
                    <Activity size={12} className="mt-0.5 shrink-0" />
                    <p>强制解冻可能导致请求因根本原因未解决而再次失败。</p>
                  </div>
                )}
              </div>
            </div>

            {/* 右侧：Cooldown + Statistics */}
            <div className="lg:col-span-7 xl:col-span-8 space-y-4">
              {/* Cooldown Warning (if in cooldown) */}
              {isInCooldown && cooldown && (
                <div className="rounded-xl border border-cyan-500/30 bg-gradient-to-br from-cyan-950/20 to-blue-950/10 p-4 space-y-3">
                  <div className="flex items-center gap-2 text-cyan-400">
                    <Snowflake size={16} className="animate-spin-slow" />
                    <span className="text-sm font-bold">冷却保护激活</span>
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    {/* Reason Section */}
                    <div className={`rounded-xl border p-3 ${REASON_INFO[cooldown.reason]?.bgColor || REASON_INFO.unknown.bgColor}`}>
                      <div className="flex gap-3">
                        <div className={`mt-0.5 flex-shrink-0 ${REASON_INFO[cooldown.reason]?.color || REASON_INFO.unknown.color}`}>
                          {(() => {
                            const Icon = REASON_INFO[cooldown.reason]?.icon || REASON_INFO.unknown.icon;
                            return <Icon size={18} />;
                          })()}
                        </div>
                        <div>
                          <h3 className={`text-sm font-bold ${REASON_INFO[cooldown.reason]?.color || REASON_INFO.unknown.color} mb-1`}>
                            {REASON_INFO[cooldown.reason]?.label || REASON_INFO.unknown.label}
                          </h3>
                          <p className="text-xs text-text-secondary leading-relaxed">
                            {REASON_INFO[cooldown.reason]?.description || REASON_INFO.unknown.description}
                          </p>
                        </div>
                      </div>
                    </div>

                    {/* Timer Section */}
                    <div className="relative overflow-hidden rounded-xl bg-gradient-to-br from-cyan-950/30 to-transparent border border-cyan-500/20 p-4 flex flex-col items-center justify-center group">
                      <div className="absolute inset-0 bg-cyan-400/5 opacity-50 group-hover:opacity-100 transition-opacity" />
                      <div className="relative flex items-center gap-1.5 text-cyan-500 mb-1">
                        <Thermometer size={12} />
                        <span className="text-[9px] font-bold uppercase tracking-widest">Remaining</span>
                      </div>
                      <div className="relative font-mono text-2xl lg:text-3xl font-bold text-cyan-400 tracking-widest tabular-nums drop-shadow-[0_0_8px_rgba(34,211,238,0.3)]">
                        {liveCountdown}
                      </div>
                      {(() => {
                        const untilDateStr = formatUntilTime(cooldown.until);
                        return (
                          <div className="relative mt-2 text-[10px] text-cyan-500/70 font-mono flex items-center gap-2">
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
                <div className="flex items-center gap-2 text-text-secondary">
                  <TrendingUp size={14} />
                  <span className="text-xs font-bold uppercase tracking-wider">Statistics</span>
                </div>

                {stats && stats.totalRequests > 0 ? (
                  <div className="grid grid-cols-2 lg:grid-cols-4 gap-2 lg:gap-3">
                    {/* Requests */}
                    <div className="p-3 rounded-lg bg-surface-secondary border border-border">
                      <div className="flex items-center gap-1.5 mb-2">
                        <Hash size={12} className="text-text-muted" />
                        <span className="text-[9px] font-bold text-text-muted uppercase tracking-wider">Requests</span>
                      </div>
                      <div className="space-y-1">
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-text-secondary">Total</span>
                          <span className="font-mono font-bold text-text-primary">{stats.totalRequests}</span>
                        </div>
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-emerald-500 flex items-center gap-1">
                            <CheckCircle2 size={10} /> OK
                          </span>
                          <span className="font-mono font-bold text-emerald-500">{stats.successfulRequests}</span>
                        </div>
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-red-400 flex items-center gap-1">
                            <XCircle size={10} /> Fail
                          </span>
                          <span className="font-mono font-bold text-red-400">{stats.failedRequests}</span>
                        </div>
                      </div>
                    </div>

                    {/* Success Rate */}
                    <div className="p-3 rounded-lg bg-surface-secondary border border-border">
                      <div className="flex items-center gap-1.5 mb-2">
                        <Activity size={12} className="text-text-muted" />
                        <span className="text-[9px] font-bold text-text-muted uppercase tracking-wider">Success Rate</span>
                      </div>
                      <div className="flex flex-col items-center justify-center h-[52px]">
                        <div className={cn(
                          "text-2xl lg:text-3xl font-black font-mono",
                          stats.successRate >= 95 ? "text-emerald-500" :
                          stats.successRate >= 90 ? "text-blue-400" : "text-amber-500"
                        )}>
                          {Math.round(stats.successRate)}%
                        </div>
                      </div>
                    </div>

                    {/* Tokens */}
                    <div className="p-3 rounded-lg bg-surface-secondary border border-border">
                      <div className="flex items-center gap-1.5 mb-2">
                        <Zap size={12} className="text-text-muted" />
                        <span className="text-[9px] font-bold text-text-muted uppercase tracking-wider">Tokens</span>
                      </div>
                      <div className="space-y-1">
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-text-secondary">In</span>
                          <span className="font-mono font-bold text-blue-400">{formatTokens(stats.totalInputTokens)}</span>
                        </div>
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-text-secondary">Out</span>
                          <span className="font-mono font-bold text-purple-400">{formatTokens(stats.totalOutputTokens)}</span>
                        </div>
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-text-secondary">Cache</span>
                          <span className="font-mono font-bold text-cyan-400">{formatTokens(stats.totalCacheRead + stats.totalCacheWrite)}</span>
                        </div>
                      </div>
                    </div>

                    {/* Cost */}
                    <div className="p-3 rounded-lg bg-surface-secondary border border-border">
                      <div className="flex items-center gap-1.5 mb-2">
                        <DollarSign size={12} className="text-text-muted" />
                        <span className="text-[9px] font-bold text-text-muted uppercase tracking-wider">Cost</span>
                      </div>
                      <div className="flex flex-col items-center justify-center h-[52px]">
                        <div className="text-xl lg:text-2xl font-black font-mono text-purple-400">
                          {formatCost(stats.totalCost)}
                        </div>
                        <div className="text-[9px] text-text-muted mt-0.5">
                          Cache: {calcCacheRate(stats).toFixed(1)}%
                        </div>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="p-6 lg:p-8 flex flex-col items-center gap-2 text-text-muted/30 rounded-lg bg-surface-secondary border border-border">
                    <Activity size={24} />
                    <span className="text-xs font-bold uppercase tracking-widest">No Statistics Available</span>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </>,
    document.body
  );
}
