import { useEffect, useMemo, useRef, useState } from 'react';
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
  ProviderModelCheckResponse,
} from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import { Button } from '@/components/ui/button';
import { ModelInput } from '@/components/ui/model-input';
import { Progress } from '@/components/ui/progress';
import { Switch } from '@/components/ui/switch';

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
  mappedModels: string[] | undefined,
  label: string,
) {
  const modelIDs = new Set<string>();
  const options = [];
  for (const model of [
    ...(providerModels || []),
    ...(configuredModels || []),
    ...(mappedModels || []),
  ]) {
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

type ModelAvailabilityScanResult = {
  model: string;
  successCount: number;
  errorCount: number;
  durationMs: number;
};

type ModelAvailabilityScanState = {
  status: 'idle' | 'running' | 'done' | 'cancelled';
  checked: number;
  total: number;
  currentModel: string;
  available: ModelAvailabilityScanResult[];
};

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
  const enabled = mapping.isEnabled !== false;

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
      <Switch
        checked={enabled}
        onCheckedChange={(checked) => onUpdate(mapping, { isEnabled: checked })}
        disabled={disabled}
        aria-label={t('modelMappings.toggleRule', {
          pattern: mapping.pattern || '*',
          target: mapping.target || '-',
        })}
      />
      <ModelInput
        value={mapping.pattern}
        onChange={(pattern) => onUpdate(mapping, { pattern })}
        placeholder={t('modelMappings.matchPattern')}
        disabled={disabled}
        className={`flex-1 min-w-0 h-8 text-sm ${!enabled ? 'opacity-50' : ''}`}
      />
      <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
      <ModelInput
        value={mapping.target}
        onChange={(target) => onUpdate(mapping, { target })}
        placeholder={t('modelMappings.targetModel')}
        disabled={disabled}
        extraModels={extraModels}
        openSearchValue=""
        className={`flex-1 min-w-0 h-8 text-sm ${!enabled ? 'opacity-50' : ''}`}
      />
      <span className={`w-[72px] text-[11px] shrink-0 ${enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground'}`}>
        {enabled ? t('common.enabled') : t('common.disabled')}
      </span>
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
  const [checkIterations, setCheckIterations] = useState(10);
  const [checkSuccessThreshold, setCheckSuccessThreshold] = useState(5);
  const [checkConcurrency, setCheckConcurrency] = useState(2);
  const [checkError, setCheckError] = useState<string | null>(null);
  const [checkElapsedSeconds, setCheckElapsedSeconds] = useState(0);
  const modelCheckAbortRef = useRef<AbortController | null>(null);
  const [availabilityScan, setAvailabilityScan] = useState<ModelAvailabilityScanState>({
    status: 'idle',
    checked: 0,
    total: 0,
    currentModel: '',
    available: [],
  });

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
        providerMappings.map((mapping) => mapping.target),
        t('modelInput.currentProviderModels'),
      ),
    [provider.supportModels, providerMappings, runtimeModels?.models, t],
  );

  const availableCheckModels = useMemo(
    () =>
      buildProviderRuntimeModelOptions(
        runtimeModels?.available ? runtimeModels.models : [],
        undefined,
        undefined,
        t('modelInput.currentProviderModels'),
      ),
    [runtimeModels?.available, runtimeModels?.models, t],
  );
  const sanitizedCheckIterations = Math.max(10, Math.min(500, checkIterations || 10));
  const sanitizedCheckSuccessThreshold = Math.max(
    1,
    Math.min(sanitizedCheckIterations, checkSuccessThreshold || 5),
  );
  const sanitizedCheckConcurrency = Math.max(1, Math.min(10, checkConcurrency || 4));
  const estimatedCheckSeconds = Math.max(
    8,
    Math.ceil(
      (availableCheckModels.length * sanitizedCheckIterations) / sanitizedCheckConcurrency,
    ) * 2,
  );
  const isModelCheckRunning = availabilityScan.status === 'running';
  const checkProgressValue = isModelCheckRunning
    ? availabilityScan.total > 0
      ? Math.min(95, Math.round((availabilityScan.checked / availabilityScan.total) * 100))
      : Math.min(95, Math.max(5, Math.round((checkElapsedSeconds / estimatedCheckSeconds) * 90)))
    : availabilityScan.status === 'done' || availabilityScan.status === 'cancelled'
      ? 100
      : 0;
  const canRunModelCheck =
    (provider.type === 'custom' || provider.type === 'newapi') &&
    runtimeModels?.available === true &&
    availableCheckModels.length > 0;

  useEffect(() => {
    if (!isModelCheckRunning) return;
    setCheckElapsedSeconds(0);
    const timer = window.setInterval(() => {
      setCheckElapsedSeconds((seconds) => seconds + 1);
    }, 1000);
    return () => window.clearInterval(timer);
  }, [isModelCheckRunning]);

  const handleRunModelCheck = async () => {
    if (!canRunModelCheck || isModelCheckRunning) return;
    modelCheckAbortRef.current?.abort();
    const controller = new AbortController();
    modelCheckAbortRef.current = controller;
    setCheckError(null);
    const candidates = availableCheckModels.map((model) => model.id);
    setAvailabilityScan({
      status: 'running',
      checked: 0,
      total: candidates.length,
      currentModel: candidates[0] ?? '',
      available: [],
    });

    try {
      const successfulModels: ModelAvailabilityScanResult[] = [];
      for (let index = 0; index < candidates.length; index++) {
        const model = candidates[index];
        if (controller.signal.aborted) break;
        setAvailabilityScan((prev) => ({ ...prev, currentModel: model }));
        const result: ProviderModelCheckResponse = await getTransport().checkProviderModel(
          provider.id,
          {
            clientType: 'openai',
            model,
            iterations: sanitizedCheckIterations,
            concurrency: sanitizedCheckConcurrency,
          },
          controller.signal,
        );
        if (result.successCount >= sanitizedCheckSuccessThreshold) {
          successfulModels.push({
            model,
            successCount: result.successCount,
            errorCount: result.errorCount,
            durationMs: result.durationMs,
          });
        }
        setAvailabilityScan({
          status: 'running',
          checked: index + 1,
          total: candidates.length,
          currentModel: candidates[index + 1] ?? '',
          available: successfulModels,
        });
      }
      setAvailabilityScan((prev) => ({
        ...prev,
        status: controller.signal.aborted ? 'cancelled' : 'done',
        currentModel: '',
      }));
    } catch (error) {
      setAvailabilityScan((prev) => ({
        ...prev,
        status: controller.signal.aborted ? 'cancelled' : prev.status,
        currentModel: '',
      }));
      setCheckError(
        controller.signal.aborted
          ? '已取消扫描'
          : error instanceof Error
            ? error.message
            : '可用模型扫描失败',
      );
    } finally {
      if (modelCheckAbortRef.current === controller) {
        modelCheckAbortRef.current = null;
      }
    }
  };

  const handleCancelModelCheck = () => {
    modelCheckAbortRef.current?.abort();
    setCheckError('正在取消扫描...');
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
          isEnabled: data.isEnabled ?? mapping.isEnabled,
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
          <div className="mb-4 rounded-lg border border-border bg-muted/30 p-2.5 space-y-2">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="shrink-0 text-sm font-medium text-foreground">扫描真实可用模型</span>
              <select
                value="runtime"
                disabled
                className="h-8 min-w-52 rounded-md border border-input bg-background px-2 text-sm text-foreground disabled:opacity-80"
                title="先从 provider 配置获取候选模型，再逐个真实请求；成功达到阈值才显示"
              >
                <option value="runtime">
                  provider 配置 · {availableCheckModels.length} 个候选
                </option>
              </select>
              <label className="flex items-center gap-1">
                <span>每模型</span>
                <input
                  type="number"
                  min={10}
                  max={500}
                  value={checkIterations}
                  onChange={(event) => setCheckIterations(Number(event.target.value))}
                  disabled={isModelCheckRunning}
                  className="h-8 w-20 rounded-md border border-input bg-background px-2 text-sm text-foreground disabled:opacity-60"
                  title="每个候选模型真实请求次数"
                />
              </label>
              <label className="flex items-center gap-1">
                <span>阈值</span>
                <input
                  type="number"
                  min={1}
                  max={sanitizedCheckIterations}
                  value={checkSuccessThreshold}
                  onChange={(event) => setCheckSuccessThreshold(Number(event.target.value))}
                  disabled={isModelCheckRunning}
                  className="h-8 w-16 rounded-md border border-input bg-background px-2 text-sm text-foreground disabled:opacity-60"
                  title="达到该成功次数才显示为可用"
                />
              </label>
              <label className="flex items-center gap-1">
                <span>并发</span>
                <input
                  type="number"
                  min={1}
                  max={10}
                  value={checkConcurrency}
                  onChange={(event) => setCheckConcurrency(Number(event.target.value))}
                  disabled={isModelCheckRunning}
                  className="h-8 w-16 rounded-md border border-input bg-background px-2 text-sm text-foreground disabled:opacity-60"
                  title="单个模型内部请求并发"
                />
              </label>
              <span className="shrink-0 text-muted-foreground">
                ≥{sanitizedCheckSuccessThreshold}/{sanitizedCheckIterations} 显示
              </span>
              {(isModelCheckRunning || availabilityScan.status !== 'idle') && (
                <div className="flex min-w-44 flex-1 items-center gap-2">
                  <Progress value={checkProgressValue} />
                  <span className="shrink-0">
                    {availabilityScan.checked}/{availabilityScan.total} · 可用{' '}
                    {availabilityScan.available.length}
                  </span>
                  {isModelCheckRunning && availabilityScan.currentModel && (
                    <span
                      className="max-w-48 truncate font-mono"
                      title={availabilityScan.currentModel}
                    >
                      {availabilityScan.currentModel}
                    </span>
                  )}
                </div>
              )}
              <div className="ml-auto flex items-center gap-2">
                {isModelCheckRunning && (
                  <Button size="sm" variant="outline" onClick={handleCancelModelCheck}>
                    取消
                  </Button>
                )}
                <Button
                  size="sm"
                  onClick={handleRunModelCheck}
                  disabled={!canRunModelCheck || isModelCheckRunning}
                >
                  {isModelCheckRunning ? '扫描中...' : '扫描可用模型'}
                </Button>
              </div>
              {runtimeModels && !runtimeModels.available && runtimeModels.error && (
                <div className="basis-full text-xs text-amber-600">
                  获取模型列表失败：{runtimeModels.error}
                </div>
              )}
            </div>
            {checkError && <div className="text-xs text-destructive">{checkError}</div>}
            {availabilityScan.available.length > 0 && (
              <div className="flex flex-wrap gap-2 text-xs">
                {availabilityScan.available.map((result) => (
                  <span
                    key={result.model}
                    className="rounded-full border border-green-500/30 bg-green-500/10 px-2 py-1 font-mono text-green-700 dark:text-green-300"
                  >
                    {result.model} · {result.successCount}/{sanitizedCheckIterations}
                  </span>
                ))}
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
