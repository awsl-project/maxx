import { useState, useEffect } from 'react';
import { Wand2, Mail, ChevronLeft, Trash2, RefreshCw, Clock, Lock } from 'lucide-react';
import { ClientIcon } from '@/components/icons/client-icons';
import type { Provider, AntigravityQuotaData, AntigravityModelQuota } from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import { ANTIGRAVITY_COLOR } from '../types';

interface AntigravityProviderViewProps {
  provider: Provider;
  onDelete: () => void;
  onClose: () => void;
}

// 友好的模型名称
const modelDisplayNames: Record<string, string> = {
  'gemini-3-pro-high': 'Gemini 3 Pro',
  'gemini-3-flash': 'Gemini 3 Flash',
  'gemini-3-pro-image': 'Gemini 3 Pro Image',
  'claude-sonnet-4-5-thinking': 'Claude Sonnet 4.5',
};

// 配额条的颜色
function getQuotaColor(percentage: number): string {
  if (percentage >= 50) return 'bg-success';
  if (percentage >= 20) return 'bg-warning';
  return 'bg-error';
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
      return `${days}d ${hours % 24}h`;
    }
    if (hours > 0) {
      return `${hours}h ${minutes}m`;
    }
    return `${minutes}m`;
  } catch {
    return '-';
  }
}

// 订阅等级徽章
function SubscriptionBadge({ tier }: { tier: string }) {
  const styles: Record<string, string> = {
    ULTRA: 'bg-gradient-to-r from-purple-500 to-pink-500 text-white',
    PRO: 'bg-gradient-to-r from-blue-500 to-indigo-500 text-white',
    FREE: 'bg-gray-500/20 text-gray-400',
  };

  return (
    <span className={`px-2.5 py-1 rounded-full text-xs font-semibold ${styles[tier] || styles.FREE}`}>
      {tier || 'FREE'}
    </span>
  );
}

// 模型配额卡片
function ModelQuotaCard({ model }: { model: AntigravityModelQuota }) {
  const displayName = modelDisplayNames[model.name] || model.name;
  const color = getQuotaColor(model.percentage);

  return (
    <div className="bg-surface-primary border border-border rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <span className="font-medium text-text-primary text-sm">{displayName}</span>
        <span className="text-xs text-text-secondary flex items-center gap-1">
          <Clock size={12} />
          {formatResetTime(model.resetTime)}
        </span>
      </div>
      <div className="flex items-center gap-3">
        <div className="flex-1 h-2 bg-surface-hover rounded-full overflow-hidden">
          <div
            className={`h-full ${color} transition-all duration-300`}
            style={{ width: `${model.percentage}%` }}
          />
        </div>
        <span className="text-sm font-medium text-text-primary min-w-[3rem] text-right">
          {model.percentage}%
        </span>
      </div>
    </div>
  );
}

export function AntigravityProviderView({ provider, onDelete, onClose }: AntigravityProviderViewProps) {
  const [quota, setQuota] = useState<AntigravityQuotaData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchQuota = async (forceRefresh = false) => {
    setLoading(true);
    setError(null);
    try {
      const data = await getTransport().getAntigravityProviderQuota(provider.id, forceRefresh);
      setQuota(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch quota');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchQuota(false);
  }, [provider.id]);

  return (
    <div className="flex flex-col h-full">
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary">
        <div className="flex items-center gap-md">
          <button
            onClick={onClose}
            className="p-1.5 -ml-1 rounded-lg hover:bg-surface-hover text-text-secondary hover:text-text-primary transition-colors"
          >
            <ChevronLeft size={20} />
          </button>
          <div>
            <h2 className="text-headline font-semibold text-text-primary">{provider.name}</h2>
            <p className="text-caption text-text-secondary">Antigravity Provider</p>
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
                  style={{ backgroundColor: `${ANTIGRAVITY_COLOR}15` }}
                >
                  <Wand2 size={32} style={{ color: ANTIGRAVITY_COLOR }} />
                </div>
                <div>
                  <div className="flex items-center gap-3">
                    <h3 className="text-xl font-bold text-text-primary">{provider.name}</h3>
                    {quota?.subscriptionTier && <SubscriptionBadge tier={quota.subscriptionTier} />}
                  </div>
                  <div className="text-sm text-text-secondary flex items-center gap-1.5 mt-1">
                    <Mail size={14} />
                    {provider.config?.antigravity?.email || 'Unknown'}
                  </div>
                </div>
              </div>

              <div className="flex flex-col items-end gap-1 text-right">
                <div className="text-xs text-text-secondary uppercase tracking-wider font-semibold">Project ID</div>
                <div className="text-sm font-mono text-text-primary bg-surface-primary px-2 py-1 rounded border border-border/50">
                  {provider.config?.antigravity?.projectID || '-'}
                </div>
              </div>
            </div>

            <div className="mt-6 pt-6 border-t border-border/50 grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <div className="text-xs text-text-secondary uppercase tracking-wider font-semibold mb-1.5">Endpoint</div>
                <div className="font-mono text-sm text-text-primary break-all">
                  {provider.config?.antigravity?.endpoint || '-'}
                </div>
              </div>
            </div>
          </div>

          {/* Quota Section */}
          <div>
            <div className="flex items-center justify-between mb-4 border-b border-border pb-2">
              <h4 className="text-lg font-semibold text-text-primary">Model Quotas</h4>
              <button
                onClick={() => fetchQuota(true)}
                disabled={loading}
                className="btn bg-surface-secondary hover:bg-surface-hover text-text-primary flex items-center gap-2 text-sm"
              >
                <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
                Refresh
              </button>
            </div>

            {error && (
              <div className="bg-error/10 border border-error/20 rounded-lg p-4 mb-4">
                <p className="text-sm text-error">{error}</p>
              </div>
            )}

            {quota?.isForbidden ? (
              <div className="bg-error/10 border border-error/20 rounded-xl p-6 flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-error/20 flex items-center justify-center">
                  <Lock size={24} className="text-error" />
                </div>
                <div>
                  <h5 className="font-semibold text-error">Access Forbidden</h5>
                  <p className="text-sm text-error/80">
                    This account has been restricted. Please check your Google Cloud account status.
                  </p>
                </div>
              </div>
            ) : quota?.models && quota.models.length > 0 ? (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {quota.models.map((model) => (
                  <ModelQuotaCard key={model.name} model={model} />
                ))}
              </div>
            ) : !loading ? (
              <div className="text-center py-8 text-text-muted bg-surface-secondary/30 rounded-xl border border-dashed border-border">
                No quota information available
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {[1, 2, 3, 4].map((i) => (
                  <div key={i} className="bg-surface-primary border border-border rounded-xl p-4 animate-pulse">
                    <div className="h-4 bg-surface-hover rounded w-24 mb-3" />
                    <div className="h-2 bg-surface-hover rounded w-full" />
                  </div>
                ))}
              </div>
            )}

            {quota?.lastUpdated && (
              <p className="text-xs text-text-muted mt-4 text-right">
                Last updated: {new Date(quota.lastUpdated * 1000).toLocaleString()}
              </p>
            )}
          </div>

          {/* Supported Clients */}
          <div>
            <h4 className="text-lg font-semibold text-text-primary mb-4 border-b border-border pb-2">Supported Clients</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
              {provider.supportedClientTypes?.length > 0 ? (
                provider.supportedClientTypes.map((ct) => (
                  <div key={ct} className="flex items-center gap-3 bg-surface-primary border border-border rounded-xl p-4 shadow-sm">
                    <ClientIcon type={ct} size={28} />
                    <div>
                      <div className="text-sm font-semibold text-text-primary capitalize">{ct}</div>
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
  );
}
