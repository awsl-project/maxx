import { useCallback, useEffect, useState } from 'react';
import {
  Cloud,
  ChevronLeft,
  Trash2,
  Key,
  Globe,
  Eye,
  EyeOff,
  Copy,
  Check,
  RefreshCw,
  PackageSearch,
  AlertTriangle,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { BedrockDiscoveredModelsResult, Provider } from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import { useUpdateProvider } from '@/hooks/queries';
import { Button, Switch } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { BEDROCK_COLOR } from '../types';
import { ProviderProxyURLCard } from './provider-proxy-url-card';

interface BedrockProviderViewProps {
  provider: Provider;
  onDelete: () => void;
  onClose: () => void;
}

export function BedrockProviderView({ provider, onDelete, onClose }: BedrockProviderViewProps) {
  const { t } = useTranslation();
  const updateProvider = useUpdateProvider();
  const config = provider.config?.bedrock;

  const [showSecret, setShowSecret] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);

  // Runtime discovery catalog for this provider — what Bedrock actually
  // lets these credentials invoke right now, as opposed to anything the
  // operator has statically configured in modelMapping.
  const [discovered, setDiscovered] = useState<BedrockDiscoveredModelsResult | null>(null);
  const [discoveryLoading, setDiscoveryLoading] = useState(false);
  const [discoveryError, setDiscoveryError] = useState<string | null>(null);

  const fetchDiscovery = useCallback(
    async (force: boolean) => {
      setDiscoveryLoading(true);
      setDiscoveryError(null);
      try {
        const result = force
          ? await getTransport().refreshBedrockDiscoveredModels(provider.id)
          : await getTransport().getBedrockDiscoveredModels(provider.id);
        setDiscovered(result);
        // The refresh endpoint may return the stale catalog + an error
        // field when the forced AWS round-trip itself failed. Surface
        // it as a non-fatal error so the operator sees both the old
        // list and why the new one couldn't land.
        if (result.refreshError) {
          setDiscoveryError(result.refreshError);
        }
      } catch (err) {
        // 503 from the admin endpoint means the adapter hasn't been
        // initialized yet — from the operator's perspective that is
        // another flavour of "discovery isn't working right now", so
        // treat it the same as an Available=false response rather than
        // a red transport error.
        const status = (err as { response?: { status?: number } })?.response?.status;
        if (status === 503) {
          setDiscovered({ available: false, region: provider.config?.bedrock?.region || '', models: [] });
        } else {
          setDiscoveryError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        setDiscoveryLoading(false);
      }
    },
    [provider.id, provider.config?.bedrock?.region],
  );

  // Mount: cheap GET that returns the persisted/cached catalog. The
  // refresh button (below) is the only path that triggers a live AWS
  // round-trip so routine navigation doesn't burn API quota.
  const loadDiscoveredModels = useCallback(() => fetchDiscovery(false), [fetchDiscovery]);
  const refreshDiscoveredModels = useCallback(() => fetchDiscovery(true), [fetchDiscovery]);

  useEffect(() => {
    loadDiscoveredModels();
  }, [loadDiscoveredModels]);

  const handleCopy = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  };

  const handleToggleCooldown = async (checked: boolean) => {
    await updateProvider.mutateAsync({
      id: provider.id,
      data: {
        config: {
          ...provider.config,
          disableErrorCooldown: checked,
        },
      },
    });
  };

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={<ChevronLeft className="cursor-pointer" onClick={onClose} />}
        title={provider.name}
        description={`AWS Bedrock (${config?.region || 'us-east-1'})`}
      >
        <Button onClick={onDelete} variant="destructive">
          <Trash2 size={14} />
          {t('provider.delete')}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          {/* Provider Info Card */}
          <div
            className="rounded-xl border p-6 space-y-4"
            style={{ borderColor: `color-mix(in oklch, ${BEDROCK_COLOR} 30%, transparent)` }}
          >
            <div className="flex items-center gap-3">
              <div
                className="size-12 rounded-lg flex items-center justify-center"
                style={{ backgroundColor: `color-mix(in oklch, ${BEDROCK_COLOR} 15%, transparent)` }}
              >
                <Cloud className="size-6" style={{ color: BEDROCK_COLOR }} />
              </div>
              <div>
                <h3 className="text-lg font-semibold">{provider.name}</h3>
                <p className="text-sm text-muted-foreground">AWS Bedrock</p>
              </div>
            </div>

            <div className="grid gap-4">
              {/* Region */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2 text-sm">
                  <Globe size={14} className="text-muted-foreground" />
                  <span className="text-muted-foreground">Region</span>
                </div>
                <span className="text-sm font-mono">{config?.region || 'us-east-1'}</span>
              </div>

              {/* Model Prefix */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2 text-sm">
                  <span className="text-muted-foreground">Model Prefix</span>
                </div>
                <span className="text-sm font-mono">{config?.modelPrefix || 'us'}</span>
              </div>

              {/* Access Key ID */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2 text-sm">
                  <Key size={14} className="text-muted-foreground" />
                  <span className="text-muted-foreground">Access Key ID</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-mono">
                    {config?.accessKeyId
                      ? config.accessKeyId.slice(0, 8) + '...' + config.accessKeyId.slice(-4)
                      : '-'}
                  </span>
                  {config?.accessKeyId && (
                    <button
                      onClick={() => handleCopy(config.accessKeyId, 'akid')}
                      className="text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {copied === 'akid' ? (
                        <Check className="h-3.5 w-3.5" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                    </button>
                  )}
                </div>
              </div>

              {/* Secret Access Key */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2 text-sm">
                  <Key size={14} className="text-muted-foreground" />
                  <span className="text-muted-foreground">Secret Access Key</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-mono">
                    {showSecret
                      ? config?.secretAccessKey || '-'
                      : config?.secretAccessKey
                        ? '***' + config.secretAccessKey.slice(-4)
                        : '-'}
                  </span>
                  <button
                    onClick={() => setShowSecret(!showSecret)}
                    className="text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {showSecret ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                  </button>
                  {config?.secretAccessKey && (
                    <button
                      onClick={() => handleCopy(config.secretAccessKey, 'sak')}
                      className="text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {copied === 'sak' ? (
                        <Check className="h-3.5 w-3.5" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                    </button>
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* Proxy URL Card */}
          <ProviderProxyURLCard provider={provider} />

          {/* Runtime discovered models — authoritative list of what
              these creds can actually invoke in this region. */}
          <div className="space-y-4">
            <div className="flex items-center justify-between border-b border-border pb-2">
              <div className="flex items-center gap-2">
                <PackageSearch size={16} className="text-muted-foreground" />
                <h3 className="text-lg font-semibold text-text-primary">
                  {t('provider.bedrock.discoveredModels', 'Discovered Models')}
                </h3>
                {discovered && (
                  <span className="text-xs text-muted-foreground font-mono">
                    {discovered.region} · {discovered.models.length}
                  </span>
                )}
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={refreshDiscoveredModels}
                disabled={discoveryLoading}
              >
                <RefreshCw size={14} className={discoveryLoading ? 'animate-spin' : ''} />
                {t('common.refresh', 'Refresh')}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              {t(
                'provider.bedrock.discoveredModelsDesc',
                'Pulled live from bedrock:ListInferenceProfiles and ListFoundationModels with these credentials. Inference profiles take priority over foundation models when both exist.',
              )}
            </p>
            {discoveryError && (
              <div className="flex items-start gap-2 p-3 rounded-lg border border-destructive/30 bg-destructive/5 text-sm text-destructive">
                <AlertTriangle size={14} className="shrink-0 mt-0.5" />
                <span>{discoveryError}</span>
              </div>
            )}
            {discovered && !discoveryError && !discovered.available && (
              <div className="flex items-start gap-2 p-3 rounded-lg border border-amber-500/30 bg-amber-500/5 text-sm text-amber-700 dark:text-amber-500">
                <AlertTriangle size={14} className="shrink-0 mt-0.5" />
                <span>
                  {t(
                    'provider.bedrock.discoveryUnavailable',
                    'Discovery unavailable — grant bedrock:ListInferenceProfiles and bedrock:ListFoundationModels, or set an explicit modelMapping.',
                  )}
                </span>
              </div>
            )}
            {discovered && discovered.models.length > 0 && (
              <div className="rounded-xl border border-border overflow-hidden">
                <table className="w-full text-sm">
                  <thead className="bg-muted/50 text-xs text-muted-foreground">
                    <tr>
                      <th className="text-left px-4 py-2 font-medium">
                        {t('provider.bedrock.shortName', 'Short Name')}
                      </th>
                      <th className="text-left px-4 py-2 font-medium">
                        {t('provider.bedrock.source', 'Source')}
                      </th>
                      <th className="text-left px-4 py-2 font-medium">
                        {t('provider.bedrock.bedrockId', 'Bedrock ID')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {discovered.models.map((m) => (
                      <tr key={m.shortName} className="border-t border-border/50 hover:bg-muted/30">
                        <td className="px-4 py-2 font-mono text-foreground">{m.shortName}</td>
                        <td className="px-4 py-2">
                          <span
                            className={
                              'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ' +
                              (m.source === 'inference-profile'
                                ? 'bg-blue-500/10 text-blue-700 dark:text-blue-400'
                                : 'bg-purple-500/10 text-purple-700 dark:text-purple-400')
                            }
                          >
                            {m.source === 'inference-profile' ? 'IP' : 'FM'}
                          </span>
                        </td>
                        <td className="px-4 py-2 font-mono text-xs text-muted-foreground break-all">
                          {m.bedrockId}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {discovered && discovered.available && discovered.models.length === 0 && (
              <p className="text-sm text-muted-foreground italic">
                {t(
                  'provider.bedrock.noModels',
                  'No Anthropic models returned by AWS for this region. Check model access grants.',
                )}
              </p>
            )}
          </div>

          {/* Error Cooldown */}
          <div className="space-y-4">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('provider.errorCooldownTitle')}
            </h3>
            <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">
                  {t('provider.disableErrorCooldown')}
                </div>
                <p className="text-xs text-muted-foreground mt-1">
                  {t('provider.disableErrorCooldownDesc')}
                </p>
              </div>
              <Switch
                checked={!!provider.config?.disableErrorCooldown}
                onCheckedChange={handleToggleCooldown}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
