import { useEffect, useMemo, useState } from 'react';
import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui';
import { useSettings, useUpdateSetting } from '@/hooks/queries';

type AutoCooldownUnit = 'seconds' | 'minutes' | 'hours' | 'days';

const AUTO_COOLDOWN_MAX_SECONDS = 7 * 24 * 60 * 60;
const AUTO_COOLDOWN_UNITS: Array<{ value: AutoCooldownUnit; multiplier: number }> = [
  { value: 'seconds', multiplier: 1 },
  { value: 'minutes', multiplier: 60 },
  { value: 'hours', multiplier: 60 * 60 },
  { value: 'days', multiplier: 24 * 60 * 60 },
];
const AUTO_COOLDOWN_SHORTCUTS: Array<{ label: string; amount: string; unit: AutoCooldownUnit }> = [
  { label: '5s', amount: '5', unit: 'seconds' },
  { label: '1h', amount: '1', unit: 'hours' },
  { label: '24h', amount: '24', unit: 'hours' },
  { label: '7d', amount: '7', unit: 'days' },
];

function getAutoCooldownMultiplier(unit: AutoCooldownUnit) {
  return AUTO_COOLDOWN_UNITS.find((item) => item.value === unit)?.multiplier ?? 1;
}

function parseAutoCooldownSeconds(amount: string, unit: AutoCooldownUnit) {
  const trimmed = amount.trim();
  if (!/^\d+$/.test(trimmed)) return NaN;
  return Number(trimmed) * getAutoCooldownMultiplier(unit);
}

export function APITokenLimitsPage() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const currentLimit = settings?.api_token_concurrent_limit || '5';
  const currentAutoCooldown = settings?.cooldown_rate_limit_default_seconds || '5';
  const [limitDraft, setLimitDraft] = useState('');
  const [autoCooldownDraft, setAutoCooldownDraft] = useState('');
  const [autoCooldownUnit, setAutoCooldownUnit] = useState<AutoCooldownUnit>('seconds');
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (!isLoading && !initialized) {
      setLimitDraft(currentLimit);
      setAutoCooldownDraft(currentAutoCooldown);
      setAutoCooldownUnit('seconds');
      setInitialized(true);
    }
  }, [isLoading, initialized, currentLimit, currentAutoCooldown]);

  const hasLimitChanges = initialized && limitDraft !== currentLimit;
  const parsedAutoCooldown = useMemo(
    () => parseAutoCooldownSeconds(autoCooldownDraft, autoCooldownUnit),
    [autoCooldownDraft, autoCooldownUnit],
  );
  const autoCooldownPreview = Number.isFinite(parsedAutoCooldown) ? parsedAutoCooldown : 0;
  const hasAutoCooldownChanges = initialized && String(parsedAutoCooldown) !== currentAutoCooldown;
  const hasChanges = hasLimitChanges || hasAutoCooldownChanges;

  useEffect(() => {
    if (initialized && !hasChanges) {
      setLimitDraft(currentLimit);
      setAutoCooldownDraft(currentAutoCooldown);
      setAutoCooldownUnit('seconds');
    }
  }, [currentLimit, currentAutoCooldown, initialized, hasChanges]);

  const parsedLimit = /^\d+$/.test(limitDraft.trim()) ? Number(limitDraft.trim()) : NaN;
  const isLimitValid = Number.isInteger(parsedLimit) && parsedLimit >= 1;
  const isAutoCooldownValid =
    Number.isInteger(parsedAutoCooldown) &&
    parsedAutoCooldown >= 1 &&
    parsedAutoCooldown <= AUTO_COOLDOWN_MAX_SECONDS;
  const isValid = isLimitValid && isAutoCooldownValid;

  const handleSave = async () => {
    if (!isValid || !hasChanges) return;
    if (hasLimitChanges) {
      await updateSetting.mutateAsync({
        key: 'api_token_concurrent_limit',
        value: String(parsedLimit),
      });
    }
    if (hasAutoCooldownChanges) {
      await updateSetting.mutateAsync({
        key: 'cooldown_rate_limit_default_seconds',
        value: String(parsedAutoCooldown),
      });
    }
  };

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Activity}
        iconClassName="text-zinc-500"
        title={t('apiTokenLimits.title')}
        description={t('apiTokenLimits.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="space-y-6">
          <Card className="border-border bg-card">
            <CardHeader className="border-b border-border py-4">
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="text-base font-medium flex items-center gap-2">
                    <Activity className="h-4 w-4 text-muted-foreground" />
                    {t('apiTokenLimits.concurrencyTitle')}
                  </CardTitle>
                </div>
                <Button
                  onClick={handleSave}
                  disabled={
                    !hasChanges || !isValid || updateSetting.isPending || isLoading || !initialized
                  }
                  size="sm"
                >
                  {updateSetting.isPending ? t('common.saving') : t('common.save')}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="p-6 space-y-1.5">
              <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
                <Label className="text-sm font-medium text-muted-foreground shrink-0">
                  {t('apiTokenLimits.concurrentLimit')}
                </Label>
                <Input
                  type="number"
                  value={limitDraft}
                  onChange={(e) => setLimitDraft(e.target.value)}
                  className="w-24"
                  min={1}
                  disabled={updateSetting.isPending || isLoading || !initialized}
                />
                <span className="text-xs text-muted-foreground">
                  {t('apiTokenLimits.concurrentRequestsUnit')}
                </span>
                <span className="text-xs text-muted-foreground">
                  ({t('settings.defaultValue', { value: 5 })})
                </span>
              </div>
              {!isLimitValid && initialized && (
                <p className="text-xs text-destructive">
                  {t('apiTokenLimits.concurrentLimitInvalid')}
                </p>
              )}
            </CardContent>
          </Card>

          <Card className="overflow-hidden rounded-2xl border-border bg-card shadow-sm">
            <CardHeader className="border-b border-border py-4">
              <CardTitle className="text-base font-medium flex items-center gap-2">
                <Activity className="h-4 w-4 text-muted-foreground" />
                {t('apiTokenLimits.autoCooldownTitle')}
              </CardTitle>
            </CardHeader>
            <CardContent className="p-6 space-y-5">
              <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
                {t('apiTokenLimits.autoCooldownDesc')}
              </p>

              <div
                className="grid max-w-2xl grid-cols-2 gap-2 sm:grid-cols-4"
                aria-label={t('apiTokenLimits.autoCooldownShortcuts')}
              >
                {AUTO_COOLDOWN_SHORTCUTS.map((shortcut) => {
                  const shortcutSeconds = parseAutoCooldownSeconds(shortcut.amount, shortcut.unit);
                  const isSelected =
                    autoCooldownDraft.trim() === shortcut.amount &&
                    autoCooldownUnit === shortcut.unit;

                  return (
                    <Button
                      key={shortcut.label}
                      type="button"
                      variant="outline"
                      className={`h-auto justify-start rounded-xl px-4 py-3 text-left transition-colors ${
                        isSelected
                          ? 'border-primary/70 bg-primary/10 text-primary shadow-sm'
                          : 'border-border bg-background/60 hover:bg-muted/70'
                      }`}
                      onClick={() => {
                        setAutoCooldownDraft(shortcut.amount);
                        setAutoCooldownUnit(shortcut.unit);
                      }}
                      disabled={updateSetting.isPending || isLoading || !initialized}
                    >
                      <span className="flex flex-col items-start gap-1">
                        <span className="font-mono text-sm font-semibold">{shortcut.label}</span>
                        <span className="text-[11px] font-normal text-muted-foreground">
                          {t('apiTokenLimits.autoCooldownPresetSeconds', {
                            seconds: shortcutSeconds,
                          })}
                        </span>
                      </span>
                    </Button>
                  );
                })}
              </div>

              <div className="max-w-2xl rounded-xl border border-border bg-muted/20 p-4">
                <Label
                  htmlFor="auto-cooldown-duration"
                  className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
                >
                  {t('apiTokenLimits.autoCooldownInputLabel')}
                </Label>
                <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(0,1fr)_12rem]">
                  <Input
                    id="auto-cooldown-duration"
                    type="number"
                    value={autoCooldownDraft}
                    onChange={(e) => setAutoCooldownDraft(e.target.value)}
                    className="h-10 bg-background"
                    min={1}
                    max={AUTO_COOLDOWN_MAX_SECONDS}
                    step={1}
                    placeholder={t('apiTokenLimits.autoCooldownPlaceholder')}
                    disabled={updateSetting.isPending || isLoading || !initialized}
                  />
                  <Select
                    value={autoCooldownUnit}
                    onValueChange={(value) => setAutoCooldownUnit(value as AutoCooldownUnit)}
                    disabled={updateSetting.isPending || isLoading || !initialized}
                  >
                    <SelectTrigger className="h-10 bg-background">
                      <SelectValue>
                        {t(`apiTokenLimits.autoCooldownUnits.${autoCooldownUnit}`)}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {AUTO_COOLDOWN_UNITS.map((unit) => (
                        <SelectItem key={unit.value} value={unit.value}>
                          {t(`apiTokenLimits.autoCooldownUnits.${unit.value}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="max-w-2xl rounded-xl border border-dashed border-border px-4 py-3">
                <p className="text-sm font-medium text-foreground">
                  {isAutoCooldownValid
                    ? t('apiTokenLimits.autoCooldownPreview', { seconds: autoCooldownPreview })
                    : t('apiTokenLimits.autoCooldownPreviewPending')}
                </p>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {t('apiTokenLimits.autoCooldownHint')}
                </p>
              </div>

              {!isAutoCooldownValid && initialized && (
                <p className="text-xs text-destructive">
                  {t('apiTokenLimits.autoCooldownInvalid')}
                </p>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

export default APITokenLimitsPage;
