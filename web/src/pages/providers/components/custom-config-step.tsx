import { Globe, ChevronLeft, Key, Check, Plus, Trash2, ArrowRight } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCreateProvider, useCreateModelMapping } from '@/hooks/queries';
import type { ClientType, CreateProviderData } from '@/lib/transport';
import { ClientsConfigSection } from './clients-config-section';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ModelInput } from '@/components/ui/model-input';
import { useProviderForm } from '../context/provider-form-context';
import { useProviderNavigation } from '../hooks/use-provider-navigation';

export function CustomConfigStep() {
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
          custom: {
            baseURL: formData.baseURL,
            apiKey: formData.apiKey,
            clientBaseURL: Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
            clientMultiplier: Object.keys(clientMultiplier).length > 0 ? clientMultiplier : undefined,
          },
        },
        supportedClientTypes,
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
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-card">
        <div className="flex items-center gap-4">
          <Button onClick={goToSelectType} variant="ghost" size="sm">
            <ChevronLeft size={20} />
          </Button>
          <div>
            <h2 className="text-headline font-semibold text-foreground">
              {t('provider.configure')}
            </h2>
            <p className="text-caption text-muted-foreground">
              {t('provider.configureDescription')}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
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
        </div>
      </div>

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
                  <Input
                    type="password"
                    value={formData.apiKey}
                    onChange={(e) => updateFormData({ apiKey: e.target.value })}
                    placeholder={t('provider.keyPlaceholder')}
                    className="w-full"
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('provider.clientConfig')}
            </h3>
            <ClientsConfigSection clients={formData.clients} onUpdateClient={updateClient} />
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
