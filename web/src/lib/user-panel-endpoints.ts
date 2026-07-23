import { isProxyRouteVisible } from '@/lib/proxy-route-exposure';

export interface UserPanelEndpointHint {
  id: 'openai-codex' | 'claude' | 'gemini';
  url: string;
}

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, '');
}

export function buildUserPanelEndpointHints(
  origin: string,
  settings?: Record<string, string>,
): UserPanelEndpointHint[] {
  const baseUrl = trimTrailingSlash(origin);
  const endpoints: UserPanelEndpointHint[] = [];

  if (isProxyRouteVisible(settings, 'openai') || isProxyRouteVisible(settings, 'codex')) {
    endpoints.push({ id: 'openai-codex', url: `${baseUrl}/v1` });
  }
  if (isProxyRouteVisible(settings, 'claude')) {
    endpoints.push({ id: 'claude', url: baseUrl });
  }
  if (isProxyRouteVisible(settings, 'gemini')) {
    endpoints.push({ id: 'gemini', url: `${baseUrl}/v1beta/models/{model}:generateContent` });
  }

  return endpoints;
}

export function buildUserPanelChatCompletionsExample(params: { origin: string }): string {
  const baseUrl = trimTrailingSlash(params.origin);
  return `curl ${baseUrl}/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'`;
}
