import { useMemo, useState } from 'react';
import {
  Globe,
  ChevronLeft,
  Key,
  Check,
  Plus,
  Trash2,
  ArrowRight,
  Eye,
  EyeOff,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  useCreateProvider,
  useCreateModelMapping,
  useProviderRuntimeModelsPreview,
} from '@/hooks/queries';
import type {
  ClientType,
  CreateProviderData,
  ProviderRuntimeModelsPreviewRequest,
} from '@/lib/transport';
import { ClientsConfigSection } from './clients-config-section';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ModelInput } from '@/components/ui/model-input';
import { PageHeader } from '@/components/layout/page-header';
import { useProviderForm } from '../context/provider-form-context';
import { useProviderNavigation } from '../hooks/use-provider-navigation';
import { buildDisguisePayload } from '../utils/disguise';
import { buildProviderRuntimeModelOptions } from './provider-model-mappings';
import { SmartMappingRetrySettings } from './smart-mapping-retry-settings';
import { ReasoningPolicySettings } from './reasoning-policy-settings';
import {
  normalizeMaxConcurrency,
  ProviderMaxConcurrencyField,
} from './provider-max-concurrency-field';

export function CustomConfigStep() {
  const [showApiKey, setShowApiKey] = useState(false);
  const { t } = useTranslation();
  const {
    formData,
    updateFormData,
    updateClient,
    isValid,
    isSaving,
    setSaving,
    saveStatus,
    setSaveStatus,
  } = useProviderForm();
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();
  const createModelMapping = useCreateModelMapping();
  const runtimeModelsPreview = useMemo<ProviderRuntimeModelsPreviewRequest | undefined>(() => {
    const clientBaseURL: Partial<Record<ClientType, string>> = {};
    formData.clients.forEach((client) => {
      const url = client.urlOverride.trim();
      if (client.enabled && url) {
        clientBaseURL[client.id] = url;
      }
    });

    const baseURL = formData.baseURL.trim();
    if (!baseURL && !clientBaseURL.openai) return undefined;

    return {
      type: 'custom',
      config: {
        custom: {
          baseURL,
          backend: formData.backend === 'ollama' ? 'ollama' : undefined,
          apiKey: formData.apiKey.trim(),
          clientBaseURL: Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
        },
      },
    };
  }, [formData.apiKey, formData.backend, formData.baseURL, formData.clients]);
  const { data: runtimeModels } = useProviderRuntimeModelsPreview(
    runtimeModelsPreview,
    !!runtimeModelsPreview,
  );
  const runtimeModelOptions = useMemo(
    () =>
      buildProviderRuntimeModelOptions(
        runtimeModels?.models,
        undefined,
        formData.modelMappings?.map((mapping) => mapping.target),
        t('modelInput.currentProviderModels'),
      ),
    [formData.modelMappings, runtimeModels?.models, t],
  );
  const mappingTargetCount = useMemo(
    () =>
      new Set(
        (formData.modelMappings ?? []).map((mapping) => mapping.target.trim()).filter(Boolean),
      ).size,
    [formData.modelMappings],
  );
  const smartMappingRetryEnabled =
    !!formData.disableErrorCooldown &&
    mappingTargetCount > 1 &&
    !!formData.smartMappingRetryEnabled;

  const handleSave = async () => {
    if (!isValid()) return;

    setSaving(true);
    setSaveStatus('idle');

    try {
      const supportedClientTypes = formData.clients.filter((c) => c.enabled).map((c) => c.id);
      const clientBaseURL: Partial<Record<ClientType, string>> = {};
      const clientMultiplier: Partial<Record<ClientType, number>> = {};
      formData.clients.forEach((c) => {
        if (c.enabled && c.urlOverride) {
          clientBaseURL[c.id] = c.urlOverride;
        }
        if (c.enabled && c.multiplier !== 10000) {
          clientMultiplier[c.id] = c.multiplier;
        }
      });

      const disguise = buildDisguisePayload(
        formData.disguiseType,
        formData.cloakMode,
        !!formData.cloakStrictMode,
        formData.cloakSensitiveWords || '',
      );

      const data: CreateProviderData = {
        type: 'custom',
        name: formData.name,
        logo: formData.logo,
        maxConcurrency: normalizeMaxConcurrency(formData.maxConcurrency),
        config: {
          disableErrorCooldown: !!formData.disableErrorCooldown,
          smartMappingRetryEnabled,
          smartMappingRetryLimit: formData.smartMappingRetryLimit ?? 1,
          reasoning: formData.reasoning,
          custom: {
            baseURL: formData.baseURL,
            backend: formData.backend === 'ollama' ? 'ollama' : undefined,
            apiKey: formData.apiKey,
            responsesPassthrough: formData.responsesPassthrough,
            responsesWebSocket: formData.responsesWebSocket === true,
            clientBaseURL: Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
            clientMultiplier:
              Object.keys(clientMultiplier).length > 0 ? clientMultiplier : undefined,
            disguise,
          },
        },
        supportedClientTypes,
        excludeFromExport: !!formData.excludeFromExport || !!formData.blackBox,
        blackBox: !!formData.blackBox,
      };

      const provider = await createProvider.mutateAsync(data);

      // Create model mappings if template has any
      if (formData.modelMappings && formData.modelMappings.length > 0) {
        for (const mapping of formData.modelMappings) {
          await createModelMapping.mutateAsync({
            scope: 'provider',
            providerID: provider.id,
            pattern: mapping.pattern,
            target: mapping.target,
          });
        }
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
        title={t('provider.configure')}
        description={t('provider.configureDescription')}
      >
        <Button onClick={goToProviders} variant={'secondary'}>
          {t('common.cancel')}
        </Button>
        <Button onClick={handleSave} disabled={isSaving || !isValid()} variant={'default'}>
          {isSaving ? (
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
                  value={formData.name}
                  onChange={(e) => updateFormData({ name: e.target.value })}
                  placeholder={t('provider.namePlaceholder')}
                  className="w-full"
                />
              </div>
              <ProviderMaxConcurrencyField
                value={formData.maxConcurrency ?? 0}
                onChange={(maxConcurrency) => updateFormData({ maxConcurrency })}
              />

              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  {t('provider.customBackend')}
                </label>
                <Select
                  value={formData.backend}
                  onValueChange={(backend) =>
                    updateFormData({ backend: backend === 'ollama' ? 'ollama' : 'http' })
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="http">{t('provider.customBackendHttp')}</SelectItem>
                    <SelectItem value="ollama">{t('provider.customBackendOllama')}</SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground mt-1">
                  {formData.backend === 'ollama'
                    ? t('provider.customBackendOllamaDesc')
                    : t('provider.customBackendHttpDesc')}
                </p>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <label className="text-sm font-medium text-foreground block mb-2">
                    <div className="flex items-center gap-2">
                      <Globe size={14} />
                      <span>{t('provider.apiEndpoint')}</span>
                    </div>
                  </label>
                  <Input
                    type="text"
                    value={formData.baseURL}
                    onChange={(e) => updateFormData({ baseURL: e.target.value })}
                    placeholder={t('provider.endpointPlaceholder')}
                    className="w-full"
                  />
                  <p className="text-xs text-text-secondary mt-1">
                    {t('provider.optionalUrlNote')}
                  </p>
                </div>

                <div>
                  <label className="text-sm font-medium text-foreground block mb-2">
                    <div className="flex items-center gap-2">
                      <Key size={14} />
                      <span>
                        {formData.backend === 'ollama'
                          ? t('provider.apiKeyOptional')
                          : t('provider.apiKey')}
                      </span>
                    </div>
                  </label>
                  <div className="relative">
                    <Input
                      type={showApiKey ? 'text' : 'password'}
                      value={formData.apiKey}
                      onChange={(e) => updateFormData({ apiKey: e.target.value })}
                      placeholder={
                        formData.backend === 'ollama'
                          ? t('provider.keyPlaceholderOptional')
                          : t('provider.keyPlaceholder')
                      }
                      className="w-full pr-10"
                    />
                    <button
                      type="button"
                      onClick={() => setShowApiKey(!showApiKey)}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      tabIndex={-1}
                    >
                      {showApiKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('provider.clientConfig')}
            </h3>
            <ClientsConfigSection
              clients={formData.clients}
              onUpdateClient={updateClient}
              disguise={{
                type: formData.disguiseType ?? 'claude-code',
                claudeCodeMode: formData.cloakMode ?? 'auto',
                claudeCodeStrictMode: !!formData.cloakStrictMode,
                claudeCodeSensitiveWords: formData.cloakSensitiveWords ?? '',
              }}
              onUpdateDisguise={(updates) =>
                updateFormData({
                  disguiseType: updates?.type ?? formData.disguiseType,
                  cloakMode: updates?.claudeCodeMode ?? formData.cloakMode,
                  cloakStrictMode: updates?.claudeCodeStrictMode ?? formData.cloakStrictMode,
                  cloakSensitiveWords:
                    updates?.claudeCodeSensitiveWords ?? formData.cloakSensitiveWords,
                })
              }
              responsesWebSocket={formData.responsesWebSocket === true}
              onUpdateResponsesWebSocket={(checked) =>
                updateFormData({ responsesWebSocket: checked })
              }
            />
          </div>

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
                checked={!!formData.disableErrorCooldown}
                onCheckedChange={(checked) =>
                  updateFormData({
                    disableErrorCooldown: checked,
                    smartMappingRetryEnabled: checked ? formData.smartMappingRetryEnabled : false,
                  })
                }
              />
            </div>
            <SmartMappingRetrySettings
              disableErrorCooldown={!!formData.disableErrorCooldown}
              enabled={formData.smartMappingRetryEnabled}
              retryLimit={formData.smartMappingRetryLimit}
              mappingTargetCount={mappingTargetCount}
              onEnabledChange={(checked) => updateFormData({ smartMappingRetryEnabled: checked })}
              onRetryLimitChange={(limit) => updateFormData({ smartMappingRetryLimit: limit })}
            />
            <ReasoningPolicySettings
              value={formData.reasoning}
              onChange={(reasoning) => updateFormData({ reasoning })}
            />
            <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">
                  {t('provider.responsesPassthrough')}
                </div>
                <p className="text-xs text-muted-foreground mt-1">
                  {t('provider.responsesPassthroughDesc')}
                </p>
              </div>
              <Switch
                checked={formData.responsesPassthrough !== false}
                onCheckedChange={(checked) => updateFormData({ responsesPassthrough: checked })}
              />
            </div>
          </div>

          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('provider.visibilityAndExport')}
            </h3>
            <div className="space-y-3">
              <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
                <div className="pr-4">
                  <div className="text-sm font-medium text-foreground">
                    {t('provider.blackBox')}
                  </div>
                  <p className="text-xs text-muted-foreground mt-1">{t('provider.blackBoxDesc')}</p>
                </div>
                <Switch
                  checked={!!formData.blackBox}
                  onCheckedChange={(checked) =>
                    updateFormData({
                      blackBox: checked,
                      excludeFromExport: checked ? true : formData.excludeFromExport,
                    })
                  }
                />
              </div>
              <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
                <div className="pr-4">
                  <div className="text-sm font-medium text-foreground">
                    {t('provider.excludeFromExport')}
                  </div>
                  <p className="text-xs text-muted-foreground mt-1">
                    {t('provider.excludeFromExportDesc')}
                  </p>
                </div>
                <Switch
                  checked={!!formData.excludeFromExport || !!formData.blackBox}
                  disabled={!!formData.blackBox}
                  onCheckedChange={(checked) => updateFormData({ excludeFromExport: checked })}
                />
              </div>
            </div>
          </div>

          {/* Model Mapping Section */}
          <div className="space-y-6">
            <div className="flex items-center justify-between border-b border-border pb-2">
              <h3 className="text-lg font-semibold text-text-primary">
                {t('modelMappings.title')}
              </h3>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const newMappings = [
                    ...(formData.modelMappings || []),
                    { pattern: '', target: '' },
                  ];
                  updateFormData({ modelMappings: newMappings });
                }}
              >
                <Plus size={14} />
                {t('routes.modelMapping.addMapping')}
              </Button>
            </div>

            {formData.modelMappings && formData.modelMappings.length > 0 ? (
              <div className="space-y-3">
                {formData.modelMappings.map((mapping, index) => (
                  <div key={index} className="flex items-center gap-3 p-3 bg-muted/50 rounded-lg">
                    <div className="flex-1">
                      <label className="text-xs text-muted-foreground mb-1 block">
                        {t('settings.matchPattern')}
                      </label>
                      <Input
                        type="text"
                        value={mapping.pattern}
                        onChange={(e) => {
                          const newMappings = [...(formData.modelMappings || [])];
                          newMappings[index] = { ...newMappings[index], pattern: e.target.value };
                          updateFormData({ modelMappings: newMappings });
                        }}
                        placeholder="*claude*, *sonnet*, *"
                        className="font-mono text-sm"
                      />
                    </div>
                    <ArrowRight size={16} className="text-muted-foreground shrink-0 mt-5" />
                    <div className="flex-1">
                      <label className="text-xs text-muted-foreground mb-1 block">
                        {t('settings.targetModel')}
                      </label>
                      <ModelInput
                        value={mapping.target}
                        onChange={(value) => {
                          const newMappings = [...(formData.modelMappings || [])];
                          newMappings[index] = { ...newMappings[index], target: value };
                          updateFormData({ modelMappings: newMappings });
                        }}
                        placeholder={t('modelInput.selectOrEnter')}
                        extraModels={runtimeModelOptions}
                        openSearchValue=""
                      />
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="shrink-0 mt-5 text-muted-foreground hover:text-destructive"
                      onClick={() => {
                        const newMappings = (formData.modelMappings || []).filter(
                          (_, i) => i !== index,
                        );
                        updateFormData({ modelMappings: newMappings });
                      }}
                    >
                      <Trash2 size={14} />
                    </Button>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-sm text-muted-foreground p-4 bg-muted/30 rounded-lg text-center">
                {t('modelMappings.noMappings')}
              </div>
            )}
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
