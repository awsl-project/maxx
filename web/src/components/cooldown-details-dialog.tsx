import { Snowflake, Clock, AlertCircle, Server, Wifi, Zap, Ban, HelpCircle, X } from 'lucide-react';
import type { Cooldown, CooldownReason } from '@/lib/transport/types';

interface CooldownDetailsDialogProps {
  cooldown: Cooldown | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onClear: () => void;
  isClearing: boolean;
}

// Reason 中文说明和图标
const REASON_INFO: Record<CooldownReason, { label: string; description: string; icon: typeof Server }> = {
  server_error: {
    label: '服务器错误',
    description: '上游服务器返回 5xx 错误，系统自动进入冷却保护',
    icon: Server,
  },
  network_error: {
    label: '网络错误',
    description: '无法连接到上游服务器，可能是网络故障或服务器宕机',
    icon: Wifi,
  },
  quota_exhausted: {
    label: '配额耗尽',
    description: 'API 配额已用完，等待配额重置',
    icon: AlertCircle,
  },
  rate_limit_exceeded: {
    label: '速率限制',
    description: '请求速率超过限制，触发了速率保护',
    icon: Zap,
  },
  concurrent_limit: {
    label: '并发限制',
    description: '并发请求数超过限制',
    icon: Ban,
  },
  unknown: {
    label: '未知原因',
    description: '因未知原因进入冷却状态',
    icon: HelpCircle,
  },
};

export function CooldownDetailsDialog({
  cooldown,
  open,
  onOpenChange,
  onClear,
  isClearing,
}: CooldownDetailsDialogProps) {
  if (!open || !cooldown) return null;

  const reasonInfo = REASON_INFO[cooldown.reason] || REASON_INFO.unknown;
  const Icon = reasonInfo.icon;

  const formatUntilTime = (until: string) => {
    const date = new Date(until);
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/50 backdrop-blur-sm"
        onClick={() => onOpenChange(false)}
      />

      {/* Dialog */}
      <div className="relative bg-surface-primary border border-border rounded-xl shadow-2xl max-w-md w-full mx-4 overflow-hidden">
        {/* Header */}
        <div className="px-6 py-4 border-b border-border flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Snowflake className="text-cyan-400" size={20} />
            <h2 className="text-lg font-semibold text-text-primary">Provider 冷却详情</h2>
          </div>
          <button
            onClick={() => onOpenChange(false)}
            className="p-1 rounded-md hover:bg-surface-hover transition-colors"
          >
            <X size={18} className="text-text-muted" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 space-y-4">
          {/* Provider 信息 */}
          <div className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/50 border border-border">
            <div>
              <div className="text-xs text-text-muted mb-1">Provider</div>
              <div className="font-medium text-text-primary">{cooldown.providerName || `Provider #${cooldown.providerID}`}</div>
            </div>
            {cooldown.clientType && cooldown.clientType !== '' && (
              <div className="text-right">
                <div className="text-xs text-text-muted mb-1">Client Type</div>
                <div className="text-sm font-mono text-text-secondary">{cooldown.clientType}</div>
              </div>
            )}
          </div>

          {/* 冷却原因 */}
          <div className="p-4 rounded-lg bg-cyan-500/5 border border-cyan-400/30">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-lg bg-cyan-500/10">
                <Icon className="text-cyan-400" size={20} />
              </div>
              <div className="flex-1">
                <div className="font-medium text-cyan-400 mb-1">{reasonInfo.label}</div>
                <div className="text-sm text-text-muted">{reasonInfo.description}</div>
              </div>
            </div>
          </div>

          {/* 时间信息 */}
          <div className="space-y-3">
            <div className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/30">
              <div className="flex items-center gap-2 text-text-muted">
                <Clock size={16} />
                <span className="text-sm">恢复时间</span>
              </div>
              <div className="text-sm font-mono text-text-primary">{formatUntilTime(cooldown.until)}</div>
            </div>

            <div className="flex items-center justify-between p-3 rounded-lg bg-surface-secondary/30">
              <div className="flex items-center gap-2 text-text-muted">
                <Snowflake size={16} />
                <span className="text-sm">剩余时间</span>
              </div>
              <div className="text-sm font-mono font-bold text-cyan-400">{cooldown.remaining}</div>
            </div>
          </div>

          {/* 解冻按钮 */}
          <div className="pt-2">
            <button
              onClick={onClear}
              disabled={isClearing}
              className="w-full px-4 py-2.5 bg-gradient-to-r from-cyan-500 to-blue-500 hover:from-cyan-600 hover:to-blue-600 text-white font-medium rounded-lg transition-all disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isClearing ? '解冻中...' : '立即解冻'}
            </button>
            <p className="text-xs text-text-muted text-center mt-2">
              解冻后将立即恢复该 Provider 的使用
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
