import { useMemo, useState, type KeyboardEvent, type MouseEvent } from 'react';
import { Activity, Ban, Check, Copy, Globe, Mail, Repeat2, Share2, Snowflake } from 'lucide-react';
import { CooldownTimer } from '@/components/cooldown-timer';
import { useCooldowns } from '@/hooks/use-cooldowns';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';

import { ClientIcon } from '@/components/icons/client-icons';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { MarqueeBackground } from '@/components/ui/marquee-background';
import type {
  Provider,
  ProviderStats,
  AntigravityQuotaData,
  KiroQuotaData,
  CodexQuotaData,
  ModelMapping,
} from '@/lib/transport';
import { getProviderTypeConfig } from '../types';
import { cn } from '@/lib/utils';
import { useAntigravityQuotaFromContext } from '@/contexts/antigravity-quotas-context';
import { useCodexQuotaFromContext } from '@/contexts/codex-quotas-context';
import { useKiroQuota } from '@/hooks/queries';
import { useTranslation } from 'react-i18next';
import { buildCustomProviderShareCommand } from '../utils/bulk-custom-provider-import';

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

interface ProviderRowProps {
  provider: Provider;
  stats?: ProviderStats;
  statsLoading?: boolean;
  streamingCount: number;
  providerModelMappings?: ModelMapping[];
  onClick?: () => void;
  title?: string;
  className?: string;
}

// 获取 Claude 模型额度百分比和重置时间
function getClaudeQuotaInfo(
  quota: AntigravityQuotaData | undefined,
): { percentage: number; resetTime: string; lastUpdated: number } | null {
  if (!quota || quota.isForbidden || !quota.models) return null;
  const claudeModel = quota.models.find((m) => m.name.toLowerCase().includes('claude'));
  if (!claudeModel) return null;
  return {
    percentage: claudeModel.percentage,
    resetTime: claudeModel.resetTime,
    lastUpdated: quota.lastUpdated,
  };
}

// 获取 Image 模型额度百分比
function getImageQuotaInfo(
  quota: AntigravityQuotaData | undefined,
): { percentage: number; resetTime: string } | null {
  if (!quota || quota.isForbidden || !quota.models) return null;
  const imageModel = quota.models.find((m) => m.name.toLowerCase().includes('image'));
  if (!imageModel) return null;
  return {
    percentage: imageModel.percentage,
    resetTime: imageModel.resetTime,
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

// 格式化 lastUpdated 为相对时间
function formatLastUpdated(timestamp: number, t: (key: string) => string): string {
  if (!timestamp) return '';
  const now = Date.now();
  const diff = now - timestamp * 1000;
  const minutes = Math.floor(diff / (1000 * 60));

  if (minutes < 1) return t('common.now');
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
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

// 获取 Codex 5H 配额信息 (Primary Window)
function getCodex5HQuotaInfo(
  quota: CodexQuotaData | undefined,
): { percentage: number; resetTime: string; lastUpdated: number } | null {
  if (!quota || quota.isForbidden) return null;
  const primary = quota.primaryWindow;
  if (!primary) return null;

  // Calculate remaining percentage
  const usedPercent = primary.usedPercent ?? 0;
  const percentage = Math.round(100 - usedPercent);

  // Calculate reset time
  let resetTime = '';
  if (primary.resetAt && primary.resetAt > 0) {
    resetTime = new Date(primary.resetAt * 1000).toISOString();
  } else if (primary.resetAfterSeconds && primary.resetAfterSeconds > 0) {
    resetTime = new Date(Date.now() + primary.resetAfterSeconds * 1000).toISOString();
  }

  return {
    percentage,
    resetTime,
    lastUpdated: quota.lastUpdated,
  };
}

// 获取 Codex 周限配额信息 (Secondary Window)
function getCodexWeekQuotaInfo(
  quota: CodexQuotaData | undefined,
): { percentage: number; resetTime: string } | null {
  if (!quota || quota.isForbidden) return null;
  const secondary = quota.secondaryWindow;
  if (!secondary) return null;

  // Calculate remaining percentage
  const usedPercent = secondary.usedPercent ?? 0;
  const percentage = Math.round(100 - usedPercent);

  // Calculate reset time
  let resetTime = '';
  if (secondary.resetAt && secondary.resetAt > 0) {
    resetTime = new Date(secondary.resetAt * 1000).toISOString();
  } else if (secondary.resetAfterSeconds && secondary.resetAfterSeconds > 0) {
    resetTime = new Date(Date.now() + secondary.resetAfterSeconds * 1000).toISOString();
  }

  return {
    percentage,
    resetTime,
  };
}

export function ProviderRow({
  provider,
  stats,
  statsLoading = false,
  streamingCount,
  providerModelMappings = [],
  onClick,
  title,
  className,
}: ProviderRowProps) {
  const { t } = useTranslation();
  const [shareCommandOpen, setShareCommandOpen] = useState(false);
  const [shareCommandCopied, setShareCommandCopied] = useState(false);
  // 使用通用配置系统
  const typeConfig = getProviderTypeConfig(provider.type);
  const color = typeConfig.color;
  const displayInfo = provider.blackBox
    ? t('provider.blackBoxDisplayInfo')
    : typeConfig.getDisplayInfo(provider);

  const isAntigravity = provider.type === 'antigravity';
  const isKiro = provider.type === 'kiro';
  const isCodex = provider.type === 'codex';

  // 从批量查询上下文获取 Antigravity 额度
  const antigravityQuota = useAntigravityQuotaFromContext(provider.id);
  const claudeInfo = isAntigravity ? getClaudeQuotaInfo(antigravityQuota) : null;
  const imageInfo = isAntigravity ? getImageQuotaInfo(antigravityQuota) : null;

  // 仅为 Kiro provider 获取额度
  const { data: kiroQuota } = useKiroQuota(provider.id, isKiro);
  const kiroInfo = isKiro ? getKiroQuotaInfo(kiroQuota) : null;

  // 从批量查询上下文获取 Codex 额度
  const codexQuota = useCodexQuotaFromContext(provider.id);
  const codex5HInfo = isCodex ? getCodex5HQuotaInfo(codexQuota) : null;
  const codexWeekInfo = isCodex ? getCodexWeekQuotaInfo(codexQuota) : null;

  const { getCooldownsForProvider, getProviderHealthLevel } = useCooldowns();
  const providerCooldowns = getCooldownsForProvider(provider.id);
  const healthLevel = getProviderHealthLevel(provider.id);
  const worstCooldown = providerCooldowns[0];
  const modelCooldowns = providerCooldowns.filter((cd) => cd.model);
  const shareCommand = useMemo(
    () => buildCustomProviderShareCommand(provider, { providerModelMappings }),
    [provider, providerModelMappings],
  );
  const canShareCommand = provider.type === 'custom' && !!shareCommand && !provider.blackBox;

  const isInteractive = !!onClick;
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!onClick) return;

    if (event.key === 'Enter') {
      onClick();
      return;
    }

    if (event.key === ' ') {
      event.preventDefault();
      onClick();
    }
  };

  const stopRowClick = (event: MouseEvent) => {
    event.stopPropagation();
  };

  const handleCopyShareCommand = async () => {
    if (!shareCommand || typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
      return;
    }
    await navigator.clipboard.writeText(shareCommand);
    setShareCommandCopied(true);
    window.setTimeout(() => setShareCommandCopied(false), 1200);
  };

  return (
    <div
      onClick={onClick}
      onKeyDown={isInteractive ? handleKeyDown : undefined}
      role={isInteractive ? 'button' : undefined}
      tabIndex={isInteractive ? 0 : undefined}
      title={title}
      className={cn(
        'group relative flex items-center gap-4 p-3 rounded-xl border transition-all duration-300 overflow-hidden',
        isInteractive ? 'cursor-pointer' : 'cursor-default opacity-90',
        healthLevel === 'frozen' && 'opacity-50',
        healthLevel === 'limited' && 'opacity-75',
        streamingCount > 0
          ? 'bg-card ring-1 ring-black/5 dark:ring-white/10'
          : isInteractive
            ? 'bg-card/60 border-border hover:bg-card hover:border-primary/40 hover:shadow-[0_0_15px_rgba(var(--primary-rgb),0.15)] hover:scale-[1.01] shadow-sm'
            : 'bg-card/60 border-border shadow-sm',
        className,
      )}
      style={{
        borderColor:
          streamingCount > 0
            ? `${color}60`
            : healthLevel === 'frozen'
              ? 'rgb(6 182 212 / 0.3)'
              : healthLevel === 'limited'
                ? 'rgb(234 179 8 / 0.3)'
                : healthLevel === 'degraded'
                  ? 'rgb(249 115 22 / 0.2)'
                  : undefined,
        boxShadow: streamingCount > 0 ? `0 0 20px ${color}15` : undefined,
      }}
    >
      <MarqueeBackground show={streamingCount > 0} color={`${color}15`} opacity={0.4} />

      {/* Streaming Badge - 右上角 */}
      {streamingCount > 0 && (
        <div className="absolute top-0 right-0 z-20">
          <StreamingBadge
            count={streamingCount}
            color={color}
            variant="corner"
            className="rounded-tr-xl rounded-bl-lg"
          />
        </div>
      )}

      {/* Supported Clients - 左侧一排重叠居中 */}
      <div className="relative z-10 flex shrink-0 items-center justify-center w-[50px] md:w-[80px]">
        {provider.supportedClientTypes?.length > 0 ? (
          <div
            className="relative flex items-center h-7"
            style={{
              width: `${28 + (Math.min(provider.supportedClientTypes.length, 4) - 1) * 18}px`,
            }}
          >
            {provider.supportedClientTypes.slice(0, 4).map((ct, index) => (
              <div
                key={ct}
                className="absolute flex h-7 w-7 items-center justify-center rounded-full bg-background ring-1 ring-border transition-all hover:scale-110 hover:z-50"
                style={{
                  left: `${index * 18}px`,
                  zIndex: 4 - index,
                }}
                title={
                  index === 3 && provider.supportedClientTypes.length > 4
                    ? `${ct} +${provider.supportedClientTypes.length - 4}`
                    : ct
                }
              >
                <ClientIcon type={ct} size={16} />
              </div>
            ))}
          </div>
        ) : (
          <span className="text-xs text-muted-foreground font-mono">-</span>
        )}
      </div>

      {/* Provider Info */}
      <div className="relative z-10 flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <h3 className="text-[15px] font-bold text-foreground truncate">{provider.name}</h3>
          {provider.blackBox && (
            <span className="shrink-0 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-zinc-500/10 text-zinc-500 border border-zinc-500/20">
              {t('provider.blackBoxBadge')}
            </span>
          )}
        </div>
        <div className="flex items-center gap-3">
          {/* 对于 Antigravity，显示 Claude 和 Imagen Quota */}
          {isAntigravity && (claudeInfo || imageInfo) ? (
            <div className="flex items-center gap-3 shrink-0">
              {/* Claude Quota */}
              {claudeInfo && (
                <div className="flex items-center gap-1.5">
                  <span className="text-[9px] font-black text-muted-foreground/60 uppercase">
                    Claude
                  </span>
                  <div className="w-16 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
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
              )}
              {/* Image Quota */}
              {imageInfo && (
                <div className="flex items-center gap-1.5">
                  <span className="text-[9px] font-black text-muted-foreground/60 uppercase">
                    {t('common.image')}
                  </span>
                  <div className="w-16 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
                    <div
                      className={cn(
                        'h-full rounded-full transition-all duration-1000',
                        imageInfo.percentage >= 50
                          ? 'bg-emerald-500'
                          : imageInfo.percentage >= 20
                            ? 'bg-amber-500'
                            : 'bg-red-500',
                      )}
                      style={{ width: `${imageInfo.percentage}%` }}
                    />
                  </div>
                  <span className="text-[9px] font-mono text-muted-foreground/60">
                    {formatResetTime(imageInfo.resetTime, t)}
                  </span>
                </div>
              )}
              {/* Last Updated */}
              {claudeInfo && (
                <span
                  className="text-[8px] font-mono text-muted-foreground/40"
                  title={t('common.lastUpdated')}
                >
                  @{formatLastUpdated(claudeInfo.lastUpdated, t)}
                </span>
              )}
            </div>
          ) : isCodex && (codex5HInfo || codexWeekInfo) ? (
            <div className="flex items-center gap-3 shrink-0">
              {/* 5H Quota */}
              {codex5HInfo && (
                <div className="flex items-center gap-1.5">
                  <span className="text-[9px] font-black text-muted-foreground/60 uppercase">
                    5H
                  </span>
                  <div className="w-16 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
                    <div
                      className={cn(
                        'h-full rounded-full transition-all duration-1000',
                        codex5HInfo.percentage >= 50
                          ? 'bg-emerald-500'
                          : codex5HInfo.percentage >= 20
                            ? 'bg-amber-500'
                            : 'bg-red-500',
                      )}
                      style={{ width: `${codex5HInfo.percentage}%` }}
                    />
                  </div>
                  {codex5HInfo.resetTime && (
                    <span className="text-[9px] font-mono text-muted-foreground/60">
                      {formatResetTime(codex5HInfo.resetTime, t)}
                    </span>
                  )}
                </div>
              )}
              {/* Week Quota */}
              {codexWeekInfo && (
                <div className="flex items-center gap-1.5">
                  <span className="text-[9px] font-black text-muted-foreground/60 uppercase">
                    {t('common.week')}
                  </span>
                  <div className="w-16 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
                    <div
                      className={cn(
                        'h-full rounded-full transition-all duration-1000',
                        codexWeekInfo.percentage >= 50
                          ? 'bg-emerald-500'
                          : codexWeekInfo.percentage >= 20
                            ? 'bg-amber-500'
                            : 'bg-red-500',
                      )}
                      style={{ width: `${codexWeekInfo.percentage}%` }}
                    />
                  </div>
                  {codexWeekInfo.resetTime && (
                    <span className="text-[9px] font-mono text-muted-foreground/60">
                      {formatResetTime(codexWeekInfo.resetTime, t)}
                    </span>
                  )}
                </div>
              )}
              {/* Last Updated */}
              {codex5HInfo && (
                <span
                  className="text-[8px] font-mono text-muted-foreground/40"
                  title={t('common.lastUpdated')}
                >
                  @{formatLastUpdated(codex5HInfo.lastUpdated, t)}
                </span>
              )}
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
      {(provider.config?.disableErrorCooldown || provider.config?.smartMappingRetryEnabled) && (
        <div className="relative z-10 flex shrink-0 items-center self-stretch gap-1">
          {provider.config?.disableErrorCooldown && (
            <span
              className="inline-flex h-5 w-5 items-center justify-center rounded-full border border-amber-500/40 bg-amber-500/15 text-amber-700 shadow-[0_0_0_1px_rgba(245,158,11,0.08)] dark:text-amber-200"
              title={t('provider.disableSwitchBadgeDesc')}
              aria-label={t('provider.disableSwitchBadgeDesc')}
            >
              <span className="flex h-3.5 w-3.5 items-center justify-center rounded-full bg-amber-500 text-background shadow-sm">
                <Ban className="h-2.5 w-2.5 stroke-[3]" />
              </span>
            </span>
          )}
          {provider.config?.disableErrorCooldown && provider.config?.smartMappingRetryEnabled && (
            <span
              className="inline-flex h-5 w-5 items-center justify-center rounded-full border border-sky-500/40 bg-sky-500/15 text-sky-700 shadow-[0_0_0_1px_rgba(14,165,233,0.08)] dark:text-sky-200"
              title={t('provider.smartMappingRetryBadgeDesc', {
                count: provider.config.smartMappingRetryLimit ?? 1,
              })}
              aria-label={t('provider.smartMappingRetryBadgeDesc', {
                count: provider.config.smartMappingRetryLimit ?? 1,
              })}
            >
              <span className="flex h-3.5 w-3.5 items-center justify-center rounded-full bg-sky-500 text-background shadow-sm">
                <Repeat2 className="h-2.5 w-2.5 stroke-[3]" />
              </span>
            </span>
          )}
        </div>
      )}

      {/* Kiro Quota Area */}
      {isKiro && (
        <div className="relative z-10 w-28 flex flex-col gap-1 shrink-0">
          <div className="flex items-center justify-between px-0.5">
            <span className="text-[9px] font-black text-muted-foreground/80 uppercase tracking-tighter">
              {t('providers.quota')}
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
                <span className="text-[8px] font-bold text-red-500 uppercase">
                  {t('providers.banned')}
                </span>
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

      {/* Cooldown Status */}
      {healthLevel !== 'healthy' && (
        <div className="relative z-10 flex items-center gap-1.5 shrink-0">
          {healthLevel === 'frozen' && worstCooldown && (
            <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-cyan-500/10 border border-cyan-500/20">
              <Snowflake size={12} className="text-cyan-500 animate-pulse" />
              <CooldownTimer
                cooldown={worstCooldown}
                className="text-[11px] font-mono font-bold text-cyan-500 tabular-nums"
              />
            </div>
          )}
          {healthLevel === 'limited' && worstCooldown && (
            <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-yellow-500/10 border border-yellow-500/20">
              <Snowflake size={12} className="text-yellow-500" />
              <CooldownTimer
                cooldown={worstCooldown}
                className="text-[11px] font-mono font-bold text-yellow-500 tabular-nums"
              />
            </div>
          )}
          {healthLevel === 'degraded' && modelCooldowns.length > 0 && (
            <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-orange-500/10 border border-orange-500/20">
              <Snowflake size={12} className="text-orange-500 shrink-0" />
              <div className="flex items-center gap-1.5 flex-wrap">
                {modelCooldowns.slice(0, 3).map((cd, i) => (
                  <div key={i} className="flex items-center gap-1">
                    <span className="text-[10px] font-mono text-orange-400 truncate max-w-[120px]">
                      {cd.model}
                    </span>
                    <CooldownTimer
                      cooldown={cd}
                      className="text-[10px] font-mono font-bold text-orange-500 tabular-nums"
                    />
                  </div>
                ))}
                {modelCooldowns.length > 3 && (
                  <span className="text-[10px] text-orange-400">+{modelCooldowns.length - 3}</span>
                )}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Custom Provider Share Command */}
      {provider.type === 'custom' && (
        <div className="relative z-10 shrink-0" onClick={stopRowClick} onMouseDown={stopRowClick}>
          <Dialog open={shareCommandOpen} onOpenChange={setShareCommandOpen}>
            <DialogTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon-sm"
                  disabled={!canShareCommand}
                  aria-label={t('provider.shareCommand')}
                  title={
                    provider.blackBox
                      ? t('provider.shareCommandBlackBoxDisabled')
                      : t('provider.shareCommand')
                  }
                />
              }
            >
              <Share2 className="h-3.5 w-3.5" />
            </DialogTrigger>
            <DialogContent className="grid-cols-[minmax(0,1fr)] sm:max-w-2xl">
              <DialogHeader>
                <DialogTitle>{t('provider.shareCommandTitle')}</DialogTitle>
                <DialogDescription>{t('provider.shareCommandDesc')}</DialogDescription>
              </DialogHeader>
              <div className="space-y-3 min-w-0">
                {provider.excludeFromExport && (
                  <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                    {t('provider.shareCommandHiddenSecretHint')}
                  </div>
                )}
                <pre className="max-h-64 overflow-auto rounded-lg border bg-muted/50 p-3 text-xs leading-relaxed text-foreground whitespace-pre-wrap break-all">
                  {shareCommand}
                </pre>
                <p className="text-xs text-muted-foreground">
                  {t('provider.shareCommandApiKeyHint')}
                </p>
              </div>
              <DialogFooter>
                <Button type="button" className="gap-2" onClick={handleCopyShareCommand}>
                  {shareCommandCopied ? (
                    <Check className="h-4 w-4" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                  {shareCommandCopied ? t('proxy.copied') : t('provider.copyShareCommand')}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      )}

      {/* Stats Grid */}
      <div className="relative z-10 flex items-center gap-px bg-muted/50 rounded-xl border border-border/60 p-0.5 backdrop-blur-sm shrink-0">
        {stats && stats.totalRequests > 0 ? (
          <>
            <div className="flex flex-col items-center min-w-[36px] md:min-w-[45px] px-1 md:px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                SR
              </span>
              <span
                className={cn(
                  'font-mono font-black text-[11px] md:text-[12px]',
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
            <div className="flex flex-col items-center min-w-[36px] md:min-w-[45px] px-1 md:px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                REQ
              </span>
              <span className="font-mono font-black text-[11px] md:text-[12px] text-foreground">
                {stats.totalRequests}
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[36px] md:min-w-[45px] px-1 md:px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                TKN
              </span>
              <span className="font-mono font-black text-[11px] md:text-[12px] text-blue-400">
                {formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[40px] md:min-w-[55px] px-1 md:px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                {t('common.cost')}
              </span>
              <span className="font-mono font-black text-[11px] md:text-[12px] text-purple-400">
                {formatCost(stats.totalCost)}
              </span>
            </div>
          </>
        ) : statsLoading ? (
          <div className="px-6 py-2 flex items-center gap-2 text-muted-foreground/50">
            <Activity size={12} className="animate-pulse" />
            <span className="text-[10px] font-bold uppercase tracking-widest">—</span>
          </div>
        ) : (
          <div className="px-6 py-2 flex items-center gap-2 text-muted-foreground/30">
            <Activity size={12} />
            <span className="text-[10px] font-bold uppercase tracking-widest">
              {t('common.noActivity')}
            </span>
          </div>
        )}
      </div>
    </div>
  );
}
