import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui';
import {
  useCreateModelPrice,
  useFetchExternalModelPrices,
  useUpdateModelPrice,
} from '@/hooks/queries';
import type { ModelPrice, ModelPriceChange, ModelPriceInput } from '@/lib/transport/types';
import { RotateCcw } from 'lucide-react';
import { UpstreamPricesDialog } from './upstream-prices-dialog';

const modelPriceUpstreamSources = [{ value: 'litellm', label: 'LiteLLM' }] as const;

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

interface UpstreamPricesImportProps {
  currentPrices: ModelPrice[];
  disabled?: boolean;
}

export function UpstreamPricesImport({ currentPrices, disabled }: UpstreamPricesImportProps) {
  const { t } = useTranslation();
  const createPrice = useCreateModelPrice();
  const updatePrice = useUpdateModelPrice();
  const fetchExternalPrices = useFetchExternalModelPrices();

  const [open, setOpen] = useState(false);
  const [source, setSource] =
    useState<(typeof modelPriceUpstreamSources)[number]['value']>('litellm');
  const [result, setResult] = useState<Awaited<
    ReturnType<typeof fetchExternalPrices.mutateAsync>
  > | null>(null);
  const [changes, setChanges] = useState<ModelPriceChange[]>([]);

  const isPending =
    disabled || createPrice.isPending || updatePrice.isPending || fetchExternalPrices.isPending;

  const handleFetch = async () => {
    const upstreamResult = await fetchExternalPrices.mutateAsync(source);
    setResult(upstreamResult);
    setChanges(buildUpstreamChanges(upstreamResult.prices, currentPrices));
    setOpen(true);
  };

  const handleApply = async (selectedChanges: ModelPriceChange[]) => {
    for (const change of selectedChanges) {
      const input = modelPriceToInput(change.after);
      if (change.action === 'create') {
        await createPrice.mutateAsync(input);
      } else if (change.action === 'update') {
        await updatePrice.mutateAsync({ id: change.after.id, data: input });
      }
    }
    setOpen(false);
  };

  return (
    <>
      <Select value={source} onValueChange={(value) => setSource(value as typeof source)}>
        <SelectTrigger className="w-32 h-8 text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {modelPriceUpstreamSources.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button variant="outline" size="sm" onClick={handleFetch} disabled={isPending}>
        <RotateCcw className="h-4 w-4 mr-1" />
        {t('modelPrices.fetchUpstreamPrices')}
      </Button>
      <UpstreamPricesDialog
        open={open}
        onOpenChange={setOpen}
        upstreamPrices={result}
        upstreamSource={source}
        changes={changes}
        isPending={Boolean(isPending)}
        onApply={handleApply}
      />
    </>
  );
}
