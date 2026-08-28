import { useState } from 'react';
import { ChevronLeft, Key, Check, Eye, EyeOff, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useUpdateProvider } from '@/hooks/queries';
import type { CreateProviderData, Provider } from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { ProviderProxyURLCard } from './provider-proxy-url-card';
import { ProviderModelMappings } from './provider-model-mappings';
import {
  normalizeMaxConcurrency,
  ProviderMaxConcurrencyField,
} from './provider-max-concurrency-field';

interface FalProviderViewProps {
  provider: Provider;
  onDelete: () => void;
  onClose: () => void;
}

// Edit view for a fal (fal.ai) provider. fal's supported client types (openai
// images + video) are set canonically by the backend, so this view exposes only
// the API key (full id:secret) and standard provider knobs. Model routing is via
// the generic ModelMapping (mapped model id becomes the fal URL path segment).
export function FalProviderView({ provider, onDelete, onClose }: FalProviderViewProps) {
  const { t } = useTranslation();
  const updateProvider = useUpdateProvider();

  // Secrets are redacted on read whenever the provider is export-excluded, so the
  // apiKey arrives blank; a blank submit preserves the stored key (the backend
  // keeps the existing secret when the incoming apiKey is empty — see
  // preserveEmptyProviderSecrets case "fal").
  const secretsAreWriteOnly = !!provider.excludeFromExport;

  const [name, setName] = useState(provider.name);
  const [apiKey, setApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [disableErrorCooldown, setDisableErrorCooldown] = useState(
    !!provider.config?.disableErrorCooldown,
  );
  const [maxConcurrency, setMaxConcurrency] = useState(
    normalizeMaxConcurrency(provider.maxConcurrency),
  );
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');

  const isValid = () => name.trim() !== '';

  const handleSave = async () => {
    if (!isValid()) return;
    setSaving(true);
    setSaveStatus('idle');
    try {
      const data: Partial<CreateProviderData> = {
        name: name.trim(),
        type: 'fal',
        maxConcurrency: normalizeMaxConcurrency(maxConcurrency),
        config: {
          disableErrorCooldown,
          fal: {
            // Blank preserves the stored key (backend keeps existing secret).
            apiKey: apiKey.trim(),
          },
        },
      };

      await updateProvider.mutateAsync({ id: Number(provider.id), data });
      setSaveStatus('success');
      setTimeout(() => onClose(), 500);
    } catch (error) {
      console.error('Failed to update provider:', error);
      setSaveStatus('error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={<ChevronLeft className="cursor-pointer" onClick={onClose} />}
        title={t('provider.edit')}
        description={t('addProvider.fal.description', {
          defaultValue:
            'fal.ai image (synchronous) + video (async) generation, exposed via OpenAI images and new-api video surfaces.',
        })}
      >
        <Button onClick={onDelete} variant="destructive">
          <Trash2 size={14} />
          {t('provider.delete')}
        </Button>
        <Button onClick={onClose} variant="secondary">
          {t('provider.cancel')}
        </Button>
        <Button onClick={handleSave} disabled={saving || !isValid()} variant="default">
          {saving ? (
            t('common.saving')
          ) : saveStatus === 'success' ? (
            <>
              <Check size={14} /> {t('common.saved')}
            </>
          ) : (
            t('provider.saveChanges')
          )}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          <ProviderProxyURLCard provider={provider} />

          {/* Basic Info */}
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-foreground border-b border-border pb-2">
              {t('provider.basicInfo')}
            </h3>
            <div className="grid gap-6">
              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  {t('provider.displayName')}
                </label>
                <Input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="fal.ai"
                  className="w-full"
                />
              </div>

              <ProviderMaxConcurrencyField value={maxConcurrency} onChange={setMaxConcurrency} />

              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  <div className="flex items-center gap-2">
                    <Key size={14} />
                    <span>{t('provider.apiKeyEdit')}</span>
                  </div>
                </label>
                <div className="relative">
                  <Input
                    type={showApiKey && !secretsAreWriteOnly ? 'text' : 'password'}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    placeholder={
                      secretsAreWriteOnly ? t('provider.keyPlaceholderWriteOnly') : 'id:secret'
                    }
                    className="w-full pr-10 font-mono"
                  />
                  {!secretsAreWriteOnly && (
                    <button
                      type="button"
                      onClick={() => setShowApiKey(!showApiKey)}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      aria-label={showApiKey ? t('common.hide') : t('common.show')}
                    >
                      {showApiKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  )}
                </div>
                {secretsAreWriteOnly && (
                  <div className="mt-2 p-3 bg-muted/50 border border-border rounded-lg text-xs text-muted-foreground">
                    {t('provider.apiKeyExcludedHint')}
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Model Mappings (provider-scoped ModelMapping entities) */}
          <ProviderModelMappings provider={provider} />

          {/* Error Cooldown */}
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-foreground border-b border-border pb-2">
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
              <Switch checked={disableErrorCooldown} onCheckedChange={setDisableErrorCooldown} />
            </div>
          </div>

          {saveStatus === 'error' && (
            <div className="p-4 bg-error/10 border border-error/30 rounded-lg text-sm text-error flex items-center gap-2">
              <div className="w-1.5 h-1.5 rounded-full bg-error" />
              {t('provider.updateError')}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
