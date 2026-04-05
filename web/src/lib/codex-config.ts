export interface CodexConfigBundle {
  baseUrl: string;
  configToml: string;
  authJson: string;
  combined: string;
}

type ProxyStatusLike = {
  address?: string;
  port?: number;
};

function normalizeBaseUrl(address: string): string {
  const trimmedAddress = address.trim().replace(/\/+$/, '');
  if (/^https?:\/\//i.test(trimmedAddress)) {
    return trimmedAddress;
  }

  const protocol =
    typeof window !== 'undefined' && window.location.protocol === 'https:' ? 'https' : 'http';
  return `${protocol}://${trimmedAddress}`;
}

function ensureBaseUrlPort(address: string, fallbackPort: number): string {
  const normalized = normalizeBaseUrl(address);
  try {
    const parsed = new URL(normalized);
    if (parsed.port || !parsed.hostname) {
      return normalized;
    }
    if (parsed.protocol !== 'http:') {
      return parsed.toString().replace(/\/+$/, '');
    }
    parsed.port = String(fallbackPort);
    return parsed.toString().replace(/\/+$/, '');
  } catch {
    return normalized;
  }
}

export function buildProxyBaseUrl(proxyStatus?: ProxyStatusLike | null): string {
  const address = (proxyStatus?.address || '').trim();
  const fallbackPort = proxyStatus?.port || 9880;

  if (!address) {
    return normalizeBaseUrl(`localhost:${fallbackPort}`);
  }

  return ensureBaseUrlPort(address, fallbackPort);
}

function ensureCodexBasePath(address: string): string {
  const normalized = normalizeBaseUrl(address);

  try {
    const parsed = new URL(normalized);
    if (parsed.pathname === '/v1' || parsed.pathname.startsWith('/v1/')) {
      return parsed.toString().replace(/\/+$/, '');
    }

    const nextPath = `${parsed.pathname.replace(/\/+$/, '')}/v1`.replace(/\/+/g, '/');
    parsed.pathname = nextPath;
    return parsed.toString().replace(/\/+$/, '');
  } catch {
    return normalized.replace(/\/+$/, '') + '/v1';
  }
}

export function buildCodexConfigBundle(params: {
  token: string;
  baseUrl: string;
  providerName?: string;
}): CodexConfigBundle {
  const providerName = params.providerName || 'maxx';
  const token = params.token.trim();
  const baseUrl = ensureCodexBasePath(params.baseUrl);

  const configToml = `# Optional: set as default provider
model_provider = "${providerName}"

[model_providers.${providerName}]
name = "${providerName}"
base_url = "${baseUrl}"
wire_api = "responses"
request_max_retries = 4
stream_max_retries = 10
stream_idle_timeout_ms = 300000`;

  const authJson = JSON.stringify({ OPENAI_API_KEY: token }, null, 2);

  const combined = `# ~/.codex/config.toml
${configToml}

# ~/.codex/auth.json
${authJson}
`;

  return {
    baseUrl,
    configToml,
    authJson,
    combined,
  };
}

export const __codexConfigTestUtils = {
  ensureCodexBasePath,
};
