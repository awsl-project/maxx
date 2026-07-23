import { useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import type { Provider } from '@/lib/transport';

type ProviderUpdater = {
  mutateAsync: (variables: { id: number; data: Partial<Provider> }) => Promise<unknown>;
};

/** Clamp provider maxConcurrency for API payloads. 0 = unlimited. */
export function normalizeMaxConcurrency(value: unknown): number {
  if (value === '' || value === null || value === undefined) return 0;
  const n =
    typeof value === 'number' ? value : Number.parseInt(String(value).trim(), 10);
  if (!Number.isFinite(n) || n < 0) return 0;
  return Math.trunc(n);
}

export function useProviderMaxConcurrencyField(
  provider: Provider,
  updateProvider: ProviderUpdater,
) {
  const [maxConcurrency, setMaxConcurrency] = useState(() =>
    normalizeMaxConcurrency(provider.maxConcurrency),
  );

  useEffect(() => {
    setMaxConcurrency(normalizeMaxConcurrency(provider.maxConcurrency));
  }, [provider.maxConcurrency]);

  const handleCommitMaxConcurrency = async (next: number) => {
    const normalized = normalizeMaxConcurrency(next);
    const previous = normalizeMaxConcurrency(provider.maxConcurrency);
    if (normalized === previous) return;

    setMaxConcurrency(normalized);
    try {
      await updateProvider.mutateAsync({
        id: provider.id,
        data: { maxConcurrency: normalized },
      });
    } catch {
      setMaxConcurrency(previous);
    }
  };

  return { maxConcurrency, setMaxConcurrency, handleCommitMaxConcurrency };
}

type ProviderMaxConcurrencyFieldProps = {
  value: number;
  onChange: (value: number) => void;
  /** Called after blur with the normalized value (for immediate-save UIs). */
  onCommit?: (value: number) => void;
  disabled?: boolean;
  className?: string;
  id?: string;
};

/**
 * Compact controlled concurrency field. Allows temporary empty text while
 * editing; blur normalizes empty/invalid input to 0 (unlimited).
 */
export function ProviderMaxConcurrencyField({
  value,
  onChange,
  onCommit,
  disabled,
  className,
  id,
}: ProviderMaxConcurrencyFieldProps) {
  const { t } = useTranslation();
  const autoId = useId();
  const fieldId = id ?? autoId;
  const helperId = `${fieldId}-desc`;
  const [text, setText] = useState(() => String(normalizeMaxConcurrency(value)));

  useEffect(() => {
    setText(String(normalizeMaxConcurrency(value)));
  }, [value]);

  const commit = () => {
    const next = normalizeMaxConcurrency(text);
    setText(String(next));
    if (next !== value) {
      onChange(next);
    }
    onCommit?.(next);
  };

  return (
    <div className={cn(className)}>
      <label htmlFor={fieldId} className="mb-2 block text-sm font-medium text-foreground">
        {t('provider.maxConcurrency')}
      </label>
      <Input
        id={fieldId}
        type="number"
        inputMode="numeric"
        min={0}
        step={1}
        disabled={disabled}
        value={text}
        onChange={(event) => {
          const raw = event.target.value;
          // Allow temporary empty while editing; do not coerce to 0 yet.
          if (raw === '') {
            setText('');
            return;
          }
          // Digits only (type=number can still emit e/./- in some browsers).
          if (!/^\d+$/.test(raw)) return;
          setText(raw);
          onChange(normalizeMaxConcurrency(raw));
        }}
        onBlur={commit}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            (event.target as HTMLInputElement).blur();
          }
        }}
        aria-describedby={helperId}
        className="w-full sm:w-32"
      />
      <p id={helperId} className="mt-1 text-xs text-muted-foreground">
        {t('provider.maxConcurrencyDesc')}
      </p>
    </div>
  );
}
