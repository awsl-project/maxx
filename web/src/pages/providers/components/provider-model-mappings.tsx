import { useEffect, useMemo, useState } from 'react';
import { ArrowRight, CopyPlus, Plus, Trash2, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  useProviders,
  useModelMappings,
  useCreateModelMapping,
  useUpdateModelMapping,
  useDeleteModelMapping,
} from '@/hooks/queries';
import type { Provider, ModelMapping, ModelMappingInput } from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { ModelInput } from '@/components/ui/model-input';
import { normalizeProviderList } from '../utils/provider-normalize';

type ModelMappingPresetEntry = {
  pattern: string;
  target: string;
  clientType: string;
};

type ModelMappingPreset = {
  id: string;
  entries: ModelMappingPresetEntry[];
  source: Provider;
};

type ApplyMode = 'append' | 'overwrite' | 'replace';

function normalizeClientType(clientType: string | undefined) {
  return clientType?.trim() || 'all';
}

function formatClientType(clientType: string) {
  return clientType === 'all' ? 'All clients' : clientType;
}

function getPresetEntryKey(entry: ModelMappingPresetEntry) {
  return `${normalizeClientType(entry.clientType)}\u0000${entry.pattern}\u0000${entry.target}`;
}

function buildModelMappingPresets(
  allMappings: ModelMapping[] | undefined,
  providers: Provider[] | undefined,
  currentProvider: Provider,
) {
  const providersById = new Map((providers ?? []).map((item) => [item.id, item]));
  const mappingsByProvider = new Map<number, ModelMapping[]>();

  for (const mapping of allMappings ?? []) {
    if (mapping.scope !== 'provider') continue;
    if (mapping.isEnabled === false) continue;
    if (!mapping.providerID || mapping.providerID === currentProvider.id) continue;
    if (!mapping.pattern.trim() || !mapping.target.trim()) continue;

    const source = providersById.get(mapping.providerID);
    if (!source) continue;

    const group = mappingsByProvider.get(mapping.providerID) ?? [];
    group.push(mapping);
    mappingsByProvider.set(mapping.providerID, group);
  }

  return Array.from(mappingsByProvider.entries())
    .map(([providerID, mappings]) => {
      const source = providersById.get(providerID);
      if (!source) return null;

      const entries = mappings
        .slice()
        .sort(
          (left, right) =>
            left.priority - right.priority || left.pattern.localeCompare(right.pattern),
        )
        .map((mapping) => ({
          pattern: mapping.pattern,
          target: mapping.target,
          clientType: normalizeClientType(mapping.clientType),
        }));

      if (entries.length === 0) return null;

      return {
        id: `provider:${providerID}`,
        entries,
        source,
      };
    })
    .filter((preset): preset is ModelMappingPreset => preset !== null)
    .sort((left, right) => {
      const leftSameType = left.source.type === currentProvider.type ? 0 : 1;
      const rightSameType = right.source.type === currentProvider.type ? 0 : 1;
      return (
        leftSameType - rightSameType ||
        right.entries.length - left.entries.length ||
        left.source.name.localeCompare(right.source.name)
      );
    });
}

function getPresetPreview(entries: ModelMappingPresetEntry[], providerMappings: ModelMapping[]) {
  const currentByPattern = new Map(providerMappings.map((mapping) => [mapping.pattern, mapping]));
  let added = 0;
  let conflicts = 0;
  let unchanged = 0;

  for (const entry of entries) {
    const existing = currentByPattern.get(entry.pattern);
    if (!existing) {
      added += 1;
    } else if (
      existing.target === entry.target &&
      normalizeClientType(existing.clientType) === entry.clientType
    ) {
      unchanged += 1;
    } else {
      conflicts += 1;
    }
  }

  return { added, conflicts, unchanged };
}

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
  const { data: allProviders } = useProviders();
  const createMapping = useCreateModelMapping();
  const updateMapping = useUpdateModelMapping();
  const deleteMapping = useDeleteModelMapping();
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');
  const [isPresetDialogOpen, setIsPresetDialogOpen] = useState(false);
  const [selectedPresetId, setSelectedPresetId] = useState<string | null>(null);
  const [selectedEntryKeys, setSelectedEntryKeys] = useState<Set<string>>(new Set());
  const [applyMode, setApplyMode] = useState<ApplyMode>('append');

  // Filter mappings for this provider
  const providerMappings = useMemo(() => {
    return (allMappings || []).filter(
      (m) => m.scope === 'provider' && m.providerID === provider.id,
    );
  }, [allMappings, provider.id]);

  const providers = useMemo(() => normalizeProviderList(allProviders), [allProviders]);

  const presets = useMemo(
    () => buildModelMappingPresets(allMappings, providers, provider),
    [allMappings, providers, provider],
  );

  const selectedPreset = useMemo(
    () => presets.find((preset) => preset.id === selectedPresetId) ?? presets[0] ?? null,
    [presets, selectedPresetId],
  );

  const selectedEntries = useMemo(() => {
    if (!selectedPreset) return [];
    return selectedPreset.entries.filter((entry) =>
      selectedEntryKeys.has(getPresetEntryKey(entry)),
    );
  }, [selectedEntryKeys, selectedPreset]);

  const presetPreview = useMemo(
    () => getPresetPreview(selectedEntries, providerMappings),
    [providerMappings, selectedEntries],
  );

  useEffect(() => {
    if (!isPresetDialogOpen || !selectedPreset) return;
    setSelectedEntryKeys(new Set(selectedPreset.entries.map(getPresetEntryKey)));
  }, [isPresetDialogOpen, selectedPreset]);

  const isPending = createMapping.isPending || updateMapping.isPending || deleteMapping.isPending;

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

  const handleApplyPreset = async () => {
    if (!selectedPreset || selectedEntries.length === 0 || isPending) return;

    const currentByPattern = new Map(providerMappings.map((mapping) => [mapping.pattern, mapping]));
    const nextPriorityBase = providerMappings.length * 10 + 1000;

    if (applyMode === 'replace') {
      for (const mapping of providerMappings) {
        await deleteMapping.mutateAsync(mapping.id);
      }
    }

    const effectiveCurrentByPattern =
      applyMode === 'replace' ? new Map<string, ModelMapping>() : currentByPattern;

    for (const [index, entry] of selectedEntries.entries()) {
      const existing = effectiveCurrentByPattern.get(entry.pattern);
      if (existing) {
        if (applyMode !== 'append') {
          await handleUpdateMapping(existing, {
            target: entry.target,
            clientType: entry.clientType === 'all' ? '' : entry.clientType,
          });
        }
        continue;
      }

      await createMapping.mutateAsync({
        pattern: entry.pattern,
        target: entry.target,
        scope: 'provider',
        clientType: entry.clientType === 'all' ? '' : entry.clientType,
        providerID: provider.id,
        providerType: provider.type,
        priority: nextPriorityBase + index * 10,
        isEnabled: true,
      });
    }

    setIsPresetDialogOpen(false);
  };

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-4 border-b border-border pb-2">
        <div className="flex items-center gap-2">
          <Zap size={18} className="text-yellow-500" />
          <h4 className="text-lg font-semibold text-foreground">{t('modelMappings.title')}</h4>
          <span className="text-sm text-muted-foreground">({providerMappings.length})</span>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            const nextPreset = presets[0] ?? null;
            setSelectedPresetId(nextPreset?.id ?? null);
            setSelectedEntryKeys(new Set(nextPreset?.entries.map(getPresetEntryKey) ?? []));
            setApplyMode('append');
            setIsPresetDialogOpen(true);
          }}
          disabled={presets.length === 0 || isPending}
        >
          <CopyPlus className="h-4 w-4 mr-1" />
          {t('modelMappings.loadPreset')}
        </Button>
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

      <Dialog open={isPresetDialogOpen} onOpenChange={setIsPresetDialogOpen}>
        <DialogContent className="max-w-4xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t('modelMappings.loadPresetTitle')}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-4">
              {selectedPreset && (
                <>
                  <div className="rounded-lg border border-border bg-card p-3">
                    <div className="mb-2 text-sm font-medium text-foreground">
                      {t('modelMappings.sourceProvider')}
                    </div>
                    <div className="flex max-h-40 flex-wrap gap-2 overflow-y-auto pr-1">
                      {presets.map((preset) => (
                        <button
                          key={preset.id}
                          type="button"
                          onClick={() => {
                            setSelectedPresetId(preset.id);
                            setSelectedEntryKeys(new Set(preset.entries.map(getPresetEntryKey)));
                          }}
                          className={`rounded-md border px-3 py-2 text-left text-xs transition-colors ${
                            preset.id === selectedPreset.id
                              ? 'border-primary bg-primary/10 text-foreground'
                              : 'border-border bg-muted/30 text-muted-foreground hover:bg-muted/60'
                          }`}
                        >
                          <div className="font-medium text-foreground">{preset.source.name}</div>
                          <div>
                            {preset.source.type} ·{' '}
                            {t('modelMappings.presetMappingsCount', {
                              count: preset.entries.length,
                            })}
                          </div>
                        </button>
                      ))}
                    </div>
                  </div>

                  <div className="rounded-lg border border-border bg-card p-3">
                    <div className="mb-2 flex items-center justify-between gap-2">
                      <span className="text-sm font-medium text-foreground">
                        {t('modelMappings.presetPreview')}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {t('modelMappings.presetSelectedMappingsCount', {
                          selected: selectedEntries.length,
                          total: selectedPreset.entries.length,
                        })}
                      </span>
                    </div>
                    <div className="mb-3 flex flex-wrap gap-2 text-xs">
                      <Badge variant="success">
                        {t('modelMappings.presetAdds', { count: presetPreview.added })}
                      </Badge>
                      <Badge variant={presetPreview.conflicts > 0 ? 'warning' : 'secondary'}>
                        {t('modelMappings.presetConflicts', { count: presetPreview.conflicts })}
                      </Badge>
                      <Badge variant="secondary">
                        {t('modelMappings.presetUnchanged', { count: presetPreview.unchanged })}
                      </Badge>
                    </div>
                    <div className="mb-3 flex flex-wrap gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() =>
                          setSelectedEntryKeys(
                            new Set(selectedPreset.entries.map(getPresetEntryKey)),
                          )
                        }
                        disabled={selectedEntries.length === selectedPreset.entries.length}
                      >
                        {t('modelMappings.selectAllPresetMappings')}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setSelectedEntryKeys(new Set())}
                        disabled={selectedEntries.length === 0}
                      >
                        {t('modelMappings.clearPresetMappings')}
                      </Button>
                    </div>
                    <div className="max-h-56 space-y-2 overflow-y-auto pr-1">
                      {selectedPreset.entries.map((entry, index) => {
                        const entryKey = getPresetEntryKey(entry);
                        const isSelected = selectedEntryKeys.has(entryKey);
                        return (
                          <div
                            key={`${entryKey}:${index}`}
                            className="grid grid-cols-[auto_auto_minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-2 rounded-md bg-muted/50 px-2 py-1.5 text-xs"
                          >
                            <input
                              type="checkbox"
                              aria-label={t('modelMappings.selectPresetMapping', {
                                pattern: entry.pattern,
                                target: entry.target,
                              })}
                              checked={isSelected}
                              onChange={(event) => {
                                setSelectedEntryKeys((previous) => {
                                  const next = new Set(previous);
                                  if (event.target.checked) {
                                    next.add(entryKey);
                                  } else {
                                    next.delete(entryKey);
                                  }
                                  return next;
                                });
                              }}
                            />
                            <Badge variant="secondary">{formatClientType(entry.clientType)}</Badge>
                            <span className="truncate font-mono">{entry.pattern}</span>
                            <ArrowRight className="h-3.5 w-3.5 text-muted-foreground" />
                            <span className="truncate font-mono">{entry.target}</span>
                          </div>
                        );
                      })}
                    </div>
                  </div>

                  <div className="rounded-lg border border-border bg-card p-3">
                    <div className="mb-2 text-sm font-medium text-foreground">
                      {t('modelMappings.applyMode')}
                    </div>
                    <div className="grid gap-2">
                      {(['append', 'overwrite', 'replace'] as ApplyMode[]).map((mode) => (
                        <label
                          key={mode}
                          className="flex cursor-pointer items-start gap-2 rounded-md border border-border p-2 text-sm hover:bg-muted/50"
                        >
                          <input
                            type="radio"
                            name="model-mapping-apply-mode"
                            value={mode}
                            checked={applyMode === mode}
                            onChange={() => setApplyMode(mode)}
                            className="mt-1"
                          />
                          <span className="font-medium">
                            {t(`modelMappings.applyMode.${mode}.title`)}
                          </span>
                        </label>
                      ))}
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>

          <div className="flex justify-end gap-2 border-t border-border pt-4">
            <Button
              variant="ghost"
              onClick={() => setIsPresetDialogOpen(false)}
              disabled={isPending}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleApplyPreset}
              disabled={!selectedPreset || selectedEntries.length === 0 || isPending}
            >
              {t('modelMappings.applyPreset')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
