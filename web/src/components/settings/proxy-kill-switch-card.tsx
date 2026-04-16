import { AlertTriangle } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardHeader, CardTitle, Switch } from '@/components/ui';
import { useSettings, useUpdateSetting } from '@/hooks/queries';
import { cn } from '@/lib/utils';

interface ProxyKillSwitchCardProps {
  className?: string;
}

export function ProxyKillSwitchCard({ className }: ProxyKillSwitchCardProps) {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const proxyRequestsDisabled =
    (settings?.proxy_requests_disabled ?? '').trim().toLowerCase() === 'true';

  const handleToggle = async (checked: boolean) => {
    await updateSetting.mutateAsync({
      key: 'proxy_requests_disabled',
      value: checked ? 'true' : 'false',
    });
  };

  if (isLoading) return null;

  return (
    <Card className={cn('border-border bg-card', className)}>
      <CardHeader className="border-b border-border">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <AlertTriangle
            className={cn(
              'h-4 w-4',
              proxyRequestsDisabled ? 'text-red-500' : 'text-muted-foreground',
            )}
          />
          {t('settings.proxyKillSwitch')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-sm font-medium text-foreground">
              {t('settings.enableProxyKillSwitch')}
            </div>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.proxyKillSwitchDesc')}
            </p>
          </div>
          <Switch
            checked={proxyRequestsDisabled}
            onCheckedChange={handleToggle}
            disabled={updateSetting.isPending}
            aria-label={t('settings.enableProxyKillSwitch')}
          />
        </div>

        {proxyRequestsDisabled && (
          <div className="flex items-start gap-2 rounded-md border border-red-500/20 bg-red-500/10 p-3">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
            <p className="text-xs text-red-600 dark:text-red-400">
              {t('settings.proxyKillSwitchEnabledHint')}
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
