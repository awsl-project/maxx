import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
} from '@/components/ui/dialog'
import {
  Snowflake,
  Clock,
  AlertCircle,
  Server,
  Wifi,
  Zap,
  Ban,
  HelpCircle,
  X,
  Thermometer,
  Calendar,
  Activity,
} from 'lucide-react'
import type { Cooldown, CooldownReason } from '@/lib/transport/types'
import { useCooldowns } from '@/hooks/use-cooldowns'

interface CooldownDetailsDialogProps {
  cooldown: Cooldown | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onClear: () => void
  isClearing: boolean
  onDisable: () => void
  isDisabling: boolean
}

// Reason 信息和图标 - 使用翻译
const getReasonInfo = (t: (key: string) => string) => ({
  server_error: {
    label: t('provider.reasons.serverError'),
    description: t('provider.reasons.serverErrorDesc', '上游服务器返回 5xx 错误，系统自动进入冷却保护'),
    icon: Server,
    color: 'text-red-400',
    bgColor: 'bg-red-400/10 border-red-400/20',
  },
  network_error: {
    label: t('provider.reasons.networkError'),
    description: t('provider.reasons.networkErrorDesc', '无法连接到上游服务器，可能是网络故障或服务器宕机'),
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
  unknown: {
    label: t('provider.reasons.unknown'),
    description: t('provider.reasons.unknownDesc', '因未知原因进入冷却状态'),
    icon: HelpCircle,
    color: 'text-muted-foreground',
    bgColor: 'bg-muted border-border',
  },
})

export function CooldownDetailsDialog({
  cooldown,
  open,
  onOpenChange,
  onClear,
  isClearing,
  onDisable,
  isDisabling,
}: CooldownDetailsDialogProps) {
  const { t, i18n } = useTranslation()
  const REASON_INFO = getReasonInfo(t)
  // 获取 formatRemaining 函数用于实时倒计时
  const { formatRemaining } = useCooldowns()

  // 计算初始倒计时值
  const getInitialCountdown = useCallback(() => {
    return cooldown ? formatRemaining(cooldown) : ''
  }, [cooldown, formatRemaining])

  // 实时倒计时状态
  const [liveCountdown, setLiveCountdown] = useState<string>(getInitialCountdown)

  // 每秒更新倒计时
  useEffect(() => {
    if (!cooldown) return

    // 每秒更新
    const interval = setInterval(() => {
      setLiveCountdown(formatRemaining(cooldown))
    }, 1000)

    return () => clearInterval(interval)
  }, [cooldown, formatRemaining])

  if (!cooldown) return null

  const reasonInfo = REASON_INFO[cooldown.reason] || REASON_INFO.unknown
  const Icon = reasonInfo.icon

  const formatUntilTime = (until: string) => {
    const date = new Date(until)
    return date.toLocaleString(i18n.resolvedLanguage ?? i18n.language, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  }

  const untilDateStr = formatUntilTime(cooldown.untilTime)
  const [datePart, timePart] = untilDateStr.split(' ')

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
              <Snowflake
                size={28}
                className="text-cyan-400 animate-spin-slow"
              />
            </div>
            <div>
              <h2 className="text-xl font-bold text-text-primary">
                {t('cooldown.title')}
              </h2>
              <p className="text-xs text-cyan-500/80 font-medium uppercase tracking-wider mt-1">
                Frozen Protocol Active
              </p>
            </div>
          </div>
        </div>

        {/* Body Content */}
        <div className="px-6 pb-6 space-y-5">
          {/* Provider Card */}
          <div className="flex items-center gap-4 p-3 rounded-xl bg-muted border border-border">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
                  Target Provider
                </span>
                {cooldown.clientType && (
                  <span className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-accent text-muted-foreground">
                    {cooldown.clientType}
                  </span>
                )}
              </div>
              <div className="font-semibold text-foreground truncate">
                Provider #{cooldown.providerID}
              </div>
            </div>
          </div>

          {/* Reason Section */}
          <div className={`rounded-xl border p-4 ${reasonInfo.bgColor}`}>
            <div className="flex gap-4">
              <div className={`mt-0.5 shrink-0 ${reasonInfo.color}`}>
                <Icon size={20} />
              </div>
              <div>
                <h3 className={`text-sm font-bold ${reasonInfo.color} mb-1`}>
                  {reasonInfo.label}
                </h3>
                <p className="text-xs text-muted-foreground leading-relaxed">
                  {reasonInfo.description}
                </p>
              </div>
            </div>
          </div>

          {/* Timer Section */}
          <div className="grid grid-cols-2 gap-3">
            {/* Countdown */}
            <div className="col-span-2 relative overflow-hidden rounded-xl bg-linear-to-br from-cyan-950/30 to-transparent border border-cyan-500/20 p-5 flex flex-col items-center justify-center group">
              <div className="absolute inset-0 bg-cyan-400/5 opacity-50 group-hover:opacity-100 transition-opacity" />
              <div className="relative flex items-center gap-1.5 text-cyan-500 mb-1">
                <Thermometer size={14} />
                <span className="text-[10px] font-bold uppercase tracking-widest">
                  Remaining
                </span>
              </div>
              <div className="relative font-mono text-4xl font-bold text-cyan-400 tracking-widest tabular-nums drop-shadow-[0_0_8px_rgba(34,211,238,0.3)]">
                {liveCountdown}
              </div>
            </div>

            {/* Time Details */}
            <div className="p-3 rounded-xl bg-muted border border-border flex flex-col items-center justify-center gap-1">
              <span className="text-[10px] text-muted-foreground uppercase tracking-wider font-bold flex items-center gap-1.5">
                <Clock size={10} /> Resume
              </span>
              <div className="font-mono text-sm font-semibold text-foreground">
                {timePart}
              </div>
            </div>

            <div className="p-3 rounded-xl bg-muted border border-border flex flex-col items-center justify-center gap-1">
              <span className="text-[10px] text-muted-foreground uppercase tracking-wider font-bold flex items-center gap-1.5">
                <Calendar size={10} /> Date
              </span>
              <div className="font-mono text-sm font-semibold text-foreground">
                {datePart}
              </div>
            </div>
          </div>

          {/* Actions */}
          <div className="space-y-3 pt-2">
            <button
              onClick={onClear}
              disabled={isClearing || isDisabling}
              className="w-full relative overflow-hidden rounded-xl p-[1px] group disabled:opacity-50 disabled:cursor-not-allowed transition-all hover:scale-[1.01] active:scale-[0.99]"
            >
              <span className="absolute inset-0 bg-gradient-to-r from-cyan-500 to-blue-600 rounded-xl" />
              <div className="relative flex items-center justify-center gap-2 rounded-[11px] bg-card group-hover:bg-transparent px-4 py-3 transition-colors">
                {isClearing ? (
                  <>
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                    <span className="text-sm font-bold text-white">
                      Thawing...
                    </span>
                  </>
                ) : (
                  <>
                    <Zap
                      size={16}
                      className="text-cyan-400 group-hover:text-white transition-colors"
                    />
                    <span className="text-sm font-bold text-cyan-400 group-hover:text-white transition-colors">
                      {t('cooldown.forceThaw')}
                    </span>
                  </>
                )}
              </div>
            </button>

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
        </div>
      </DialogContent>
    </Dialog>
  )
}
