import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { Dialog, DialogContent } from '@/components/ui/dialog';
import {
  Snowflake,
  AlertCircle,
  Server,
  Wifi,
  Zap,
  Ban,
  HelpCircle,
  X,
  Activity,
  Lock,
} from 'lucide-react';
import type { Cooldown } from '@/lib/transport/types';
import { CooldownTimer } from '@/components/cooldown-timer';

interface CooldownDetailsDialogProps {
  providerName: string;
  cooldowns: Cooldown[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onClear: (options?: { clientType?: string; model?: string }) => void;
  isClearing: boolean;
  onDisable: () => void;
  isDisabling: boolean;
}

// Reason 信息和图标 - 使用翻译
const getReasonInfo = (t: TFunction) => ({
  server_error: {
    label: t('provider.reasons.serverError'),
    description: t(
      'provider.reasons.serverErrorDesc',
      '上游服务器返回 5xx 错误，系统自动进入冷却保护',
    ),
    icon: Server,
    color: 'text-red-400',
    bgColor: 'bg-red-400/10 border-red-400/20',
  },
  network_error: {
    label: t('provider.reasons.networkError'),
    description: t(
      'provider.reasons.networkErrorDesc',
      '无法连接到上游服务器，可能是网络故障或服务器宕机',
    ),
    icon: Wifi,
    color: 'text-amber-400',
    bgColor: 'bg-amber-400/10 border-amber-400/20',
  },
  quota_exhausted: {
    label: t('provider.reasons.quotaExhausted'),
    description: t('provider.reasons.quotaExhaustedDesc', 'API 配额已用完，等待配额重置'),
    icon: AlertCircle,
    color: 'text-red-400',
    bgColor: 'bg-red-400/10 border-red-400/20',
  },
  rate_limit_exceeded: {
    label: t('provider.reasons.rateLimitExceeded'),
    description: t('provider.reasons.rateLimitExceededDesc', '请求速率超过限制，触发了速率保护'),
    icon: Zap,
    color: 'text-yellow-400',
    bgColor: 'bg-yellow-400/10 border-yellow-400/20',
  },
  concurrent_limit: {
    label: t('provider.reasons.concurrentLimit'),
    description: t('provider.reasons.concurrentLimitDesc', '并发请求数超过限制'),
    icon: Ban,
    color: 'text-orange-400',
    bgColor: 'bg-orange-400/10 border-orange-400/20',
  },
  auth_failure: {
    label: t('provider.reasons.authFailure', 'Auth Failure'),
    description: t('provider.reasons.authFailureDesc', 'API key 无效或账号被封禁'),
    icon: Lock,
    color: 'text-red-400',
    bgColor: 'bg-red-400/10 border-red-400/20',
  },
  model_unavailable: {
    label: t('provider.reasons.modelUnavailable', 'Model Unavailable'),
    description: t('provider.reasons.modelUnavailableDesc', '模型不存在或无访问权限'),
    icon: Ban,
    color: 'text-gray-400',
    bgColor: 'bg-gray-400/10 border-gray-400/20',
  },
  manual: {
    label: t('provider.reasons.manual', 'Manual Freeze'),
    description: t('provider.reasons.manualDesc', '管理员手动冻结'),
    icon: Snowflake,
    color: 'text-blue-400',
    bgColor: 'bg-blue-400/10 border-blue-400/20',
  },
  unknown: {
    label: t('provider.reasons.unknown'),
    description: t('provider.reasons.unknownDesc', '因未知原因进入冷却状态'),
    icon: HelpCircle,
    color: 'text-muted-foreground',
    bgColor: 'bg-muted border-border',
  },
});

export function CooldownDetailsDialog({
  providerName,
  cooldowns,
  open,
  onOpenChange,
  onClear,
  isClearing,
  onDisable,
  isDisabling,
}: CooldownDetailsDialogProps) {
  const { t } = useTranslation();

  if (cooldowns.length === 0) return null;

  // Group cooldowns by scope
  const providerLevel = cooldowns.filter((cd) => !cd.clientType && !cd.model);
  const keyLevel = cooldowns.filter((cd) => cd.clientType && !cd.model);
  const modelLevel = cooldowns.filter((cd) => !!cd.model);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="overflow-hidden p-0 w-full max-w-[28rem] bg-card"
      >
        {/* Header with Gradient */}
        <div className="relative bg-gradient-to-b from-cyan-900/20 to-transparent p-6 pb-4">
          <button
            onClick={() => onOpenChange(false)}
            className="absolute top-4 right-4 p-2 rounded-full hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
          >
            <X size={18} />
          </button>

          <div className="flex flex-col items-center text-center space-y-3">
            <div className="p-3 rounded-2xl bg-cyan-500/10 border border-cyan-400/20 shadow-[0_0_15px_-3px_rgba(6,182,212,0.2)]">
              <Snowflake size={28} className="text-cyan-400 animate-spin-slow" />
            </div>
            <div>
              <h2 className="text-xl font-bold text-text-primary">{providerName}</h2>
              <p className="text-xs text-cyan-500/80 font-medium uppercase tracking-wider mt-1">
                {cooldowns.length} active cooldown{cooldowns.length > 1 ? 's' : ''}
              </p>
            </div>
          </div>
        </div>

        {/* Body Content */}
        <div className="px-6 pb-6 space-y-4">
          {providerLevel.length > 0 && (
            <CooldownGroup
              label="Provider-level"
              cooldowns={providerLevel}
              onClear={onClear}
              isClearing={isClearing}
            />
          )}
          {keyLevel.length > 0 && (
            <CooldownGroup
              label="Key-level"
              cooldowns={keyLevel}
              onClear={onClear}
              isClearing={isClearing}
            />
          )}
          {modelLevel.length > 0 && (
            <CooldownGroup
              label={`Model-level (${modelLevel.length})`}
              cooldowns={modelLevel}
              onClear={onClear}
              isClearing={isClearing}
            />
          )}

          {/* Disable route button */}
          <button
            onClick={onDisable}
            disabled={isDisabling || isClearing}
            className="w-full flex items-center justify-center gap-2 rounded-xl border border-border bg-muted hover:bg-accent px-4 py-3 text-sm font-medium text-muted-foreground transition-colors disabled:opacity-50"
          >
            {isDisabling ? (
              <div className="h-3 w-3 animate-spin rounded-full border-2 border-current/30 border-t-current" />
            ) : (
              <Ban size={16} />
            )}
            {isDisabling ? t('cooldown.disabling') : t('cooldown.disableRoute')}
          </button>

          <div className="flex items-start gap-2 rounded-lg bg-muted/50 p-2.5 text-[11px] text-muted-foreground">
            <Activity size={12} className="mt-0.5 shrink-0" />
            <p>{t('cooldown.forceThawWarning')}</p>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function CooldownGroup({
  label,
  cooldowns,
  onClear,
  isClearing,
}: {
  label: string;
  cooldowns: Cooldown[];
  onClear: (options?: { clientType?: string; model?: string }) => void;
  isClearing: boolean;
}) {
  return (
    <div className="space-y-2">
      <div className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
        {label}
      </div>
      {cooldowns.map((cd) => (
        <CooldownEntry
          key={`${cd.providerID}-${cd.clientType || ''}-${cd.model || ''}`}
          cooldown={cd}
          onClear={() => onClear({ clientType: cd.clientType || undefined, model: cd.model || undefined })}
          isClearing={isClearing}
        />
      ))}
    </div>
  );
}

function CooldownEntry({
  cooldown,
  onClear,
  isClearing,
}: {
  cooldown: Cooldown;
  onClear: () => void;
  isClearing: boolean;
}) {
  const { t } = useTranslation();
  const REASON_INFO = getReasonInfo(t);
  const reasonInfo = REASON_INFO[cooldown.reason] || REASON_INFO.unknown;
  const Icon = reasonInfo.icon;

  return (
    <div className={`flex items-center gap-3 p-3 rounded-xl border ${reasonInfo.bgColor}`}>
      <Icon size={16} className={`shrink-0 ${reasonInfo.color}`} />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className={`text-sm font-medium ${reasonInfo.color}`}>{reasonInfo.label}</span>
          {cooldown.model && (
            <span className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-accent text-muted-foreground truncate max-w-[200px]">
              {cooldown.model}
            </span>
          )}
          {cooldown.clientType && (
            <span className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-accent text-muted-foreground">
              {cooldown.clientType}
            </span>
          )}
        </div>
      </div>
      <CooldownTimer
        cooldown={cooldown}
        className="text-xs font-mono tabular-nums shrink-0"
      />
      <button
        onClick={onClear}
        disabled={isClearing}
        className="p-1 rounded hover:bg-accent shrink-0 disabled:opacity-50"
        title={t('cooldown.forceThaw')}
      >
        <X size={14} className="text-muted-foreground" />
      </button>
    </div>
  );
}
