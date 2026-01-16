import { useState, useEffect } from 'react'
import {
  Zap,
  Mail,
  ChevronLeft,
  Trash2,
  RefreshCw,
  Clock,
  AlertTriangle,
  Shuffle,
  Check,
} from 'lucide-react'
import { ClientIcon } from '@/components/icons/client-icons'
import type { Provider, KiroQuotaData } from '@/lib/transport'
import { getTransport } from '@/lib/transport'
import { KIRO_COLOR } from '../types'
import { ModelMappingEditor } from './model-mapping-editor'
import { useUpdateProvider } from '@/hooks/queries'
import { Button } from '@/components/ui/button'

interface KiroProviderViewProps {
  provider: Provider
  onDelete: () => void
  onClose: () => void
}

// 配额条的颜色
function getQuotaColor(percentage: number): string {
  if (percentage >= 50) return 'bg-success'
  if (percentage >= 20) return 'bg-warning'
  return 'bg-error'
}

// 格式化重置天数
function formatResetDays(days: number): string {
  if (days <= 0) return 'Soon'
  if (days === 1) return '1 day'
  return `${days} days`
}

// 订阅等级徽章
function SubscriptionBadge({ type }: { type: string }) {
  const styles: Record<string, string> = {
    PRO: 'bg-gradient-to-r from-blue-500 to-indigo-500 text-white',
    FREE: 'bg-gray-500/20 text-gray-400',
  }

  return (
    <span
      className={`px-2.5 py-1 rounded-full text-xs font-semibold ${styles[type] || styles.FREE}`}
    >
      {type || 'FREE'}
    </span>
  )
}

// 配额卡片
function QuotaCard({ quota }: { quota: KiroQuotaData }) {
  const percentage = quota.total_limit > 0
    ? Math.round((quota.available / quota.total_limit) * 100)
    : 0
  const color = getQuotaColor(percentage)

  return (
    <div className="bg-surface-primary border border-border rounded-xl p-6">
      <div className="flex items-center justify-between mb-4">
        <span className="font-medium text-text-primary text-lg">
          Usage Quota
        </span>
        <span className="text-sm text-text-secondary flex items-center gap-1.5">
          <Clock size={14} />
          Resets in {formatResetDays(quota.days_until_reset)}
        </span>
      </div>

      {/* Progress Bar */}
      <div className="flex items-center gap-4 mb-4">
        <div className="flex-1 h-3 bg-surface-hover rounded-full overflow-hidden">
          <div
            className={`h-full ${color} transition-all duration-300`}
            style={{ width: `${percentage}%` }}
          />
        </div>
        <span className="text-lg font-bold text-text-primary min-w-[4rem] text-right">
          {percentage}%
        </span>
      </div>

      {/* Quota Details */}
      <div className="grid grid-cols-3 gap-4 pt-4 border-t border-border/50">
        <div className="text-center">
          <div className="text-2xl font-bold text-text-primary">
            {quota.available.toFixed(1)}
          </div>
          <div className="text-xs text-text-secondary uppercase tracking-wider mt-1">
            Available
          </div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold text-text-muted">
            {quota.used.toFixed(1)}
          </div>
          <div className="text-xs text-text-secondary uppercase tracking-wider mt-1">
            Used
          </div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold text-text-muted">
            {quota.total_limit.toFixed(1)}
          </div>
          <div className="text-xs text-text-secondary uppercase tracking-wider mt-1">
            Total
          </div>
        </div>
      </div>

      {/* Free Trial Status */}
      {quota.free_trial_status && (
        <div className="mt-4 pt-4 border-t border-border/50">
          <span className="text-xs text-text-secondary">
            Free Trial: <span className="text-emerald-500 font-medium">{quota.free_trial_status}</span>
          </span>
        </div>
      )}
    </div>
  )
}

export function KiroProviderView({
  provider,
  onDelete,
  onClose,
}: KiroProviderViewProps) {
  const [quota, setQuota] = useState<KiroQuotaData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [modelMapping, setModelMapping] = useState<Record<string, string>>(
    provider.config?.kiro?.modelMapping || {}
  )
  const [savingMapping, setSavingMapping] = useState(false)
  const [mappingSaveStatus, setMappingSaveStatus] = useState<
    'idle' | 'success' | 'error'
  >('idle')
  const updateProvider = useUpdateProvider()

  const fetchQuota = async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await getTransport().getKiroProviderQuota(provider.id)
      setQuota(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch quota')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchQuota()
  }, [provider.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleSaveModelMapping = async () => {
    setSavingMapping(true)
    setMappingSaveStatus('idle')
    try {
      const kiroConfig = provider.config?.kiro
      if (!kiroConfig) return

      await updateProvider.mutateAsync({
        id: Number(provider.id),
        data: {
          name: provider.name,
          type: 'kiro',
          config: {
            kiro: {
              authMethod: kiroConfig.authMethod,
              email: kiroConfig.email,
              refreshToken: kiroConfig.refreshToken,
              region: kiroConfig.region,
              clientID: kiroConfig.clientID,
              clientSecret: kiroConfig.clientSecret,
              modelMapping:
                Object.keys(modelMapping).length > 0 ? modelMapping : undefined,
            },
          },
          supportedClientTypes: provider.supportedClientTypes,
        },
      })
      setMappingSaveStatus('success')
      setTimeout(() => setMappingSaveStatus('idle'), 2000)
    } catch (err) {
      console.error('Failed to save model mapping:', err)
      setMappingSaveStatus('error')
    } finally {
      setSavingMapping(false)
    }
  }

  const hasModelMappingChanged =
    JSON.stringify(modelMapping) !==
    JSON.stringify(provider.config?.kiro?.modelMapping || {})

  return (
    <div className="flex flex-col h-full">
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary">
        <div className="flex items-center gap-4">
          <button
            onClick={onClose}
            className="p-1.5 -ml-1 rounded-lg hover:bg-surface-hover text-text-secondary hover:text-text-primary transition-colors"
          >
            <ChevronLeft size={20} />
          </button>
          <div>
            <h2 className="text-headline font-semibold text-text-primary">
              {provider.name}
            </h2>
            <p className="text-caption text-text-secondary">
              Kiro Provider
            </p>
          </div>
        </div>
        <button
          onClick={onDelete}
          className="btn bg-error/10 text-error hover:bg-error/20 flex items-center gap-2"
        >
          <Trash2 size={14} />
          Delete
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          {/* Info Card */}
          <div className="bg-surface-secondary rounded-xl p-6 border border-border">
            <div className="flex items-start justify-between gap-6">
              <div className="flex items-center gap-4">
                <div
                  className="w-16 h-16 rounded-2xl flex items-center justify-center shadow-sm"
                  style={{ backgroundColor: `${KIRO_COLOR}15` }}
                >
                  <Zap size={32} style={{ color: KIRO_COLOR }} />
                </div>
                <div>
                  <div className="flex items-center gap-3">
                    <h3 className="text-xl font-bold text-text-primary">
                      {provider.name}
                    </h3>
                    {quota?.subscription_type && (
                      <SubscriptionBadge type={quota.subscription_type} />
                    )}
                  </div>
                  <div className="text-sm text-text-secondary flex items-center gap-1.5 mt-1">
                    <Mail size={14} />
                    {quota?.email || provider.config?.kiro?.email || 'Kiro Account'}
                  </div>
                </div>
              </div>

              <div className="flex flex-col items-end gap-1 text-right">
                <div className="text-xs text-text-secondary uppercase tracking-wider font-semibold">
                  Auth Method
                </div>
                <div className="text-sm font-mono text-text-primary bg-surface-primary px-2 py-1 rounded border border-border/50 uppercase">
                  {provider.config?.kiro?.authMethod || 'social'}
                </div>
              </div>
            </div>

            <div className="mt-6 pt-6 border-t border-border/50 grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <div className="text-xs text-text-secondary uppercase tracking-wider font-semibold mb-1.5">
                  Region
                </div>
                <div className="font-mono text-sm text-text-primary">
                  {provider.config?.kiro?.region || 'us-east-1'}
                </div>
              </div>
            </div>
          </div>

          {/* Quota Section */}
          <div>
            <div className="flex items-center justify-between mb-4 border-b border-border pb-2">
              <h4 className="text-lg font-semibold text-text-primary">
                Usage Quota
              </h4>
              <button
                onClick={() => fetchQuota()}
                disabled={loading}
                className="btn bg-surface-secondary hover:bg-surface-hover text-text-primary flex items-center gap-2 text-sm"
              >
                <RefreshCw
                  size={14}
                  className={loading ? 'animate-spin' : ''}
                />
                Refresh
              </button>
            </div>

            {error && (
              <div className="bg-error/10 border border-error/20 rounded-lg p-4 mb-4">
                <p className="text-sm text-error">{error}</p>
              </div>
            )}

            {quota?.is_banned ? (
              <div className="bg-error/10 border border-error/20 rounded-xl p-6 flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-error/20 flex items-center justify-center">
                  <AlertTriangle size={24} className="text-error" />
                </div>
                <div>
                  <h5 className="font-semibold text-error">Account Banned</h5>
                  <p className="text-sm text-error/80">
                    {quota.ban_reason || 'This account has been restricted.'}
                  </p>
                </div>
              </div>
            ) : quota ? (
              <QuotaCard quota={quota} />
            ) : loading ? (
              <div className="bg-surface-primary border border-border rounded-xl p-6 animate-pulse">
                <div className="h-4 bg-surface-hover rounded w-24 mb-4" />
                <div className="h-3 bg-surface-hover rounded w-full mb-4" />
                <div className="grid grid-cols-3 gap-4">
                  <div className="h-12 bg-surface-hover rounded" />
                  <div className="h-12 bg-surface-hover rounded" />
                  <div className="h-12 bg-surface-hover rounded" />
                </div>
              </div>
            ) : (
              <div className="text-center py-8 text-text-muted bg-surface-secondary/30 rounded-xl border border-dashed border-border">
                No quota information available
              </div>
            )}

            {quota?.last_updated && (
              <p className="text-xs text-text-muted mt-4 text-right">
                Last updated:{' '}
                {new Date(quota.last_updated * 1000).toLocaleString()}
              </p>
            )}
          </div>

          {/* Model Mapping */}
          <div>
            <div className="flex items-center justify-between mb-4 border-b border-border pb-2">
              <h4 className="text-lg font-semibold text-text-primary flex items-center gap-2">
                <Shuffle size={18} />
                Model Mapping
              </h4>
              {hasModelMappingChanged && (
                <Button
                  onClick={handleSaveModelMapping}
                  disabled={savingMapping}
                  variant="default"
                  size="sm"
                >
                  {savingMapping ? (
                    'Saving...'
                  ) : mappingSaveStatus === 'success' ? (
                    <>
                      <Check size={14} /> Saved
                    </>
                  ) : (
                    'Save Changes'
                  )}
                </Button>
              )}
            </div>
            <p className="text-sm text-text-secondary mb-4">
              Map request models to different upstream models. For example, map
              "claude-sonnet-4-20250514" to a Kiro-supported model.
            </p>
            <ModelMappingEditor value={modelMapping} onChange={setModelMapping} />
            {mappingSaveStatus === 'error' && (
              <p className="text-sm text-error mt-2">
                Failed to save model mapping. Please try again.
              </p>
            )}
          </div>

          {/* Supported Clients */}
          <div>
            <h4 className="text-lg font-semibold text-text-primary mb-4 border-b border-border pb-2">
              Supported Clients
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
              {provider.supportedClientTypes?.length > 0 ? (
                provider.supportedClientTypes.map(ct => (
                  <div
                    key={ct}
                    className="flex items-center gap-3 bg-surface-primary border border-border rounded-xl p-4 shadow-sm"
                  >
                    <ClientIcon type={ct} size={28} />
                    <div>
                      <div className="text-sm font-semibold text-text-primary capitalize">
                        {ct}
                      </div>
                      <div className="text-xs text-text-secondary">Enabled</div>
                    </div>
                  </div>
                ))
              ) : (
                <div className="col-span-full text-center py-8 text-text-muted bg-surface-secondary/30 rounded-xl border border-dashed border-border">
                  No clients configured
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
