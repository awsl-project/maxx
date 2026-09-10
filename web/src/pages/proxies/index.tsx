import { useMemo, useState } from 'react';
import { Network, Plus, Save, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import { Button, Card, CardContent, CardHeader, CardTitle, Input, Label, Switch } from '@/components/ui';
import { useSettings, useUpdateSetting } from '@/hooks/queries';
import {
  OUTBOUND_PROXIES_SETTING_KEY,
  isSupportedProxyURL,
  maskProxyURL,
  parseOutboundProxies,
  serializeOutboundProxies,
  type OutboundProxyDefinition,
} from './utils/proxy-settings';

function newProxy(): OutboundProxyDefinition {
  return { id: crypto.randomUUID(), name: 'Proxy', url: 'http://127.0.0.1:7890' };
}

export function ProxiesPage() {
  const { t } = useTranslation();
  const { data: settings } = useSettings();
  const updateSetting = useUpdateSetting();
  const saved = useMemo(
    () => parseOutboundProxies(settings?.[OUTBOUND_PROXIES_SETTING_KEY]),
    [settings],
  );
  const [items, setItems] = useState<OutboundProxyDefinition[] | null>(null);
  const proxies = items ?? saved;
  const [error, setError] = useState<string | null>(null);
  const dirty = items !== null;

  const update = (id: string, patch: Partial<OutboundProxyDefinition>) => {
    setItems(proxies.map((item) => (item.id === id ? { ...item, ...patch } : item)));
  };

  const save = async () => {
    const invalid = proxies.find((item) => item.url.trim() && !isSupportedProxyURL(item.url));
    if (invalid) {
      setError(t('proxies.invalidURL'));
      return;
    }
    setError(null);
    await updateSetting.mutateAsync({ key: OUTBOUND_PROXIES_SETTING_KEY, value: serializeOutboundProxies(proxies) });
    setItems(null);
  };

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader icon={Network} iconClassName="text-zinc-500" title={t('proxies.title')} description={t('proxies.description')}>
        <Button onClick={() => setItems([...proxies, newProxy()])} variant="secondary"><Plus size={14} />{t('proxies.add')}</Button>
        <Button onClick={save} disabled={!dirty || updateSetting.isPending}><Save size={14} />{t('common.save')}</Button>
      </PageHeader>
      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="mx-auto max-w-5xl space-y-4">
          <Card className="border-border bg-card">
            <CardHeader className="border-b border-border"><CardTitle className="text-base font-medium">{t('proxies.list')}</CardTitle></CardHeader>
            <CardContent className="space-y-4 pt-5">
              {proxies.length === 0 ? <p className="text-sm text-muted-foreground">{t('proxies.empty')}</p> : null}
              {proxies.map((proxy) => (
                <div key={proxy.id} data-testid="proxy-row" className="grid gap-3 rounded-xl border border-border bg-muted/20 p-4 md:grid-cols-[1fr_2fr_auto_auto] md:items-end">
                  <div className="space-y-2"><Label>{t('proxies.name')}</Label><Input value={proxy.name} onChange={(e) => update(proxy.id, { name: e.target.value })} /></div>
                  <div className="space-y-2"><Label>{t('proxies.url')}</Label><Input value={proxy.url} onChange={(e) => update(proxy.id, { url: e.target.value })} placeholder="http://127.0.0.1:7890" /></div>
                  <div className="flex items-center gap-2 pb-2"><Switch checked={!proxy.disabled} onCheckedChange={(checked) => update(proxy.id, { disabled: !checked })} /><span className="text-sm text-muted-foreground">{proxy.disabled ? t('common.disabled') : t('common.enabled')}</span></div>
                  <Button variant="ghost" size="sm" onClick={() => setItems(proxies.filter((item) => item.id !== proxy.id))}><Trash2 size={14} />{t('common.delete')}</Button>
                  <p className="md:col-span-4 text-xs text-muted-foreground">{t('proxies.preview')}: {maskProxyURL(proxy.url)}</p>
                </div>
              ))}
              {error ? <p className="text-sm text-destructive">{error}</p> : null}
              <p className="text-xs text-muted-foreground">{t('proxies.hint')}</p>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
