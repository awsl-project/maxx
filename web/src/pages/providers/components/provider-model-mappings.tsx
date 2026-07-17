import { useMemo, useState } from 'react';
import { ArrowRight, Plus, Trash2, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  useModelMappings,
  useProviderRuntimeModels,
  useCreateModelMapping,
  useUpdateModelMapping,
  useDeleteModelMapping,
} from '@/hooks/queries';
import type { Provider, ModelMapping, ModelMappingInput } from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { ModelInput } from '@/components/ui/model-input';

/**
 * Provider-scoped model mappings editor backed by the ModelMapping entity API
 * (scope='provider', providerID). This is the mechanism executor.mapModel
 * actually consults at request time — it matches by provider id and type — so
 * it is the correct home for model mapping across every provider type, not the
 * inline config maps. Shared by the custom edit form and the OpenRouter view.
 */
export function ProviderModelMappings({ provider }: { provider: Provider }) {
  const { t } = useTranslation();
  const { data: allMappings } = useModelMappings();
  const createMapping = useCreateModelMapping();
  const updateMapping = useUpdateModelMapping();
  const deleteMapping = useDeleteModelMapping();
  const { data: runtimeModels } = useProviderRuntimeModels(provider.id);
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');

  // Filter mappings for this provider
  const providerMappings = useMemo(() => {
    return (allMappings || []).filter(
      (m) => m.scope === 'provider' && m.providerID === provider.id,
    );
  }, [allMappings, provider.id]);

  const isPending = createMapping.isPending || updateMapping.isPending || deleteMapping.isPending;
  const providerRuntimeModelOptions = useMemo(() => {
    const modelIDs = new Set<string>();
    const options = [];
    for (const model of [...(provider.supportModels || []), ...(runtimeModels?.models || [])]) {
      const id = model.trim();
      if (!id || modelIDs.has(id)) continue;
      modelIDs.add(id);
      options.push({
        id,
        name: id,
        provider: t('modelInput.currentProviderModels'),
      });
    }
    return options;
  }, [provider.supportModels, runtimeModels?.models, t]);

  const handleAddMapping = async () => {
    if (!newPattern.trim() || !newTarget.trim()) return;

    await createMapping.mutateAsync({
      pattern: newPattern.trim(),
      target: newTarget.trim(),
      scope: 'provider',
      providerID: provider.id,
      providerType: provider.type,
      priority: providerMappings.length * 10 + 1000,
      isEnabled: true,
    });
    setNewPattern('');
    setNewTarget('');
  };

  const handleUpdateMapping = async (mapping: ModelMapping, data: Partial<ModelMappingInput>) => {
    await updateMapping.mutateAsync({
      id: mapping.id,
      data: {
        pattern: data.pattern ?? mapping.pattern,
        target: data.target ?? mapping.target,
        scope: 'provider',
        clientType: data.clientType ?? mapping.clientType,
        providerID: provider.id,
        providerType: provider.type,
        priority: mapping.priority,
        isEnabled: mapping.isEnabled,
      },
    });
  };

  const handleDeleteMapping = async (id: number) => {
    await deleteMapping.mutateAsync(id);
  };

  return (
    <div>
      <div className="mb-4 flex items-center gap-2 border-b border-border pb-2">
        <Zap size={18} className="text-yellow-500" />
        <h4 className="text-lg font-semibold text-foreground">{t('modelMappings.title')}</h4>
        <span className="text-sm text-muted-foreground">({providerMappings.length})</span>
      </div>

      <div className="bg-card border border-border rounded-xl p-4">
        <p className="text-xs text-muted-foreground mb-4">{t('modelMappings.pageDesc')}</p>

        {providerMappings.length > 0 && (
          <div className="space-y-2 mb-4">
            {providerMappings.map((mapping, index) => (
              <div key={mapping.id} className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground w-6 shrink-0">{index + 1}.</span>
                <ModelInput
                  value={mapping.pattern}
                  onChange={(pattern) => handleUpdateMapping(mapping, { pattern })}
                  placeholder={t('modelMappings.matchPattern')}
                  disabled={isPending}
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
                <ModelInput
                  value={mapping.target}
                  onChange={(target) => handleUpdateMapping(mapping, { target })}
                  placeholder={t('modelMappings.targetModel')}
                  disabled={isPending}
                  extraModels={providerRuntimeModelOptions}
                  openSearchValue=""
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleDeleteMapping(mapping.id)}
                  disabled={isPending}
                >
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>
            ))}
          </div>
        )}

        {providerMappings.length === 0 && (
          <div className="text-center py-6 mb-4">
            <p className="text-muted-foreground text-sm">{t('modelMappings.noMappings')}</p>
          </div>
        )}

        <div className="flex items-center gap-2 pt-4 border-t border-border">
          <ModelInput
            value={newPattern}
            onChange={setNewPattern}
            placeholder={t('modelMappings.matchPattern')}
            disabled={isPending}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
          <ModelInput
            value={newTarget}
            onChange={setNewTarget}
            placeholder={t('modelMappings.targetModel')}
            disabled={isPending}
            extraModels={providerRuntimeModelOptions}
            openSearchValue=""
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddMapping}
            disabled={!newPattern.trim() || !newTarget.trim() || isPending}
          >
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  );
}
