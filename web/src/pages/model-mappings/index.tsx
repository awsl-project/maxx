import { useState } from 'react';
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
  useDeleteModelMapping,
  useClearAllModelMappings,
  useResetModelMappingsToDefaults,
} from '@/hooks/queries';
import type { ModelMapping, ModelMappingInput } from '@/lib/transport/types';
import { Zap, Plus, Trash2, ArrowRight, RotateCcw, GripVertical } from 'lucide-react';

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
  const { data: mappings, isLoading } = useModelMappings();
  const createMapping = useCreateModelMapping();
  const updateMapping = useUpdateModelMapping();
  const deleteMapping = useDeleteModelMapping();
  const clearAllMappings = useClearAllModelMappings();
  const resetToDefaults = useResetModelMappingsToDefaults();
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');
  const [newClientType, setNewClientType] = useState('claude');
  const [newProviderType, setNewProviderType] = useState('antigravity');

  // Filter only global scope mappings
  const rules = (mappings || []).filter((m) => !m.scope || m.scope === 'global');

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = rules.findIndex((r) => `rule-${r.id}` === active.id);
    const newIndex = rules.findIndex((r) => `rule-${r.id}` === over.id);

    if (oldIndex !== -1 && newIndex !== -1) {
      const reordered = arrayMove(rules, oldIndex, newIndex);
      for (let i = 0; i < reordered.length; i++) {
        const rule = reordered[i];
        if (rule.priority !== i * 10) {
          await updateMapping.mutateAsync({
            id: rule.id,
            data: {
              pattern: rule.pattern,
              target: rule.target,
              scope: 'global',
              priority: i * 10,
              isEnabled: rule.isEnabled,
            },
          });
        }
      }
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
    if (!window.confirm(t('modelMappings.confirmReset'))) return;
    await resetToDefaults.mutateAsync();
  };

  const handleClearAll = async () => {
    if (!window.confirm(t('modelMappings.confirmClearAll'))) return;
    await clearAllMappings.mutateAsync();
  };

  if (isLoading) return null;

  const isPending =
    createMapping.isPending ||
    updateMapping.isPending ||
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
  );
}

export default ModelMappingsPage;
