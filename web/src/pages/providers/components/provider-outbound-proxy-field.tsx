import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardHeader, CardTitle, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui';
import { useSettings } from '@/hooks/queries';
import { OUTBOUND_PROXIES_SETTING_KEY, parseOutboundProxies, proxyLabel } from '@/pages/proxies/utils/proxy-settings';

export function ProviderOutboundProxyField({ value, onChange }: { value?: string; onChange: (value: string) => void }) {
  const { t } = useTranslation();
  const { data: settings } = useSettings();
  const proxies = useMemo(
    () => parseOutboundProxies(settings?.[OUTBOUND_PROXIES_SETTING_KEY]).filter((proxy) => !proxy.disabled),
    [settings],
  );
  const selected = value?.trim() || '__direct__';

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border">
        <CardTitle className="text-base font-medium">{t('provider.outboundProxy')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 pt-5">
        <Label>{t('provider.outboundProxyLabel')}</Label>
        <Select value={selected} onValueChange={(next) => onChange(next === '__direct__' ? '' : (next ?? ''))}>
          <SelectTrigger data-testid="provider-outbound-proxy-select" aria-label={t('provider.outboundProxyLabel')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__direct__">{t('provider.outboundProxyDirect')}</SelectItem>
            {proxies.map((proxy) => (
              <SelectItem key={proxy.id} value={proxy.url}>{proxyLabel(proxy)}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">{t('provider.outboundProxyDesc')}</p>
      </CardContent>
    </Card>
  );
}
