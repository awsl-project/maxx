import { ChevronRight, Globe, Mail, Server, Wand2, Activity } from 'lucide-react';
import { ClientIcon } from '@/components/icons/client-icons';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { getProviderColor } from '@/lib/provider-colors';
import type { Provider, ProviderStats, AntigravityQuotaData } from '@/lib/transport';
import { ANTIGRAVITY_COLOR } from '../types';
import { cn } from '@/lib/utils';
import { useAntigravityQuota } from '@/hooks/queries';

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

export function ProviderRow({ provider, stats, streamingCount, onClick }: ProviderRowProps) {
  const isAntigravity = provider.type === 'antigravity';
  const color = isAntigravity ? ANTIGRAVITY_COLOR : getProviderColor(provider.type);

  // 仅为 Antigravity provider 获取额度
  const { data: quota } = useAntigravityQuota(provider.id, isAntigravity);
  const claudeInfo = isAntigravity ? getClaudeQuotaInfo(quota) : null;

  const getDisplayInfo = () => {
    if (isAntigravity) {
      return provider.config?.antigravity?.email || 'Unknown';
    }
    if (provider.config?.custom?.baseURL) return provider.config.custom.baseURL;
    for (const ct of provider.supportedClientTypes || []) {
      const url = provider.config?.custom?.clientBaseURL?.[ct];
      if (url) return url;
    }
    return 'Not configured';
  };

  return (
    <div
      onClick={onClick}
      className={cn(
        "group relative flex items-center gap-4 p-3 rounded-xl border transition-all duration-300 overflow-hidden",
        streamingCount > 0
          ? "bg-surface-primary border-transparent ring-1 ring-black/5 dark:ring-white/10"
          : "bg-surface-primary/60 border-border hover:border-accent/30 hover:bg-surface-primary shadow-sm cursor-pointer"
      )}
      style={{
        borderColor: streamingCount > 0 ? `${color}40` : undefined,
        boxShadow: streamingCount > 0 ? `0 0 20px ${color}15` : undefined,
      }}
    >
      {/* Marquee 背景动画 (仅在有 streaming 请求时显示) */}
      {streamingCount > 0 && (
        <div
          className="absolute inset-0 animate-marquee pointer-events-none opacity-40"
          style={{ backgroundColor: `${color}15` }}
        />
      )}

      {/* Streaming Badge - 右上角 */}
      {streamingCount > 0 && (
        <div className="absolute top-0 right-0 z-20">
          <StreamingBadge count={streamingCount} color={color} />
        </div>
      )}

      {/* Provider Icon */}
      <div
        className="relative z-10 w-12 h-12 rounded-xl flex items-center justify-center flex-shrink-0 bg-surface-secondary border border-border shadow-inner group-hover:shadow-none transition-shadow duration-300"
        style={{ color }}
      >
        <div className="absolute inset-0 opacity-10" style={{ backgroundColor: color }} />
        {isAntigravity ? <Wand2 size={24} /> : <Server size={24} />}
      </div>

      {/* Provider Info */}
      <div className="relative z-10 flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <h3 className="text-[15px] font-bold text-text-primary truncate">{provider.name}</h3>
          <span
            className="px-2 py-0.5 rounded-full text-[10px] uppercase tracking-wider font-black flex items-center gap-1"
            style={{ backgroundColor: `${color}15`, color }}
          >
            <div className="w-1 h-1 rounded-full animate-pulse" style={{ backgroundColor: color }} />
            {isAntigravity ? 'Antigravity' : 'Custom'}
          </span>
        </div>
        <div className="flex items-center gap-1.5 text-[11px] font-medium text-text-muted truncate" title={getDisplayInfo()}>
          {isAntigravity ? <Mail size={11} className="shrink-0" /> : <Globe size={11} className="shrink-0" />}
          <span className="truncate">{getDisplayInfo()}</span>
        </div>
      </div>

      {/* Supported Clients */}
      <div className="relative z-10 w-28 flex flex-col gap-1 flex-shrink-0">
        <span className="text-[9px] font-black text-text-muted/60 uppercase tracking-tighter">Clients</span>
        <div className="flex items-center -space-x-1.5 group/clients">
          {provider.supportedClientTypes?.length > 0 ? (
            provider.supportedClientTypes.map((ct) => (
              <div 
                key={ct} 
                className="relative z-0 hover:z-10 bg-surface-primary rounded-full p-0.5 border border-border transition-all hover:scale-125 hover:-translate-y-0.5 shadow-sm"
              >
                 <ClientIcon type={ct} size={14} />
              </div>
            ))
          ) : (
            <span className="text-xs text-text-muted font-mono">-</span>
          )}
        </div>
      </div>

      {/* Claude Quota Area */}
      {isAntigravity && (
        <div className="relative z-10 w-24 flex flex-col gap-1 flex-shrink-0">
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

      {/* Stats Grid */}
      <div className="relative z-10 flex items-center gap-px bg-surface-secondary/50 rounded-xl border border-border/60 p-0.5 backdrop-blur-sm">
        {stats && stats.totalRequests > 0 ? (
          <>
            <div className="flex flex-col items-center min-w-[45px] px-2 py-1">
              <span className="text-[8px] font-bold text-text-muted uppercase tracking-tight">SR</span>
              <span className={cn(
                "font-mono font-black text-[12px]",
                stats.successRate >= 95 ? "text-emerald-500" :
                stats.successRate >= 90 ? "text-blue-400" : "text-amber-500"
              )}>
                {Math.round(stats.successRate)}%
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[45px] px-2 py-1">
              <span className="text-[8px] font-bold text-text-muted uppercase tracking-tight">REQ</span>
              <span className="font-mono font-black text-[12px] text-text-primary">{stats.totalRequests}</span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[45px] px-2 py-1">
              <span className="text-[8px] font-bold text-text-muted uppercase tracking-tight">TKN</span>
              <span className="font-mono font-black text-[12px] text-blue-400">
                {formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[55px] px-2 py-1">
              <span className="text-[8px] font-bold text-text-muted uppercase tracking-tight">COST</span>
              <span className="font-mono font-black text-[12px] text-purple-400">{formatCost(stats.totalCost)}</span>
            </div>
          </>
        ) : (
          <div className="px-6 py-2 flex items-center gap-2 text-text-muted/30">
             <Activity size={12} />
             <span className="text-[10px] font-bold uppercase tracking-widest">No Activity</span>
          </div>
        )}
      </div>

      {/* Navigation Icon */}
      <div className="relative z-10 flex-shrink-0 ml-1">
         <div className="p-2 rounded-full text-text-muted group-hover:text-text-primary group-hover:bg-surface-secondary transition-all transform group-hover:translate-x-1">
            <ChevronRight size={20} />
         </div>
      </div>
    </div>
  );
}