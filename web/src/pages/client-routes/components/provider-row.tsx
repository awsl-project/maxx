import { GripVertical, Settings, Zap, RefreshCw } from 'lucide-react';
import { Switch } from '@/components/ui';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { getProviderColor } from '@/lib/provider-colors';
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

// Sortable Provider Row
type SortableProviderRowProps = {
  item: ProviderConfigItem;
  index: number;
  clientType: ClientType;
  streamingCount: number;
  stats?: ProviderStats;
  isToggling: boolean;
  onToggle: () => void;
};

export function SortableProviderRow({
  item,
  index,
  clientType,
  streamingCount,
  stats,
  isToggling,
  onToggle,
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
}: ProviderRowContentProps) {
  const { provider, enabled, route, isNative } = item;
  const color = getProviderColor(provider.type);

  return (
    <div
      className={`
        flex items-center gap-md p-md rounded-lg border transition-all duration-200 relative overflow-hidden
        ${
          enabled
            ? 'bg-emerald-400/[0.03] border-emerald-400/30 shadow-sm'
            : 'bg-surface-secondary/50 border-dashed border-border opacity-95'
        }
        ${isOverlay ? 'shadow-xl ring-2 ring-accent opacity-100' : ''}
      `}
    >
      {/* Marquee 背景动画 (仅在有 streaming 请求时显示) */}
      {streamingCount > 0 && enabled && (
        <div
          className="absolute inset-0 animate-marquee pointer-events-none opacity-40"
          style={{ backgroundColor: `${color}15` }}
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
      {stats && stats.totalRequests > 0 && (
        <div className={`relative z-10 flex items-center gap-3 text-[10px] ${enabled ? '' : 'opacity-40'}`}>
          {/* Success Rate */}
          <div className="flex flex-col items-center" title="成功率">
            <span className={`font-bold ${stats.successRate >= 90 ? 'text-emerald-400' : stats.successRate >= 70 ? 'text-amber-400' : 'text-red-400'}`}>
              {stats.successRate.toFixed(1)}%
            </span>
            <span className="text-text-muted/60">成功</span>
          </div>
          {/* Request Count */}
          <div className="flex flex-col items-center" title={`总请求: ${stats.totalRequests}, 成功: ${stats.successfulRequests}, 失败: ${stats.failedRequests}`}>
            <span className="font-bold text-text-primary">{stats.totalRequests}</span>
            <span className="text-text-muted/60">请求</span>
          </div>
          {/* Token Usage */}
          <div className="flex flex-col items-center" title={`输入: ${stats.totalInputTokens}, 输出: ${stats.totalOutputTokens}`}>
            <span className="font-bold text-blue-400">{formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}</span>
            <span className="text-text-muted/60">Token</span>
          </div>
          {/* Cost */}
          <div className="flex flex-col items-center" title={`总成本: ${formatCost(stats.totalCost)}`}>
            <span className="font-bold text-purple-400">{formatCost(stats.totalCost)}</span>
            <span className="text-text-muted/60">成本</span>
          </div>
        </div>
      )}

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

      {/* Streaming count badge */}
      <div className="relative z-10">
        {enabled && <StreamingBadge count={streamingCount} color={color} />}
      </div>

      {/* Toggle indicator */}
      <div className="relative z-10 flex items-center gap-3">
        <span
          className={`text-[10px] font-bold tracking-wider transition-colors ${
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
