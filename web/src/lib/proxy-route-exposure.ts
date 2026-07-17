import type { ClientType } from '@/lib/transport';

export const proxyRouteExposureSettingKeys: Record<ClientType, string> = {
  claude: 'proxy_route_claude_messages_enabled',
  openai: 'proxy_route_openai_chat_enabled',
  codex: 'proxy_route_responses_enabled',
  gemini: 'proxy_route_gemini_enabled',
};

export function isProxyRouteVisible(
  settings: Record<string, string> | undefined,
  clientType: ClientType,
): boolean {
  const settingValue = settings?.[proxyRouteExposureSettingKeys[clientType]];
  if (settingValue === undefined) {
    return clientType !== 'gemini';
  }
  return settingValue !== 'false';
}

export function getVisibleProxyRouteClients(
  settings: Record<string, string> | undefined,
  clientTypes: readonly ClientType[],
): ClientType[] {
  return clientTypes.filter((clientType) => isProxyRouteVisible(settings, clientType));
}
