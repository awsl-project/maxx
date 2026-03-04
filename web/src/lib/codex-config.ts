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

export function buildProxyBaseUrl(proxyStatus?: ProxyStatusLike | null): string {
  const address = (proxyStatus?.address || '').trim();
  const fallbackAddress = `localhost:${proxyStatus?.port || 9880}`;
  return normalizeBaseUrl(address || fallbackAddress);
}

export function buildCodexConfigBundle(params: {
  token: string;
  baseUrl: string;
  providerName?: string;
}): CodexConfigBundle {
  const providerName = params.providerName || 'maxx';
  const token = params.token.trim();
  const baseUrl = normalizeBaseUrl(params.baseUrl);

  const configToml = `# Optional: set as default provider
model_provider = "${providerName}"

[model_providers.${providerName}]
name = "${providerName}"
base_url = "${baseUrl}"
wire_api = "responses"
request_max_retries = 4
stream_max_retries = 10
stream_idle_timeout_ms = 300000`;

  const authJson = `{
  "OPENAI_API_KEY": "${token}"
}`;

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
