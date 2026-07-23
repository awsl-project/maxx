import { useState } from 'react';
import { ChevronLeft, Key, Check, Eye, EyeOff, Globe } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCreateProvider, useCreateModelMapping } from '@/hooks/queries';
import type { CreateProviderData } from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { useProviderNavigation } from '../hooks/use-provider-navigation';
import { OpenRouterModelMappings } from './openrouter-model-mappings';
import {
  normalizeMaxConcurrency,
  ProviderMaxConcurrencyField,
} from './provider-max-concurrency-field';

export function OllamaConfigStep() {
  const { t } = useTranslation();
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();
  const createModelMapping = useCreateModelMapping();

  const [name, setName] = useState('Ollama');
  const [baseURL, setBaseURL] = useState('http://localhost:11434');
  const [apiKey, setApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [modelMapping, setModelMapping] = useState<Record<string, string>>({});
  const [disableErrorCooldown, setDisableErrorCooldown] = useState(false);
  const [maxConcurrency, setMaxConcurrency] = useState(0);
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');

  const isValid = () => name.trim() !== '' && baseURL.trim() !== '';

  const handleSave = async () => {
    if (!isValid()) return;

    setSaving(true);
    setSaveStatus('idle');

    try {
      const data: CreateProviderData = {
        type: 'ollama',
        name: name.trim(),
        maxConcurrency: normalizeMaxConcurrency(maxConcurrency),
        config: {
          disableErrorCooldown,
          custom: {
            baseURL: baseURL.trim(),
            apiKey: apiKey.trim(),
          },
        },
        // Ollama's Claude<->Ollama translation path only serves Claude requests.
        supportedClientTypes: ['claude'],
      };

      const provider = await createProvider.mutateAsync(data);

      for (const [pattern, target] of Object.entries(modelMapping)) {
        await createModelMapping.mutateAsync({
          scope: 'provider',
          providerID: provider.id,
          pattern,
          target,
        });
      }

      setSaveStatus('success');
      goToProviders();
    } catch (error) {
      console.error('Failed to create provider:', error);
      setSaveStatus('error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={<ChevronLeft className="cursor-pointer" onClick={goToSelectType} />}
        title={t('addProvider.ollama.name')}
        description={t('addProvider.ollama.description')}
      >
        <Button onClick={goToProviders} variant="secondary">
          {t('common.cancel')}
        </Button>
        <Button onClick={handleSave} disabled={saving || !isValid()} variant="default">
          {saving ? (
            t('common.saving')
          ) : saveStatus === 'success' ? (
            <>
              <Check size={14} /> {t('common.saved')}
            </>
          ) : (
            t('provider.create')
          )}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          {/* Basic Info */}
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('provider.basicInfo')}
            </h3>
            <div className="grid gap-6">
              <div>
                <label className="text-sm font-medium text-text-primary block mb-2">
                  {t('provider.displayName')}
                </label>
                <Input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Ollama"
                  className="w-full"
                />
              </div>

              <ProviderMaxConcurrencyField
                value={maxConcurrency}
                onChange={setMaxConcurrency}
              />

              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  <div className="flex items-center gap-2">
                    <Globe size={14} />
                    <span>{t('addProvider.ollama.baseURL')}</span>
                  </div>
                </label>
                <Input
                  type="text"
                  value={baseURL}
                  onChange={(e) => setBaseURL(e.target.value)}
                  placeholder="http://localhost:11434"
                  className="w-full font-mono"
                />
                <p className="text-xs text-muted-foreground mt-1">
                  {t('addProvider.ollama.baseURLHint')}
                </p>
              </div>

              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  <div className="flex items-center gap-2">
                    <Key size={14} />
                    <span>{t('addProvider.ollama.apiKey')}</span>
                  </div>
                </label>
                <div className="relative">
                  <Input
                    type={showApiKey ? 'text' : 'password'}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    placeholder={t('addProvider.ollama.apiKeyPlaceholder')}
                    className="w-full pr-10 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setShowApiKey(!showApiKey)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                    tabIndex={-1}
                    aria-label={showApiKey ? t('common.hide') : t('common.show')}
                  >
                    {showApiKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                <p className="text-xs text-muted-foreground mt-1">
                  {t('addProvider.ollama.apiKeyHint')}
                </p>
              </div>
            </div>
          </div>

          {/* Model Mappings */}
          <OpenRouterModelMappings mappings={modelMapping} onChange={setModelMapping} />

          {/* Error Cooldown */}
          <div className="space-y-6">
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
              <Switch checked={disableErrorCooldown} onCheckedChange={setDisableErrorCooldown} />
            </div>
          </div>

          {saveStatus === 'error' && (
            <div className="p-4 bg-error/10 border border-error/30 rounded-lg text-sm text-error flex items-center gap-2">
              <div className="w-1.5 h-1.5 rounded-full bg-error" />
              {t('provider.createError')}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
