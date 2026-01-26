import {
  Server,
  Wand2,
  ChevronLeft,
  Layers,
  Grid3X3,
  CheckCircle2,
  FilePlus,
  Cloud,
  Code2,
} from 'lucide-react';
import { quickTemplates, PROVIDER_TYPE_CONFIGS } from '../types';
import { Button } from '@/components/ui';
import { useTranslation } from 'react-i18next';
import { useProviderForm } from '../context/provider-form-context';
import { useProviderNavigation } from '../hooks/use-provider-navigation';

export function SelectTypeStep() {
  const { formData, updateFormData } = useProviderForm();
  const { goToCustomConfig, goToAntigravity, goToKiro, goToCodex, goToProviders } =
    useProviderNavigation();
  const { t } = useTranslation();

  const handleSelectType = (type: 'custom' | 'antigravity' | 'kiro' | 'codex') => {
    updateFormData({ type });
    if (type === 'antigravity') {
      goToAntigravity();
    } else if (type === 'kiro') {
      goToKiro();
    } else if (type === 'codex') {
      goToCodex();
    }
  };

  const handleApplyTemplate = (templateId: string) => {
    const template = quickTemplates.find((t) => t.id === templateId);
    if (template) {
      const updatedClients = formData.clients.map((client) => {
        const isSupported = template.supportedClients.includes(client.id);
        const baseURL = template.clientBaseURLs[client.id] || '';
        return { ...client, enabled: isSupported, urlOverride: baseURL };
      });

      updateFormData({
        selectedTemplate: templateId,
        name: template.name,
        clients: updatedClients,
        modelMappings: template.modelMappings,
        logo: template.logoUrl,
      });

      goToCustomConfig();
    }
  };
  // 计算可见的 provider 数量
  const visibleProviderCount = Object.values(PROVIDER_TYPE_CONFIGS).filter((c) => !c.hidden).length;
  const gridCols = visibleProviderCount <= 2 ? 'md:grid-cols-2' : 'md:grid-cols-3';

  return (
    <div className="flex flex-col h-full">
      <div className="px-4 sm:px-6 h-[73px] flex items-center gap-4 border-b border-border bg-card">
        <Button onClick={goToProviders} variant={'ghost'} size="icon">
          <ChevronLeft className="size-5" />
        </Button>
        <div className="flex-1 min-w-0">
          <h2 className="text-base sm:text-lg font-semibold text-foreground truncate">
            {t('addProvider.title')}
          </h2>
          <p className="text-xs sm:text-sm text-muted-foreground truncate">
            {t('addProvider.subtitle')}
          </p>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4 sm:p-6">
        <div className="max-w-7xl mx-auto space-y-6 sm:space-y-8 lg:space-y-10">
          {/* Section: Service Provider */}
          <div className="space-y-3 sm:space-y-4">
            <h3 className="text-base sm:text-lg font-semibold text-foreground border-b border-border/60 pb-2.5">
              1. {t('addProvider.chooseProvider')}
            </h3>
            <div className={`grid grid-cols-1 ${gridCols} gap-4 items-start`}>
              <Button
                onClick={() => handleSelectType('antigravity')}
                variant="ghost"
                className={`group p-0 rounded-xl border text-left h-auto w-full overflow-hidden transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 ${
                  formData.type === 'antigravity'
                    ? 'border-provider-antigravity bg-provider-antigravity/10 shadow-sm'
                    : 'border-border bg-card hover:bg-muted hover:border-accent/30 hover:shadow-sm'
                }`}
              >
                <div className="p-4 sm:p-5 flex items-center gap-3 sm:gap-4 min-w-0 w-full">
                  <div className="size-10 sm:size-11 md:size-12 rounded-lg bg-provider-antigravity/15 flex items-center justify-center shrink-0 transition-transform duration-200 group-hover:scale-105">
                    <Wand2 className="size-5 md:size-6 text-provider-antigravity" />
                  </div>

                  <div className="flex-1 min-w-0 space-y-1">
                    <h3 className="text-sm sm:text-base font-semibold text-foreground leading-tight truncate">
                      {t('addProvider.antigravity.name')}
                    </h3>
                    <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed line-clamp-2">
                      {t('addProvider.antigravity.description')}
                    </p>
                  </div>

                  {formData.type === 'antigravity' && (
                    <CheckCircle2 className="size-5 text-provider-antigravity shrink-0 self-center animate-in zoom-in-50 duration-200" />
                  )}
                </div>
              </Button>

              {!PROVIDER_TYPE_CONFIGS.kiro.hidden && (
                <Button
                  onClick={() => handleSelectType('kiro')}
                  variant="ghost"
                  className={`group p-0 rounded-lg border text-left transition-all h-auto w-full ${
                    formData.type === 'kiro'
                      ? 'border-provider-kiro bg-provider-kiro/10'
                      : 'border-border bg-card hover:bg-muted'
                  }`}
                >
                  <div className="p-5 flex items-center gap-4">
                    <div className="w-12 h-12 rounded-md bg-provider-kiro/15 flex items-center justify-center shrink-0">
                      <Cloud size={24} className="text-provider-kiro" />
                    </div>

                    <div className="flex-1 min-w-0">
                      <h3 className="text-headline font-semibold text-foreground mb-1">
                        {t('addProvider.kiro.name')}
                      </h3>
                      <p className="text-caption text-muted-foreground">
                        {t('addProvider.kiro.description')}
                      </p>
                    </div>

                    {formData.type === 'kiro' && (
                      <CheckCircle2 size={20} className="text-provider-kiro shrink-0" />
                    )}
                  </div>
                </Button>
              )}

              {!PROVIDER_TYPE_CONFIGS.codex.hidden && (
                <Button
                  onClick={() => handleSelectType('codex')}
                  variant="ghost"
                  className={`group p-0 rounded-xl border text-left h-auto w-full overflow-hidden transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 ${
                    formData.type === 'codex'
                      ? 'border-provider-codex bg-provider-codex/10 shadow-sm'
                      : 'border-border bg-card hover:bg-muted hover:border-accent/30 hover:shadow-sm'
                  }`}
                >
                  <div className="p-4 sm:p-5 flex items-center gap-3 sm:gap-4 min-w-0 w-full">
                    <div className="size-10 sm:size-11 md:size-12 rounded-lg bg-provider-codex/15 flex items-center justify-center shrink-0 transition-transform duration-200 group-hover:scale-105">
                      <Code2 className="size-5 md:size-6 text-provider-codex" />
                    </div>

                    <div className="flex-1 min-w-0 space-y-1">
                      <h3 className="text-sm sm:text-base font-semibold text-foreground leading-tight truncate">
                        {t('addProvider.codex.name', 'Codex')}
                      </h3>
                      <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed line-clamp-2">
                        {t('addProvider.codex.description', 'OpenAI Codex with OAuth authentication')}
                      </p>
                    </div>

                    {formData.type === 'codex' && (
                      <CheckCircle2 className="size-5 text-provider-codex shrink-0 self-center animate-in zoom-in-50 duration-200" />
                    )}
                  </div>
                </Button>
              )}

              <Button
                onClick={() => handleSelectType('custom')}
                variant="ghost"
                className={`group p-0 rounded-xl border text-left h-auto w-full overflow-hidden transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 ${
                  formData.type === 'custom'
                    ? 'border-provider-custom bg-provider-custom/10 shadow-sm'
                    : 'border-border bg-card hover:bg-muted hover:border-accent/30 hover:shadow-sm'
                }`}
              >
                <div className="p-4 sm:p-5 flex items-center gap-3 sm:gap-4 min-w-0 w-full">
                  <div className="size-10 sm:size-11 md:size-12 rounded-lg bg-provider-custom/15 flex items-center justify-center shrink-0 transition-transform duration-200 group-hover:scale-105">
                    <Server className="size-5 md:size-6 text-provider-custom" />
                  </div>

                  <div className="flex-1 min-w-0 space-y-1">
                    <h3 className="text-sm sm:text-base font-semibold text-foreground leading-tight truncate">
                      {t('addProvider.custom.name')}
                    </h3>
                    <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed line-clamp-2">
                      {t('addProvider.custom.description')}
                    </p>
                  </div>

                  {formData.type === 'custom' && (
                    <CheckCircle2 className="size-5 text-provider-custom shrink-0 self-center animate-in zoom-in-50 duration-200" />
                  )}
                </div>
              </Button>
            </div>
          </div>

          {/* Section: Templates (Custom only) */}
          {formData.type === 'custom' && (
            <div className="space-y-3 sm:space-y-4 animate-in fade-in slide-in-from-bottom-4 duration-300">
              <div className="flex items-center justify-between border-b border-border/60 pb-2.5">
                <h3 className="text-base sm:text-lg font-semibold text-foreground">
                  2. {t('addProvider.selectTemplate')}{' '}
                  <span className="text-muted-foreground font-normal text-sm ml-2">
                    {t('addProvider.optional')}
                  </span>
                </h3>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3 sm:gap-4 items-start">
                {/* Empty Template Card */}
                <Button
                  onClick={goToCustomConfig}
                  variant="ghost"
                  className="text-left group p-0 rounded-xl border border-dashed h-full w-full min-h-36 sm:min-h-40 transition-all duration-200 border-border bg-card hover:bg-muted hover:border-accent/30 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
                >
                  <div className="p-4 sm:p-5 flex flex-col gap-3 sm:gap-4 h-full w-full">
                    <div className="flex items-center justify-between w-full">
                      <div className="size-9 sm:size-10 rounded-lg flex items-center justify-center overflow-hidden transition-all duration-200 group-hover:scale-105 bg-muted group-hover:bg-primary/10">
                        <FilePlus className="size-4 sm:size-5 text-muted-foreground group-hover:text-primary transition-colors" />
                      </div>
                    </div>

                    <div className="flex-1 space-y-1">
                      <h4 className="text-sm font-semibold text-foreground leading-tight truncate">
                        {t('addProvider.emptyTemplate')}
                      </h4>
                      <p className="text-xs text-muted-foreground leading-relaxed line-clamp-2">
                        {t('addProvider.startFromScratch')}
                      </p>
                    </div>
                  </div>
                </Button>

                {quickTemplates.map((template) => {
                  const Icon = template.icon === 'grid' ? Grid3X3 : Layers;
                  const isSelected = formData.selectedTemplate === template.id;
                  const templateName = template.nameKey ? t(template.nameKey) : template.name;
                  const templateDescription = template.descriptionKey
                    ? t(template.descriptionKey)
                    : template.description;

                  return (
                    <Button
                      key={template.id}
                      onClick={() => handleApplyTemplate(template.id)}
                      variant="ghost"
                      className={`group p-0 rounded-xl border text-left h-full w-full min-h-36 sm:min-h-40 transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 ${
                        isSelected
                          ? 'border-primary bg-primary/10 shadow-sm'
                          : 'border-border bg-card hover:bg-muted hover:border-accent/30 hover:shadow-sm'
                      }`}
                    >
                      <div className="p-4 sm:p-5 flex flex-col gap-3 sm:gap-4 h-full w-full">
                        <div className="flex items-center justify-between w-full">
                          <div
                            className={`size-9 sm:size-10 rounded-lg flex items-center justify-center overflow-hidden transition-all duration-200 group-hover:scale-105 ${
                              isSelected ? 'bg-primary/15' : 'bg-muted group-hover:bg-primary/10'
                            }`}
                          >
                            {template.logoUrl ? (
                              <img
                                src={template.logoUrl}
                                alt={templateName}
                                className="w-full h-full object-contain"
                              />
                            ) : (
                              <Icon
                                className={`size-4 sm:size-5 ${
                                  isSelected
                                    ? 'text-primary'
                                    : 'text-muted-foreground group-hover:text-primary transition-colors'
                                }`}
                              />
                            )}
                          </div>
                          {isSelected && (
                            <CheckCircle2 className="size-4 sm:size-[18px] text-primary animate-in zoom-in-50 duration-200" />
                          )}
                        </div>

                        <div className="flex-1 space-y-1">
                          <h4
                            className={`text-sm font-semibold leading-tight truncate transition-colors ${
                              isSelected ? 'text-primary' : 'text-foreground'
                            }`}
                          >
                            {templateName}
                          </h4>
                          <p className="text-xs text-muted-foreground leading-relaxed line-clamp-2">
                            {templateDescription}
                          </p>
                        </div>
                      </div>
                    </Button>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
