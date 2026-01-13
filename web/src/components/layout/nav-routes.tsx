import { NavLink, useLocation } from 'react-router-dom'
import {
  ClientIcon,
  allClientTypes,
  getClientName,
  getClientColor,
} from '@/components/icons/client-icons'
import { StreamingBadge } from '@/components/ui/streaming-badge'
import { useStreamingRequests } from '@/hooks/use-streaming'
import type { ClientType } from '@/lib/transport'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  SidebarMenuBadge,
} from '@/components/ui/sidebar'

function ClientNavItem({ clientType }: { clientType: ClientType }) {
  const location = useLocation()
  const { countsByClient } = useStreamingRequests()
  const streamingCount = countsByClient.get(clientType) || 0
  const color = getClientColor(clientType)
  const clientName = getClientName(clientType)
  const isActive = location.pathname === `/routes/${clientType}`

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        isActive={isActive}
        size="lg"
        tooltip={clientName}
        className="relative overflow-hidden group-data-[collapsible=icon]:justify-center"
      >
        {/* Marquee 背景动画 (仅在有 streaming 请求且未激活时显示) */}
        {streamingCount > 0 && !isActive && (
          <div
            className="absolute inset-0 animate-marquee pointer-events-none opacity-50"
            style={{ backgroundColor: color }}
          />
        )}
        <NavLink
          to={`/routes/${clientType}`}
          className="flex items-center gap-2 w-full h-full relative group-data-[collapsible=icon]:gap-0 group-data-[collapsible=icon]:justify-center cursor-pointer"
        >
          <ClientIcon type={clientType} size={18} className="relative z-10" />
          <span className="relative z-10 group-data-[collapsible=icon]:hidden">{clientName}</span>
        </NavLink>
      </SidebarMenuButton>
      {streamingCount > 0 && (
        <SidebarMenuBadge>
          <StreamingBadge count={streamingCount} color={color} />
        </SidebarMenuBadge>
      )}
    </SidebarMenuItem>
  )
}

export function NavRoutes() {
  return (
    <SidebarGroup>
      <SidebarGroupLabel>ROUTES</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {allClientTypes.map(clientType => (
            <ClientNavItem key={clientType} clientType={clientType} />
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}
