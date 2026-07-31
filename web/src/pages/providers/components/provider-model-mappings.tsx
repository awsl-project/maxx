import { useMemo, useState } from 'react';
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
import { ArrowRight, GripVertical, Plus, Trash2, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  useModelMappings,
  useProviderRuntimeModels,
  useProviderRuntimeModelsPreview,
  useProviderModelCheck,
  useCreateModelMapping,
  useUpdateModelMapping,
  useReorderModelMappings,
  useDeleteModelMapping,
} from '@/hooks/queries';
import type {
  Provider,
  ModelMapping,
  ModelMappingInput,
  ProviderRuntimeModelsPreviewRequest,
  ProviderModelCheckBaseline,
} from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { ModelInput } from '@/components/ui/model-input';

/**
 * Provider-scoped model mappings editor backed by the ModelMapping entity API
 * (scope='provider', providerID). This is the mechanism executor.mapModel
 * actually consults at request time — it matches by provider id and type — so
 * it is the correct home for model mapping across every provider type, not the
 * inline config maps. Shared by the custom edit form and the OpenRouter view.
 */
export function buildProviderRuntimeModelOptions(
  providerModels: string[] | undefined,
  configuredModels: string[] | undefined,
  label: string,
) {
  const modelIDs = new Set<string>();
  const options = [];
  for (const model of [...(providerModels || []), ...(configuredModels || [])]) {
    const id = model.trim();
    if (!id || modelIDs.has(id)) continue;
    modelIDs.add(id);
    options.push({
      id,
      name: id,
      provider: label,
    });
  }
  return options;
}

function parseProviderModelCheckBaselines(raw: string): ProviderModelCheckBaseline[] {
  const text = raw.trim();
  if (!text) return [];
  const parsed = JSON.parse(text) as unknown;
  const list = Array.isArray(parsed)
    ? parsed
    : typeof parsed === 'object' &&
        parsed !== null &&
        Array.isArray((parsed as { baselines?: unknown }).baselines)
      ? (parsed as { baselines: unknown[] }).baselines
      : [];
  return list.filter((item): item is ProviderModelCheckBaseline => {
    if (typeof item !== 'object' || item === null) return false;
    const candidate = item as Partial<ProviderModelCheckBaseline>;
    return (
      typeof candidate.name === 'string' &&
      typeof candidate.model === 'string' &&
      Array.isArray(candidate.distribution) &&
      candidate.distribution.length === 355 &&
      typeof candidate.stats === 'object' &&
      candidate.stats !== null &&
      typeof candidate.stats.mode === 'number'
    );
  });
}

function formatModelCheckScore(value: number | undefined) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-';
  return `${(value * 100).toFixed(1)}%`;
}

interface SortableProviderMappingRowProps {
  mapping: ModelMapping;
  index: number;
  disabled: boolean;
  extraModels: ReturnType<typeof buildProviderRuntimeModelOptions>;
  onUpdate: (mapping: ModelMapping, data: Partial<ModelMappingInput>) => void;
  onDelete: (id: number) => void;
}

function SortableProviderMappingRow({
  mapping,
  index,
  disabled,
  extraModels,
  onUpdate,
  onDelete,
}: SortableProviderMappingRowProps) {
  const { t } = useTranslation();
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: `mapping-${mapping.id}`,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`flex items-center gap-2 ${isDragging ? 'opacity-50' : ''}`}
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
      <ModelInput
        value={mapping.pattern}
        onChange={(pattern) => onUpdate(mapping, { pattern })}
        placeholder={t('modelMappings.matchPattern')}
        disabled={disabled}
        className="flex-1 min-w-0 h-8 text-sm"
      />
      <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
      <ModelInput
        value={mapping.target}
        onChange={(target) => onUpdate(mapping, { target })}
        placeholder={t('modelMappings.targetModel')}
        disabled={disabled}
        extraModels={extraModels}
        openSearchValue=""
        className="flex-1 min-w-0 h-8 text-sm"
      />
      <Button variant="ghost" size="sm" onClick={() => onDelete(mapping.id)} disabled={disabled}>
        <Trash2 className="h-4 w-4 text-destructive" />
      </Button>
    </div>
  );
}

export function ProviderModelMappings({
  provider,
  runtimeModelsPreview,
}: {
  provider: Provider;
  runtimeModelsPreview?: ProviderRuntimeModelsPreviewRequest;
}) {
  const { t } = useTranslation();
  const { data: allMappings } = useModelMappings();
  const createMapping = useCreateModelMapping();
  const updateMapping = useUpdateModelMapping();
  const reorderMappings = useReorderModelMappings();
  const deleteMapping = useDeleteModelMapping();
  const hasPreviewConfig = !!runtimeModelsPreview;
  const { data: savedRuntimeModels } = useProviderRuntimeModels(provider.id, !hasPreviewConfig);
  const { data: previewRuntimeModels } = useProviderRuntimeModelsPreview(
    runtimeModelsPreview,
    hasPreviewConfig,
  );
  const runtimeModels = previewRuntimeModels ?? savedRuntimeModels;
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');
  const [checkModel, setCheckModel] = useState('');
  const [checkIterations, setCheckIterations] = useState(50);
  const [checkConcurrency, setCheckConcurrency] = useState(4);
  const [checkBaselinesText, setCheckBaselinesText] = useState('');
  const [checkError, setCheckError] = useState<string | null>(null);
  const modelCheck = useProviderModelCheck(provider.id);

  // Filter mappings for this provider
  const providerMappings = useMemo(() => {
    return (allMappings || []).filter(
      (m) => m.scope === 'provider' && m.providerID === provider.id,
    );
  }, [allMappings, provider.id]);

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const isPending =
    createMapping.isPending ||
    updateMapping.isPending ||
    reorderMappings.isPending ||
    deleteMapping.isPending;
  const providerRuntimeModelOptions = useMemo(
    () =>
      buildProviderRuntimeModelOptions(
        runtimeModels?.models,
        provider.supportModels,
        t('modelInput.currentProviderModels'),
      ),
    [provider.supportModels, runtimeModels?.models, t],
  );

  const canRunModelCheck =
    (provider.type === 'custom' || provider.type === 'newapi') && checkModel.trim().length > 0;

  const handleRunModelCheck = async () => {
    if (!canRunModelCheck || modelCheck.isPending) return;
    setCheckError(null);
    try {
      await modelCheck.mutateAsync({
        clientType: 'openai',
        model: checkModel.trim(),
        iterations: Math.max(40, Math.min(500, checkIterations || 50)),
        concurrency: Math.max(1, Math.min(10, checkConcurrency || 4)),
        baselines: parseProviderModelCheckBaselines(checkBaselinesText),
      });
    } catch (error) {
      setCheckError(error instanceof Error ? error.message : '模型检验失败');
    }
  };

  const handleDragStart = () => {
    document.body.classList.add('is-dragging');
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    document.body.classList.remove('is-dragging');
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = providerMappings.findIndex((m) => `mapping-${m.id}` === active.id);
    const newIndex = providerMappings.findIndex((m) => `mapping-${m.id}` === over.id);
    if (oldIndex === -1 || newIndex === -1) return;

    const reordered = arrayMove(providerMappings, oldIndex, newIndex);
    await reorderMappings.mutateAsync({
      scope: 'provider',
      providerID: provider.id,
      orderedIDs: reordered.map((mapping) => mapping.id),
    });
  };

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

        {(provider.type === 'custom' || provider.type === 'newapi') && (
          <div className="mb-4 rounded-lg border border-border bg-muted/30 p-3 space-y-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-sm font-medium text-foreground">模型检验</div>
                <div className="text-xs text-muted-foreground">
                  对当前自定义提供商发起随机数指纹测试；结果只作概率参考，不自动改映射。
                </div>
              </div>
              <Button
                size="sm"
                onClick={handleRunModelCheck}
                disabled={!canRunModelCheck || modelCheck.isPending}
              >
                {modelCheck.isPending ? '检验中...' : '开始检验'}
              </Button>
            </div>
            <div className="grid gap-2 md:grid-cols-[1fr_96px_96px]">
              <ModelInput
                value={checkModel}
                onChange={setCheckModel}
                placeholder="要检验的上游模型，例如 gpt-4o"
                extraModels={providerRuntimeModelOptions}
                className="h-8 text-sm"
              />
              <input
                type="number"
                min={40}
                max={500}
                value={checkIterations}
                onChange={(event) => setCheckIterations(Number(event.target.value))}
                className="h-8 rounded-md border border-input bg-background px-2 text-sm"
                title="测试次数"
              />
              <input
                type="number"
                min={1}
                max={10}
                value={checkConcurrency}
                onChange={(event) => setCheckConcurrency(Number(event.target.value))}
                className="h-8 rounded-md border border-input bg-background px-2 text-sm"
                title="并发"
              />
            </div>
            <textarea
              value={checkBaselinesText}
              onChange={(event) => setCheckBaselinesText(event.target.value)}
              placeholder="可选：粘贴 hlwy-ai-checker 导出的 baseline JSON，用于匹配排名"
              className="min-h-20 w-full rounded-md border border-input bg-background px-3 py-2 text-xs"
            />
            {checkError && <div className="text-xs text-destructive">{checkError}</div>}
            {modelCheck.data && (
              <div className="rounded-md border border-border bg-background p-3 text-xs space-y-2">
                <div className="flex flex-wrap gap-x-4 gap-y-1 text-muted-foreground">
                  <span>成功 {modelCheck.data.successCount}</span>
                  <span>失败 {modelCheck.data.errorCount}</span>
                  <span>有效样本 {modelCheck.data.validCount}</span>
                  <span>耗时 {(modelCheck.data.durationMs / 1000).toFixed(1)}s</span>
                  <span className={modelCheck.data.reliable ? 'text-green-600' : 'text-amber-600'}>
                    {modelCheck.data.reliable ? '样本可靠' : '样本不足，仅供参考'}
                  </span>
                </div>
                <div className="flex flex-wrap gap-x-4 gap-y-1">
                  <span>众数：{modelCheck.data.stats.mode || '-'}</span>
                  <span>
                    均值：{modelCheck.data.stats.mean ? modelCheck.data.stats.mean.toFixed(1) : '-'}
                  </span>
                  <span>唯一值：{modelCheck.data.stats.unique || '-'}</span>
                </div>
                {modelCheck.data.matches && modelCheck.data.matches.length > 0 && (
                  <div className="space-y-1">
                    <div className="font-medium">匹配排名</div>
                    {modelCheck.data.matches.slice(0, 3).map((match, index) => (
                      <div
                        key={`${match.baseline.name}-${index}`}
                        className="flex flex-wrap gap-x-3 text-muted-foreground"
                      >
                        <span>
                          #{index + 1} {match.baseline.name}
                        </span>
                        <span>{match.baseline.model}</span>
                        <span>综合 {formatModelCheckScore(match.overallScore)}</span>
                        <span>众数{match.modeMatch ? '匹配' : '不匹配'}</span>
                      </div>
                    ))}
                  </div>
                )}
                {modelCheck.data.errors && modelCheck.data.errors.length > 0 && (
                  <div className="text-muted-foreground">错误样例：{modelCheck.data.errors[0]}</div>
                )}
              </div>
            )}
          </div>
        )}

        {providerMappings.length > 0 && (
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragStart={handleDragStart}
            onDragEnd={handleDragEnd}
          >
            <SortableContext
              items={providerMappings.map((mapping) => `mapping-${mapping.id}`)}
              strategy={verticalListSortingStrategy}
            >
              <div className="space-y-2 mb-4">
                {providerMappings.map((mapping, index) => (
                  <SortableProviderMappingRow
                    key={mapping.id}
                    mapping={mapping}
                    index={index}
                    disabled={isPending}
                    extraModels={providerRuntimeModelOptions}
                    onUpdate={handleUpdateMapping}
                    onDelete={handleDeleteMapping}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
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
