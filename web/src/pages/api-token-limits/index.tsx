import { useEffect, useState } from 'react';
import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import { Button, Card, CardContent, CardHeader, CardTitle, Input, Label } from '@/components/ui';
import { useSettings, useUpdateSetting } from '@/hooks/queries';

export function APITokenLimitsPage() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const currentLimit = settings?.api_token_concurrent_limit || '5';
  const currentAutoCooldown = settings?.cooldown_rate_limit_default_seconds || '5';
  const [limitDraft, setLimitDraft] = useState('');
  const [autoCooldownDraft, setAutoCooldownDraft] = useState('');
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (!isLoading && !initialized) {
      setLimitDraft(currentLimit);
      setAutoCooldownDraft(currentAutoCooldown);
      setInitialized(true);
    }
  }, [isLoading, initialized, currentLimit, currentAutoCooldown]);

  const hasLimitChanges = initialized && limitDraft !== currentLimit;
  const hasAutoCooldownChanges = initialized && autoCooldownDraft !== currentAutoCooldown;
  const hasChanges = hasLimitChanges || hasAutoCooldownChanges;

  useEffect(() => {
    if (initialized && !hasChanges) {
      setLimitDraft(currentLimit);
      setAutoCooldownDraft(currentAutoCooldown);
    }
  }, [currentLimit, currentAutoCooldown, initialized, hasChanges]);

  const parsedLimit = /^\d+$/.test(limitDraft.trim()) ? Number(limitDraft.trim()) : NaN;
  const isLimitValid = Number.isInteger(parsedLimit) && parsedLimit >= 1;
  const parsedAutoCooldown = /^\d+$/.test(autoCooldownDraft.trim())
    ? Number(autoCooldownDraft.trim())
    : NaN;
  const isAutoCooldownValid =
    Number.isInteger(parsedAutoCooldown) && parsedAutoCooldown >= 1 && parsedAutoCooldown <= 86400;
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
                  <p className="text-xs text-muted-foreground mt-1">
                    {t('apiTokenLimits.concurrencyDesc')}
                  </p>
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
              <p className="text-xs text-muted-foreground">{t('apiTokenLimits.concurrencyHint')}</p>
              {!isLimitValid && initialized && (
                <p className="text-xs text-destructive">
                  {t('apiTokenLimits.concurrentLimitInvalid')}
                </p>
              )}
            </CardContent>
          </Card>

          <Card className="border-border bg-card">
            <CardHeader className="border-b border-border py-4">
              <CardTitle className="text-base font-medium flex items-center gap-2">
                <Activity className="h-4 w-4 text-muted-foreground" />
                {t('apiTokenLimits.autoCooldownTitle')}
              </CardTitle>
              <p className="text-xs text-muted-foreground mt-1">
                {t('apiTokenLimits.autoCooldownDesc')}
              </p>
            </CardHeader>
            <CardContent className="p-6 space-y-1.5">
              <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
                <Label className="text-sm font-medium text-muted-foreground shrink-0">
                  {t('apiTokenLimits.autoCooldownSeconds')}
                </Label>
                <Input
                  type="number"
                  value={autoCooldownDraft}
                  onChange={(e) => setAutoCooldownDraft(e.target.value)}
                  className="w-24"
                  min={1}
                  max={86400}
                  step={1}
                  disabled={updateSetting.isPending || isLoading || !initialized}
                />
                <span className="text-xs text-muted-foreground">{t('common.seconds')}</span>
                <span className="text-xs text-muted-foreground">
                  ({t('settings.defaultValue', { value: 5 })})
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('apiTokenLimits.autoCooldownHint')}
              </p>
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
