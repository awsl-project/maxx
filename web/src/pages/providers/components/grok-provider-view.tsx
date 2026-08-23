import { useState } from 'react';
import { Check, ChevronLeft, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { useUpdateProvider } from '@/hooks/queries';
import type { ClientType, CreateProviderData, Provider } from '@/lib/transport';
import { ProviderProxyURLCard } from './provider-proxy-url-card';
import { ProviderModelMappings } from './provider-model-mappings';
import {
  normalizeMaxConcurrency,
  ProviderMaxConcurrencyField,
} from './provider-max-concurrency-field';
import {
  ConsecutiveErrorFreezeSettings,
  normalizeConsecutiveErrorFreezeThreshold,
} from './consecutive-error-freeze-settings';

interface GrokProviderViewProps {
  provider: Provider;
  onDelete: () => void;
  onClose: () => void;
}

const GROK_CLIENT_TYPES = ['openai'] as const satisfies readonly ClientType[];

export function GrokProviderView({ provider, onDelete, onClose }: GrokProviderViewProps) {
  const { t } = useTranslation();
  const updateProvider = useUpdateProvider();
  const grok = provider.config?.grok;

  const [name, setName] = useState(provider.name);
  const [email, setEmail] = useState(grok?.email || '');
  const [baseURL, setBaseURL] = useState(grok?.baseURL || '');
  const [disableErrorCooldown, setDisableErrorCooldown] = useState(
    !!provider.config?.disableErrorCooldown,
  );
  const [consecutiveErrorFreezeEnabled, setConsecutiveErrorFreezeEnabled] = useState(
    !!provider.config?.consecutiveErrorFreezeEnabled,
  );
  const [consecutiveErrorFreezeThreshold, setConsecutiveErrorFreezeThreshold] = useState(
    normalizeConsecutiveErrorFreezeThreshold(provider.config?.consecutiveErrorFreezeThreshold),
  );
  const [maxConcurrency, setMaxConcurrency] = useState(
    normalizeMaxConcurrency(provider.maxConcurrency),
  );
  const [blackBox, setBlackBox] = useState(!!provider.blackBox || !!provider.excludeFromExport);
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
        type: 'grok',
        maxConcurrency: normalizeMaxConcurrency(maxConcurrency),
        config: {
          disableErrorCooldown,
          consecutiveErrorFreezeEnabled: disableErrorCooldown && consecutiveErrorFreezeEnabled,
          consecutiveErrorFreezeThreshold,
          grok: {
            type: 'xai',
            authKind: 'oauth',
            email: email.trim() || grok?.email,
            sub: grok?.sub,
            // Blank/omitted secrets preserve the stored tokens on the backend.
            accessToken: '',
            refreshToken: '',
            idToken: '',
            tokenType: grok?.tokenType,
            expiresIn: grok?.expiresIn,
            expired: grok?.expired,
            lastRefresh: grok?.lastRefresh,
            redirectURI: grok?.redirectURI,
            tokenEndpoint: grok?.tokenEndpoint,
            baseURL: baseURL.trim() || grok?.baseURL,
            disabled: grok?.disabled,
            headers: grok?.headers,
          },
        },
        supportedClientTypes: [...GROK_CLIENT_TYPES],
        excludeFromExport: blackBox,
        blackBox,
      };
      await updateProvider.mutateAsync({ id: Number(provider.id), data });
      setSaveStatus('success');
      setTimeout(() => onClose(), 500);
    } catch (error) {
      console.error('Failed to update Grok provider:', error);
      setSaveStatus('error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={<ChevronLeft className="cursor-pointer" onClick={onClose} />}
        title={t('provider.edit')}
        description={t('addProvider.grok.description')}
      >
        <Button onClick={onDelete} variant="destructive">
          <Trash2 size={14} />
          {t('provider.delete')}
        </Button>
        <Button onClick={onClose} variant="secondary">
          {t('provider.cancel')}
        </Button>
        <Button onClick={handleSave} disabled={saving || !isValid()}>
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

          <div className="space-y-6">
            <h3 className="border-b border-border pb-2 text-lg font-semibold text-foreground">
              {t('provider.basicInfo')}
            </h3>
            <div className="grid gap-6">
              <div>
                <label className="mb-2 block text-sm font-medium text-foreground">
                  {t('provider.displayName')}
                </label>
                <Input value={name} onChange={(event) => setName(event.target.value)} />
              </div>
              <ProviderMaxConcurrencyField value={maxConcurrency} onChange={setMaxConcurrency} />
              <div>
                <label className="mb-2 block text-sm font-medium text-foreground">
                  {t('addProvider.grok.email')}
                </label>
                <Input value={email} onChange={(event) => setEmail(event.target.value)} />
              </div>
              <div>
                <label className="mb-2 block text-sm font-medium text-foreground">
                  {t('addProvider.grok.baseURL')}
                </label>
                <Input
                  value={baseURL}
                  onChange={(event) => setBaseURL(event.target.value)}
                  placeholder="https://cli-chat-proxy.grok.com/v1"
                />
              </div>
              <div className="rounded-lg border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
                {t('addProvider.grok.secretWriteOnlyHint')}
              </div>
            </div>
          </div>

          <div className="space-y-6">
            <h3 className="border-b border-border pb-2 text-lg font-semibold text-foreground">
              {t('addProvider.grok.clientsTitle')}
            </h3>
            <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">OpenAI</div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('addProvider.grok.clientOpenAIDesc')}
                </p>
              </div>
              <Switch checked disabled />
            </div>
          </div>

          <ProviderModelMappings provider={provider} />

          <div className="space-y-6">
            <h3 className="border-b border-border pb-2 text-lg font-semibold text-foreground">
              {t('provider.errorCooldownTitle')}
            </h3>
            <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">
                  {t('provider.disableErrorCooldown')}
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('provider.disableErrorCooldownDesc')}
                </p>
              </div>
              <Switch checked={disableErrorCooldown} onCheckedChange={setDisableErrorCooldown} />
            </div>
            <ConsecutiveErrorFreezeSettings
              disableErrorCooldown={disableErrorCooldown}
              enabled={consecutiveErrorFreezeEnabled}
              threshold={consecutiveErrorFreezeThreshold}
              onEnabledChange={setConsecutiveErrorFreezeEnabled}
              onThresholdChange={setConsecutiveErrorFreezeThreshold}
            />
          </div>

          <div className="space-y-6">
            <h3 className="border-b border-border pb-2 text-lg font-semibold text-foreground">
              {t('provider.visibilityAndExport')}
            </h3>
            <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">{t('provider.blackBox')}</div>
                <p className="mt-1 text-xs text-muted-foreground">{t('provider.blackBoxDesc')}</p>
              </div>
              <Switch checked={blackBox} onCheckedChange={setBlackBox} />
            </div>
          </div>

          {saveStatus === 'error' && (
            <div className="flex items-center gap-2 rounded-lg border border-error/30 bg-error/10 p-4 text-sm text-error">
              <div className="h-1.5 w-1.5 rounded-full bg-error" />
              {t('provider.updateError')}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
