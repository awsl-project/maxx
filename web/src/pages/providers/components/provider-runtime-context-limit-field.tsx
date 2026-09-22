import { useMemo, useState } from 'react';
import { Brain, Plus, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ModelInput } from '@/components/ui/model-input';

export function normalizeRuntimeContextLimit(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return 0;
  return Math.floor(value);
}

export function normalizeRuntimeContextLimits(
  value: Record<string, number> | undefined | null,
): Record<string, number> {
  const result: Record<string, number> = {};
  for (const [rawModel, rawLimit] of Object.entries(value ?? {})) {
    const model = rawModel.trim();
    const limit = normalizeRuntimeContextLimit(rawLimit);
    if (model && limit > 0) result[model] = limit;
  }
  return result;
}

export function ProviderRuntimeContextLimitField({
  limit,
  modelLimits,
  onLimitChange,
  onModelLimitsChange,
}: {
  limit: number;
  modelLimits: Record<string, number>;
  onLimitChange: (limit: number) => void;
  onModelLimitsChange: (limits: Record<string, number>) => void;
}) {
  const { t } = useTranslation();
  const [newModel, setNewModel] = useState('');
  const [newLimit, setNewLimit] = useState('');
  const entries = useMemo(() => Object.entries(modelLimits), [modelLimits]);

  const handleAdd = () => {
    const model = newModel.trim();
    const limitValue = normalizeRuntimeContextLimit(Number(newLimit));
    if (!model || limitValue <= 0) return;
    onModelLimitsChange({ ...modelLimits, [model]: limitValue });
    setNewModel('');
    setNewLimit('');
  };

  const handleRemove = (model: string) => {
    const next = { ...modelLimits };
    delete next[model];
    onModelLimitsChange(next);
  };

  const handleRename = (oldModel: string, nextModel: string) => {
    const model = nextModel.trim();
    const next = { ...modelLimits };
    const current = next[oldModel];
    delete next[oldModel];
    if (model && current > 0) next[model] = current;
    onModelLimitsChange(next);
  };

  const handleLimitChange = (model: string, raw: string) => {
    const limitValue = normalizeRuntimeContextLimit(Number(raw));
    const next = { ...modelLimits };
    if (limitValue > 0) next[model] = limitValue;
    else delete next[model];
    onModelLimitsChange(next);
  };

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="mb-4 flex items-center gap-2 border-b border-border pb-2">
        <Brain size={18} className="text-purple-500" />
        <h4 className="text-lg font-semibold text-foreground">
          {t('provider.runtimeContextLimit')}
        </h4>
        <span className="text-sm text-muted-foreground">({entries.length})</span>
      </div>

      <p className="mb-4 text-xs leading-5 text-muted-foreground">
        {t('provider.runtimeContextLimitDesc')}
      </p>

      <div className="mb-4 max-w-sm">
        <label className="mb-2 block text-sm font-medium text-foreground">
          {t('provider.runtimeContextLimitDefault')}
        </label>
        <Input
          type="number"
          min={0}
          step={1}
          value={limit || ''}
          onChange={(event) => onLimitChange(normalizeRuntimeContextLimit(Number(event.target.value)))}
          placeholder="0"
        />
        <p className="mt-1 text-xs text-muted-foreground">
          {t('provider.runtimeContextLimitUnset')}
        </p>
      </div>

      {entries.length > 0 && (
        <div className="mb-4 space-y-2">
          {entries.map(([model, modelLimit]) => (
            <div key={model} className="flex items-center gap-2">
              <ModelInput
                value={model}
                onChange={(value) => handleRename(model, value)}
                placeholder={t('provider.runtimeContextLimitModelPlaceholder')}
                className="min-w-0 flex-1 text-sm"
              />
              <Input
                type="number"
                min={1}
                step={1}
                value={modelLimit}
                onChange={(event) => handleLimitChange(model, event.target.value)}
                className="w-36 text-sm"
              />
              <Button type="button" variant="ghost" size="sm" onClick={() => handleRemove(model)}>
                <Trash2 className="h-4 w-4 text-destructive" />
              </Button>
            </div>
          ))}
        </div>
      )}

      <div className="flex items-center gap-2 border-t border-border pt-4">
        <ModelInput
          value={newModel}
          onChange={setNewModel}
          placeholder={t('provider.runtimeContextLimitModelPlaceholder')}
          className="min-w-0 flex-1 text-sm"
        />
        <Input
          type="number"
          min={1}
          step={1}
          value={newLimit}
          onChange={(event) => setNewLimit(event.target.value)}
          placeholder={t('provider.runtimeContextLimitTokensPlaceholder')}
          className="w-40 text-sm"
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleAdd}
          disabled={!newModel.trim() || normalizeRuntimeContextLimit(Number(newLimit)) <= 0}
        >
          <Plus className="mr-1 h-4 w-4" />
          {t('common.add')}
        </Button>
      </div>
    </div>
  );
}
