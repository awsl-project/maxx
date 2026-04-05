import { useState } from 'react';
import { ChevronLeft, Key, Check, Eye, EyeOff, Globe } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCreateProvider } from '@/hooks/queries';
import type { CreateProviderData } from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { useProviderNavigation } from '../hooks/use-provider-navigation';

const BEDROCK_REGIONS = [
  'us-east-1',
  'us-west-2',
  'eu-west-1',
  'eu-west-3',
  'eu-central-1',
  'ap-northeast-1',
  'ap-southeast-1',
  'ap-southeast-2',
  'ap-south-1',
  'sa-east-1',
];

export function BedrockConfigStep() {
  const { t } = useTranslation();
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();

  const [name, setName] = useState('AWS Bedrock');
  const [accessKeyId, setAccessKeyId] = useState('');
  const [secretAccessKey, setSecretAccessKey] = useState('');
  const [region, setRegion] = useState('us-east-1');
  const [modelPrefix, setModelPrefix] = useState('us');
  const [showSecret, setShowSecret] = useState(false);
  const [disableErrorCooldown, setDisableErrorCooldown] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');

  const isValid = () => name.trim() !== '' && accessKeyId.trim() !== '' && secretAccessKey.trim() !== '';

  const handleSave = async () => {
    if (!isValid()) return;

    setSaving(true);
    setSaveStatus('idle');

    try {
      const data: CreateProviderData = {
        type: 'bedrock',
        name: name.trim(),
        config: {
          disableErrorCooldown,
          bedrock: {
            accessKeyId: accessKeyId.trim(),
            secretAccessKey: secretAccessKey.trim(),
            region,
            modelPrefix: modelPrefix || undefined,
          },
        },
        supportedClientTypes: ['claude'],
      };

      await createProvider.mutateAsync(data);
      setSaveStatus('success');
      setTimeout(() => goToProviders(), 500);
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
        title={t('addProvider.bedrock.name')}
        description={t('addProvider.bedrock.description')}
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
                  placeholder="AWS Bedrock"
                  className="w-full"
                />
              </div>
            </div>
          </div>

          {/* AWS Credentials */}
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('addProvider.bedrock.credentials')}
            </h3>
            <div className="grid gap-6">
              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  <div className="flex items-center gap-2">
                    <Key size={14} />
                    <span>{t('addProvider.bedrock.accessKeyId', 'Access Key ID')}</span>
                  </div>
                </label>
                <Input
                  type="text"
                  value={accessKeyId}
                  onChange={(e) => setAccessKeyId(e.target.value)}
                  placeholder="AKIA..."
                  className="w-full font-mono"
                />
              </div>

              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  <div className="flex items-center gap-2">
                    <Key size={14} />
                    <span>{t('addProvider.bedrock.secretAccessKey', 'Secret Access Key')}</span>
                  </div>
                </label>
                <div className="relative">
                  <Input
                    type={showSecret ? 'text' : 'password'}
                    value={secretAccessKey}
                    onChange={(e) => setSecretAccessKey(e.target.value)}
                    placeholder={t('addProvider.bedrock.secretPlaceholder')}
                    className="w-full pr-10 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setShowSecret(!showSecret)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                    tabIndex={-1}
                  >
                    {showSecret ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <label className="text-sm font-medium text-foreground block mb-2">
                    <div className="flex items-center gap-2">
                      <Globe size={14} />
                      <span>{t('addProvider.bedrock.region', 'Region')}</span>
                    </div>
                  </label>
                  <select
                    value={region}
                    onChange={(e) => setRegion(e.target.value)}
                    className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                  >
                    {BEDROCK_REGIONS.map((r) => (
                      <option key={r} value={r}>{r}</option>
                    ))}
                  </select>
                  <p className="text-xs text-muted-foreground mt-1">
                    {t('addProvider.bedrock.regionHint')}
                  </p>
                </div>

                <div>
                  <label className="text-sm font-medium text-foreground block mb-2">
                    {t('addProvider.bedrock.modelPrefix', 'Model Prefix')}
                  </label>
                  <Input
                    type="text"
                    value={modelPrefix}
                    onChange={(e) => setModelPrefix(e.target.value)}
                    placeholder="us"
                    className="w-full font-mono"
                  />
                  <p className="text-xs text-muted-foreground mt-1">
                    {t('addProvider.bedrock.modelPrefixHint')}
                  </p>
                </div>
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
              <Switch
                checked={disableErrorCooldown}
                onCheckedChange={setDisableErrorCooldown}
              />
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
