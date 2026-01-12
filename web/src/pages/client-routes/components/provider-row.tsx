import { GripVertical, Zap, RefreshCw, Activity, Snowflake, Info } from 'lucide-react';
import { Switch } from '@/components/ui';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { getProviderColor } from '@/lib/provider-colors';
import { cn } from '@/lib/utils';
import type { ClientType, ProviderStats, AntigravityQuotaData } from '@/lib/transport';
import type { ProviderConfigItem } from '../types';
import { useAntigravityQuota } from '@/hooks/queries';
import { useCooldowns } from '@/hooks/use-cooldowns';
import { ProviderDetailsDialog } from '@/components/provider-details-dialog';
import { useState, useEffect } from 'react';

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
  const { getCooldownForProvider, clearCooldown, isClearingCooldown } = useCooldowns();
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
      <div
        ref={setNodeRef}
        style={style}
        {...attributes}
        {...listeners}
        className="active:cursor-grabbing"
      >
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
};

// 获取 Claude 模型额度百分比和重置时间
function getClaudeQuotaInfo(quota: AntigravityQuotaData | undefined): { percentage: number; resetTime: string } | null {
  if (!quota || quota.isForbidden) return null;
  const claudeModel = quota.models.find(m => m.name.includes('claude'));
  if (!claudeModel) return null;
  return { percentage: claudeModel.percentage, resetTime: claudeModel.resetTime };
}

// 格式化重置时间
function formatResetTime(resetTime: string): string {
  try {
    const reset = new Date(resetTime);
    const now = new Date();
    const diff = reset.getTime() - now.getTime();

    if (diff <= 0) return 'Soon';

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

export function ProviderRowContent({
  item,
  index,
  clientType,
  streamingCount,
  stats,
  isToggling,
  isOverlay: _isOverlay,
  onToggle,
  onRowClick,
  isInCooldown: isInCooldownProp,
}: ProviderRowContentProps) {
  const { provider, enabled, isNative } = item;
  const color = getProviderColor(provider.type);
  const isAntigravity = provider.type === 'antigravity';

  // 仅为 Antigravity provider 获取额度
  const { data: quota } = useAntigravityQuota(provider.id, isAntigravity);
  const claudeInfo = isAntigravity ? getClaudeQuotaInfo(quota) : null;

  // 获取 cooldown 状态
  const { getCooldownForProvider, formatRemaining } = useCooldowns();
  const cooldown = getCooldownForProvider(provider.id, clientType);
  const isInCooldown = isInCooldownProp ?? !!cooldown;

  // 实时倒计时状态
  const [liveCountdown, setLiveCountdown] = useState<string>('');

  // 每秒更新倒计时
  useEffect(() => {
    if (!cooldown) {
      setLiveCountdown('');
      return;
    }

    // 立即更新一次
    setLiveCountdown(formatRemaining(cooldown));

    // 每秒更新
    const interval = setInterval(() => {
      setLiveCountdown(formatRemaining(cooldown));
    }, 1000);

    return () => clearInterval(interval);
  }, [cooldown, formatRemaining]);

  const handleContentClick = (e: React.MouseEvent) => {
    // 所有状态都打开详情弹窗
    onRowClick?.(e);
  };

  return (
    <div
      onClick={handleContentClick}
      className={cn(
        "group relative flex items-center gap-4 p-3 rounded-xl border transition-all duration-300 overflow-hidden",
        isInCooldown
          ? "bg-gradient-to-r from-cyan-950/40 to-blue-950/40 border-cyan-500/30 shadow-[0_0_20px_rgba(6,182,212,0.1)] cursor-pointer"
          : enabled
          ? streamingCount > 0
            ? "bg-surface-primary border-transparent ring-1 ring-black/5 dark:ring-white/10"
            : "bg-surface-primary/60 border-border hover:border-emerald-500/30 hover:bg-surface-primary shadow-sm cursor-pointer"
          : "bg-surface-secondary/40 border-dashed border-border opacity-70 cursor-pointer grayscale-[0.5] hover:opacity-100 hover:grayscale-0"
      )}
      style={{
        borderColor: !isInCooldown && enabled && streamingCount > 0 ? `${color}40` : undefined,
        boxShadow: !isInCooldown && enabled && streamingCount > 0 ? `0 0 20px ${color}15` : undefined,
      }}
    >
      {/* Marquee 背景动画 (仅在有 streaming 请求时显示) */}
      {streamingCount > 0 && enabled && !isInCooldown && (
        <div
          className="absolute inset-0 animate-marquee pointer-events-none opacity-40"
          style={{ backgroundColor: `${color}15` }}
        />
      )}

      {/* Cooldown 冰冻效果 - 增强版 */}
      {isInCooldown && (
        <>
          <div className="absolute inset-0 bg-gradient-to-br from-cyan-500/5 via-transparent to-blue-600/5 pointer-events-none animate-pulse" />
          <div className="absolute inset-x-0 top-0 h-[1px] bg-gradient-to-r from-transparent via-cyan-400/20 to-transparent" />
          {/* 雪花动画 (CSS Background) */}
          <div className="absolute inset-0 animate-snowing pointer-events-none opacity-60" />
          <div className="absolute inset-0 animate-snowing-secondary pointer-events-none opacity-60" />
        </>
      )}

      {/* Drag Handle & Index */}
      <div className="relative z-10 flex flex-col items-center gap-1.5 w-7 flex-shrink-0">
        <div className="p-1 rounded-md hover:bg-surface-hover transition-colors">
          <GripVertical size={14} className="text-text-muted group-hover:text-text-secondary" />
        </div>
        <span 
          className="text-[10px] font-mono font-bold w-5 h-5 flex items-center justify-center rounded-full border border-border bg-surface-secondary shadow-inner"
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
            "relative w-11 h-11 rounded-xl flex items-center justify-center flex-shrink-0 transition-all duration-500 overflow-hidden",
            isInCooldown ? "bg-cyan-900/40 border border-cyan-500/30" : "bg-surface-secondary border border-border shadow-inner"
          )}
          style={!isInCooldown && enabled ? { color } : {}}
        >
          <span className={cn(
            "text-xl font-black transition-all",
            isInCooldown ? "text-cyan-400 opacity-20 scale-150 blur-[1px]" : enabled ? "scale-100" : "opacity-30 grayscale"
          )}>
            {provider.name.charAt(0).toUpperCase()}
          </span>
          {isInCooldown && (
            <Snowflake size={22} className="absolute text-cyan-400 animate-pulse drop-shadow-[0_0_8px_rgba(34,211,238,0.5)]" />
          )}
          {enabled && streamingCount > 0 && !isInCooldown && (
            <div className="absolute inset-0 bg-black/5 dark:bg-white/5 animate-pulse" />
          )}
        </div>

        {/* Text Info */}
        <div className="flex flex-col min-w-0">
          <div className="flex items-center gap-2">
            <span className={cn(
              "text-[14px] font-bold truncate transition-colors",
              isInCooldown ? "text-cyan-200" : enabled ? "text-text-primary" : "text-text-muted"
            )}>
              {provider.name}
            </span>
            {/* Badges */}
            <div className="flex items-center gap-1.5 flex-shrink-0">
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
          <div className={cn(
            "text-[11px] font-medium truncate flex items-center gap-1",
            isInCooldown ? "text-cyan-500/70" : enabled ? "text-text-muted" : "text-text-muted/50"
          )}>
            <Info size={10} className="shrink-0" />
            {provider.config?.custom?.clientBaseURL?.[clientType] ||
              provider.config?.custom?.baseURL ||
              provider.config?.antigravity?.endpoint ||
              'Default endpoint'}
          </div>
        </div>
      </div>

      {/* Quota & Center Countdown Area */}
      <div className="relative z-10 flex items-center gap-4 flex-shrink-0">
        {/* Claude Quota */}
        {isAntigravity && (
          <div className={cn("w-24 flex flex-col gap-1", !enabled && "opacity-40")}>
             <div className="flex items-center justify-between px-0.5">
               <span className="text-[9px] font-black text-text-muted/80 uppercase tracking-tighter">Claude</span>
               {claudeInfo && <span className="text-[9px] font-mono text-text-muted/60">{formatResetTime(claudeInfo.resetTime)}</span>}
             </div>
             {claudeInfo ? (
               <div className="h-2 bg-surface-secondary rounded-full overflow-hidden border border-border/50 p-[1px]">
                 <div
                   className={cn(
                     "h-full rounded-full transition-all duration-1000",
                     claudeInfo.percentage >= 50 ? "bg-emerald-500" :
                     claudeInfo.percentage >= 20 ? "bg-amber-500" : "bg-red-500"
                   )}
                   style={{ width: `${claudeInfo.percentage}%`, boxShadow: `0 0 8px ${claudeInfo.percentage >= 50 ? '#10b98140' : '#f59e0b40'}` }}
                 />
               </div>
             ) : <div className="h-1.5 bg-surface-secondary rounded-full" />}
          </div>
        )}

        {/* Center-placed Countdown (when in cooldown) or Stats Grid */}
        {isInCooldown && cooldown ? (
          <div
            className="flex items-center gap-3 bg-cyan-950/40 rounded-xl border border-cyan-500/40 p-1 px-3 backdrop-blur-md shadow-[0_0_15px_rgba(6,182,212,0.1)]"
          >
            <div className="flex flex-col items-center">
              <span className="text-[8px] font-black text-cyan-500/60 uppercase tracking-tight">Remaining</span>
              <div className="flex items-center gap-1.5">
                <Snowflake size={12} className="text-cyan-400 animate-spin-slow" />
                <span className="text-sm font-mono font-black text-cyan-400">{liveCountdown}</span>
              </div>
            </div>
            <div className="w-px h-6 bg-cyan-500/20" />
            <div className="flex flex-col items-center text-cyan-500/40">
               <Zap size={14} />
               <span className="text-[8px] font-bold">FROZEN</span>
            </div>
          </div>
        ) : (
          <div className={cn(
            "flex items-center gap-px bg-surface-secondary/50 rounded-xl border border-border/60 p-0.5 backdrop-blur-sm",
            !enabled && "opacity-40"
          )}>
            {stats && stats.totalRequests > 0 ? (
              <>
                {/* Success */}
                <div className="flex flex-col items-center min-w-[50px] px-2 py-1">
                  <span className="text-[8px] font-bold text-text-muted uppercase tracking-tight">SR</span>
                  <span className={cn(
                    "font-mono font-black text-xs",
                    stats.successRate >= 95 ? "text-emerald-500" :
                    stats.successRate >= 90 ? "text-blue-400" : "text-amber-500"
                  )}>
                    {Math.round(stats.successRate)}%
                  </span>
                </div>
                <div className="w-[1px] h-6 bg-border/40" />
                {/* Tokens */}
                <div className="flex flex-col items-center min-w-[50px] px-2 py-1">
                  <span className="text-[8px] font-bold text-text-muted uppercase tracking-tight">TOKEN</span>
                  <span className="font-mono font-black text-xs text-blue-400">
                    {formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}
                  </span>
                </div>
                <div className="w-[1px] h-6 bg-border/40" />
                {/* Cost */}
                <div className="flex flex-col items-center min-w-[60px] px-2 py-1">
                  <span className="text-[8px] font-bold text-text-muted uppercase tracking-tight">COST</span>
                  <span className="font-mono font-black text-xs text-purple-400">
                    {formatCost(stats.totalCost)}
                  </span>
                </div>
              </>
            ) : (
              <div className="px-6 py-2 flex items-center gap-2 text-text-muted/30">
                <Activity size={12} />
                <span className="text-[10px] font-bold uppercase tracking-widest">No Data</span>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Control Area - Switch */}
      <div className="relative z-10 flex items-center flex-shrink-0 ml-auto pl-2">
        <Switch
          checked={enabled}
          onCheckedChange={() => {
            if (!isInCooldown) {
              onToggle();
            }
          }}
          onClick={(e) => e.stopPropagation()}
          disabled={isToggling || isInCooldown}
        />
      </div>

      {/* Streaming Indicator - Top Right */}
      {enabled && streamingCount > 0 && !isInCooldown && (
        <div className="absolute top-0 right-0 z-20">
          <StreamingBadge count={streamingCount} color={color} />
        </div>
      )}
    </div>
  );
}
