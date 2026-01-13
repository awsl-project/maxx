import { Server, Wand2, ChevronLeft, Layers, Grid3X3, CheckCircle2, FilePlus } from 'lucide-react';
import { ANTIGRAVITY_COLOR, quickTemplates, type ProviderFormData } from '../types';

interface SelectTypeStepProps {
  formData: ProviderFormData;
  onSelectType: (type: 'custom' | 'antigravity') => void;
  onApplyTemplate: (templateId: string) => void;
  onSkipToConfig: () => void;
  onBack: () => void;
}

export function SelectTypeStep({
  formData,
  onSelectType,
  onApplyTemplate,
  onSkipToConfig,
  onBack,
}: SelectTypeStepProps) {
  return (
    <div className="flex flex-col h-full">
      <div className="h-[73px] flex items-center gap-md p-lg border-b border-border bg-surface-primary">
        <button
          onClick={onBack}
          className="p-1.5 -ml-1 rounded-lg hover:bg-surface-hover text-text-secondary hover:text-text-primary transition-colors"
        >
          <ChevronLeft size={20} />
        </button>
        <div>
          <h2 className="text-headline font-semibold text-text-primary">Add Provider</h2>
          <p className="text-caption text-text-secondary">Choose a service provider to get started</p>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-lg">
        <div className="container mx-auto max-w-[1600px] space-y-10">
          
          {/* Section: Service Provider */}
          <div className="space-y-4">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              1. Choose Service Provider
            </h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <button
                onClick={() => onSelectType('antigravity')}
                className={`relative group flex items-start gap-5 p-6 rounded-xl border-2 text-left transition-all duration-200 ${
                  formData.type === 'antigravity'
                    ? 'border-accent bg-accent/5'
                    : 'border-border bg-surface-secondary hover:bg-surface-hover hover:border-accent/50'
                }`}
              >
                {formData.type === 'antigravity' && (
                  <div className="absolute top-4 right-4 text-accent animate-in zoom-in duration-200">
                    <CheckCircle2 size={24} className="fill-accent/10" />
                  </div>
                )}
                
                <div
                  className="w-16 h-16 rounded-2xl flex items-center justify-center shrink-0 shadow-sm transition-transform group-hover:scale-105"
                  style={{ backgroundColor: `${ANTIGRAVITY_COLOR}15` }}
                >
                  <Wand2 size={32} style={{ color: ANTIGRAVITY_COLOR }} />
                </div>
                <div>
                  <div className="text-lg font-bold text-text-primary mb-1">Antigravity Cloud</div>
                  <p className="text-sm text-text-secondary leading-relaxed pr-6">
                    Zero-config managed service. Connects to multiple AI models securely via OAuth.
                  </p>
                </div>
              </button>

              <button
                onClick={() => onSelectType('custom')}
                className={`relative group flex items-start gap-5 p-6 rounded-xl border-2 text-left transition-all duration-200 ${
                  formData.type === 'custom'
                    ? 'border-accent bg-accent/5'
                    : 'border-border bg-surface-secondary hover:bg-surface-hover hover:border-accent/50'
                }`}
              >
                {formData.type === 'custom' && (
                  <div className="absolute top-4 right-4 text-accent animate-in zoom-in duration-200">
                    <CheckCircle2 size={24} className="fill-accent/10" />
                  </div>
                )}

                <div className="w-16 h-16 rounded-2xl bg-surface-primary flex items-center justify-center shrink-0 shadow-sm border border-border/50 transition-transform group-hover:scale-105">
                  <Server size={32} className="text-text-secondary group-hover:text-text-primary transition-colors" />
                </div>
                <div>
                  <div className="text-lg font-bold text-text-primary mb-1">Custom Provider</div>
                  <p className="text-sm text-text-secondary leading-relaxed pr-6">
                    Manually configure any compatible AI provider using your own API endpoint and keys.
                  </p>
                </div>
              </button>
            </div>
          </div>

          {/* Section: Templates (Custom only) */}
          {formData.type === 'custom' && (
            <div className="space-y-4 animate-in fade-in slide-in-from-bottom-4 duration-300">
              <div className="flex items-center justify-between border-b border-border pb-2">
                <h3 className="text-lg font-semibold text-text-primary">
                  2. Select a Template <span className="text-text-secondary font-normal text-sm ml-2">(Optional)</span>
                </h3>
              </div>
              
              <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
                {/* Empty Template Card */}
                <button
                  onClick={onSkipToConfig}
                  className="relative group flex flex-col gap-4 p-5 rounded-xl border-2 border-border bg-surface-secondary hover:bg-surface-hover hover:border-accent/30 transition-all duration-200"
                >
                  <div className="flex items-center justify-between w-full">
                     <div className="w-12 h-12 rounded-lg bg-surface-primary flex items-center justify-center border border-border/50 group-hover:border-accent/30 transition-colors">
                        <FilePlus size={24} className="text-text-secondary group-hover:text-accent" />
                     </div>
                  </div>
                  
                  <div className="text-left">
                    <div className="text-base font-semibold text-text-primary mb-1 group-hover:text-accent transition-colors">
                      Empty Template
                    </div>
                    <div className="text-xs text-text-secondary leading-relaxed">
                      Start from scratch with a blank configuration.
                    </div>
                  </div>
                </button>

                {quickTemplates.map((template) => {
                  const Icon = template.icon === 'grid' ? Grid3X3 : Layers;
                  const isSelected = formData.selectedTemplate === template.id;
                  return (
                    <button
                      key={template.id}
                      onClick={() => onApplyTemplate(template.id)}
                      className={`relative group flex flex-col gap-4 p-5 rounded-xl border-2 transition-all duration-200 ${
                        isSelected
                          ? 'border-accent bg-accent/5'
                          : 'border-border bg-surface-secondary hover:bg-surface-hover hover:border-accent/30'
                      }`}
                    >
                      <div className="flex items-center justify-between w-full">
                         <div className={`w-12 h-12 rounded-lg flex items-center justify-center border transition-colors ${
                           isSelected 
                             ? 'bg-accent border-accent/20' 
                             : 'bg-surface-primary border-border/50 group-hover:border-accent/30'
                         }`}>
                            <Icon size={24} className={isSelected ? 'text-accent' : 'text-text-secondary group-hover:text-accent'} />
                         </div>
                         {isSelected && (
                            <div className="text-accent animate-in zoom-in duration-200">
                               <CheckCircle2 size={20} className="fill-accent/10" />
                            </div>
                         )}
                      </div>
                      
                      <div className="text-left">
                        <div className={`text-base font-semibold mb-1 transition-colors ${isSelected ? 'text-accent' : 'text-text-primary'}`}>
                          {template.name}
                        </div>
                        <div className="text-xs text-text-secondary leading-relaxed">{template.description}</div>
                      </div>
                    </button>
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
