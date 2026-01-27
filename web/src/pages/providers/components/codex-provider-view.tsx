import { useState, useMemo } from 'react';
import {
  Code2,
  Mail,
  ChevronLeft,
  Trash2,
  RefreshCw,
  Clock,
  User,
  Calendar,
  Crown,
  Plus,
  ArrowRight,
  Zap,
  AlertCircle,
  Gauge,
  Copy,
  Check,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { ClientIcon } from '@/components/icons/client-icons';
import type { Provider, ModelMapping, ModelMappingInput, CodexUsageResponse, CodexUsageWindow } from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import {
  useModelMappings,
  useCreateModelMapping,
  useUpdateModelMapping,
  useDeleteModelMapping,
} from '@/hooks/queries';
import { Button } from '@/components/ui';
import { ModelInput } from '@/components/ui/model-input';
import { CODEX_COLOR } from '../types';
import { useCodexBatchQuotas } from '@/hooks/queries';

interface CodexProviderViewProps {
  provider: Provider;
  onDelete: () => void;
  onClose: () => void;
}

// Plan type display names
const planTypeDisplayNames: Record<string, string> = {
  chatgptplusplan: 'ChatGPT Plus',
  chatgptteamplan: 'ChatGPT Team',
  chatgptenterprise: 'ChatGPT Enterprise',
  chatgptpro: 'ChatGPT Pro',
};

// Plan type badge styles
function PlanTypeBadge({ planType }: { planType: string }) {
  const displayName = planTypeDisplayNames[planType?.toLowerCase()] || planType || 'Unknown';

  const styles: Record<string, string> = {
    chatgptplusplan: 'bg-gradient-to-r from-green-500 to-emerald-500 text-white',
    chatgptteamplan: 'bg-gradient-to-r from-blue-500 to-indigo-500 text-white',
    chatgptenterprise: 'bg-gradient-to-r from-purple-500 to-pink-500 text-white',
    chatgptpro: 'bg-gradient-to-r from-amber-500 to-orange-500 text-white',
  };

  const style = styles[planType?.toLowerCase()] || 'bg-gray-500/20 text-gray-400';

  return (
    <span className={`px-2.5 py-1 rounded-full text-xs font-semibold ${style}`}>{displayName}</span>
  );
}

// Format date for display
function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  try {
    const date = new Date(dateStr);
    return date.toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  } catch {
    return '-';
  }
}

// Format relative time
function formatRelativeTime(dateStr: string | undefined, t: (key: string) => string): string {
  if (!dateStr) return '-';
  try {
    const date = new Date(dateStr);
    const now = new Date();
    const diff = date.getTime() - now.getTime();

    if (diff <= 0) return t('providers.expired');

    const days = Math.floor(diff / (1000 * 60 * 60 * 24));
    const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));

    if (days > 30) {
      const months = Math.floor(days / 30);
      return `${months}mo ${days % 30}d`;
    }
    if (days > 0) {
      return `${days}d ${hours}h`;
    }
    return `${hours}h`;
  } catch {
    return '-';
  }
}

// Format reset time from seconds
function formatResetTime(window: CodexUsageWindow | undefined | null, t: (key: string, params?: Record<string, unknown>) => string): string {
  if (!window) return '-';

  // Try resetAfterSeconds first
  if (window.resetAfterSeconds !== undefined && window.resetAfterSeconds !== null) {
    const seconds = window.resetAfterSeconds;
    if (seconds <= 0) return t('providers.codex.resetNow');

    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);

    if (hours > 0) {
      return t('providers.codex.resetIn', { time: `${hours}h ${minutes}m` });
    }
    return t('providers.codex.resetIn', { time: `${minutes}m` });
  }

  // Try resetAt (unix timestamp)
  if (window.resetAt !== undefined && window.resetAt !== null) {
    const resetDate = new Date(window.resetAt * 1000);
    const now = new Date();
    const diff = resetDate.getTime() - now.getTime();

    if (diff <= 0) return t('providers.codex.resetNow');

    const hours = Math.floor(diff / (1000 * 60 * 60));
    const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

    if (hours > 0) {
      return t('providers.codex.resetIn', { time: `${hours}h ${minutes}m` });
    }
    return t('providers.codex.resetIn', { time: `${minutes}m` });
  }

  return '-';
}

// Calculate remaining percentage from used percentage
function getRemainingPercent(window: CodexUsageWindow | undefined | null): number | null {
  if (!window || window.usedPercent === undefined || window.usedPercent === null) {
    return null;
  }
  return Math.max(0, Math.min(100, 100 - window.usedPercent));
}

// Quota progress bar component
function QuotaProgressBar({ percent, label, resetLabel }: { percent: number | null; label: string; resetLabel: string }) {
  const displayPercent = percent ?? 0;
  const isUnknown = percent === null;

  // Color based on remaining percentage
  const getBarColor = () => {
    if (isUnknown) return 'bg-gray-400';
    if (displayPercent >= 60) return 'bg-green-500';
    if (displayPercent >= 20) return 'bg-yellow-500';
    return 'bg-red-500';
  };

  return (
    <div className="bg-card border border-border rounded-xl p-4">
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm font-medium text-foreground">{label}</span>
        <span className="text-sm font-semibold text-foreground">
          {isUnknown ? '--' : `${Math.round(displayPercent)}%`}
        </span>
      </div>
      <div className="w-full bg-muted rounded-full h-2.5 overflow-hidden">
        <div
          className={`h-full rounded-full transition-all duration-300 ${getBarColor()}`}
          style={{ width: `${isUnknown ? 0 : displayPercent}%` }}
        />
      </div>
      <div className="text-xs text-muted-foreground mt-2">{resetLabel}</div>
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
      providerType: 'codex',
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
        providerType: 'codex',
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

export function CodexProviderView({ provider, onDelete, onClose }: CodexProviderViewProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [usage, setUsage] = useState<CodexUsageResponse | null>(null);
  const [usageLoading, setUsageLoading] = useState(false);
  const [usageError, setUsageError] = useState<string | null>(null);
  const [tokenCopied, setTokenCopied] = useState(false);

  const config = provider.config?.codex;

  const handleCopyToken = async () => {
    const token = config?.refreshToken;
    if (!token) return;
    try {
      await navigator.clipboard.writeText(token);
      setTokenCopied(true);
      setTimeout(() => setTokenCopied(false), 2000);
    } catch {
      // Failed to copy
    }
  };

  // 从 React Query 缓存获取配额数据（不会自动请求 API，因为 staleTime 设置为 Infinity）
  const { data: batchQuotas } = useCodexBatchQuotas(true);
  const cachedQuota = batchQuotas?.[provider.id];

  // 将缓存的配额数据转换为 usage 格式
  const displayUsage = useMemo((): CodexUsageResponse | null => {
    // 优先使用手动刷新获取的数据
    if (usage) return usage;
    // 否则使用缓存的数据
    if (!cachedQuota) return null;
    return {
      planType: cachedQuota.planType,
      rateLimit: {
        allowed: true,
        primaryWindow: cachedQuota.primaryWindow,
        secondaryWindow: cachedQuota.secondaryWindow,
      },
      codeReviewRateLimit: cachedQuota.codeReviewWindow ? {
        allowed: true,
        primaryWindow: cachedQuota.codeReviewWindow,
      } : undefined,
    };
  }, [usage, cachedQuota]);

  const handleRefresh = async () => {
    setLoading(true);
    setError(null);
    try {
      await getTransport().refreshCodexProviderInfo(provider.id);
      // Invalidate providers query to refresh the data
      queryClient.invalidateQueries({ queryKey: ['providers'] });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to refresh');
    } finally {
      setLoading(false);
    }
  };

  const handleRefreshUsage = async () => {
    setUsageLoading(true);
    setUsageError(null);
    try {
      const data = await getTransport().getCodexProviderUsage(provider.id);
      setUsage(data);
    } catch (err) {
      setUsageError(err instanceof Error ? err.message : 'Failed to fetch usage');
    } finally {
      setUsageLoading(false);
    }
  };

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
            <p className="text-caption text-muted-foreground">{t('providers.codexType')}</p>
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
                  style={{ backgroundColor: `${CODEX_COLOR}15` }}
                >
                  <Code2 size={32} style={{ color: CODEX_COLOR }} />
                </div>
                <div>
                  <div className="flex items-center gap-3">
                    <h3 className="text-xl font-bold text-foreground">{provider.name}</h3>
                    {config?.planType && <PlanTypeBadge planType={config.planType} />}
                  </div>
                  <div className="text-sm text-muted-foreground flex items-center gap-1.5 mt-1">
                    <Mail size={14} />
                    {config?.email || t('common.unknown')}
                  </div>
                  {config?.name && (
                    <div className="text-sm text-muted-foreground flex items-center gap-1.5 mt-0.5">
                      <User size={14} />
                      {config.name}
                    </div>
                  )}
                </div>
              </div>

              {config?.accountId && (
                <div className="flex flex-col items-end gap-1 text-right">
                  <div className="text-xs text-muted-foreground uppercase tracking-wider font-semibold">
                    {t('providers.accountId')}
                  </div>
                  <div className="text-sm font-mono text-foreground bg-card px-2 py-1 rounded border border-border/50 max-w-[200px] truncate">
                    {config.accountId}
                  </div>
                </div>
              )}
            </div>

            {config?.refreshToken && (
              <div className="mt-6 pt-6 border-t border-border/50">
                <div className="text-xs text-muted-foreground uppercase tracking-wider font-semibold mb-1.5">
                  {t('providers.refreshToken')}
                </div>
                <div className="flex items-center gap-2">
                  <div className="font-mono text-sm text-foreground bg-card px-2 py-1 rounded border border-border/50 flex-1 truncate">
                    {config.refreshToken.slice(0, 30)}...
                  </div>
                  <button
                    onClick={handleCopyToken}
                    className="p-1.5 rounded-lg hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
                    title={t('common.copy')}
                  >
                    {tokenCopied ? <Check size={16} className="text-green-500" /> : <Copy size={16} />}
                  </button>
                </div>
              </div>
            )}
          </div>

          {/* Subscription Section */}
          <div>
            <div className="flex items-center justify-between mb-4 border-b border-border pb-2">
              <div className="flex items-center gap-2">
                <Crown size={18} style={{ color: CODEX_COLOR }} />
                <h4 className="text-lg font-semibold text-foreground">
                  {t('providers.subscription')}
                </h4>
              </div>
              <button
                onClick={handleRefresh}
                disabled={loading}
                className="btn bg-muted hover:bg-accent text-foreground flex items-center gap-2 text-sm"
              >
                <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
                {t('providers.refresh')}
              </button>
            </div>

            {error && (
              <div className="bg-error/10 border border-error/20 rounded-lg p-4 mb-4 flex items-start gap-3">
                <AlertCircle size={18} className="text-error shrink-0 mt-0.5" />
                <p className="text-sm text-error">{error}</p>
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Plan Type Card */}
              <div className="bg-card border border-border rounded-xl p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="font-medium text-foreground text-sm flex items-center gap-2">
                    <Crown size={16} className="text-amber-500" />
                    {t('providers.planType')}
                  </span>
                </div>
                <div className="text-lg font-semibold text-foreground">
                  {config?.planType ? (
                    <PlanTypeBadge planType={config.planType} />
                  ) : (
                    <span className="text-muted-foreground">{t('providers.unknown')}</span>
                  )}
                </div>
              </div>

              {/* Subscription Period Card */}
              <div className="bg-card border border-border rounded-xl p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="font-medium text-foreground text-sm flex items-center gap-2">
                    <Calendar size={16} className="text-blue-500" />
                    {t('providers.subscriptionPeriod')}
                  </span>
                </div>
                <div className="space-y-1">
                  <div className="text-sm text-muted-foreground">
                    {t('providers.from')}: {formatDate(config?.subscriptionStart)}
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {t('providers.until')}: {formatDate(config?.subscriptionEnd)}
                  </div>
                </div>
              </div>

              {/* Time Remaining Card */}
              {config?.subscriptionEnd && (
                <div className="bg-card border border-border rounded-xl p-4">
                  <div className="flex items-center justify-between mb-3">
                    <span className="font-medium text-foreground text-sm flex items-center gap-2">
                      <Clock size={16} className="text-green-500" />
                      {t('providers.timeRemaining')}
                    </span>
                  </div>
                  <div className="text-2xl font-bold text-foreground">
                    {formatRelativeTime(config.subscriptionEnd, t)}
                  </div>
                </div>
              )}

              {/* Access Token Expiry Card */}
              {config?.expiresAt && (
                <div className="bg-card border border-border rounded-xl p-4">
                  <div className="flex items-center justify-between mb-3">
                    <span className="font-medium text-foreground text-sm flex items-center gap-2">
                      <Clock size={16} className="text-orange-500" />
                      {t('providers.tokenExpiry')}
                    </span>
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {formatDate(config.expiresAt)} ({formatRelativeTime(config.expiresAt, t)})
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Usage/Quota Section */}
          <div>
            <div className="flex items-center justify-between mb-4 border-b border-border pb-2">
              <div className="flex items-center gap-2">
                <Gauge size={18} style={{ color: CODEX_COLOR }} />
                <h4 className="text-lg font-semibold text-foreground">
                  {t('providers.codex.usageQuota')}
                </h4>
              </div>
              <button
                onClick={handleRefreshUsage}
                disabled={usageLoading}
                className="btn bg-muted hover:bg-accent text-foreground flex items-center gap-2 text-sm"
              >
                <RefreshCw size={14} className={usageLoading ? 'animate-spin' : ''} />
                {t('providers.refresh')}
              </button>
            </div>

            {usageError && (
              <div className="bg-error/10 border border-error/20 rounded-lg p-4 mb-4 flex items-start gap-3">
                <AlertCircle size={18} className="text-error shrink-0 mt-0.5" />
                <p className="text-sm text-error">{usageError}</p>
              </div>
            )}

            {usageLoading && !displayUsage ? (
              <div className="text-center py-8 text-muted-foreground">
                <RefreshCw size={24} className="animate-spin mx-auto mb-2" />
                <p>{t('common.loading')}</p>
              </div>
            ) : displayUsage ? (
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                {/* Primary Window (5h limit) */}
                <QuotaProgressBar
                  percent={getRemainingPercent(displayUsage.rateLimit?.primaryWindow)}
                  label={t('providers.codex.primaryLimit')}
                  resetLabel={formatResetTime(displayUsage.rateLimit?.primaryWindow, t)}
                />

                {/* Secondary Window (weekly limit) */}
                <QuotaProgressBar
                  percent={getRemainingPercent(displayUsage.rateLimit?.secondaryWindow)}
                  label={t('providers.codex.weeklyLimit')}
                  resetLabel={formatResetTime(displayUsage.rateLimit?.secondaryWindow, t)}
                />

                {/* Code Review Limit */}
                <QuotaProgressBar
                  percent={getRemainingPercent(displayUsage.codeReviewRateLimit?.primaryWindow)}
                  label={t('providers.codex.codeReviewLimit')}
                  resetLabel={formatResetTime(displayUsage.codeReviewRateLimit?.primaryWindow, t)}
                />
              </div>
            ) : (
              <div className="text-center py-8 text-muted-foreground bg-muted/30 rounded-xl border border-dashed border-border">
                {t('providers.codex.noUsageData')}
              </div>
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
