import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronDownIcon, ServerIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { getBackendUrl, setBackendUrl, BackendStorageError } from '@/lib/backend-config';

interface BackendAddressControlProps {
  /** When true, render expanded by default (e.g. on a settings page). */
  defaultOpen?: boolean;
  /** When true, render without the collapsible toggle (always expanded). */
  alwaysOpen?: boolean;
}

/**
 * Lets the user point this UI at a backend on a different origin. The override
 * is stored in localStorage; saving reloads the page so the transport layer
 * re-initializes against the new address.
 */
export function BackendAddressControl({
  defaultOpen = false,
  alwaysOpen = false,
}: BackendAddressControlProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(defaultOpen || alwaysOpen);
  const [value, setValue] = useState(() => getBackendUrl());
  const [error, setError] = useState('');

  const current = getBackendUrl();

  const apply = (next: string) => {
    try {
      setBackendUrl(next);
    } catch (err) {
      setError(
        err instanceof BackendStorageError
          ? t('backendAddress.storageError')
          : t('backendAddress.invalid'),
      );
      return;
    }
    // Reload so the transport singleton is rebuilt with the new config.
    window.location.reload();
  };

  const handleSave = () => apply(value);
  const handleReset = () => {
    setValue('');
    apply('');
  };

  const body = (
    <div className="space-y-3 rounded-2xl border border-border/70 bg-muted/25 p-4">
      <p className="text-muted-foreground text-xs leading-5">{t('backendAddress.description')}</p>
      <div className="space-y-2">
        <Label htmlFor="backend-url">{t('backendAddress.label')}</Label>
        <Input
          id="backend-url"
          type="url"
          inputMode="url"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          value={value}
          placeholder={t('backendAddress.placeholder')}
          aria-invalid={error ? 'true' : undefined}
          onChange={(e) => {
            setValue(e.target.value);
            setError('');
          }}
        />
        {error ? (
          <p className="text-destructive text-xs">{error}</p>
        ) : (
          <p className="text-muted-foreground text-xs">
            {t('backendAddress.current', {
              value: current || t('backendAddress.sameOrigin'),
            })}
          </p>
        )}
        <p className="text-muted-foreground text-xs">{t('backendAddress.corsHint')}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="button" size="sm" onClick={handleSave}>
          {t('backendAddress.save')}
        </Button>
        {current && (
          <Button type="button" size="sm" variant="outline" onClick={handleReset}>
            {t('backendAddress.reset')}
          </Button>
        )}
      </div>
    </div>
  );

  if (alwaysOpen) {
    return body;
  }

  return (
    <div className="space-y-3">
      <button
        type="button"
        className="text-muted-foreground hover:text-foreground flex w-full items-center gap-2 text-xs transition-colors"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <ServerIcon className="size-3.5" />
        <span>{t('backendAddress.advanced')}</span>
        <ChevronDownIcon className={`size-3.5 transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>
      {open && body}
    </div>
  );
}
