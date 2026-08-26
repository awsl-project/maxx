import { useState } from 'react';
import { ChevronLeft, Key, Check, Eye, EyeOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCreateProvider, useCreateModelMapping } from '@/hooks/queries';
import type { CreateProviderData } from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { useProviderNavigation } from '../hooks/use-provider-navigation';
import zhipuLogo from '@/assets/icons/zhipu.svg';
import { OpenRouterModelMappings } from './openrouter-model-mappings';
import {
  normalizeMaxConcurrency,
  ProviderMaxConcurrencyField,
} from './provider-max-concurrency-field';

// z.ai (智谱 GLM) natively serves BOTH the Anthropic Messages endpoint (Claude
// clients) and the OpenAI Chat Completions endpoint (OpenAI clients). The user
// picks which protocols to enable and which z.ai plan (coding vs standard API) —
// the plan only changes the OpenAI upstream root; the backend synthesizes the
// right z.ai base URL per client type.
export function ZaiConfigStep() {
  const { t } = useTranslation();
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();
  const createModelMapping = useCreateModelMapping();

  const [name, setName] = useState('z.ai');
  const [apiKey, setApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [plan, setPlan] = useState<'coding' | 'api'>('coding');
  const [claudeEnabled, setClaudeEnabled] = useState(true);
  const [openaiEnabled, setOpenaiEnabled] = useState(true);
  const [codexEnabled, setCodexEnabled] = useState(true);
  const [modelMapping, setModelMapping] = useState<Record<string, string>>({});
  const [disableErrorCooldown, setDisableErrorCooldown] = useState(false);
  const [maxConcurrency, setMaxConcurrency] = useState(0);
  const [blackBox, setBlackBox] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');

  const supportedClientTypes: ('claude' | 'openai' | 'codex')[] = [
    ...(claudeEnabled ? (['claude'] as const) : []),
    ...(openaiEnabled ? (['openai'] as const) : []),
    ...(codexEnabled ? (['codex'] as const) : []),
  ];

  const isValid = () =>
    name.trim() !== '' && apiKey.trim() !== '' && supportedClientTypes.length > 0;

  const handleSave = async () => {
    if (!isValid()) return;

    setSaving(true);
    setSaveStatus('idle');

    try {
      const data: CreateProviderData = {
        type: 'zai',
        name: name.trim(),
        logo: zhipuLogo,
        maxConcurrency: normalizeMaxConcurrency(maxConcurrency),
        config: {
          disableErrorCooldown,
          zai: {
            apiKey: apiKey.trim(),
            plan,
          },
        },
        supportedClientTypes,
        excludeFromExport: blackBox,
        blackBox,
      };

      const provider = await createProvider.mutateAsync(data);

      // Persist model mappings as provider-scoped ModelMapping entities — the
      // mechanism executor.mapModel actually consults at request time (e.g.
      // claude-sonnet-4-5 → glm-4.6).
      const entries = Object.entries(modelMapping);
      for (const [pattern, target] of entries) {
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
        title={t('addProvider.zai.name')}
        description={t('addProvider.zai.description')}
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
                  placeholder="z.ai"
                  className="w-full"
                />
              </div>

              <ProviderMaxConcurrencyField value={maxConcurrency} onChange={setMaxConcurrency} />

              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  <div className="flex items-center gap-2">
                    <Key size={14} />
                    <span>{t('addProvider.zai.apiKey')}</span>
                  </div>
                </label>
                <div className="relative">
                  <Input
                    type={showApiKey ? 'text' : 'password'}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    placeholder="••••••••"
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
                  {t('addProvider.zai.apiKeyHint')}
                </p>
              </div>
            </div>
          </div>

          {/* Plan (coding vs standard API) */}
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('addProvider.zai.planTitle')}
            </h3>
            <div className="grid gap-3 sm:grid-cols-2">
              {(['coding', 'api'] as const).map((option) => (
                <button
                  key={option}
                  type="button"
                  onClick={() => setPlan(option)}
                  className={`text-left p-4 rounded-xl border transition-colors ${
                    plan === option
                      ? 'border-provider-zai bg-provider-zai/10'
                      : 'border-border bg-card hover:border-provider-zai/50'
                  }`}
                >
                  <div className="text-sm font-medium text-foreground">
                    {t(`addProvider.zai.plan_${option}`)}
                  </div>
                  <p className="text-xs text-muted-foreground mt-1">
                    {t(`addProvider.zai.plan_${option}Desc`)}
                  </p>
                </button>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">{t('addProvider.zai.planHint')}</p>
          </div>

          {/* Client Types (Claude and/or OpenAI) */}
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('addProvider.zai.clientsTitle')}
            </h3>
            <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground capitalize">claude</div>
                <p className="text-xs text-muted-foreground mt-1">
                  {t('addProvider.zai.clientClaudeDesc')}
                </p>
              </div>
              <Switch checked={claudeEnabled} onCheckedChange={setClaudeEnabled} />
            </div>
            <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground capitalize">openai</div>
                <p className="text-xs text-muted-foreground mt-1">
                  {t('addProvider.zai.clientOpenaiDesc')}
                </p>
              </div>
              <Switch checked={openaiEnabled} onCheckedChange={setOpenaiEnabled} />
            </div>
            <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground capitalize">codex</div>
                <p className="text-xs text-muted-foreground mt-1">
                  {t('addProvider.zai.clientCodexDesc')}
                </p>
              </div>
              <Switch checked={codexEnabled} onCheckedChange={setCodexEnabled} />
            </div>
            {supportedClientTypes.length === 0 && (
              <p className="text-xs text-error">{t('addProvider.zai.clientRequired')}</p>
            )}
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
