import { useState } from 'react';
import { ChevronLeft, Key, Check, Eye, EyeOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCreateProvider } from '@/hooks/queries';
import type { CreateProviderData } from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { useProviderNavigation } from '../hooks/use-provider-navigation';
import {
  normalizeMaxConcurrency,
  ProviderMaxConcurrencyField,
} from './provider-max-concurrency-field';

// fal (fal.ai) is a queue-based inference platform, NOT OpenAI-compatible. maxx
// exposes it via a translation layer over two existing surfaces: OpenAI images
// (/v1/images/generations, synchronous) and async video (/v1/video/generations).
// The backend sets supportedClientTypes canonically (openai + video), so this
// step collects only the API key (the full id:secret string) — model routing is
// via the generic ModelMapping (mapped model id becomes the fal URL path segment,
// e.g. fal-ai/flux/dev, fal-ai/veo3.1).
export function FalConfigStep() {
  const { t } = useTranslation();
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();

  const [name, setName] = useState('fal.ai');
  const [apiKey, setApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [disableErrorCooldown, setDisableErrorCooldown] = useState(false);
  const [maxConcurrency, setMaxConcurrency] = useState(0);
  const [blackBox, setBlackBox] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');

  const isValid = () => name.trim() !== '' && apiKey.trim() !== '';

  const handleSave = async () => {
    if (!isValid()) return;
    setSaving(true);
    setSaveStatus('idle');
    try {
      const data: CreateProviderData = {
        type: 'fal',
        name: name.trim(),
        maxConcurrency: normalizeMaxConcurrency(maxConcurrency),
        config: {
          disableErrorCooldown,
          fal: {
            apiKey: apiKey.trim(),
          },
        },
        // supportedClientTypes omitted: the backend normalizes fal to
        // [openai (images), video] regardless of what the UI sends.
        excludeFromExport: blackBox,
        blackBox,
      };

      await createProvider.mutateAsync(data);
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
        title="fal.ai"
        description={t('addProvider.fal.description', {
          defaultValue:
            'fal.ai image (synchronous) + video (async) generation, exposed via OpenAI images and new-api video surfaces.',
        })}
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
                  placeholder="fal.ai"
                  className="w-full"
                />
              </div>

              <ProviderMaxConcurrencyField value={maxConcurrency} onChange={setMaxConcurrency} />

              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  <div className="flex items-center gap-2">
                    <Key size={14} />
                    <span>{t('addProvider.fal.apiKey', { defaultValue: 'API Key' })}</span>
                  </div>
                </label>
                <div className="relative">
                  <Input
                    type={showApiKey ? 'text' : 'password'}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    placeholder="id:secret"
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
                  {t('addProvider.fal.apiKeyHint', {
                    defaultValue:
                      'The full fal key in "id:secret" format. Sent upstream as "Authorization: Key <id:secret>".',
                  })}
                </p>
              </div>
            </div>
          </div>

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

          {/* Visibility */}
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('provider.visibilityAndExport')}
            </h3>
            <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">{t('provider.blackBox')}</div>
                <p className="text-xs text-muted-foreground mt-1">{t('provider.blackBoxDesc')}</p>
              </div>
              <Switch checked={blackBox} onCheckedChange={setBlackBox} />
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
