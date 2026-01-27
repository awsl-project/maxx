import { GripVertical, Zap, RefreshCw, Activity, Snowflake, Info } from 'lucide-react';
import { Button, Switch } from '@/components/ui';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { MarqueeBackground } from '@/components/ui/marquee-background';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { getProviderColorVar, type ProviderType } from '@/lib/theme';
import { cn } from '@/lib/utils';
import type { ClientType, ProviderStats, AntigravityQuotaData } from '@/lib/transport';
import type { ProviderConfigItem } from '../types';
import { useAntigravityQuotaFromContext } from '@/contexts/antigravity-quotas-context';
import { useCooldownsContext } from '@/contexts/cooldowns-context';
import { ProviderDetailsDialog } from '@/components/provider-details-dialog';
import { useState, useEffect } from 'react';
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

// Sortable Provider Row
type SortableProviderRowProps = {
  item: ProviderConfigItem;
  index: number;
  clientType: ClientType;
  streamingCount: number;
  stats?: ProviderStats;
  isToggling: boolean;
  onToggle: () => void;
  onDelete?: () => void;
};

export function SortableProviderRow({
  item,
  index,
  clientType,
  streamingCount,
  stats,
  isToggling,
  onToggle,
  onDelete,
}: SortableProviderRowProps) {
  const [showDetailsDialog, setShowDetailsDialog] = useState(false);
  const { getCooldownForProvider, clearCooldown, isClearingCooldown } = useCooldownsContext();
  const cooldown = getCooldownForProvider(item.provider.id, clientType);

  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: item.id,
    transition: {
      duration: 200,
      easing: 'ease',
    },
  });

  const style: React.CSSProperties = {
    transform: transform ? CSS.Translate.toString(transform) : undefined,
    transition,
    opacity: isDragging ? 0 : 1,
    pointerEvents: isDragging ? 'none' : undefined,
  };

  const handleRowClick = (e: React.MouseEvent) => {
    // 所有状态都打开详情弹窗
    e.stopPropagation();
    setShowDetailsDialog(true);
  };

  const handleClearCooldown = () => {
    clearCooldown(item.provider.id);
  };

  return (
    <>
      <div ref={setNodeRef} style={style} {...attributes}>
        <ProviderRowContent
          item={item}
          index={index}
          clientType={clientType}
          streamingCount={streamingCount}
          stats={stats}
          isToggling={isToggling}
          onToggle={onToggle}
          onRowClick={handleRowClick}
          isInCooldown={!!cooldown}
          dragHandleListeners={listeners}
          onClearCooldown={handleClearCooldown}
          isClearingCooldown={isClearingCooldown}
        />
      </div>

      {/* Provider Details Dialog */}
      <ProviderDetailsDialog
        item={item}
        clientType={clientType}
        open={showDetailsDialog}
        onOpenChange={setShowDetailsDialog}
        stats={stats}
        cooldown={cooldown || null}
        streamingCount={streamingCount}
        onToggle={onToggle}
        isToggling={isToggling}
        onDelete={onDelete}
        onClearCooldown={handleClearCooldown}
        isClearingCooldown={isClearingCooldown}
      />
    </>
  );
}

// Provider Row Content (used both in sortable and overlay)
type ProviderRowContentProps = {
  item: ProviderConfigItem;
  index: number;
  clientType: ClientType;
  streamingCount: number;
  stats?: ProviderStats;
  isToggling: boolean;
  isOverlay?: boolean;
  onToggle: () => void;
  onRowClick?: (e: React.MouseEvent) => void;
  isInCooldown?: boolean;
  dragHandleListeners?: any;
  onClearCooldown?: () => void;
  isClearingCooldown?: boolean;
};

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

export function ProviderRowContent({
  item,
  index,
  clientType,
  streamingCount,
  stats,
  isToggling,
  isOverlay: _isOverlay, // eslint-disable-line @typescript-eslint/no-unused-vars
  onToggle,
  onRowClick,
  isInCooldown: isInCooldownProp,
  dragHandleListeners,
  onClearCooldown,
  isClearingCooldown,
}: ProviderRowContentProps) {
  const { t } = useTranslation();
  const { provider, enabled, isNative } = item;
  const color = getProviderColorVar(provider.type as ProviderType);
  const isAntigravity = provider.type === 'antigravity';

  // 从批量查询上下文获取 Antigravity 额度
  const quota = useAntigravityQuotaFromContext(provider.id);
  const claudeInfo = isAntigravity ? getClaudeQuotaInfo(quota) : null;
  const imageInfo = isAntigravity ? getImageQuotaInfo(quota) : null;

  // 获取 cooldown 状态
  const { getCooldownForProvider, formatRemaining, getRemainingSeconds } = useCooldownsContext();
  const cooldown = getCooldownForProvider(provider.id, clientType);
  const isInCooldown = isInCooldownProp ?? !!cooldown;

  // 实时倒计时状态
  const [liveCountdown, setLiveCountdown] = useState<string>('');
  // 本地过期状态，用于在倒计时结束时立即更新 UI
  const [isLocalExpired, setIsLocalExpired] = useState(false);

  // 每秒更新倒计时 (使用递归 setTimeout 而不是 setInterval)
  useEffect(() => {
    if (!cooldown) {
      setLiveCountdown('');
      setIsLocalExpired(false);
      return;
    }

    // Reset local expired state when cooldown changes
    setIsLocalExpired(false);

    let timeoutId: ReturnType<typeof setTimeout>;

    const tick = () => {
      const remaining = getRemainingSeconds(cooldown);
      if (remaining <= 0) {
        // Cooldown expired, clear the countdown display and mark as locally expired
        setLiveCountdown('');
        setIsLocalExpired(true);
        return;
      }
      setLiveCountdown(formatRemaining(cooldown));
      // Schedule next tick
      timeoutId = setTimeout(tick, 1000);
    };

    // Start immediately
    tick();

    return () => {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
    };
  }, [cooldown, formatRemaining, getRemainingSeconds]);

  // 如果本地已过期，则不显示 cooldown 状态
  const effectiveIsInCooldown = isInCooldown && !isLocalExpired;

  const handleContentClick = (e: React.MouseEvent) => {
    // 所有状态都打开详情弹窗
    onRowClick?.(e);
  };

  return (
    <Button
      variant={null}
      onClick={handleContentClick}
      className={cn(
        'group relative flex items-center gap-4 p-3 rounded-xl border transition-all duration-300 overflow-hidden w-full h-auto cursor-pointer active:cursor-grab',
        effectiveIsInCooldown
          ? 'bg-transparent border-slate-400/50 dark:border-slate-500/40 hover:bg-slate-200/50 dark:hover:bg-slate-700/30 hover:border-slate-500 dark:hover:border-slate-400 hover:shadow-md'
          : enabled
            ? streamingCount > 0
              ? 'bg-accent/5 border-transparent ring-1 ring-black/5 dark:ring-white/10'
              : 'bg-card/60 border-border hover:border-emerald-500/30 hover:bg-card shadow-sm'
            : 'bg-muted/40 border-dashed border-border opacity-70 grayscale-[0.5] hover:opacity-100 hover:grayscale-0',
      )}
      style={{
        borderColor: !effectiveIsInCooldown && enabled && streamingCount > 0 ? `${color}40` : undefined,
        boxShadow:
          !effectiveIsInCooldown && enabled && streamingCount > 0 ? `0 0 20px ${color}15` : undefined,
      }}
      {...dragHandleListeners}
    >
      <MarqueeBackground
        show={streamingCount > 0 && enabled && !effectiveIsInCooldown}
        color={`${color}15`}
        opacity={0.4}
      />

      {/* Cooldown 冰冻效果 - 落雪 */}
      {effectiveIsInCooldown && (
        <>
          {/* 雪花动画 (CSS Background) - z-0 置于所有元素后面 */}
          <div className="absolute inset-0 z-0 animate-snowing pointer-events-none opacity-80" />
          <div className="absolute inset-0 z-0 animate-snowing-secondary pointer-events-none opacity-80" />
        </>
      )}

      {/* Drag Handle & Index */}
      <div className="relative z-10 flex flex-col items-center gap-1.5 w-7 shrink-0">
        <div className="p-1 rounded-md hover:bg-accent transition-colors">
          <GripVertical
            size={14}
            className="text-muted-foreground group-hover:text-muted-foreground"
          />
        </div>
        <span
          className="text-[10px] font-mono font-bold w-5 h-5 flex items-center justify-center rounded-full border border-border bg-muted shadow-inner"
          style={{ color: enabled ? color : 'var(--color-text-muted)' }}
        >
          {index + 1}
        </span>
      </div>

      {/* Provider Main Info */}
      <div className="relative z-10 flex items-center gap-3 flex-1 min-w-0">
        {/* Icon */}
        <div
          className={cn(
            'relative w-11 h-11 rounded-xl flex items-center justify-center shrink-0 transition-all duration-500 overflow-hidden',
            effectiveIsInCooldown
              ? 'bg-slate-200 dark:bg-slate-800 border border-slate-400/30 dark:border-slate-600/30'
              : 'bg-muted border border-border shadow-inner',
          )}
          style={!effectiveIsInCooldown && enabled ? { color } : {}}
        >
          <span
            className={cn(
              'text-xl font-black transition-all',
              effectiveIsInCooldown
                ? 'opacity-0'
                : enabled
                  ? 'scale-100'
                  : 'opacity-30 grayscale',
            )}
          >
            {provider.name.charAt(0).toUpperCase()}
          </span>
          {effectiveIsInCooldown && (
            <Snowflake
              size={22}
              className="absolute text-slate-500/70 dark:text-white/70 animate-pulse drop-shadow-[0_0_8px_rgba(100,116,139,0.4)] dark:drop-shadow-[0_0_8px_rgba(255,255,255,0.4)]"
            />
          )}
          {enabled && streamingCount > 0 && !effectiveIsInCooldown && (
            <div className="absolute inset-0 bg-black/5 dark:bg-white/5 animate-pulse" />
          )}
        </div>

        {/* Text Info */}
        <div className="flex flex-col min-w-0">
          <div className="flex items-center gap-2">
            <span
              className={cn(
                'text-[14px] font-bold truncate transition-colors',
                effectiveIsInCooldown
                  ? 'text-foreground'
                  : enabled
                    ? 'text-foreground'
                    : 'text-muted-foreground',
              )}
            >
              {provider.name}
            </span>
            {/* Badges */}
            <div className="flex items-center gap-1.5 shrink-0">
              {isNative ? (
                <span className="flex items-center gap-1 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/10 text-emerald-500 border border-emerald-500/20">
                  <Zap size={10} className="fill-emerald-500/20" /> NATIVE
                </span>
              ) : (
                <span className="flex items-center gap-1 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-amber-500/10 text-amber-500 border border-amber-500/20">
                  <RefreshCw size={10} /> CONV
                </span>
              )}
            </div>
          </div>
          <div className="flex items-center gap-3">
            {/* 对于 Antigravity，显示 Claude 和 Imagen Quota；对于其他类型，显示 endpoint */}
            {isAntigravity && (claudeInfo || imageInfo) ? (
              <div className={cn('flex items-center gap-3 shrink-0', !enabled && 'opacity-40')}>
                {/* Claude Quota */}
                {claudeInfo && (
                  <div className="flex items-center gap-1.5">
                    <span className="text-[9px] font-black text-muted-foreground/60 uppercase">
                      Claude
                    </span>
                    <div className="w-14 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
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
                    <div className="w-14 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
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
            ) : (
              <div
                className={cn(
                  'text-[11px] font-medium truncate flex items-center gap-1',
                  effectiveIsInCooldown
                    ? 'text-muted-foreground'
                    : enabled
                      ? 'text-muted-foreground'
                      : 'text-muted-foreground/50',
                )}
              >
                <Info size={10} className="shrink-0" />
                {provider.config?.custom?.clientBaseURL?.[clientType] ||
                  provider.config?.custom?.baseURL ||
                  provider.config?.antigravity?.endpoint ||
                  t('provider.defaultEndpoint')}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Quota & Center Countdown Area */}
      <div className="relative z-10 flex items-center gap-4 shrink-0">
        {/* Center-placed Countdown (when in cooldown) or Stats Grid */}
        {effectiveIsInCooldown && cooldown ? (
          <button
            onClick={(e) => {
              e.stopPropagation();
              onClearCooldown?.();
            }}
            disabled={isClearingCooldown}
            title={t('provider.clearCooldown')}
            className="flex items-center gap-3 bg-transparent rounded-xl border border-slate-400/50 dark:border-slate-500/40 p-1 px-3 cursor-pointer hover:bg-slate-100/50 dark:hover:bg-slate-700/30 hover:border-slate-500 dark:hover:border-slate-400 transition-all disabled:opacity-50"
          >
            <div className="flex flex-col items-center">
              <span className="text-[8px] font-black text-slate-500 dark:text-slate-400/60 uppercase tracking-tight">
                {t('common.remaining')}
              </span>
              <div className="flex items-center gap-1.5">
                <Snowflake
                  size={12}
                  className="text-slate-500 dark:text-slate-400 animate-spin-slow"
                />
                <span className="text-sm font-mono font-black text-slate-600 dark:text-slate-300">
                  {liveCountdown}
                </span>
              </div>
            </div>
            <div className="w-px h-6 bg-slate-300/40 dark:bg-slate-600/40" />
            <div className="flex flex-col items-center text-slate-500/70 dark:text-slate-400/50">
              <Zap size={14} />
              <span className="text-[8px] font-bold">{t('provider.unfreeze')}</span>
            </div>
          </button>
        ) : (
          <div
            className={cn(
              'flex items-center gap-px bg-muted/50 rounded-xl border border-border/60 p-0.5 backdrop-blur-sm',
              !enabled && 'opacity-40',
            )}
          >
            {stats && stats.totalRequests > 0 ? (
              <>
                {/* Success */}
                <div className="flex flex-col items-center min-w-[50px] px-2 py-1">
                  <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                    SR
                  </span>
                  <span
                    className={cn(
                      'font-mono font-black text-xs',
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
                {/* Tokens */}
                <div className="flex flex-col items-center min-w-[50px] px-2 py-1">
                  <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                    {t('common.tokens')}
                  </span>
                  <span className="font-mono font-black text-xs text-blue-400">
                    {formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}
                  </span>
                </div>
                <div className="w-[1px] h-6 bg-border/40" />
                {/* Cost */}
                <div className="flex flex-col items-center min-w-[60px] px-2 py-1">
                  <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                    {t('common.cost')}
                  </span>
                  <span className="font-mono font-black text-xs text-purple-400">
                    {formatCost(stats.totalCost)}
                  </span>
                </div>
              </>
            ) : (
              <div className="px-6 py-2 flex items-center gap-2 text-muted-foreground/30">
                <Activity size={12} />
                <span className="text-[10px] font-bold uppercase tracking-widest">
                  {t('common.noData')}
                </span>
              </div>
            )}
          </div>
        )}
      </div>
      {/* Streaming Indicator - Inline before Switch */}
      {enabled && streamingCount > 0 && !effectiveIsInCooldown && (
        <div className="relative z-10 flex items-center shrink-0">
          <StreamingBadge count={streamingCount} color={color} />
        </div>
      )}
      {/* Control Area - Switch */}
      <div
        className="relative z-10 flex items-center shrink-0  pl-2"
        onClick={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
      >
        <Switch checked={enabled} onCheckedChange={onToggle} disabled={isToggling} />
      </div>
    </Button>
  );
}
