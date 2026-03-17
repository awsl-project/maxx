import { useState } from 'react';
import { Globe, ChevronLeft, Key, Check, Plus, Trash2, ArrowRight, Eye, EyeOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCreateProvider, useCreateModelMapping } from '@/hooks/queries';
import type { ClientType, CreateProviderData } from '@/lib/transport';
import { ClientsConfigSection } from './clients-config-section';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import { ModelInput } from '@/components/ui/model-input';
import { PageHeader } from '@/components/layout/page-header';
import { useProviderForm } from '../context/provider-form-context';
import { useProviderNavigation } from '../hooks/use-provider-navigation';

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

  const parseSensitiveWords = (value: string): string[] => {
    return value
      .split(/[\n,]/)
      .map((item) => item.trim())
      .filter(Boolean);
  };

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

      const data: CreateProviderData = {
        type: 'custom',
        name: formData.name,
        logo: formData.logo,
        config: {
          disableErrorCooldown: !!formData.disableErrorCooldown,
          custom: {
            baseURL: formData.baseURL,
            apiKey: formData.apiKey,
            clientBaseURL: Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
            clientMultiplier:
              Object.keys(clientMultiplier).length > 0 ? clientMultiplier : undefined,
            cloak:
              formData.cloakMode !== 'auto' ||
              formData.cloakStrictMode ||
              parseSensitiveWords(formData.cloakSensitiveWords || '').length > 0
                ? {
                    mode: formData.cloakMode,
                    strictMode: formData.cloakStrictMode,
                    sensitiveWords: parseSensitiveWords(formData.cloakSensitiveWords || ''),
                  }
                : undefined,
          },
        },
        supportedClientTypes,
        excludeFromExport: !!formData.excludeFromExport,
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
                      <span>{t('provider.apiKey')}</span>
                    </div>
                  </label>
                  <div className="relative">
                    <Input
                      type={showApiKey ? 'text' : 'password'}
                      value={formData.apiKey}
                      onChange={(e) => updateFormData({ apiKey: e.target.value })}
                      placeholder={t('provider.keyPlaceholder')}
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
              cloak={{
                mode: formData.cloakMode || 'auto',
                strictMode: !!formData.cloakStrictMode,
                sensitiveWords: formData.cloakSensitiveWords || '',
              }}
              onUpdateCloak={(updates) =>
                updateFormData({
                  cloakMode: updates?.mode ?? formData.cloakMode,
                  cloakStrictMode: updates?.strictMode ?? formData.cloakStrictMode,
                  cloakSensitiveWords: updates?.sensitiveWords ?? formData.cloakSensitiveWords,
                })
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
                onCheckedChange={(checked) => updateFormData({ disableErrorCooldown: checked })}
              />
            </div>
          </div>

          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('provider.excludeFromExport')}
            </h3>
            <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
              <div className="pr-4">
                <p className="text-xs text-muted-foreground mt-1">
                  {t('provider.excludeFromExportDesc')}
                </p>
              </div>
              <Switch
                checked={!!formData.excludeFromExport}
                onCheckedChange={(checked) => updateFormData({ excludeFromExport: checked })}
              />
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
