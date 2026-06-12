import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import {
  Snowflake,
  AlertCircle,
  Server,
  Wifi,
  Zap,
  Ban,
  HelpCircle,
  X,
  Info,
  Trash2,
  Hand,
  Lock,
} from 'lucide-react';
import dayjs from 'dayjs';
import customParseFormat from 'dayjs/plugin/customParseFormat';

dayjs.extend(customParseFormat);
import type { Cooldown, ProviderStats, ClientType } from '@/lib/transport/types';
import type { ProviderConfigItem } from '@/pages/client-routes/types';
import { useCooldownsContext } from '@/contexts/cooldowns-context';
import { Button, Switch } from '@/components/ui';
import { getProviderColor, type ProviderType } from '@/lib/theme';
import { cn } from '@/lib/utils';
import { Dialog, DialogContent } from '@/components/ui/dialog';
import { CooldownTimer } from '@/components/cooldown-timer';

interface ProviderDetailsDialogProps {
  item: ProviderConfigItem | null;
  clientType: ClientType;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  stats?: ProviderStats;
  cooldowns?: Cooldown[];
  streamingCount: number;
  onToggle: () => void;
  isToggling: boolean;
  onDelete?: () => void;
  onClearCooldown?: (options?: { clientType?: string; model?: string }) => void;
  isClearingCooldown?: boolean;
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
    color: 'text-rose-500 dark:text-rose-400',
    bgColor: 'bg-rose-500/10 dark:bg-rose-500/15 border-rose-500/30 dark:border-rose-500/25',
  },
  network_error: {
    label: t('provider.reasons.networkError'),
    description: t(
      'provider.reasons.networkErrorDesc',
      '无法连接到上游服务器，可能是网络故障或服务器宕机',
    ),
    icon: Wifi,
    color: 'text-amber-600 dark:text-amber-400',
    bgColor: 'bg-amber-500/10 dark:bg-amber-500/15 border-amber-500/30 dark:border-amber-500/25',
  },
  quota_exhausted: {
    label: t('provider.reasons.quotaExhausted'),
    description: t('provider.reasons.quotaExhaustedDesc', 'API 配额已用完，等待配额重置'),
    icon: AlertCircle,
    color: 'text-rose-500 dark:text-rose-400',
    bgColor: 'bg-rose-500/10 dark:bg-rose-500/15 border-rose-500/30 dark:border-rose-500/25',
  },
  rate_limit_exceeded: {
    label: t('provider.reasons.rateLimitExceeded'),
    description: t('provider.reasons.rateLimitExceededDesc', '请求速率超过限制，触发了速率保护'),
    icon: Zap,
    color: 'text-yellow-600 dark:text-yellow-400',
    bgColor:
      'bg-yellow-500/10 dark:bg-yellow-500/15 border-yellow-500/30 dark:border-yellow-500/25',
  },
  concurrent_limit: {
    label: t('provider.reasons.concurrentLimit'),
    description: t('provider.reasons.concurrentLimitDesc', '并发请求数超过限制'),
    icon: Ban,
    color: 'text-orange-600 dark:text-orange-400',
    bgColor:
      'bg-orange-500/10 dark:bg-orange-500/15 border-orange-500/30 dark:border-orange-500/25',
  },
  auth_failure: {
    label: t('provider.reasons.authFailure', 'Auth Failure'),
    description: t('provider.reasons.authFailureDesc', 'API 认证失败，请检查密钥配置'),
    icon: Lock,
    color: 'text-rose-600 dark:text-rose-400',
    bgColor: 'bg-rose-500/10 dark:bg-rose-500/15 border-rose-500/30 dark:border-rose-500/25',
  },
  model_unavailable: {
    label: t('provider.reasons.modelUnavailable', 'Model Unavailable'),
    description: t('provider.reasons.modelUnavailableDesc', '请求的模型当前不可用'),
    icon: Ban,
    color: 'text-slate-600 dark:text-slate-400',
    bgColor: 'bg-slate-500/10 dark:bg-slate-500/15 border-slate-500/30 dark:border-slate-500/25',
  },
  unknown: {
    label: t('provider.reasons.unknown'),
    description: t('provider.reasons.unknownDesc', '因未知原因进入冷却状态'),
    icon: HelpCircle,
    color: 'text-muted-foreground',
    bgColor: 'bg-muted/50 border-border',
  },
  manual: {
    label: t('provider.reasons.manual'),
    description: t('provider.reasons.manualDesc', 'Provider 已被管理员手动冷冻'),
    icon: Hand,
    color: 'text-indigo-500 dark:text-indigo-400',
    bgColor:
      'bg-indigo-500/10 dark:bg-indigo-500/15 border-indigo-500/30 dark:border-indigo-500/25',
  },
});

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

// 解析用户输入的时间字符串
function parseTimeInput(input: string): dayjs.Dayjs | null {
  const trimmed = input.trim().toLowerCase();
  if (!trimmed) return null;

  const now = dayjs();

  // 1. 相对时间格式: "5m", "30min", "2h", "1hour", "3d", "1day"
  const relativeMatch = trimmed.match(
    /^(\d+)\s*(m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)$/,
  );
  if (relativeMatch) {
    const value = parseInt(relativeMatch[1], 10);
    const unit = relativeMatch[2];
    if (unit.startsWith('m')) {
      return now.add(value, 'minute');
    } else if (unit.startsWith('h')) {
      return now.add(value, 'hour');
    } else if (unit.startsWith('d')) {
      return now.add(value, 'day');
    }
  }

  // 2. 纯时间格式: "14:30", "2:30pm", "14:30:00"
  const timeOnlyMatch = trimmed.match(/^(\d{1,2}):(\d{2})(?::(\d{2}))?(?:\s*(am|pm))?$/);
  if (timeOnlyMatch) {
    let hours = parseInt(timeOnlyMatch[1], 10);
    const minutes = parseInt(timeOnlyMatch[2], 10);
    const seconds = timeOnlyMatch[3] ? parseInt(timeOnlyMatch[3], 10) : 0;
    const ampm = timeOnlyMatch[4];

    if (ampm === 'pm' && hours < 12) hours += 12;
    if (ampm === 'am' && hours === 12) hours = 0;

    let result = now.hour(hours).minute(minutes).second(seconds).millisecond(0);
    // 如果时间已过，设为明天
    if (result.isBefore(now) || result.isSame(now)) {
      result = result.add(1, 'day');
    }
    return result;
  }

  // 3. 常见日期时间格式
  const formats = [
    'YYYY-MM-DD HH:mm:ss',
    'YYYY-MM-DD HH:mm',
    'YYYY/MM/DD HH:mm:ss',
    'YYYY/MM/DD HH:mm',
    'MM-DD HH:mm',
    'MM/DD HH:mm',
    'DD HH:mm',
  ];

  for (const fmt of formats) {
    const parsed = dayjs(trimmed, fmt, true);
    if (parsed.isValid() && parsed.isAfter(now)) {
      return parsed;
    }
  }

  // 4. 尝试 dayjs 自动解析（ISO 格式等）
  const autoParsed = dayjs(trimmed);
  if (autoParsed.isValid() && autoParsed.isAfter(now)) {
    return autoParsed;
  }

  return null;
}

export function ProviderDetailsDialog({
  item,
  clientType,
  open,
  onOpenChange,
  stats,
  cooldowns = [],
  onToggle,
  isToggling,
  onDelete,
  onClearCooldown,
  isClearingCooldown,
}: ProviderDetailsDialogProps) {
  const { t } = useTranslation();
  const REASON_INFO = getReasonInfo(t);
  const { setCooldown, isSettingCooldown } = useCooldownsContext();
  const [showCustomTime, setShowCustomTime] = useState(false);
  const [customTimeInput, setCustomTimeInput] = useState('');
  const [freezeMode, setFreezeMode] = useState<'provider' | 'model'>('provider');
  const [freezeModel, setFreezeModel] = useState('');

  // 实时解析输入的时间
  const parsedTime = useMemo(() => parseTimeInput(customTimeInput), [customTimeInput]);

  if (!item) return null;

  const { provider, enabled, route, isNative } = item;
  const color = getProviderColor(provider.type as ProviderType);

  const activeCooldowns = cooldowns; // already filtered by hook
  const isInCooldown = activeCooldowns.length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="overflow-hidden p-0 w-full max-w-lg bg-card grid-cols-[minmax(0,1fr)]">
        {/* Header */}
        <div className="flex items-center gap-3 p-4 border-b border-border">
          <div className={cn(
            'w-10 h-10 rounded-xl flex items-center justify-center shrink-0',
            isInCooldown ? 'bg-slate-200 dark:bg-slate-800' : 'bg-muted',
          )}>
            {isInCooldown ? (
              <Snowflake size={20} className="text-slate-500 dark:text-slate-400 animate-pulse" />
            ) : (
              <span className="text-lg font-black" style={{ color }}>{provider.name.charAt(0)}</span>
            )}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 min-w-0">
              <h2 className="text-base font-bold truncate min-w-0">{provider.name}</h2>
              {isNative ? (
                <span className="shrink-0 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/10 text-emerald-500 border border-emerald-500/20">NATIVE</span>
              ) : (
                <span className="shrink-0 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-amber-500/10 text-amber-500 border border-amber-500/20">CONV</span>
              )}
              <span className="shrink-0 text-[10px] text-muted-foreground">{provider.type}</span>
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Switch checked={enabled} onCheckedChange={onToggle} disabled={isToggling} />
            <button onClick={() => onOpenChange(false)} className="p-1.5 rounded-lg hover:bg-accent">
              <X size={16} />
            </button>
          </div>
        </div>

        {/* Provider Info */}
        <div className="px-4 py-3 text-xs text-muted-foreground flex items-center gap-4 border-b border-border/50">
          <span className="flex items-center gap-1.5 min-w-0">
            <Info size={11} className="shrink-0" />
            <span className="truncate">
              {provider.config?.custom?.baseURL || provider.config?.custom?.clientBaseURL?.[clientType] || '\u2014'}
            </span>
          </span>
          <span className="shrink-0">{clientType}</span>
          <span className="shrink-0">Priority #{route?.position || '\u2014'}</span>
        </div>

        {/* Cooldowns Section */}
        {activeCooldowns.length > 0 && (
          <div className="px-4 py-3 space-y-2">
            <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider">
              Cooldowns ({activeCooldowns.length})
            </div>
            {activeCooldowns.map((cd, i) => {
              const reasonInfo = REASON_INFO[cd.reason] || REASON_INFO.unknown;
              const Icon = reasonInfo.icon;
              const isFrozen = !cd.clientType && !cd.model;
              return (
                <div key={i} className={cn('flex items-center gap-3 p-3 rounded-xl border', reasonInfo.bgColor)}>
                  <Icon size={16} className={cn('shrink-0', reasonInfo.color)} />
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-semibold truncate">
                      {cd.model || (isFrozen ? 'Provider' : cd.clientType || 'All')}
                    </div>
                    <div className="text-[11px] text-muted-foreground">{reasonInfo.label}</div>
                  </div>
                  <CooldownTimer cooldown={cd} className="text-sm font-mono font-bold tabular-nums shrink-0" />
                  <button
                    onClick={() => onClearCooldown?.({ clientType: cd.clientType || undefined, model: cd.model || undefined })}
                    disabled={isClearingCooldown}
                    className="p-1.5 rounded-lg hover:bg-accent/50 transition-colors disabled:opacity-50 shrink-0"
                    title="Unfreeze"
                  >
                    <Zap size={14} className="text-muted-foreground" />
                  </button>
                </div>
              );
            })}
          </div>
        )}

        {/* Freeze Section */}
        <div className="px-4 py-3 border-t border-border/50">
          <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2">
            {t('provider.manualFreeze')}
          </div>

          {!showCustomTime ? (
            <div className="space-y-2">
              {/* Freeze mode selector */}
              <div className="flex gap-1.5">
                <button
                  onClick={() => { setFreezeMode('provider'); setFreezeModel(''); }}
                  className={cn(
                    'px-2.5 py-1 text-[11px] rounded-lg border transition-colors',
                    freezeMode === 'provider'
                      ? 'bg-indigo-500/15 border-indigo-500/30 text-indigo-400'
                      : 'border-border text-muted-foreground hover:bg-accent',
                  )}
                >
                  Entire Provider
                </button>
                <button
                  onClick={() => setFreezeMode('model')}
                  className={cn(
                    'px-2.5 py-1 text-[11px] rounded-lg border transition-colors',
                    freezeMode === 'model'
                      ? 'bg-indigo-500/15 border-indigo-500/30 text-indigo-400'
                      : 'border-border text-muted-foreground hover:bg-accent',
                  )}
                >
                  Specific Model
                </button>
              </div>

              {/* Model input */}
              {freezeMode === 'model' && (
                <input
                  type="text"
                  value={freezeModel}
                  onChange={(e) => setFreezeModel(e.target.value)}
                  placeholder="e.g. gemini-2.5-flash-image"
                  className="w-full rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-mono"
                />
              )}

              {/* Quick freeze buttons */}
              <div className="flex flex-wrap gap-1.5">
                {[
                  { label: '5m', minutes: 5 },
                  { label: '15m', minutes: 15 },
                  { label: '30m', minutes: 30 },
                  { label: '1h', minutes: 60 },
                  { label: '2h', minutes: 120 },
                  { label: '6h', minutes: 360 },
                ].map(({ label, minutes }) => (
                  <Button
                    key={label}
                    disabled={isSettingCooldown || isToggling || (freezeMode === 'model' && !freezeModel.trim())}
                    onClick={() => {
                      const until = new Date(Date.now() + minutes * 60 * 1000);
                      setCooldown(
                        provider.id,
                        until.toISOString(),
                        freezeMode === 'provider' ? '' : clientType,
                        freezeMode === 'model' ? freezeModel.trim() : undefined,
                      );
                    }}
                    className="px-3 py-1.5 text-xs rounded-lg border border-indigo-500/30 dark:border-indigo-500/25 bg-indigo-500/5 dark:bg-indigo-500/10 hover:bg-indigo-500/15 dark:hover:bg-indigo-500/20 text-indigo-600 dark:text-indigo-400 disabled:opacity-50"
                  >
                    {label}
                  </Button>
                ))}
                <Button
                  disabled={isSettingCooldown || isToggling}
                  onClick={() => setShowCustomTime(true)}
                  className="px-3 py-1.5 text-xs rounded-lg border border-dashed border-indigo-500/30 dark:border-indigo-500/25 bg-transparent hover:bg-indigo-500/5 dark:hover:bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 disabled:opacity-50"
                >
                  {t('provider.customTime')}
                </Button>
              </div>
            </div>
          ) : (
            /* Custom Time Input */
            <div className="rounded-xl border border-indigo-500/30 dark:border-indigo-500/25 bg-indigo-500/5 dark:bg-indigo-500/10 p-3 space-y-2">
              <div className="text-xs font-medium text-indigo-600 dark:text-indigo-400">
                {t('provider.freezeUntil')}
              </div>
              <input
                type="text"
                value={customTimeInput}
                onChange={(e) => setCustomTimeInput(e.target.value)}
                placeholder="e.g. 30m, 2h, 14:30, 12:00:30, 2025-01-25 18:00"
                className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm font-mono"
                autoFocus
              />
              {/* 实时解析预览 */}
              <div className="text-xs text-muted-foreground">
                {customTimeInput ? (
                  parsedTime ? (
                    <span className="text-emerald-600 dark:text-emerald-400">
                      &rarr; {parsedTime.format('YYYY-MM-DD HH:mm:ss')}
                    </span>
                  ) : (
                    <span className="text-rose-500">{t('provider.invalidTimeFormat')}</span>
                  )
                ) : (
                  <span>{t('provider.timeFormatHint')}</span>
                )}
              </div>
              <div className="flex gap-2">
                <Button
                  onClick={() => {
                    setShowCustomTime(false);
                    setCustomTimeInput('');
                  }}
                  className="flex-1 rounded-lg border border-border bg-muted/50 px-3 py-1.5 text-xs"
                >
                  {t('common.cancel')}
                </Button>
                <Button
                  onClick={() => {
                    if (parsedTime) {
                      setCooldown(
                        provider.id,
                        parsedTime.toISOString(),
                        freezeMode === 'provider' ? '' : clientType,
                        freezeMode === 'model' ? freezeModel.trim() : undefined,
                      );
                      setShowCustomTime(false);
                      setCustomTimeInput('');
                    }
                  }}
                  disabled={!parsedTime}
                  className="flex-1 rounded-lg bg-indigo-500 text-white px-3 py-1.5 text-xs hover:bg-indigo-600 disabled:opacity-50"
                >
                  {t('provider.freezeConfirm')}
                </Button>
              </div>
            </div>
          )}
        </div>

        {/* Statistics Section */}
        <div className="px-4 py-3 border-t border-border/50">
          <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2">Statistics</div>
          {stats && stats.totalRequests > 0 ? (
            <div className="grid grid-cols-4 gap-2">
              <div className="p-2 rounded-lg bg-muted/50 text-center">
                <div className="text-[10px] text-muted-foreground">Requests</div>
                <div className="text-sm font-bold">{stats.totalRequests}</div>
              </div>
              <div className="p-2 rounded-lg bg-muted/50 text-center">
                <div className="text-[10px] text-muted-foreground">Success</div>
                <div className={cn('text-sm font-bold', stats.successRate >= 95 ? 'text-emerald-500' : 'text-amber-500')}>
                  {Math.round(stats.successRate)}%
                </div>
              </div>
              <div className="p-2 rounded-lg bg-muted/50 text-center">
                <div className="text-[10px] text-muted-foreground">Tokens</div>
                <div className="text-sm font-bold text-blue-500">{formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}</div>
              </div>
              <div className="p-2 rounded-lg bg-muted/50 text-center">
                <div className="text-[10px] text-muted-foreground">Cost</div>
                <div className="text-sm font-bold text-purple-500">{formatCost(stats.totalCost)}</div>
              </div>
            </div>
          ) : (
            <div className="py-4 text-center text-sm text-muted-foreground">No statistics available</div>
          )}
        </div>

        {/* Delete Section */}
        {onDelete && (
          <div className="px-4 py-3 border-t border-border/50">
            <Button onClick={onDelete} variant="destructive" className="w-full" size="sm">
              <Trash2 size={14} /> {t('provider.deleteRoute')}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
