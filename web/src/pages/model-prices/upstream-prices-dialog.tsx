import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui';
import type { ModelPriceChange, UpstreamModelPricesResult } from '@/lib/transport/types';

function formatMicroPrice(microUsd: number): string {
  return `$${(microUsd / 1_000_000).toFixed(2)}`;
}

function upstreamChangeKey(action: string, modelId: string): string {
  return `${action}:${modelId}`;
}

interface UpstreamPricesDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  upstreamPrices: UpstreamModelPricesResult | null;
  upstreamSource: string;
  changes: ModelPriceChange[];
  isPending: boolean;
  onApply: (selectedChanges: ModelPriceChange[]) => void;
}

export function UpstreamPricesDialog({
  open,
  onOpenChange,
  upstreamPrices,
  upstreamSource,
  changes,
  isPending,
  onApply,
}: UpstreamPricesDialogProps) {
  const { t } = useTranslation();
  const [selectedChangeKeys, setSelectedChangeKeys] = useState<string[]>([]);
  const visibleChanges = changes.slice(0, 50);

  useEffect(() => {
    if (!open) return;
    setSelectedChangeKeys(
      changes.slice(0, 50).map((change) => upstreamChangeKey(change.action, change.after.modelId)),
    );
  }, [changes, open]);

  const handleToggleChange = (key: string, checked: boolean) => {
    setSelectedChangeKeys((current) =>
      checked ? [...current, key] : current.filter((item) => item !== key),
    );
  };

  const handleToggleAllChanges = (checked: boolean) => {
    setSelectedChangeKeys(
      checked
        ? visibleChanges.map((change) => upstreamChangeKey(change.action, change.after.modelId))
        : [],
    );
  };

  const handleApply = () => {
    const selected = new Set(selectedChangeKeys);
    onApply(
      visibleChanges.filter((change) =>
        selected.has(upstreamChangeKey(change.action, change.after.modelId)),
      ),
    );
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="max-w-2xl">
        <AlertDialogHeader>
          <AlertDialogTitle>{t('modelPrices.upstreamPricesTitle')}</AlertDialogTitle>
          <AlertDialogDescription>{t('modelPrices.upstreamPricesDesc')}</AlertDialogDescription>
        </AlertDialogHeader>
        {upstreamPrices && (
          <div className="space-y-3 text-left">
            <div className="rounded border border-border bg-muted/30 p-3 text-xs space-y-1">
              <div className="break-all">
                {t('modelPrices.upstreamSourceLabel')}: {upstreamPrices.source || upstreamSource} ·{' '}
                {upstreamPrices.sourceUrl}
              </div>
              <div>
                {t('modelPrices.upstreamPricesStats', {
                  total: upstreamPrices.total,
                  created: changes.filter((change) => change.action === 'create').length,
                  updated: changes.filter((change) => change.action === 'update').length,
                  skipped: upstreamPrices.total - changes.length,
                })}
              </div>
            </div>
            {changes.length > 0 && (
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                <input
                  type="checkbox"
                  checked={selectedChangeKeys.length === visibleChanges.length}
                  ref={(el) => {
                    if (el) {
                      el.indeterminate =
                        selectedChangeKeys.length > 0 &&
                        selectedChangeKeys.length < visibleChanges.length;
                    }
                  }}
                  onChange={(event) => handleToggleAllChanges(event.target.checked)}
                  disabled={isPending}
                />
                {t('modelPrices.upstreamSelectAll', {
                  selected: selectedChangeKeys.length,
                  total: visibleChanges.length,
                })}
              </label>
            )}
            <div className="max-h-72 overflow-y-auto rounded border border-border">
              {changes.length === 0 ? (
                <div className="p-3 text-xs text-muted-foreground">
                  {t('modelPrices.upstreamNoChanges')}
                </div>
              ) : (
                visibleChanges.map((change, index) => {
                  const key = upstreamChangeKey(change.action, change.after.modelId);
                  return (
                    <div
                      key={`${change.action}-${change.after.modelId}-${index}`}
                      className="flex items-center gap-3 border-b border-border last:border-b-0 p-2 text-xs"
                    >
                      <input
                        type="checkbox"
                        checked={selectedChangeKeys.includes(key)}
                        onChange={(event) => handleToggleChange(key, event.target.checked)}
                        disabled={isPending}
                      />
                      <span className="w-14 shrink-0 uppercase text-muted-foreground">
                        {change.action}
                      </span>
                      <span className="flex-1 min-w-0 truncate font-mono">
                        {change.after.modelId}
                      </span>
                      <span className="font-mono">
                        {formatMicroPrice(change.before?.inputPriceMicro || 0)} →{' '}
                        {formatMicroPrice(change.after.inputPriceMicro)}
                      </span>
                      <span className="font-mono">
                        {formatMicroPrice(change.before?.outputPriceMicro || 0)} →{' '}
                        {formatMicroPrice(change.after.outputPriceMicro)}
                      </span>
                    </div>
                  );
                })
              )}
            </div>
            {changes.length > 50 && (
              <p className="text-xs text-muted-foreground">
                {t('modelPrices.upstreamPricesMore', {
                  count: changes.length - 50,
                })}
              </p>
            )}
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isPending}>{t('common.cancel')}</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleApply}
            disabled={isPending || !upstreamPrices || selectedChangeKeys.length === 0}
          >
            {t('modelPrices.applySelectedPrices')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
