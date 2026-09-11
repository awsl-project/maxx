export const PROXY_MANAGEMENT_ENABLED_SETTING_KEY = 'ui_proxy_management_enabled';
export const OUTBOUND_PROXIES_SETTING_KEY = 'outbound_proxies';

export type OutboundProxyDefinition = {
  id: string;
  name: string;
  url: string;
  disabled?: boolean;
};

export function parseOutboundProxies(raw?: string): OutboundProxyDefinition[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map((item): OutboundProxyDefinition | null => {
        if (!item || typeof item !== 'object') return null;
        const id = String((item as any).id || '').trim();
        const name = String((item as any).name || '').trim();
        const url = String((item as any).url || '').trim();
        if (!id || !name || !url) return null;
        return { id, name, url, disabled: !!(item as any).disabled };
      })
      .filter((item): item is OutboundProxyDefinition => !!item);
  } catch {
    return [];
  }
}

export function serializeOutboundProxies(proxies: OutboundProxyDefinition[]): string {
  return JSON.stringify(proxies);
}

export function proxyLabel(proxy: OutboundProxyDefinition): string {
  return `${proxy.name} · ${maskProxyURL(proxy.url)}`;
}

export function maskProxyURL(raw: string): string {
  try {
    const url = new URL(raw);
    if (url.username || url.password) {
      url.username = url.username ? '***' : '';
      url.password = url.password ? '***' : '';
    }
    return url.toString();
  } catch {
    return raw;
  }
}

export function isSupportedProxyURL(raw: string): boolean {
  try {
    const url = new URL(raw.trim());
    return ['http:', 'https:', 'socks5:', 'socks5h:'].includes(url.protocol) && !!url.host;
  } catch {
    return false;
  }
}
