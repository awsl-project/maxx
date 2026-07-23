import {
  ClientIcon,
  allClientTypes,
  getClientName,
  getClientColor,
} from '@/components/icons/client-icons';
import { usePublicSettings } from '@/hooks/queries/use-settings';
import { useStreamingRequests } from '@/hooks/use-streaming';
import type { ClientType } from '@/lib/transport';
import { getVisibleProxyRouteClients } from '@/lib/proxy-route-exposure';
import { AnimatedNavItem } from './animated-nav-item';

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
  const visibleClientTypes = getVisibleProxyRouteClients(publicSettings, allClientTypes);

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
