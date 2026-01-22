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
import {
  useModelPrices,
  useCreateModelPrice,
  useUpdateModelPrice,
  useDeleteModelPrice,
  useResetModelPricesToDefaults,
} from '@/hooks/queries';
import type { ModelPrice, ModelPriceInput } from '@/lib/transport/types';
import { DollarSign, Plus, Trash2, Pencil, RotateCcw } from 'lucide-react';

// Helper to format micro USD price to display format (e.g., $3.00 / M tokens)
function formatMicroPrice(microUsd: number): string {
  return `$${(microUsd / 1_000_000).toFixed(2)}`;
}

// Helper to parse display price to micro USD
function parsePriceToMicro(priceStr: string): number {
  const value = parseFloat(priceStr);
  if (isNaN(value)) return 0;
  return Math.round(value * 1_000_000);
}

interface PriceFormData {
  modelId: string;
  inputPrice: string;
  outputPrice: string;
  cacheReadPrice: string;
  cache5mWritePrice: string;
  cache1hWritePrice: string;
  has1mContext: boolean;
  context1mThreshold: string;
  inputPremiumNum: string;
  inputPremiumDenom: string;
  outputPremiumNum: string;
  outputPremiumDenom: string;
}

const defaultFormData: PriceFormData = {
  modelId: '',
  inputPrice: '3.00',
  outputPrice: '15.00',
  cacheReadPrice: '0.30',
  cache5mWritePrice: '3.75',
  cache1hWritePrice: '6.00',
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
    has1mContext: price.has1mContext,
    context1mThreshold: price.context1mThreshold.toString(),
    inputPremiumNum: price.inputPremiumNum.toString(),
    inputPremiumDenom: price.inputPremiumDenom.toString(),
    outputPremiumNum: price.outputPremiumNum.toString(),
    outputPremiumDenom: price.outputPremiumDenom.toString(),
  };
}

function formDataToInput(form: PriceFormData): ModelPriceInput {
  return {
    modelId: form.modelId,
    inputPriceMicro: parsePriceToMicro(form.inputPrice),
    outputPriceMicro: parsePriceToMicro(form.outputPrice),
    cacheReadPriceMicro: parsePriceToMicro(form.cacheReadPrice),
    cache5mWritePriceMicro: parsePriceToMicro(form.cache5mWritePrice),
    cache1hWritePriceMicro: parsePriceToMicro(form.cache1hWritePrice),
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
  const { data: prices, isLoading } = useModelPrices();
  const createPrice = useCreateModelPrice();
  const updatePrice = useUpdateModelPrice();
  const deletePrice = useDeleteModelPrice();
  const resetPrices = useResetModelPricesToDefaults();

  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingPrice, setEditingPrice] = useState<ModelPrice | null>(null);
  const [formData, setFormData] = useState<PriceFormData>(defaultFormData);
  const [deleteConfirmId, setDeleteConfirmId] = useState<number | null>(null);
  const [resetConfirmOpen, setResetConfirmOpen] = useState(false);

  const handleOpenCreate = () => {
    setEditingPrice(null);
    setFormData(defaultFormData);
    setIsDialogOpen(true);
  };

  const handleOpenEdit = (price: ModelPrice) => {
    setEditingPrice(price);
    setFormData(priceToFormData(price));
    setIsDialogOpen(true);
  };

  const handleSave = async () => {
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
    if (deleteConfirmId === null) return;
    await deletePrice.mutateAsync(deleteConfirmId);
    setDeleteConfirmId(null);
  };

  const handleResetConfirm = async () => {
    await resetPrices.mutateAsync();
    setResetConfirmOpen(false);
  };

  const isPending =
    createPrice.isPending || updatePrice.isPending || deletePrice.isPending || resetPrices.isPending;

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
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => setResetConfirmOpen(true)} disabled={isPending}>
              <RotateCcw className="h-4 w-4 mr-1" />
              {t('modelPrices.resetToDefaults')}
            </Button>
            <Button variant="default" size="sm" onClick={handleOpenCreate} disabled={isPending}>
              <Plus className="h-4 w-4 mr-1" />
              {t('common.add')}
            </Button>
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto p-6">
        <Card className="border-border bg-card">
          <CardContent className="p-6">
            <p className="text-xs text-muted-foreground mb-4">{t('modelPrices.pageDesc')}</p>

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
                    <div className="flex-1 min-w-0 font-mono text-sm truncate">
                      {price.modelId}
                    </div>
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
                        <span className="text-green-500 text-xs">Yes</span>
                      ) : (
                        <span className="text-muted-foreground text-xs">-</span>
                      )}
                    </div>
                    <div className="w-20 shrink-0 flex items-center gap-1 justify-end">
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
                  value={formData.cache1hWritePrice}
                  onChange={(e) => setFormData({ ...formData, cache1hWritePrice: e.target.value })}
                  className="font-mono text-sm"
                />
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
            <Button
              onClick={handleSave}
              disabled={!formData.modelId.trim() || isPending}
            >
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={deleteConfirmId !== null} onOpenChange={(open) => !open && setDeleteConfirmId(null)}>
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
    </div>
  );
}

export default ModelPricesPage;
