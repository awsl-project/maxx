import { useState, useMemo } from 'react';
import {
  Sparkles,
  Mail,
  ChevronLeft,
  Trash2,
  RefreshCw,
  Clock,
  Building2,
  Plus,
  ArrowRight,
  Zap,
  AlertCircle,
  Copy,
  Check,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { ClientIcon } from '@/components/icons/client-icons';
import type { Provider, ModelMapping, ModelMappingInput } from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import {
  useUpdateProvider,
  useModelMappings,
  useCreateModelMapping,
  useUpdateModelMapping,
  useDeleteModelMapping,
} from '@/hooks/queries';
import { Button, Switch } from '@/components/ui';
import { ModelInput } from '@/components/ui/model-input';
import { CLAUDE_COLOR } from '../types';

interface ClaudeProviderViewProps {
  provider: Provider;
  onDelete: () => void;
  onClose: () => void;
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
      providerType: 'claude',
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
        providerType: 'claude',
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

export function ClaudeProviderView({ provider, onDelete, onClose }: ClaudeProviderViewProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tokenCopied, setTokenCopied] = useState(false);

  const config = provider.config?.claude;
  const updateProvider = useUpdateProvider();

  const [disableErrorCooldown, setDisableErrorCooldown] = useState(
    () => provider.config?.disableErrorCooldown ?? false,
  );

  const handleToggleDisableErrorCooldown = async (checked: boolean) => {
    if (!config) return;
    const prev = disableErrorCooldown;
    setDisableErrorCooldown(checked);
    try {
      await updateProvider.mutateAsync({
        id: provider.id,
        data: {
          ...provider,
          config: {
            ...provider.config,
            disableErrorCooldown: checked,
            claude: config,
          },
        },
      });
    } catch {
      setDisableErrorCooldown(prev);
    }
  };

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

  const handleRefresh = async () => {
    setLoading(true);
    setError(null);
    try {
      await getTransport().refreshClaudeProviderInfo(provider.id);
      // Invalidate providers query to refresh the data
      queryClient.invalidateQueries({ queryKey: ['providers'] });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to refresh');
    } finally {
      setLoading(false);
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
            <p className="text-caption text-muted-foreground">{t('providers.claudeType')}</p>
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
                  className="w-16 h-16 rounded-2xl flex items-center justify-center shadow-sm bg-provider-claude/15"
                >
                  <Sparkles size={32} className="text-provider-claude" />
                </div>
                <div>
                  <h3 className="text-xl font-bold text-foreground">{provider.name}</h3>
                  <div className="text-sm text-muted-foreground flex items-center gap-1.5 mt-1">
                    <Mail size={14} />
                    {config?.email || t('common.unknown')}
                  </div>
                  {config?.organizationId && (
                    <div className="text-sm text-muted-foreground flex items-center gap-1.5 mt-0.5">
                      <Building2 size={14} />
                      {config.organizationId}
                    </div>
                  )}
                </div>
              </div>
            </div>

            {config?.refreshToken && (
              <div className="mt-6 pt-6 border-t border-border/50">
                <div>
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
                      {tokenCopied ? (
                        <Check size={16} className="text-green-500" />
                      ) : (
                        <Copy size={16} />
                      )}
                    </button>
                  </div>
                </div>
              </div>
            )}

            <div className="mt-4">
              <div className="flex items-center justify-between p-3 bg-muted rounded-lg border border-border">
                <div className="pr-4">
                  <div className="text-sm font-medium text-foreground">
                    {t('provider.disableErrorCooldown')}
                  </div>
                  <p className="text-xs text-muted-foreground mt-1">
                    {t('provider.disableErrorCooldownDesc')}
                  </p>
                </div>
                <Switch
                  checked={disableErrorCooldown}
                  onCheckedChange={handleToggleDisableErrorCooldown}
                  disabled={updateProvider.isPending}
                />
              </div>
            </div>
          </div>

          {/* Account Details Section */}
          <div>
            <div className="flex items-center justify-between mb-4 border-b border-border pb-2">
              <div className="flex items-center gap-2">
                <Sparkles size={18} style={{ color: CLAUDE_COLOR }} />
                <h4 className="text-lg font-semibold text-foreground">
                  {t('providers.claudeAccountDetails')}
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
              {/* Organization ID Card */}
              {config?.organizationId && (
                <div className="bg-card border border-border rounded-xl p-4">
                  <div className="flex items-center justify-between mb-3">
                    <span className="font-medium text-foreground text-sm flex items-center gap-2">
                      <Building2 size={16} className="text-blue-500" />
                      {t('providers.organizationId')}
                    </span>
                  </div>
                  <div className="text-sm font-mono text-foreground truncate">
                    {config.organizationId}
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
