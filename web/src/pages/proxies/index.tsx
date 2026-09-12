import { useMemo, useState } from 'react';
import { Loader2, Network, PlugZap, Plus, Save, Trash2 } from 'lucide-react';
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
  Switch,
} from '@/components/ui';
import { useSettings, useUpdateSetting } from '@/hooks/queries';
import { getTransport, type ProxyConnectivityResult } from '@/lib/transport';
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

type ProxyCheckState =
  | { status: 'checking'; url: string }
  | ({ status: 'success'; url: string } & ProxyConnectivityResult)
  | ({ status: 'error'; url: string } & ProxyConnectivityResult);

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
  const [checks, setChecks] = useState<Record<string, ProxyCheckState>>({});
  const dirty = items !== null;

  const update = (id: string, patch: Partial<OutboundProxyDefinition>) => {
    setItems(proxies.map((item) => (item.id === id ? { ...item, ...patch } : item)));
    if (patch.url !== undefined) {
      setChecks((current) => {
        const next = { ...current };
        delete next[id];
        return next;
      });
    }
  };

  const checkProxy = async (proxy: OutboundProxyDefinition) => {
    const requestURL = proxy.url;
    if (!isSupportedProxyURL(requestURL)) {
      setChecks((current) => ({
        ...current,
        [proxy.id]: { status: 'error', url: requestURL, ok: false, durationMs: 0, error: t('proxies.invalidURL') },
      }));
      return;
    }
    setChecks((current) => ({ ...current, [proxy.id]: { status: 'checking', url: requestURL } }));
    try {
      const result = await getTransport().checkOutboundProxy(requestURL);
      setChecks((current) => {
        if (current[proxy.id]?.url !== requestURL) return current;
        return {
          ...current,
          [proxy.id]: { ...result, url: requestURL, status: result.ok ? 'success' : 'error' },
        };
      });
    } catch (err) {
      const message = err instanceof Error && err.message ? err.message : t('proxies.checkFailed');
      setChecks((current) => {
        if (current[proxy.id]?.url !== requestURL) return current;
        return {
          ...current,
          [proxy.id]: { status: 'error', url: requestURL, ok: false, durationMs: 0, error: message },
        };
      });
    }
  };

  const save = async () => {
    const invalid = proxies.find((item) => item.url.trim() && !isSupportedProxyURL(item.url));
    if (invalid) {
      setError(t('proxies.invalidURL'));
      return;
    }
    setError(null);
    try {
      await updateSetting.mutateAsync({
        key: OUTBOUND_PROXIES_SETTING_KEY,
        value: serializeOutboundProxies(proxies),
      });
      setItems(null);
    } catch {
      setError(t('proxies.saveFailed'));
    }
  };

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Network}
        iconClassName="text-zinc-500"
        title={t('proxies.title')}
        description={t('proxies.description')}
      >
        <Button onClick={() => setItems([...proxies, newProxy()])} variant="secondary">
          <Plus size={14} />
          {t('proxies.add')}
        </Button>
        <Button onClick={save} disabled={!dirty || updateSetting.isPending}>
          <Save size={14} />
          {t('common.save')}
        </Button>
      </PageHeader>
      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="mx-auto max-w-5xl space-y-4">
          <Card className="border-border bg-card">
            <CardHeader className="border-b border-border">
              <CardTitle className="text-base font-medium">{t('proxies.list')}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 pt-5">
              {proxies.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t('proxies.empty')}</p>
              ) : null}
              {proxies.map((proxy) => {
                const check = checks[proxy.id];
                return (
                <div
                  key={proxy.id}
                  data-testid="proxy-row"
                  className="grid gap-3 rounded-xl border border-border bg-muted/20 p-4 md:grid-cols-[1fr_2fr_auto_auto_auto] md:items-end"
                >
                  <div className="space-y-2">
                    <Label htmlFor={`proxy-name-${proxy.id}`}>{t('proxies.name')}</Label>
                    <Input
                      id={`proxy-name-${proxy.id}`}
                      value={proxy.name}
                      onChange={(e) => update(proxy.id, { name: e.target.value })}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor={`proxy-url-${proxy.id}`}>{t('proxies.url')}</Label>
                    <Input
                      id={`proxy-url-${proxy.id}`}
                      value={proxy.url}
                      onChange={(e) => update(proxy.id, { url: e.target.value })}
                      placeholder={t('proxies.placeholder')}
                    />
                  </div>
                  <div className="flex items-center gap-2 pb-2">
                    <Switch
                      checked={!proxy.disabled}
                      onCheckedChange={(checked) => update(proxy.id, { disabled: !checked })}
                    />
                    <span className="text-sm text-muted-foreground">
                      {proxy.disabled ? t('common.disabled') : t('common.enabled')}
                    </span>
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    disabled={check?.status === 'checking' || proxy.disabled || !proxy.url.trim()}
                    onClick={() => void checkProxy(proxy)}
                  >
                    {check?.status === 'checking' ? <Loader2 size={14} className="animate-spin" /> : <PlugZap size={14} />}
                    {check?.status === 'checking' ? t('proxies.checking') : t('proxies.check')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setItems(proxies.filter((item) => item.id !== proxy.id))}
                  >
                    <Trash2 size={14} />
                    {t('common.delete')}
                  </Button>
                  <div className="md:col-span-5 space-y-1 text-xs">
                    <p className="text-muted-foreground">
                      {t('proxies.preview')}: {maskProxyURL(proxy.url)}
                    </p>
                    {check?.status === 'success' ? (
                      <p className="text-emerald-600">
                        {t('proxies.checkSuccess', { ip: check.outboundIP, duration: check.durationMs })}
                      </p>
                    ) : null}
                    {check?.status === 'error' ? (
                      <p className="text-destructive">
                        {t('proxies.checkError', { error: check.error || t('proxies.checkFailed') })}
                      </p>
                    ) : null}
                  </div>
                </div>
                );
              })}
              {error ? <p className="text-sm text-destructive">{error}</p> : null}
              <p className="text-xs text-muted-foreground">{t('proxies.hint')}</p>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
