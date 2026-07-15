import {
  ClientIcon,
  allClientTypes,
  getClientName,
  getClientColor,
} from '@/components/icons/client-icons';
import { usePublicSettings } from '@/hooks/queries/use-settings';
import { useStreamingRequests } from '@/hooks/use-streaming';
import type { ClientType } from '@/lib/transport';
import { AnimatedNavItem } from './animated-nav-item';

const clientRouteSettingKeys: Record<ClientType, string> = {
  claude: 'proxy_route_claude_messages_enabled',
  openai: 'proxy_route_openai_chat_enabled',
  codex: 'proxy_route_responses_enabled',
  gemini: 'proxy_route_gemini_enabled',
};

function isClientRouteVisible(
  settings: Record<string, string> | undefined,
  clientType: ClientType,
) {
  const settingValue = settings?.[clientRouteSettingKeys[clientType]];
  if (settingValue === undefined) {
    return clientType !== 'gemini';
  }
  return settingValue !== 'false';
}

function ClientNavItem({
  clientType,
  streamingCount,
}: {
  clientType: ClientType;
  streamingCount: number;
}) {
  const color = getClientColor(clientType);
  const clientName = getClientName(clientType);

  return (
    <AnimatedNavItem
      to={`/routes/${clientType}`}
      isActive={(pathname) => pathname === `/routes/${clientType}`}
      tooltip={clientName}
      icon={<ClientIcon type={clientType} size={18} />}
      label={clientName}
      streamingCount={streamingCount}
      color={color}
    />
  );
}

/**
 * Renders all client route items dynamically
 */
export function ClientRoutesItems() {
  const { countsByClient } = useStreamingRequests();
  const { data: publicSettings } = usePublicSettings();
  const visibleClientTypes = allClientTypes.filter((clientType) =>
    isClientRouteVisible(publicSettings, clientType),
  );

  return (
    <>
      {visibleClientTypes.map((clientType) => (
        <ClientNavItem
          key={clientType}
          clientType={clientType}
          streamingCount={countsByClient.get(clientType) || 0}
        />
      ))}
    </>
  );
}
