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
  const scheme = getProxyScheme(raw);
  if (['vmess', 'vless', 'trojan', 'ss'].includes(scheme)) {
    return `${scheme}://***`;
  }
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

function getProxyScheme(raw: string): string {
  const trimmed = raw.trim();
  const separator = trimmed.indexOf('://');
  if (separator <= 0) return '';
  return trimmed.slice(0, separator).toLowerCase();
}

function validPort(value: string | null): boolean {
  if (!value || !/^\d+$/.test(value)) return false;
  const port = Number(value);
  return port > 0 && port <= 65535;
}

function decodeBase64Loose(value: string): string | null {
  const normalized = value.trim().replace(/-/g, '+').replace(/_/g, '/');
  if (!normalized) return null;
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4);
  try {
    return atob(padded);
  } catch {
    return null;
  }
}

function isSupportedVMessURL(raw: string): boolean {
  const decoded = decodeBase64Loose(raw.slice('vmess://'.length));
  if (!decoded) return false;
  try {
    const payload = JSON.parse(decoded) as { add?: unknown; port?: unknown; id?: unknown };
    return (
      typeof payload.add === 'string' &&
      payload.add.trim() !== '' &&
      typeof payload.id === 'string' &&
      payload.id.trim() !== '' &&
      validPort(String(payload.port ?? ''))
    );
  } catch {
    return false;
  }
}

function isSupportedURLNode(raw: string, scheme: 'vless' | 'trojan' | 'ss'): boolean {
  try {
    const url = new URL(raw);
    if (url.protocol !== `${scheme}:` || !url.hostname || !validPort(url.port)) return false;
    if (scheme === 'ss') {
      const user = url.username ? `${url.username}${url.password ? `:${url.password}` : ''}` : '';
      if (!user) return false;
      if (url.password) return true;
      const decoded = decodeBase64Loose(user);
      return !!decoded && decoded.includes(':');
    }
    return !!url.username;
  } catch {
    return false;
  }
}

export function isSupportedProxyURL(raw: string): boolean {
  const trimmed = raw.trim();
  const scheme = getProxyScheme(trimmed);
  if (scheme === 'vmess') return isSupportedVMessURL(trimmed);
  if (scheme === 'vless' || scheme === 'trojan' || scheme === 'ss') return isSupportedURLNode(trimmed, scheme);
  try {
    const url = new URL(trimmed);
    return ['http:', 'https:', 'socks5:', 'socks5h:'].includes(url.protocol) && !!url.host;
  } catch {
    return false;
  }
}
