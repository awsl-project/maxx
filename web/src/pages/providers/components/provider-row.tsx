import { ChevronRight, Activity, Mail, Globe } from 'lucide-react';
import { ClientIcon } from '@/components/icons/client-icons';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { MarqueeBackground } from '@/components/ui/marquee-background';
import type { Provider, ProviderStats, AntigravityQuotaData, KiroQuotaData } from '@/lib/transport';
import { getProviderTypeConfig } from '../types';
import { cn } from '@/lib/utils';
import { useAntigravityQuotaFromContext } from '@/contexts/antigravity-quotas-context';
import { useKiroQuota } from '@/hooks/queries';
import { useTranslation } from 'react-i18next';

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

interface ProviderRowProps {
  provider: Provider;
  stats?: ProviderStats;
  streamingCount: number;
  onClick: () => void;
}

// 获取 Claude 模型额度百分比和重置时间
function getClaudeQuotaInfo(
  quota: AntigravityQuotaData | undefined,
): { percentage: number; resetTime: string } | null {
  if (!quota || quota.isForbidden || !quota.models) return null;
  const claudeModel = quota.models.find((m) => m.name.includes('claude'));
  if (!claudeModel) return null;
  return {
    percentage: claudeModel.percentage,
    resetTime: claudeModel.resetTime,
  };
}

// 格式化重置时间
function formatResetTime(resetTime: string, t: (key: string) => string): string {
  try {
    const reset = new Date(resetTime);
    const now = new Date();
    const diff = reset.getTime() - now.getTime();

    if (diff <= 0) return t('proxy.comingSoon');

    const hours = Math.floor(diff / (1000 * 60 * 60));
    const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

    if (hours > 24) {
      const days = Math.floor(hours / 24);
      return `${days}d`;
    }
    if (hours > 0) {
      return `${hours}h`;
    }
    return `${minutes}m`;
  } catch {
    return '-';
  }
}

// 格式化 Kiro 重置天数
function formatKiroResetDays(days: number, t: (key: string) => string): string {
  if (days <= 0) return t('proxy.comingSoon');
  if (days === 1) return '1d';
  return `${days}d`;
}

// 获取 Kiro 配额信息
function getKiroQuotaInfo(quota: KiroQuotaData | undefined): {
  percentage: number;
  resetDays: number;
  isBanned: boolean;
  totalLimit: number;
  available: number;
  used: number;
} | null {
  if (!quota) return null;
  // 计算百分比: available / total_limit * 100
  const percentage =
    quota.total_limit > 0 ? Math.round((quota.available / quota.total_limit) * 100) : 0;
  return {
    percentage,
    resetDays: quota.days_until_reset,
    isBanned: quota.is_banned,
    totalLimit: quota.total_limit,
    available: quota.available,
    used: quota.used,
  };
}

export function ProviderRow({ provider, stats, streamingCount, onClick }: ProviderRowProps) {
  const { t } = useTranslation();
  // 使用通用配置系统
  const typeConfig = getProviderTypeConfig(provider.type);
  const color = typeConfig.color;
  const displayInfo = typeConfig.getDisplayInfo(provider);

  const isAntigravity = provider.type === 'antigravity';
  const isKiro = provider.type === 'kiro';

  // 从批量查询上下文获取 Antigravity 额度
  const antigravityQuota = useAntigravityQuotaFromContext(provider.id);
  const claudeInfo = isAntigravity ? getClaudeQuotaInfo(antigravityQuota) : null;

  // 仅为 Kiro provider 获取额度
  const { data: kiroQuota } = useKiroQuota(provider.id, isKiro);
  const kiroInfo = isKiro ? getKiroQuotaInfo(kiroQuota) : null;

  return (
    <div
      onClick={onClick}
      className={cn(
        'group relative flex items-center gap-4 p-3 rounded-xl border transition-all duration-300 overflow-hidden',
        streamingCount > 0
          ? 'bg-card border-transparent ring-1 ring-black/5 dark:ring-white/10'
          : 'bg-card/60 border-border hover:border-accent/30 hover:bg-card shadow-sm cursor-pointer',
      )}
      style={{
        borderColor: streamingCount > 0 ? `${color}40` : undefined,
        boxShadow: streamingCount > 0 ? `0 0 20px ${color}15` : undefined,
      }}
    >
      <MarqueeBackground show={streamingCount > 0} color={`${color}15`} opacity={0.4} />

      {/* Streaming Badge - 右上角 */}
      {streamingCount > 0 && (
        <div className="absolute top-0 right-0 z-20">
          <StreamingBadge count={streamingCount} color={color} />
        </div>
      )}

      {/* Supported Clients - 左侧居中 */}
      <div className="relative z-10 w-20 flex items-center justify-center shrink-0">
        <div className="flex flex-wrap items-center justify-center gap-1 w-full">
          {provider.supportedClientTypes?.length > 0 ? (
            provider.supportedClientTypes.map((ct) => (
              <div
                key={ct}
                className="relative bg-background rounded-md p-1 border border-border/60 transition-all hover:border-accent/50 hover:shadow-md hover:-translate-y-0.5 shadow-sm"
                title={ct}
              >
                <ClientIcon type={ct} size={18} />
              </div>
            ))
          ) : (
            <span className="text-xs text-muted-foreground font-mono">-</span>
          )}
        </div>
      </div>

      {/* Provider Info */}
      <div className="relative z-10 flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <h3 className="text-[15px] font-bold text-foreground truncate">{provider.name}</h3>
          <span
            className="px-2 py-0.5 rounded-full text-[10px] uppercase tracking-wider font-black flex items-center gap-1"
            style={{ backgroundColor: `${color}15`, color }}
          >
            <div
              className="w-1 h-1 rounded-full animate-pulse"
              style={{ backgroundColor: color }}
            />
            {typeConfig.label}
          </span>
        </div>
        <div className="flex items-center gap-3">
          {/* 对于 Antigravity，显示 Claude Quota；对于其他类型，显示邮箱/endpoint */}
          {isAntigravity && claudeInfo ? (
            <div className="flex items-center gap-2 shrink-0">
              <span className="text-[9px] font-black text-muted-foreground/60 uppercase">
                Claude
              </span>
              <div className="w-20 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
                <div
                  className={cn(
                    'h-full rounded-full transition-all duration-1000',
                    claudeInfo.percentage >= 50
                      ? 'bg-emerald-500'
                      : claudeInfo.percentage >= 20
                        ? 'bg-amber-500'
                        : 'bg-red-500',
                  )}
                  style={{ width: `${claudeInfo.percentage}%` }}
                />
              </div>
              <span className="text-[9px] font-mono text-muted-foreground/60">
                {formatResetTime(claudeInfo.resetTime, t)}
              </span>
            </div>
          ) : (
            <div
              className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground truncate"
              title={displayInfo}
            >
              {typeConfig.isAccountBased ? (
                <Mail size={11} className="shrink-0" />
              ) : (
                <Globe size={11} className="shrink-0" />
              )}
              <span className="truncate">{displayInfo}</span>
            </div>
          )}
        </div>
      </div>

      {/* Kiro Quota Area */}
      {isKiro && (
        <div className="relative z-10 w-28 flex flex-col gap-1 shrink-0">
          <div className="flex items-center justify-between px-0.5">
            <span className="text-[9px] font-black text-muted-foreground/80 uppercase tracking-tighter">
              Quota
            </span>
            {kiroInfo && !kiroInfo.isBanned && (
              <span className="text-[9px] font-mono text-text-muted/60">
                {formatKiroResetDays(kiroInfo.resetDays, t)}
              </span>
            )}
          </div>
          {kiroInfo ? (
            kiroInfo.isBanned ? (
              <div className="h-2 bg-red-500/20 rounded-full flex items-center justify-center">
                <span className="text-[8px] font-bold text-red-500 uppercase">Banned</span>
              </div>
            ) : (
              <>
                <div className="h-2 bg-muted rounded-full overflow-hidden border border-border/50 p-[1px]">
                  <div
                    className={cn(
                      'h-full rounded-full transition-all duration-1000',
                      kiroInfo.percentage >= 50
                        ? 'bg-emerald-500'
                        : kiroInfo.percentage >= 20
                          ? 'bg-amber-500'
                          : 'bg-red-500',
                    )}
                    style={{
                      width: `${kiroInfo.percentage}%`,
                      boxShadow: `0 0 8px ${kiroInfo.percentage >= 50 ? '#10b98140' : '#f59e0b40'}`,
                    }}
                  />
                </div>
                <div className="flex items-center justify-between px-0.5">
                  <span className="text-[9px] font-mono text-muted-foreground/60">
                    {kiroInfo.available.toFixed(1)}
                  </span>
                  <span className="text-[9px] font-mono text-muted-foreground/40">
                    / {kiroInfo.totalLimit.toFixed(1)}
                  </span>
                </div>
              </>
            )
          ) : (
            <div className="h-1.5 bg-muted rounded-full" />
          )}
        </div>
      )}

      {/* Stats Grid */}
      <div className="relative z-10 flex items-center gap-px bg-muted/50 rounded-xl border border-border/60 p-0.5 backdrop-blur-sm">
        {stats && stats.totalRequests > 0 ? (
          <>
            <div className="flex flex-col items-center min-w-[45px] px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                SR
              </span>
              <span
                className={cn(
                  'font-mono font-black text-[12px]',
                  stats.successRate >= 95
                    ? 'text-emerald-500'
                    : stats.successRate >= 90
                      ? 'text-blue-400'
                      : 'text-amber-500',
                )}
              >
                {Math.round(stats.successRate)}%
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[45px] px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                REQ
              </span>
              <span className="font-mono font-black text-[12px] text-foreground">
                {stats.totalRequests}
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[45px] px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                TKN
              </span>
              <span className="font-mono font-black text-[12px] text-blue-400">
                {formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[55px] px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                COST
              </span>
              <span className="font-mono font-black text-[12px] text-purple-400">
                {formatCost(stats.totalCost)}
              </span>
            </div>
          </>
        ) : (
          <div className="px-6 py-2 flex items-center gap-2 text-muted-foreground/30">
            <Activity size={12} />
            <span className="text-[10px] font-bold uppercase tracking-widest">No Activity</span>
          </div>
        )}
      </div>

      {/* Navigation Icon */}
      <div className="relative z-10 shrink-0 ml-1">
        <div className="p-2 rounded-full text-muted-foreground group-hover:text-foreground group-hover:bg-muted transition-all transform group-hover:translate-x-1">
          <ChevronRight size={20} />
        </div>
      </div>
    </div>
  );
}
