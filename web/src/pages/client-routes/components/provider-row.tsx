import { GripVertical, Settings, Zap, RefreshCw, Trash2, Activity } from 'lucide-react';
import { Switch } from '@/components/ui';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { getProviderColor } from '@/lib/provider-colors';
import { cn } from '@/lib/utils';
import type { ClientType, ProviderStats } from '@/lib/transport';
import type { ProviderConfigItem } from '../types';

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

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      onClick={() => !isDragging && onToggle()}
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

  return (
    <div
      className={`
        flex items-center gap-md p-md rounded-lg border transition-all duration-200 relative overflow-hidden
        ${
          enabled
            ? streamingCount > 0
              ? 'bg-emerald-400/[0.08] border-emerald-400/60 shadow-[0_0_15px_rgba(52,211,153,0.15)]'
              : 'bg-emerald-400/[0.03] border-emerald-400/30 shadow-sm'
            : 'bg-surface-secondary/50 border-dashed border-border opacity-95'
        }
        ${isOverlay ? 'shadow-xl ring-2 ring-accent opacity-100' : ''}
      `}
    >
      {/* Marquee 背景动画 (仅在有 streaming 请求时显示) */}
      {streamingCount > 0 && enabled && (
        <div
          className="absolute inset-0 animate-marquee pointer-events-none opacity-60"
          style={{ backgroundColor: `${color}25` }}
        />
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
          enabled ? '' : 'opacity-30 grayscale'
        }`}
        style={{ backgroundColor: `${color}15`, color }}
      >
        <span className="text-lg font-bold">{provider.name.charAt(0).toUpperCase()}</span>
        {enabled && (
          <div className="absolute -top-2 -right-2">
            <StreamingBadge count={streamingCount} color={color} className="scale-90" />
          </div>
        )}
      </div>

      {/* Provider Info */}
      <div className="relative z-10 flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className={`text-body font-medium transition-colors ${enabled ? 'text-text-primary' : 'text-text-muted'}`}>
            {provider.name}
          </span>
          {/* Native/Converted badge */}
          {isNative ? (
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

      {/* Provider Stats */}
      <div className={`relative z-10 flex items-center gap-2 bg-surface-secondary/30 rounded-lg p-1 border border-border/30 ${enabled ? '' : 'opacity-40'}`}>
        {stats && stats.totalRequests > 0 ? (
          <>
            {/* Success Rate */}
            <div className="flex flex-col items-center justify-center px-3 py-1 min-w-[60px]">
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
            <div className="flex flex-col items-center justify-center px-3 py-1 min-w-[60px]" title={`成功: ${stats.successfulRequests}, 失败: ${stats.failedRequests}`}>
              <span className="text-[10px] text-text-muted uppercase tracking-wider font-medium mb-0.5">请求</span>
              <span className="font-mono font-bold text-sm text-text-primary">{stats.totalRequests}</span>
            </div>

            <div className="w-px h-8 bg-border/40" />

            {/* Token Usage */}
            <div className="flex flex-col items-center justify-center px-3 py-1 min-w-[60px]" title={`输入: ${stats.totalInputTokens}, 输出: ${stats.totalOutputTokens}`}>
              <span className="text-[10px] text-text-muted uppercase tracking-wider font-medium mb-0.5">Token</span>
              <span className="font-mono font-bold text-sm text-blue-400">
                {formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}
              </span>
            </div>

            <div className="w-px h-8 bg-border/40" />

            {/* Cache Rate */}
            <div
              className="flex flex-col items-center justify-center px-3 py-1 min-w-[60px]"
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
            <div className="flex flex-col items-center justify-center px-3 py-1 min-w-[60px]" title={`总成本: ${formatCost(stats.totalCost)}`}>
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
          className={`text-[10px] font-bold tracking-wider transition-colors w-6 text-right ${
            enabled ? 'text-emerald-400' : 'text-text-muted/40'
          }`}
        >
          {enabled ? 'ON' : 'OFF'}
        </span>
        <Switch checked={enabled} onCheckedChange={() => onToggle()} disabled={isToggling} />
      </div>
    </div>
  );
}
