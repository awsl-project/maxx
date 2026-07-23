import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';

type SmartMappingRetrySettingsProps = {
  disableErrorCooldown: boolean;
  enabled?: boolean;
  retryLimit?: number;
  mappingTargetCount: number;
  onEnabledChange: (enabled: boolean) => void;
  onRetryLimitChange: (limit: number) => void;
};

export function SmartMappingRetrySettings({
  disableErrorCooldown,
  enabled,
  retryLimit,
  mappingTargetCount,
  onEnabledChange,
  onRetryLimitChange,
}: SmartMappingRetrySettingsProps) {
  const { t } = useTranslation();

  if (!disableErrorCooldown) return null;

  const canEnable = mappingTargetCount > 1;
  const checked = !!enabled && canEnable;

  return (
    <div className="space-y-3 rounded-xl border border-border bg-card p-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="text-sm font-medium text-foreground">
            {t('provider.smartMappingRetry')}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {canEnable
              ? t('provider.smartMappingRetryDesc')
              : t('provider.smartMappingRetryUnavailable')}
          </p>
        </div>
        <Switch
          checked={checked}
          disabled={!canEnable}
          onCheckedChange={(next) => onEnabledChange(canEnable && next)}
        />
      </div>
      <div className="grid gap-2 sm:max-w-xs">
        <label className="text-xs font-medium text-muted-foreground">
          {t('provider.smartMappingRetryLimit')}
        </label>
        <Input
          type="number"
          min={1}
          max={20}
          step={1}
          disabled={!checked}
          value={retryLimit ?? 1}
          onChange={(event) => {
            const value = Number.parseInt(event.target.value, 10);
            onRetryLimitChange(Number.isFinite(value) ? Math.min(Math.max(value, 1), 20) : 1);
          }}
        />
        <p className="text-xs text-muted-foreground">{t('provider.smartMappingRetryLimitDesc')}</p>
      </div>
    </div>
  );
}
