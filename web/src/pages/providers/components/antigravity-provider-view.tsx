import { useState, useEffect, useMemo } from 'react';
import {
  Wand2,
  Mail,
  ChevronLeft,
  Trash2,
  RefreshCw,
  Clock,
  Lock,
  Plus,
  ArrowRight,
  Zap,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { ClientIcon } from '@/components/icons/client-icons';
import type {
  Provider,
  AntigravityQuotaData,
  AntigravityModelQuota,
  ModelMapping,
  ModelMappingInput,
} from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import {
  useModelMappings,
  useCreateModelMapping,
  useUpdateModelMapping,
  useDeleteModelMapping,
} from '@/hooks/queries';
import { Button } from '@/components/ui';
import { ModelInput } from '@/components/ui/model-input';
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
function formatResetTime(resetTime: string, t: (key: string) => string): string {
  if (!resetTime) return t('proxy.comingSoon');

  try {
    const reset = new Date(resetTime);
    const now = new Date();
    const diff = reset.getTime() - now.getTime();

    if (diff <= 0) return t('proxy.comingSoon');

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
    <span
      className={`px-2.5 py-1 rounded-full text-xs font-semibold ${styles[tier] || styles.FREE}`}
    >
      {tier || 'FREE'}
    </span>
  );
}

// 模型配额卡片
function ModelQuotaCard({ model }: { model: AntigravityModelQuota }) {
  const { t } = useTranslation();
  const displayName = modelDisplayNames[model.name] || model.name;
  const color = getQuotaColor(model.percentage);

  return (
    <div className="bg-card border border-border rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <span className="font-medium text-foreground text-sm">{displayName}</span>
        <span className="text-xs text-muted-foreground flex items-center gap-1">
          <Clock size={12} />
          {t('proxy.resetsIn')} {formatResetTime(model.resetTime, t)}
        </span>
      </div>
      <div className="flex items-center gap-3">
        <div className="flex-1 h-2 bg-accent rounded-full overflow-hidden">
          <div
            className={`h-full ${color} transition-all duration-300`}
            style={{ width: `${model.percentage}%` }}
          />
        </div>
        <span className="text-sm font-medium text-foreground min-w-[3rem] text-right">
          {model.percentage}%
        </span>
      </div>
    </div>
  );
}

// Provider Model Mappings Section
function ProviderModelMappings({ provider }: { provider: Provider }) {
  const { t } = useTranslation();
  const { data: allMappings } = useModelMappings();
  const createMapping = useCreateModelMapping();
  const updateMapping = useUpdateModelMapping();
  const deleteMapping = useDeleteModelMapping();
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');

  // Filter mappings for this provider
  const providerMappings = useMemo(() => {
    return (allMappings || []).filter(
      (m) => m.scope === 'provider' && m.providerID === provider.id,
    );
  }, [allMappings, provider.id]);

  const isPending = createMapping.isPending || updateMapping.isPending || deleteMapping.isPending;

  const handleAddMapping = async () => {
    if (!newPattern.trim() || !newTarget.trim()) return;

    await createMapping.mutateAsync({
      pattern: newPattern.trim(),
      target: newTarget.trim(),
      scope: 'provider',
      providerID: provider.id,
      providerType: 'antigravity',
      priority: providerMappings.length * 10 + 1000,
      isEnabled: true,
    });
    setNewPattern('');
    setNewTarget('');
  };

  const handleUpdateMapping = async (mapping: ModelMapping, data: Partial<ModelMappingInput>) => {
    await updateMapping.mutateAsync({
      id: mapping.id,
      data: {
        pattern: data.pattern ?? mapping.pattern,
        target: data.target ?? mapping.target,
        scope: 'provider',
        providerID: provider.id,
        providerType: 'antigravity',
        priority: mapping.priority,
        isEnabled: mapping.isEnabled,
      },
    });
  };

  const handleDeleteMapping = async (id: number) => {
    await deleteMapping.mutateAsync(id);
  };

  return (
    <div>
      <div className="flex items-center gap-2 mb-4 border-b border-border pb-2">
        <Zap size={18} className="text-yellow-500" />
        <h4 className="text-lg font-semibold text-foreground">{t('modelMappings.title')}</h4>
        <span className="text-sm text-muted-foreground">({providerMappings.length})</span>
      </div>

      <div className="bg-card border border-border rounded-xl p-4">
        <p className="text-xs text-muted-foreground mb-4">{t('modelMappings.pageDesc')}</p>

        {providerMappings.length > 0 && (
          <div className="space-y-2 mb-4">
            {providerMappings.map((mapping, index) => (
              <div key={mapping.id} className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground w-6 shrink-0">{index + 1}.</span>
                <ModelInput
                  value={mapping.pattern}
                  onChange={(pattern) => handleUpdateMapping(mapping, { pattern })}
                  placeholder={t('modelMappings.matchPattern')}
                  disabled={isPending}
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
                <ModelInput
                  value={mapping.target}
                  onChange={(target) => handleUpdateMapping(mapping, { target })}
                  placeholder={t('modelMappings.targetModel')}
                  disabled={isPending}
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleDeleteMapping(mapping.id)}
                  disabled={isPending}
                >
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>
            ))}
          </div>
        )}

        {providerMappings.length === 0 && (
          <div className="text-center py-6 mb-4">
            <p className="text-muted-foreground text-sm">{t('modelMappings.noMappings')}</p>
          </div>
        )}

        <div className="flex items-center gap-2 pt-4 border-t border-border">
          <ModelInput
            value={newPattern}
            onChange={setNewPattern}
            placeholder={t('modelMappings.matchPattern')}
            disabled={isPending}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
          <ModelInput
            value={newTarget}
            onChange={setNewTarget}
            placeholder={t('modelMappings.targetModel')}
            disabled={isPending}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddMapping}
            disabled={!newPattern.trim() || !newTarget.trim() || isPending}
          >
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function AntigravityProviderView({
  provider,
  onDelete,
  onClose,
}: AntigravityProviderViewProps) {
  const { t } = useTranslation();
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
  }, [provider.id]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex flex-col h-full">
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-card">
        <div className="flex items-center gap-4">
          <button
            onClick={onClose}
            className="p-1.5 -ml-1 rounded-lg hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
          >
            <ChevronLeft size={20} />
          </button>
          <div>
            <h2 className="text-headline font-semibold text-foreground">{provider.name}</h2>
            <p className="text-caption text-muted-foreground">{t('providers.antigravityType')}</p>
          </div>
        </div>
        <Button onClick={onDelete} variant="destructive">
          <Trash2 size={14} />
          {t('common.delete')}
        </Button>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          {/* Info Card */}
          <div className="bg-muted rounded-xl p-6 border border-border">
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
                    <h3 className="text-xl font-bold text-foreground">{provider.name}</h3>
                    {quota?.subscriptionTier && <SubscriptionBadge tier={quota.subscriptionTier} />}
                  </div>
                  <div className="text-sm text-muted-foreground flex items-center gap-1.5 mt-1">
                    <Mail size={14} />
                    {provider.config?.antigravity?.email || 'Unknown'}
                  </div>
                </div>
              </div>

              <div className="flex flex-col items-end gap-1 text-right">
                <div className="text-xs text-muted-foreground uppercase tracking-wider font-semibold">
                  {t('providers.projectId')}
                </div>
                <div className="text-sm font-mono text-foreground bg-card px-2 py-1 rounded border border-border/50">
                  {provider.config?.antigravity?.projectID || '-'}
                </div>
              </div>
            </div>

            <div className="mt-6 pt-6 border-t border-border/50 grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <div className="text-xs text-muted-foreground uppercase tracking-wider font-semibold mb-1.5">
                  {t('providers.endpoint')}
                </div>
                <div className="font-mono text-sm text-foreground break-all">
                  {provider.config?.antigravity?.endpoint || '-'}
                </div>
              </div>
            </div>
          </div>

          {/* Quota Section */}
          <div>
            <div className="flex items-center justify-between mb-4 border-b border-border pb-2">
              <h4 className="text-lg font-semibold text-foreground">{t('providers.modelQuotas')}</h4>
              <button
                onClick={() => fetchQuota(true)}
                disabled={loading}
                className="btn bg-muted hover:bg-accent text-foreground flex items-center gap-2 text-sm"
              >
                <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
                {t('providers.refresh')}
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
                  <h5 className="font-semibold text-error">{t('providers.accessForbidden')}</h5>
                  <p className="text-sm text-error/80">
                    {t('providers.accountRestricted')}
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
              <div className="text-center py-8 text-muted-foreground bg-muted/30 rounded-xl border border-dashed border-border">
                {t('providers.noQuotaInfo')}
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {[1, 2, 3, 4].map((i) => (
                  <div
                    key={i}
                    className="bg-card border border-border rounded-xl p-4 animate-pulse"
                  >
                    <div className="h-4 bg-accent rounded w-24 mb-3" />
                    <div className="h-2 bg-accent rounded w-full" />
                  </div>
                ))}
              </div>
            )}

            {quota?.lastUpdated && (
              <p className="text-xs text-muted-foreground mt-4 text-right">
                {t('providers.lastUpdated', { time: new Date(quota.lastUpdated * 1000).toLocaleString() })}
              </p>
            )}
          </div>

          {/* Provider Model Mappings */}
          <ProviderModelMappings provider={provider} />

          {/* Supported Clients */}
          <div>
            <h4 className="text-lg font-semibold text-foreground mb-4 border-b border-border pb-2">
              {t('providers.supportedClients')}
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
              {provider.supportedClientTypes?.length > 0 ? (
                provider.supportedClientTypes.map((ct) => (
                  <div
                    key={ct}
                    className="flex items-center gap-3 bg-card border border-border rounded-xl p-4 shadow-sm"
                  >
                    <ClientIcon type={ct} size={28} />
                    <div>
                      <div className="text-sm font-semibold text-foreground capitalize">{ct}</div>
                      <div className="text-xs text-muted-foreground">{t('providers.enabled')}</div>
                    </div>
                  </div>
                ))
              ) : (
                <div className="col-span-full text-center py-8 text-muted-foreground bg-muted/30 rounded-xl border border-dashed border-border">
                  {t('providers.noClientsConfigured')}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
