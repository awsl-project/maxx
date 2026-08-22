import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import type { DragEndEvent } from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  Button,
  Card,
  CardContent,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui';
import { ModelInput } from '@/components/ui/model-input';
import { PageHeader } from '@/components/layout/page-header';
import {
  useModelMappings,
  useCreateModelMapping,
  useUpdateModelMapping,
  useReorderModelMappings,
  useDeleteModelMapping,
  useClearAllModelMappings,
  useResetModelMappingsToDefaults,
  usePublicSettings,
} from '@/hooks/queries';
import type { ModelMapping, ModelMappingInput } from '@/lib/transport/types';
import { Zap, Plus, Trash2, ArrowRight, RotateCcw, GripVertical, Search } from 'lucide-react';
import { useDialog } from '@/contexts/dialog-context';

const MODEL_MAPPING_DEBUGGER_SETTING_KEY = 'ui_model_mapping_debugger_enabled';

function matchWildcard(pattern: string, input: string): boolean {
  const trimmedPattern = pattern.trim();
  const trimmedInput = input.trim();
  if (!trimmedPattern) return false;
  if (trimmedPattern === '*') return true;

  let patternIndex = 0;
  let inputIndex = 0;
  let starIndex = -1;
  let matchIndex = 0;

  while (inputIndex < trimmedInput.length) {
    if (
      patternIndex < trimmedPattern.length &&
      trimmedPattern[patternIndex] === trimmedInput[inputIndex]
    ) {
      patternIndex += 1;
      inputIndex += 1;
      continue;
    }
    if (patternIndex < trimmedPattern.length && trimmedPattern[patternIndex] === '*') {
      starIndex = patternIndex;
      matchIndex = inputIndex;
      patternIndex += 1;
      continue;
    }
    if (starIndex !== -1) {
      patternIndex = starIndex + 1;
      matchIndex += 1;
      inputIndex = matchIndex;
      continue;
    }
    return false;
  }

  while (patternIndex < trimmedPattern.length && trimmedPattern[patternIndex] === '*') {
    patternIndex += 1;
  }
  return patternIndex === trimmedPattern.length;
}

function scopePriority(scope?: string): number {
  if (scope === 'route') return 1;
  if (scope === 'provider') return 2;
  return 3;
}

function describeMappingScope(rule: ModelMapping, t: (key: string) => string) {
  const scope = rule.scope || 'global';
  const parts = [
    scope === 'provider'
      ? t('modelMappings.scopeProvider')
      : scope === 'route'
        ? t('modelMappings.scopeRoute')
        : t('modelMappings.scopeGlobal'),
  ];
  if (rule.clientType) parts.push(`${t('modelMappings.client')}: ${rule.clientType}`);
  if (rule.providerType) parts.push(`${t('modelMappings.providerType')}: ${rule.providerType}`);
  if (rule.providerID) parts.push(`${t('modelMappings.providerID')}: ${rule.providerID}`);
  if (rule.projectID) parts.push(`${t('modelMappings.projectID')}: ${rule.projectID}`);
  if (rule.routeID) parts.push(`Route# ${rule.routeID}`);
  if (rule.apiTokenID) parts.push(`Token# ${rule.apiTokenID}`);
  return parts.join(' · ');
}

interface SortableRuleItemProps {
  id: string;
  index: number;
  rule: ModelMapping;
  onRemove: () => void;
  onUpdate: (data: Partial<ModelMappingInput>) => void;
  disabled: boolean;
}

function SortableRuleItem({
  id,
  index,
  rule,
  onRemove,
  onUpdate,
  disabled,
}: SortableRuleItemProps) {
  const { t } = useTranslation();
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`flex items-center gap-3 py-2 ${isDragging ? 'opacity-50' : ''}`}
    >
      <button
        {...attributes}
        {...listeners}
        className="cursor-grab active:cursor-grabbing p-1 hover:bg-accent rounded shrink-0"
        disabled={disabled}
      >
        <GripVertical className="h-4 w-4 text-muted-foreground" />
      </button>
      <span className="text-xs text-muted-foreground w-6 shrink-0">{index + 1}.</span>

      {/* Pattern -> Target */}
      <ModelInput
        value={rule.pattern}
        onChange={(pattern) => onUpdate({ pattern })}
        placeholder={t('modelMappings.matchPattern')}
        disabled={disabled}
        className="flex-1 min-w-0 h-7 text-xs"
      />
      <ArrowRight className="h-3 w-3 text-muted-foreground shrink-0" />
      <ModelInput
        value={rule.target}
        onChange={(target) => onUpdate({ target })}
        placeholder={t('modelMappings.targetModel')}
        disabled={disabled}
        className="flex-1 min-w-0 h-7 text-xs"
      />

      {/* Client Type (read-only) */}
      <span className="w-[80px] h-7 text-xs shrink-0 flex items-center text-muted-foreground">
        {rule.clientType || t('modelMappings.allClients')}
      </span>

      {/* Provider Type (read-only) */}
      <span className="w-[90px] h-7 text-xs shrink-0 flex items-center text-muted-foreground">
        {rule.providerType || t('modelMappings.allProviderTypes')}
      </span>

      {/* Provider ID (read-only) */}
      <span className="w-[50px] h-7 text-xs shrink-0 flex items-center text-muted-foreground">
        {rule.providerID || '-'}
      </span>

      {/* Project ID (read-only) */}
      <span className="w-[50px] h-7 text-xs shrink-0 flex items-center text-muted-foreground">
        {rule.projectID || '-'}
      </span>

      <Button variant="ghost" size="sm" onClick={onRemove} disabled={disabled} className="shrink-0">
        <Trash2 className="h-4 w-4 text-destructive" />
      </Button>
    </div>
  );
}

export function ModelMappingsPage() {
  const { t } = useTranslation();
  const { confirm } = useDialog();
  const { data: mappings, isLoading } = useModelMappings();
  const { data: publicSettings } = usePublicSettings();
  const createMapping = useCreateModelMapping();
  const updateMapping = useUpdateModelMapping();
  const reorderMappings = useReorderModelMappings();
  const deleteMapping = useDeleteModelMapping();
  const clearAllMappings = useClearAllModelMappings();
  const resetToDefaults = useResetModelMappingsToDefaults();
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');
  const [newClientType, setNewClientType] = useState('claude');
  const [newProviderType, setNewProviderType] = useState('antigravity');
  const [debugModel, setDebugModel] = useState('');
  const debuggerEnabled = publicSettings?.[MODEL_MAPPING_DEBUGGER_SETTING_KEY] === 'true';

  // Filter only global scope mappings
  const rules = (mappings || []).filter((m) => !m.scope || m.scope === 'global');
  const debugQuery = debugModel.trim();
  const debugMatches = useMemo(() => {
    if (!debugQuery) return [];
    const lowerQuery = debugQuery.toLowerCase();
    return (mappings || [])
      .filter((rule) => {
        const textMatches = [
          rule.id,
          rule.scope,
          rule.clientType,
          rule.providerType,
          rule.providerID,
          rule.projectID,
          rule.routeID,
          rule.apiTokenID,
          rule.pattern,
          rule.target,
        ]
          .map((value) => String(value || '').toLowerCase())
          .some((value) => value.includes(lowerQuery));
        return textMatches || matchWildcard(rule.pattern, debugQuery);
      })
      .sort((a, b) => {
        const scopeDiff = scopePriority(a.scope) - scopePriority(b.scope);
        if (scopeDiff !== 0) return scopeDiff;
        if (a.priority !== b.priority) return a.priority - b.priority;
        return a.id - b.id;
      });
  }, [debugQuery, mappings]);
  const effectiveDebugMatches = debugMatches.filter((rule) =>
    matchWildcard(rule.pattern, debugQuery),
  );

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const handleDragStart = () => {
    document.body.classList.add('is-dragging');
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    document.body.classList.remove('is-dragging');
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = rules.findIndex((r) => `rule-${r.id}` === active.id);
    const newIndex = rules.findIndex((r) => `rule-${r.id}` === over.id);

    if (oldIndex !== -1 && newIndex !== -1) {
      const reordered = arrayMove(rules, oldIndex, newIndex);
      await reorderMappings.mutateAsync({
        scope: 'global',
        orderedIDs: reordered.map((rule) => rule.id),
      });
    }
  };

  const handleAddRule = async () => {
    if (!newPattern.trim() || !newTarget.trim()) return;

    await createMapping.mutateAsync({
      pattern: newPattern.trim(),
      target: newTarget.trim(),
      scope: 'global',
      clientType: newClientType,
      providerType: newProviderType,
      priority: rules.length * 10 + 1000,
      isEnabled: true,
    });
    setNewPattern('');
    setNewTarget('');
    setNewClientType('claude');
    setNewProviderType('antigravity');
  };

  const handleRemoveRule = async (id: number) => {
    await deleteMapping.mutateAsync(id);
  };

  const handleRemoveDebugRule = async (rule: ModelMapping) => {
    const confirmed = await confirm({
      title: t('common.confirm'),
      description: t('modelMappings.debuggerConfirmDelete', {
        id: rule.id,
        pattern: rule.pattern,
        target: rule.target,
      }),
      confirmText: t('common.delete'),
      confirmVariant: 'destructive',
    });
    if (!confirmed) return;

    await deleteMapping.mutateAsync(rule.id);
  };

  const handleRemoveAllDebugMatches = async () => {
    if (debugMatches.length === 0) return;
    const confirmed = await confirm({
      title: t('common.confirm'),
      description: t('modelMappings.debuggerConfirmDeleteAll', { count: debugMatches.length }),
      confirmText: t('common.delete'),
      confirmVariant: 'destructive',
    });
    if (!confirmed) return;

    for (const rule of debugMatches) {
      await deleteMapping.mutateAsync(rule.id);
    }
  };

  const handleUpdateRule = async (rule: ModelMapping, data: Partial<ModelMappingInput>) => {
    await updateMapping.mutateAsync({
      id: rule.id,
      data: {
        pattern: data.pattern ?? rule.pattern,
        target: data.target ?? rule.target,
        scope: 'global',
        clientType: data.clientType ?? rule.clientType,
        providerType: data.providerType ?? rule.providerType,
        providerID: data.providerID ?? rule.providerID,
        projectID: data.projectID ?? rule.projectID,
        priority: rule.priority,
        isEnabled: rule.isEnabled,
      },
    });
  };

  const handleReset = async () => {
    const confirmed = await confirm({
      title: t('common.confirm'),
      description: t('modelMappings.confirmReset'),
      confirmText: t('common.reset'),
      confirmVariant: 'destructive',
    });
    if (!confirmed) return;

    await resetToDefaults.mutateAsync();
  };

  const handleClearAll = async () => {
    const confirmed = await confirm({
      title: t('common.confirm'),
      description: t('modelMappings.confirmClearAll'),
      confirmText: t('modelMappings.clearAll'),
      confirmVariant: 'destructive',
    });
    if (!confirmed) return;

    await clearAllMappings.mutateAsync();
  };

  if (isLoading) return null;

  const isPending =
    createMapping.isPending ||
    updateMapping.isPending ||
    reorderMappings.isPending ||
    deleteMapping.isPending ||
    resetToDefaults.isPending ||
    clearAllMappings.isPending;

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Zap}
        iconClassName="text-yellow-500"
        title={t('modelMappings.title')}
        description={t('modelMappings.description', { count: rules.length })}
        actions={
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={handleReset} disabled={isPending}>
              <RotateCcw className="h-4 w-4 mr-1" />
              {t('modelMappings.resetToPreset')}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={handleClearAll}
              disabled={isPending || rules.length === 0}
            >
              <Trash2 className="h-4 w-4 mr-1" />
              {t('modelMappings.clearAll')}
            </Button>
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto p-6">
        <div className="space-y-4">
          {debuggerEnabled && (
            <Card className="border-border bg-card">
              <CardContent className="p-6 space-y-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2 font-medium">
                      <Search className="h-4 w-4 text-primary" />
                      {t('modelMappings.debuggerTitle')}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {t('modelMappings.debuggerDesc')}
                    </p>
                  </div>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={handleRemoveAllDebugMatches}
                    disabled={!debugQuery || debugMatches.length === 0 || isPending}
                  >
                    <Trash2 className="h-4 w-4 mr-1" />
                    {t('modelMappings.debuggerDeleteAll')}
                  </Button>
                </div>

                <ModelInput
                  value={debugModel}
                  onChange={setDebugModel}
                  placeholder={t('modelMappings.debuggerPlaceholder')}
                  disabled={isPending}
                  className="h-9 text-sm"
                />

                {debugQuery && (
                  <div className="rounded-md border border-border bg-muted/30 p-3 space-y-3">
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span>
                        {t('modelMappings.debuggerMatched', { count: debugMatches.length })}
                      </span>
                      <span>·</span>
                      <span>
                        {t('modelMappings.debuggerEffectiveMatched', {
                          count: effectiveDebugMatches.length,
                        })}
                      </span>
                    </div>

                    {debugMatches.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        {t('modelMappings.debuggerNoMatches')}
                      </p>
                    ) : (
                      <div className="space-y-2">
                        {debugMatches.map((rule) => {
                          const effective = matchWildcard(rule.pattern, debugQuery);
                          const hiddenFromGlobalList = rule.scope && rule.scope !== 'global';
                          return (
                            <div
                              key={`debug-rule-${rule.id}`}
                              className="rounded-md border border-border bg-background p-3 space-y-2"
                            >
                              <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0 space-y-1">
                                  <div className="flex flex-wrap items-center gap-2 text-sm">
                                    <span className="font-mono">#{rule.id}</span>
                                    <span className="font-mono break-all">{rule.pattern}</span>
                                    <ArrowRight className="h-3 w-3 text-muted-foreground" />
                                    <span className="font-mono break-all">{rule.target}</span>
                                  </div>
                                  <p className="text-xs text-muted-foreground">
                                    {describeMappingScope(rule, t)} · priority: {rule.priority}
                                  </p>
                                  <div className="flex flex-wrap gap-2 text-[11px]">
                                    {effective && (
                                      <span className="rounded bg-primary/10 px-2 py-0.5 text-primary">
                                        {t('modelMappings.debuggerEffective')}
                                      </span>
                                    )}
                                    {hiddenFromGlobalList && (
                                      <span className="rounded bg-destructive/10 px-2 py-0.5 text-destructive">
                                        {t('modelMappings.debuggerHidden')}
                                      </span>
                                    )}
                                  </div>
                                </div>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => handleRemoveDebugRule(rule)}
                                  disabled={isPending}
                                  className="shrink-0"
                                >
                                  <Trash2 className="h-4 w-4 text-destructive" />
                                </Button>
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          <Card className="border-border bg-card">
            <CardContent className="p-6 space-y-4">
              <p className="text-xs text-muted-foreground">{t('modelMappings.pageDesc')}</p>

              {/* Header row */}
              <div className="flex items-center gap-3 text-xs text-muted-foreground font-medium border-b pb-2">
                <div className="w-6 shrink-0"></div>
                <div className="w-6 shrink-0">#</div>
                <div className="flex-1 min-w-0">{t('modelMappings.matchPattern')}</div>
                <div className="w-3"></div>
                <div className="flex-1 min-w-0">{t('modelMappings.targetModel')}</div>
                <div className="w-[100px] shrink-0">{t('modelMappings.client')}</div>
                <div className="w-[110px] shrink-0">{t('modelMappings.providerType')}</div>
                <div className="w-[70px] shrink-0">{t('modelMappings.providerID')}</div>
                <div className="w-[70px] shrink-0">{t('modelMappings.projectID')}</div>
                <div className="w-8 shrink-0"></div>
              </div>

              {rules.length > 0 && (
                <DndContext
                  sensors={sensors}
                  collisionDetection={closestCenter}
                  onDragStart={handleDragStart}
                  onDragEnd={handleDragEnd}
                >
                  <SortableContext
                    items={rules.map((r) => `rule-${r.id}`)}
                    strategy={verticalListSortingStrategy}
                  >
                    <div className="space-y-0">
                      {rules.map((rule, index) => (
                        <SortableRuleItem
                          key={`rule-${rule.id}`}
                          id={`rule-${rule.id}`}
                          index={index}
                          rule={rule}
                          onRemove={() => handleRemoveRule(rule.id)}
                          onUpdate={(data) => handleUpdateRule(rule, data)}
                          disabled={isPending}
                        />
                      ))}
                    </div>
                  </SortableContext>
                </DndContext>
              )}

              {rules.length === 0 && (
                <div className="text-center py-8">
                  <p className="text-muted-foreground">{t('modelMappings.noMappings')}</p>
                  <p className="text-xs text-muted-foreground mt-1">
                    {t('modelMappings.noMappingsHint')}
                  </p>
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
                <Select
                  value={newClientType || '_all'}
                  onValueChange={(v) => setNewClientType(v === '_all' ? '' : (v ?? ''))}
                  disabled={isPending}
                >
                  <SelectTrigger className="w-[100px] h-8 text-xs shrink-0">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="_all">{t('modelMappings.allClients')}</SelectItem>
                    <SelectItem value="claude">claude</SelectItem>
                    <SelectItem value="openai">openai</SelectItem>
                    <SelectItem value="gemini">gemini</SelectItem>
                    <SelectItem value="codex">codex</SelectItem>
                  </SelectContent>
                </Select>
                <Select
                  value={newProviderType || '_all'}
                  onValueChange={(v) => setNewProviderType(v === '_all' ? '' : (v ?? ''))}
                  disabled={isPending}
                >
                  <SelectTrigger className="w-[110px] h-8 text-xs shrink-0">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="_all">{t('modelMappings.allProviderTypes')}</SelectItem>
                    <SelectItem value="antigravity">antigravity</SelectItem>
                    <SelectItem value="kiro">kiro</SelectItem>
                    <SelectItem value="custom">custom</SelectItem>
                  </SelectContent>
                </Select>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleAddRule}
                  disabled={!newPattern.trim() || !newTarget.trim() || isPending}
                >
                  <Plus className="h-4 w-4 mr-1" />
                  {t('common.add')}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

export default ModelMappingsPage;
