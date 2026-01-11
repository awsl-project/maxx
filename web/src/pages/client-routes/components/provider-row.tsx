import { GripVertical, Settings, Zap, RefreshCw, Trash2, Activity, Snowflake, X } from 'lucide-react';
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
import { CooldownDetailsDialog } from '@/components/cooldown-details-dialog';
import { useState } from 'react';

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

// 计算缓存利用率: (CacheRead + CacheWrite) / (Input + Output + CacheRead + CacheWrite) × 100
function calcCacheRate(stats: ProviderStats): number {
  const cacheTotal = stats.totalCacheRead + stats.totalCacheWrite;
  const total = stats.totalInputTokens + stats.totalOutputTokens + cacheTotal;
  if (total === 0) return 0;
  return (cacheTotal / total) * 100;
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
  const [showCooldownDialog, setShowCooldownDialog] = useState(false);
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

  const handleClick = () => {
    if (isDragging) return;

    // 如果在 cooldown，打开弹窗
    if (cooldown) {
      setShowCooldownDialog(true);
    } else {
      // 否则 toggle
      onToggle();
    }
  };

  const handleClearCooldown = () => {
    clearCooldown(item.provider.id);
    setShowCooldownDialog(false);
  };

  return (
    <>
      <div
        ref={setNodeRef}
        style={style}
        {...attributes}
        {...listeners}
        onClick={handleClick}
        className="cursor-pointer active:cursor-grabbing"
      >
        <ProviderRowContent
          item={item}
          index={index}
          clientType={clientType}
          streamingCount={streamingCount}
          stats={stats}
          isToggling={isToggling}
          onToggle={onToggle}
          onDelete={onDelete}
        />
      </div>

      {/* Cooldown Details Dialog */}
      <CooldownDetailsDialog
        cooldown={cooldown || null}
        open={showCooldownDialog}
        onOpenChange={setShowCooldownDialog}
        onClear={handleClearCooldown}
        isClearing={isClearingCooldown}
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
  onDelete?: () => void;
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
  isOverlay,
  onToggle,
  onDelete,
}: ProviderRowContentProps) {
  const { provider, enabled, route, isNative } = item;
  const color = getProviderColor(provider.type);
  const isAntigravity = provider.type === 'antigravity';

  // 仅为 Antigravity provider 获取额度
  const { data: quota } = useAntigravityQuota(provider.id, isAntigravity);
  const claudeInfo = isAntigravity ? getClaudeQuotaInfo(quota) : null;

  // 获取 cooldown 状态
  const { getCooldownForProvider, formatRemaining, clearCooldown, isClearingCooldown } = useCooldowns();
  const cooldown = getCooldownForProvider(provider.id, clientType);
  const isInCooldown = !!cooldown;

  const handleClearCooldown = (e: React.MouseEvent) => {
    e.stopPropagation();
    clearCooldown(provider.id);
  };

  return (
    <div
      className={`
        flex items-center gap-md p-md rounded-lg border transition-all duration-200 relative overflow-hidden
        ${
          isInCooldown
            ? 'bg-cyan-500/5 border-cyan-400/40 shadow-[0_0_15px_rgba(6,182,212,0.2)]'
            : enabled
            ? streamingCount > 0
              ? 'bg-surface-primary'
              : 'bg-emerald-400/[0.03] border-emerald-400/30 shadow-sm'
            : 'bg-surface-secondary/50 border-dashed border-border opacity-95'
        }
        ${isOverlay ? 'shadow-xl ring-2 ring-accent opacity-100' : ''}
      `}
      style={{
        borderColor: isInCooldown
          ? 'rgba(6, 182, 212, 0.4)'
          : enabled && streamingCount > 0 ? `${color}80` : undefined,
        boxShadow: enabled && streamingCount > 0 ? `0 0 15px ${color}20` : undefined,
      }}
    >
      {/* Marquee 背景动画 (仅在有 streaming 请求时显示) */}
      {streamingCount > 0 && enabled && !isInCooldown && (
        <div
          className="absolute inset-0 animate-marquee pointer-events-none opacity-60"
          style={{ backgroundColor: `${color}25` }}
        />
      )}

      {/* Cooldown 冰冻效果 */}
      {isInCooldown && (
        <div className="absolute inset-0 bg-gradient-to-br from-cyan-500/10 via-cyan-400/5 to-blue-500/10 pointer-events-none animate-pulse" />
      )}

      {/* Streaming Badge - 右上角 */}
      {enabled && streamingCount > 0 && !isInCooldown && (
        <div className="absolute -top-1 -right-1 z-20">
          <StreamingBadge count={streamingCount} color={color} />
        </div>
      )}

      {/* Cooldown Badge - 右上角 */}
      {isInCooldown && cooldown && (
        <div className="absolute -top-1 -right-1 z-20 flex items-center gap-1 bg-cyan-500/90 text-white text-xs font-bold px-2 py-1 rounded-lg shadow-lg backdrop-blur-sm border border-cyan-400/30">
          <Snowflake size={12} className="animate-pulse" />
          <span className="font-mono">{formatRemaining(cooldown)}</span>
          <button
            onClick={handleClearCooldown}
            disabled={isClearingCooldown}
            className="ml-0.5 p-0.5 rounded hover:bg-cyan-400/30 transition-colors disabled:opacity-50"
            title="清除 cooldown"
          >
            <X size={10} />
          </button>
        </div>
      )}

      {/* Drag Handle */}
      <div className={`relative z-10 flex flex-col items-center gap-1 w-6 ${enabled ? '' : 'opacity-40'}`}>
        <GripVertical size={14} className="text-text-muted" />
        <span className="text-[10px] font-bold px-1 rounded" style={{ backgroundColor: `${color}20`, color }}>
          {index + 1}
        </span>
      </div>

      {/* Provider Icon */}
      <div
        className={`relative z-10 w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0 transition-opacity ${
          isInCooldown
            ? 'opacity-50 grayscale'
            : enabled
            ? ''
            : 'opacity-30 grayscale'
        }`}
        style={{ backgroundColor: `${color}15`, color }}
      >
        <span className="text-lg font-bold">{provider.name.charAt(0).toUpperCase()}</span>
        {isInCooldown && (
          <div className="absolute inset-0 flex items-center justify-center bg-cyan-500/20 rounded-lg backdrop-blur-[1px]">
            <Snowflake size={16} className="text-cyan-400 animate-pulse" />
          </div>
        )}
      </div>

      {/* Provider Info */}
      <div className="relative z-10 flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className={`text-body font-medium transition-colors ${
            isInCooldown
              ? 'text-cyan-400'
              : enabled
              ? 'text-text-primary'
              : 'text-text-muted'
          }`}>
            {provider.name}
          </span>
          {/* Cooldown indicator */}
          {isInCooldown && (
            <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-cyan-500/20 text-cyan-400 border border-cyan-400/30">
              <Snowflake size={10} className="animate-pulse" />
              冷却中
            </span>
          )}
          {/* Native/Converted badge */}
          {!isInCooldown && isNative ? (
            <span
              className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-emerald-400/10 text-emerald-400"
              title="原生支持"
            >
              <Zap size={10} />
              原生
            </span>
          ) : (
            <span
              className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-400/10 text-amber-400"
              title="API 转换"
            >
              <RefreshCw size={10} />
              转换
            </span>
          )}
        </div>
        <div className={`text-caption truncate transition-colors ${enabled ? 'text-text-muted' : 'text-text-muted/50'}`}>
          {provider.config?.custom?.clientBaseURL?.[clientType] ||
            provider.config?.custom?.baseURL ||
            provider.config?.antigravity?.endpoint ||
            'Default endpoint'}
        </div>
      </div>

      {/* Claude Quota (仅 Antigravity) */}
      {isAntigravity && (
        <div className={`relative z-10 w-24 flex flex-col items-center gap-1 flex-shrink-0 ${enabled ? '' : 'opacity-40'}`}>
          <div className="flex items-center gap-1.5 w-full">
            <span className="text-[10px] text-text-muted uppercase tracking-wider font-medium">Claude</span>
            {claudeInfo && (
              <span className="text-[10px] text-text-muted/70" title="重置时间">
                ({formatResetTime(claudeInfo.resetTime)})
              </span>
            )}
          </div>
          {claudeInfo !== null ? (
            <div className="flex items-center gap-1.5 w-full">
              <div className="flex-1 h-1.5 bg-surface-hover rounded-full overflow-hidden">
                <div
                  className={cn(
                    "h-full transition-all duration-300",
                    claudeInfo.percentage >= 50 ? "bg-emerald-400" :
                    claudeInfo.percentage >= 20 ? "bg-amber-400" : "bg-red-400"
                  )}
                  style={{ width: `${claudeInfo.percentage}%` }}
                />
              </div>
              <span className={cn(
                "text-xs font-mono font-bold min-w-[2.5rem] text-right",
                claudeInfo.percentage >= 50 ? "text-emerald-400" :
                claudeInfo.percentage >= 20 ? "text-amber-400" : "text-red-400"
              )}>
                {claudeInfo.percentage}%
              </span>
            </div>
          ) : (
            <span className="text-xs text-text-muted">-</span>
          )}
        </div>
      )}

      {/* Provider Stats */}
      <div className={`relative z-10 flex items-center gap-2 bg-surface-secondary/30 rounded-lg p-1 border border-border/30 ${enabled ? '' : 'opacity-40'}`}>
        {stats && stats.totalRequests > 0 ? (
          <>
            {/* Success Rate */}
            <div className="flex flex-col items-center justify-center px-2 py-1 w-[60px]">
              <span className="text-[10px] text-text-muted uppercase tracking-wider font-medium mb-0.5">成功</span>
              <span className={cn(
                "font-mono font-bold text-sm",
                stats.successRate >= 95 ? "text-emerald-400" :
                stats.successRate >= 90 ? "text-blue-400" :
                stats.successRate >= 80 ? "text-amber-400" : "text-red-400"
              )}>
                {stats.successRate.toFixed(1)}%
              </span>
            </div>

            <div className="w-px h-8 bg-border/40" />

            {/* Request Count */}
            <div className="flex flex-col items-center justify-center px-2 py-1 w-[60px]" title={`成功: ${stats.successfulRequests}, 失败: ${stats.failedRequests}`}>
              <span className="text-[10px] text-text-muted uppercase tracking-wider font-medium mb-0.5">请求</span>
              <span className="font-mono font-bold text-sm text-text-primary">{stats.totalRequests}</span>
            </div>

            <div className="w-px h-8 bg-border/40" />

            {/* Token Usage */}
            <div className="flex flex-col items-center justify-center px-2 py-1 w-[60px]" title={`输入: ${stats.totalInputTokens}, 输出: ${stats.totalOutputTokens}`}>
              <span className="text-[10px] text-text-muted uppercase tracking-wider font-medium mb-0.5">Token</span>
              <span className="font-mono font-bold text-sm text-blue-400">
                {formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}
              </span>
            </div>

            <div className="w-px h-8 bg-border/40" />

            {/* Cache Rate */}
            <div
              className="flex flex-col items-center justify-center px-2 py-1 w-[60px]"
              title={`Read: ${formatTokens(stats.totalCacheRead)} | Write: ${formatTokens(stats.totalCacheWrite)}`}
            >
              <span className="text-[10px] text-text-muted uppercase tracking-wider font-medium mb-0.5">缓存</span>
              <span className={cn(
                "font-mono font-bold text-sm",
                calcCacheRate(stats) >= 50 ? "text-emerald-400" :
                calcCacheRate(stats) >= 20 ? "text-cyan-400" : "text-text-secondary"
              )}>
                {calcCacheRate(stats).toFixed(1)}%
              </span>
            </div>

            <div className="w-px h-8 bg-border/40" />

            {/* Cost */}
            <div className="flex flex-col items-center justify-center px-2 py-1 w-[70px]" title={`总成本: ${formatCost(stats.totalCost)}`}>
              <span className="text-[10px] text-text-muted uppercase tracking-wider font-medium mb-0.5">成本</span>
              <span className="font-mono font-bold text-sm text-purple-400">{formatCost(stats.totalCost)}</span>
            </div>
          </>
        ) : (
          <div className="px-4 py-2 flex items-center gap-2 text-text-muted/50">
            <Activity size={14} />
            <span className="text-xs font-medium">暂无数据</span>
          </div>
        )}
      </div>

      {/* Settings button */}
      {route && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            // TODO: Navigate to route settings
          }}
          className={`relative z-10 p-2 rounded-md transition-colors ${
            enabled
              ? 'text-text-muted hover:text-text-primary hover:bg-emerald-400/10'
              : 'text-text-muted/30 cursor-not-allowed'
          }`}
          title="Route Settings"
          disabled={!enabled}
        >
          <Settings size={16} />
        </button>
      )}

      {/* Delete button (only for non-native converted routes) */}
      {route && !isNative && onDelete && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            if (confirm('确定要删除这个转换路由吗？')) {
              onDelete();
            }
          }}
          className="relative z-10 p-2 rounded-md text-text-muted hover:text-red-400 hover:bg-red-400/10 transition-colors"
          title="删除转换路由"
        >
          <Trash2 size={16} />
        </button>
      )}

      {/* Toggle indicator */}
      <div className="relative z-10 flex items-center gap-3">
        <span
          className={`text-[10px] font-mono font-bold tracking-wider transition-colors w-6 text-right ${
            isInCooldown
              ? 'text-cyan-400'
              : enabled
              ? 'text-emerald-400'
              : 'text-text-muted/40'
          }`}
        >
          {isInCooldown ? '冻结' : enabled ? 'ON' : 'OFF'}
        </span>
        <Switch
          checked={enabled}
          onCheckedChange={() => !isInCooldown && onToggle()}
          disabled={isToggling || isInCooldown}
        />
      </div>
    </div>
  );
}
