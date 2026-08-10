import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  CardContent,
  Input,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { useAuth } from '@/lib/auth-context';
import {
  useModelPrices,
  useCreateModelPrice,
  useUpdateModelPrice,
  useDeleteModelPrice,
  useResetModelPricesToDefaults,
  useFetchExternalModelPrices,
} from '@/hooks/queries';
import type {
  ModelPrice,
  ModelPriceChange,
  ModelPriceInput,
  UpstreamModelPricesResult,
} from '@/lib/transport/types';
import { DollarSign, Plus, Trash2, Pencil, RotateCcw } from 'lucide-react';
import { UpstreamPricesDialog } from './upstream-prices-dialog';

// Helper to format micro USD price to display format (e.g., $3.00 / M tokens)
function formatMicroPrice(microUsd: number): string {
  return `$${(microUsd / 1_000_000).toFixed(2)}`;
}

// Helper to parse display price to micro USD
function parsePriceToMicro(priceStr: string): number {
  const value = parseFloat(priceStr);
  if (isNaN(value) || value < 0) return 0;
  return Math.round(value * 1_000_000);
}

interface PriceFormData {
  modelId: string;
  inputPrice: string;
  outputPrice: string;
  cacheReadPrice: string;
  cache5mWritePrice: string;
  cache1hWritePrice: string;
  imageInputPrice: string;
  imageOutputPrice: string;
  has1mContext: boolean;
  context1mThreshold: string;
  inputPremiumNum: string;
  inputPremiumDenom: string;
  outputPremiumNum: string;
  outputPremiumDenom: string;
}

const modelPriceUpstreamSources = [{ value: 'litellm', label: 'LiteLLM' }] as const;

const defaultFormData: PriceFormData = {
  modelId: '',
  inputPrice: '3.00',
  outputPrice: '15.00',
  cacheReadPrice: '0.30',
  cache5mWritePrice: '3.75',
  cache1hWritePrice: '6.00',
  imageInputPrice: '0.00',
  imageOutputPrice: '0.00',
  has1mContext: false,
  context1mThreshold: '200000',
  inputPremiumNum: '2',
  inputPremiumDenom: '1',
  outputPremiumNum: '2',
  outputPremiumDenom: '1',
};

function priceToFormData(price: ModelPrice): PriceFormData {
  return {
    modelId: price.modelId,
    inputPrice: (price.inputPriceMicro / 1_000_000).toFixed(2),
    outputPrice: (price.outputPriceMicro / 1_000_000).toFixed(2),
    cacheReadPrice: (price.cacheReadPriceMicro / 1_000_000).toFixed(2),
    cache5mWritePrice: (price.cache5mWritePriceMicro / 1_000_000).toFixed(2),
    cache1hWritePrice: (price.cache1hWritePriceMicro / 1_000_000).toFixed(2),
    imageInputPrice: ((price.imageInputPriceMicro || 0) / 1_000_000).toFixed(2),
    imageOutputPrice: ((price.imageOutputPriceMicro || 0) / 1_000_000).toFixed(2),
    has1mContext: price.has1mContext,
    context1mThreshold: price.context1mThreshold.toString(),
    inputPremiumNum: price.inputPremiumNum.toString(),
    inputPremiumDenom: price.inputPremiumDenom.toString(),
    outputPremiumNum: price.outputPremiumNum.toString(),
    outputPremiumDenom: price.outputPremiumDenom.toString(),
  };
}

function modelPriceToInput(price: ModelPrice): ModelPriceInput {
  return {
    modelId: price.modelId,
    inputPriceMicro: price.inputPriceMicro,
    outputPriceMicro: price.outputPriceMicro,
    cacheReadPriceMicro: price.cacheReadPriceMicro,
    cache5mWritePriceMicro: price.cache5mWritePriceMicro,
    cache1hWritePriceMicro: price.cache1hWritePriceMicro,
    imageInputPriceMicro: price.imageInputPriceMicro,
    imageOutputPriceMicro: price.imageOutputPriceMicro,
    has1mContext: price.has1mContext,
    context1mThreshold: price.context1mThreshold,
    inputPremiumNum: price.inputPremiumNum,
    inputPremiumDenom: price.inputPremiumDenom,
    outputPremiumNum: price.outputPremiumNum,
    outputPremiumDenom: price.outputPremiumDenom,
  };
}

function modelPricesEqual(a: ModelPrice, b: ModelPrice): boolean {
  return (
    a.inputPriceMicro === b.inputPriceMicro &&
    a.outputPriceMicro === b.outputPriceMicro &&
    a.cacheReadPriceMicro === b.cacheReadPriceMicro &&
    a.cache5mWritePriceMicro === b.cache5mWritePriceMicro &&
    a.cache1hWritePriceMicro === b.cache1hWritePriceMicro &&
    a.imageInputPriceMicro === b.imageInputPriceMicro &&
    a.imageOutputPriceMicro === b.imageOutputPriceMicro &&
    a.has1mContext === b.has1mContext &&
    a.context1mThreshold === b.context1mThreshold &&
    a.inputPremiumNum === b.inputPremiumNum &&
    a.inputPremiumDenom === b.inputPremiumDenom &&
    a.outputPremiumNum === b.outputPremiumNum &&
    a.outputPremiumDenom === b.outputPremiumDenom
  );
}

function buildUpstreamChanges(
  upstreamPrices: ModelPrice[],
  currentPrices: ModelPrice[],
): ModelPriceChange[] {
  const currentByModelId = new Map(currentPrices.map((price) => [price.modelId, price]));

  return upstreamPrices.flatMap<ModelPriceChange>((price) => {
    const current = currentByModelId.get(price.modelId);
    if (!current) {
      return [{ action: 'create', after: price }];
    }
    const after = { ...price, id: current.id };
    if (modelPricesEqual(current, after)) {
      return [];
    }
    return [{ action: 'update', before: current, after }];
  });
}

function formDataToInput(form: PriceFormData): ModelPriceInput {
  return {
    modelId: form.modelId,
    inputPriceMicro: parsePriceToMicro(form.inputPrice),
    outputPriceMicro: parsePriceToMicro(form.outputPrice),
    cacheReadPriceMicro: parsePriceToMicro(form.cacheReadPrice),
    cache5mWritePriceMicro: parsePriceToMicro(form.cache5mWritePrice),
    cache1hWritePriceMicro: parsePriceToMicro(form.cache1hWritePrice),
    imageInputPriceMicro: parsePriceToMicro(form.imageInputPrice),
    imageOutputPriceMicro: parsePriceToMicro(form.imageOutputPrice),
    has1mContext: form.has1mContext,
    context1mThreshold: parseInt(form.context1mThreshold) || 0,
    inputPremiumNum: parseInt(form.inputPremiumNum) || 0,
    inputPremiumDenom: parseInt(form.inputPremiumDenom) || 1,
    outputPremiumNum: parseInt(form.outputPremiumNum) || 0,
    outputPremiumDenom: parseInt(form.outputPremiumDenom) || 1,
  };
}

export function ModelPricesPage() {
  const { t } = useTranslation();
  const { user } = useAuth();
  const { data: prices, isLoading } = useModelPrices();
  const createPrice = useCreateModelPrice();
  const updatePrice = useUpdateModelPrice();
  const deletePrice = useDeleteModelPrice();
  const resetPrices = useResetModelPricesToDefaults();
  const fetchExternalPrices = useFetchExternalModelPrices();
  const canManagePrices = user?.role === 'admin';

  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingPrice, setEditingPrice] = useState<ModelPrice | null>(null);
  const [formData, setFormData] = useState<PriceFormData>(defaultFormData);
  const [deleteConfirmId, setDeleteConfirmId] = useState<number | null>(null);
  const [resetConfirmOpen, setResetConfirmOpen] = useState(false);
  const [upstreamPricesOpen, setUpstreamPricesOpen] = useState(false);
  const [upstreamSource, setUpstreamSource] =
    useState<(typeof modelPriceUpstreamSources)[number]['value']>('litellm');
  const [upstreamPrices, setUpstreamPrices] = useState<UpstreamModelPricesResult | null>(null);
  const [upstreamChanges, setUpstreamChanges] = useState<ModelPriceChange[]>([]);
  const [importResultText, setImportResultText] = useState<string | null>(null);

  const handleOpenCreate = () => {
    if (!canManagePrices) return;
    setEditingPrice(null);
    setFormData(defaultFormData);
    setIsDialogOpen(true);
  };

  const handleOpenEdit = (price: ModelPrice) => {
    if (!canManagePrices) return;
    setEditingPrice(price);
    setFormData(priceToFormData(price));
    setIsDialogOpen(true);
  };

  const handleSave = async () => {
    if (!canManagePrices) return;
    if (!formData.modelId.trim()) return;

    const input = formDataToInput(formData);

    if (editingPrice) {
      await updatePrice.mutateAsync({ id: editingPrice.id, data: input });
    } else {
      await createPrice.mutateAsync(input);
    }

    setIsDialogOpen(false);
  };

  const handleDeleteConfirm = async () => {
    if (!canManagePrices) return;
    if (deleteConfirmId === null) return;
    await deletePrice.mutateAsync(deleteConfirmId);
    setDeleteConfirmId(null);
  };

  const handleResetConfirm = async () => {
    if (!canManagePrices) return;
    await resetPrices.mutateAsync();
    setImportResultText(null);
    setResetConfirmOpen(false);
  };

  const handleFetchExternalPrices = async () => {
    if (!canManagePrices) return;
    const result = await fetchExternalPrices.mutateAsync(upstreamSource);
    const changes = buildUpstreamChanges(result.prices, prices || []);
    setUpstreamPrices(result);
    setUpstreamChanges(changes);
    setUpstreamPricesOpen(true);
  };

  const handleApplyUpstreamPrices = async (selectedChanges: ModelPriceChange[]) => {
    if (!canManagePrices || !upstreamPrices) return;

    let created = 0;
    let updated = 0;
    let failed = 0;
    for (const change of selectedChanges) {
      const input = modelPriceToInput(change.after);
      try {
        if (change.action === 'create') {
          await createPrice.mutateAsync(input);
          created++;
        } else {
          await updatePrice.mutateAsync({ id: change.after.id, data: input });
          updated++;
        }
      } catch {
        failed++;
      }
    }

    setImportResultText(
      t('modelPrices.upstreamImportResult', {
        created,
        updated,
        failed,
        skipped: upstreamPrices.total - selectedChanges.length,
      }),
    );
    setUpstreamPricesOpen(false);
  };

  const isPending =
    createPrice.isPending ||
    updatePrice.isPending ||
    deletePrice.isPending ||
    resetPrices.isPending ||
    fetchExternalPrices.isPending;

  if (isLoading) return null;

  const sortedPrices = [...(prices || [])].sort((a, b) => a.modelId.localeCompare(b.modelId));

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={DollarSign}
        iconClassName="text-green-500"
        title={t('modelPrices.title')}
        description={t('modelPrices.description', { count: prices?.length || 0 })}
        actions={
          canManagePrices ? (
            <div className="flex items-center gap-2">
              <Select
                value={upstreamSource}
                onValueChange={(value) => setUpstreamSource(value as typeof upstreamSource)}
              >
                <SelectTrigger className="w-32 h-8 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {modelPriceUpstreamSources.map((source) => (
                    <SelectItem key={source.value} value={source.value}>
                      {source.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="sm"
                onClick={handleFetchExternalPrices}
                disabled={isPending}
              >
                <RotateCcw className="h-4 w-4 mr-1" />
                {t('modelPrices.fetchUpstreamPrices')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setResetConfirmOpen(true)}
                disabled={isPending}
              >
                <RotateCcw className="h-4 w-4 mr-1" />
                {t('modelPrices.resetToDefaults')}
              </Button>
              <Button variant="default" size="sm" onClick={handleOpenCreate} disabled={isPending}>
                <Plus className="h-4 w-4 mr-1" />
                {t('common.add')}
              </Button>
            </div>
          ) : undefined
        }
      />

      <div className="flex-1 overflow-y-auto p-6">
        <Card className="border-border bg-card">
          <CardContent className="p-6">
            <p className="text-xs text-muted-foreground mb-4">{t('modelPrices.pageDesc')}</p>
            {importResultText && (
              <p className="text-xs text-green-600 dark:text-green-400 mb-4">{importResultText}</p>
            )}

            {/* Header row */}
            <div className="flex items-center gap-3 text-xs text-muted-foreground font-medium border-b pb-2 mb-2">
              <div className="flex-1 min-w-0">{t('modelPrices.modelId')}</div>
              <div className="w-24 text-right">{t('modelPrices.inputPrice')}</div>
              <div className="w-24 text-right">{t('modelPrices.outputPrice')}</div>
              <div className="w-24 text-right">{t('modelPrices.cacheRead')}</div>
              <div className="w-16 text-center">{t('modelPrices.has1mContext')}</div>
              <div className="w-20 shrink-0"></div>
            </div>

            {sortedPrices.length === 0 ? (
              <div className="text-center py-8">
                <p className="text-muted-foreground">{t('modelPrices.noData')}</p>
              </div>
            ) : (
              <div className="space-y-1">
                {sortedPrices.map((price) => (
                  <div
                    key={price.id}
                    className="flex items-center gap-3 py-2 hover:bg-accent/50 rounded px-2 -mx-2"
                  >
                    <div className="flex-1 min-w-0 font-mono text-sm truncate">{price.modelId}</div>
                    <div className="w-24 text-right text-sm font-mono">
                      {formatMicroPrice(price.inputPriceMicro)}
                    </div>
                    <div className="w-24 text-right text-sm font-mono">
                      {formatMicroPrice(price.outputPriceMicro)}
                    </div>
                    <div className="w-24 text-right text-sm font-mono text-muted-foreground">
                      {formatMicroPrice(price.cacheReadPriceMicro)}
                    </div>
                    <div className="w-16 text-center">
                      {price.has1mContext ? (
                        <span className="text-green-500 text-xs">{t('common.yes')}</span>
                      ) : (
                        <span className="text-muted-foreground text-xs">-</span>
                      )}
                    </div>
                    <div className="w-20 shrink-0 flex items-center gap-1 justify-end">
                      {canManagePrices && (
                        <>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleOpenEdit(price)}
                            disabled={isPending}
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setDeleteConfirmId(price.id)}
                            disabled={isPending}
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Create/Edit Dialog */}
      <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editingPrice ? t('modelPrices.editTitle') : t('modelPrices.createTitle')}
            </DialogTitle>
          </DialogHeader>

          <div className="space-y-4 py-4">
            {/* Model ID */}
            <div className="space-y-2">
              <Label>{t('modelPrices.modelId')}</Label>
              <Input
                value={formData.modelId}
                onChange={(e) => setFormData({ ...formData, modelId: e.target.value })}
                placeholder="claude-sonnet-4"
                disabled={!!editingPrice}
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">{t('modelPrices.modelIdHint')}</p>
            </div>

            {/* Prices Grid */}
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>{t('modelPrices.inputPrice')} ($/M)</Label>
                <Input
                  type="number"
                  step="0.01"
                  min="0"
                  value={formData.inputPrice}
                  onChange={(e) => setFormData({ ...formData, inputPrice: e.target.value })}
                  className="font-mono"
                />
              </div>
              <div className="space-y-2">
                <Label>{t('modelPrices.outputPrice')} ($/M)</Label>
                <Input
                  type="number"
                  step="0.01"
                  min="0"
                  value={formData.outputPrice}
                  onChange={(e) => setFormData({ ...formData, outputPrice: e.target.value })}
                  className="font-mono"
                />
              </div>
            </div>

            {/* Cache Prices */}
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label>{t('modelPrices.cacheRead')} ($/M)</Label>
                <Input
                  type="number"
                  step="0.01"
                  min="0"
                  value={formData.cacheReadPrice}
                  onChange={(e) => setFormData({ ...formData, cacheReadPrice: e.target.value })}
                  className="font-mono text-sm"
                />
              </div>
              <div className="space-y-2">
                <Label>{t('modelPrices.cache5mWrite')} ($/M)</Label>
                <Input
                  type="number"
                  step="0.01"
                  min="0"
                  value={formData.cache5mWritePrice}
                  onChange={(e) => setFormData({ ...formData, cache5mWritePrice: e.target.value })}
                  className="font-mono text-sm"
                />
              </div>
              <div className="space-y-2">
                <Label>{t('modelPrices.cache1hWrite')} ($/M)</Label>
                <Input
                  type="number"
                  step="0.01"
                  min="0"
                  value={formData.cache1hWritePrice}
                  onChange={(e) => setFormData({ ...formData, cache1hWritePrice: e.target.value })}
                  className="font-mono text-sm"
                />
              </div>
            </div>

            {/* Image token prices (gpt-image-*); leave 0 for text models */}
            <div className="space-y-2">
              <p className="text-xs text-muted-foreground">{t('modelPrices.imagePricesHint')}</p>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t('modelPrices.imageInputPrice')} ($/M)</Label>
                  <Input
                    type="number"
                    step="0.01"
                    min="0"
                    value={formData.imageInputPrice}
                    onChange={(e) => setFormData({ ...formData, imageInputPrice: e.target.value })}
                    className="font-mono text-sm"
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('modelPrices.imageOutputPrice')} ($/M)</Label>
                  <Input
                    type="number"
                    step="0.01"
                    min="0"
                    value={formData.imageOutputPrice}
                    onChange={(e) => setFormData({ ...formData, imageOutputPrice: e.target.value })}
                    className="font-mono text-sm"
                  />
                </div>
              </div>
            </div>

            {/* 1M Context */}
            <div className="flex items-center gap-3 pt-2">
              <Switch
                checked={formData.has1mContext}
                onCheckedChange={(checked) => setFormData({ ...formData, has1mContext: checked })}
              />
              <Label>{t('modelPrices.has1mContext')}</Label>
            </div>

            {formData.has1mContext && (
              <div className="space-y-4 pl-4 border-l-2 border-border">
                <div className="space-y-2">
                  <Label>{t('modelPrices.context1mThreshold')}</Label>
                  <Input
                    type="number"
                    value={formData.context1mThreshold}
                    onChange={(e) =>
                      setFormData({ ...formData, context1mThreshold: e.target.value })
                    }
                    className="font-mono"
                  />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label>{t('modelPrices.inputPremium')}</Label>
                    <div className="flex items-center gap-1">
                      <Input
                        type="number"
                        value={formData.inputPremiumNum}
                        onChange={(e) =>
                          setFormData({ ...formData, inputPremiumNum: e.target.value })
                        }
                        className="font-mono w-16"
                      />
                      <span className="text-muted-foreground">/</span>
                      <Input
                        type="number"
                        value={formData.inputPremiumDenom}
                        onChange={(e) =>
                          setFormData({ ...formData, inputPremiumDenom: e.target.value })
                        }
                        className="font-mono w-16"
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Label>{t('modelPrices.outputPremium')}</Label>
                    <div className="flex items-center gap-1">
                      <Input
                        type="number"
                        value={formData.outputPremiumNum}
                        onChange={(e) =>
                          setFormData({ ...formData, outputPremiumNum: e.target.value })
                        }
                        className="font-mono w-16"
                      />
                      <span className="text-muted-foreground">/</span>
                      <Input
                        type="number"
                        value={formData.outputPremiumDenom}
                        onChange={(e) =>
                          setFormData({ ...formData, outputPremiumDenom: e.target.value })
                        }
                        className="font-mono w-16"
                      />
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setIsDialogOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button onClick={handleSave} disabled={!formData.modelId.trim() || isPending}>
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog
        open={deleteConfirmId !== null}
        onOpenChange={(open) => !open && setDeleteConfirmId(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>{t('modelPrices.confirmDelete')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleDeleteConfirm}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Reset Confirmation Dialog */}
      <AlertDialog open={resetConfirmOpen} onOpenChange={setResetConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>{t('modelPrices.confirmReset')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleResetConfirm}>
              {t('modelPrices.resetToDefaults')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <UpstreamPricesDialog
        open={upstreamPricesOpen}
        onOpenChange={setUpstreamPricesOpen}
        upstreamPrices={upstreamPrices}
        upstreamSource={upstreamSource}
        changes={upstreamChanges}
        isPending={isPending}
        onApply={handleApplyUpstreamPrices}
      />
    </div>
  );
}

export default ModelPricesPage;
