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
      <NavLink
        to={`/routes/${clientType}`}
        data-size="lg"
        className={({ isActive: linkIsActive }) =>
          `ring-sidebar-ring hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:bg-sidebar-accent active:text-sidebar-accent-foreground data-active:bg-sidebar-accent data-active:text-sidebar-accent-foreground gap-2 rounded-md p-2 text-left text-sm transition-[width,height,padding] group-has-data-[sidebar=menu-action]/menu-item:pr-8 group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:p-2! focus-visible:ring-2 data-active:font-medium peer/menu-button flex w-full items-center overflow-hidden outline-hidden group/menu-button disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50 [&>span:last-child]:truncate [&_svg]:size-4 [&_svg]:shrink-0 h-12 text-sm group-data-[collapsible=icon]:p-0! relative overflow-hidden group-data-[collapsible=icon]:justify-center ${isActive || linkIsActive ? 'bg-sidebar-accent text-sidebar-accent-foreground font-medium' : ''}`
        }
      >
        {/* Marquee 背景动画 (仅在有 streaming 请求且未激活时显示) */}
        {streamingCount > 0 && !isActive && (
          <div
            className="absolute inset-0 animate-marquee pointer-events-none opacity-50"
            style={{ backgroundColor: color }}
          />
        )}
        <ClientIcon type={clientType} size={18} className="relative z-10" />
        <span className="relative z-10 group-data-[collapsible=icon]:hidden">{clientName}</span>
      </NavLink>
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
