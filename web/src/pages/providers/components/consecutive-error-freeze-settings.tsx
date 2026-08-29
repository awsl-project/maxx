import { AlertTriangle } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';

interface ConsecutiveErrorFreezeSettingsProps {
  disableErrorCooldown: boolean;
  enabled: boolean;
  threshold: number;
  onEnabledChange: (enabled: boolean) => void;
  onThresholdChange: (threshold: number) => void;
}

export function normalizeConsecutiveErrorFreezeThreshold(value: number | undefined | null): number {
  if (!Number.isFinite(value || 0)) return 3;
  const integer = Math.trunc(Number(value));
  if (integer <= 0) return 3;
  if (integer > 100) return 100;
  return integer;
}

export function ConsecutiveErrorFreezeSettings({
  disableErrorCooldown,
  enabled,
  threshold,
  onEnabledChange,
  onThresholdChange,
}: ConsecutiveErrorFreezeSettingsProps) {
  const { t } = useTranslation();

  if (!disableErrorCooldown) return null;

  return (
    <div className="space-y-3 rounded-xl border border-warning/30 bg-warning/5 p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="pr-4">
          <div className="flex items-center gap-2 text-sm font-medium text-foreground">
            <AlertTriangle className="h-4 w-4 text-warning" />
            {t('provider.consecutiveErrorFreeze')}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('provider.consecutiveErrorFreezeDesc')}
          </p>
        </div>
        <Switch checked={enabled} onCheckedChange={onEnabledChange} />
      </div>

      {enabled && (
        <div className="grid gap-2 sm:grid-cols-[180px_1fr] sm:items-center">
          <label className="text-xs font-medium text-foreground">
            {t('provider.consecutiveErrorFreezeThreshold')}
          </label>
          <Input
            type="number"
            min={1}
            max={100}
            value={threshold}
            onChange={(event) =>
              onThresholdChange(
                normalizeConsecutiveErrorFreezeThreshold(Number(event.target.value)),
              )
            }
            className="h-9 max-w-32"
          />
          <div className="sm:col-span-2 text-xs text-muted-foreground">
            {t('provider.consecutiveErrorFreezeHint')}
          </div>
        </div>
      )}
    </div>
  );
}
