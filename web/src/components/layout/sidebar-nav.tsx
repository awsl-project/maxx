import { NavLink, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  Activity,
  Server,
  FolderKanban,
  Users,
  RefreshCw,
  Terminal,
  Settings,
} from 'lucide-react'
import { StreamingBadge } from '@/components/ui/streaming-badge'
import { useStreamingRequests } from '@/hooks/use-streaming'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenuBadge,
  SidebarMenuItem,
  SidebarTrigger,
} from '@/components/ui/sidebar'
import { NavMain } from './nav-main'
import { NavRoutes } from './nav-routes'
import { NavManagement } from './nav-management'
import { NavProxyStatus } from './nav-proxy-status'
import { ThemeToggle } from '@/components/theme-toggle'

const mainNavItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/console', icon: Terminal, label: 'Console' },
]

const managementItems = [
  { to: '/providers', icon: Server, label: 'Providers' },
  { to: '/projects', icon: FolderKanban, label: 'Projects' },
  { to: '/sessions', icon: Users, label: 'Sessions' },
]

const configItems = [
  { to: '/retry-configs', icon: RefreshCw, label: 'Retry Configs' },
  { to: '/settings', icon: Settings, label: 'Settings' },
]

/**
 * Requests 导航项 - 带 Streaming Badge
 */
function RequestsNavItem() {
  const location = useLocation()
  const { total } = useStreamingRequests()
  const isActive = location.pathname.startsWith('/requests')
  const color = 'var(--color-success)' // emerald-500

  return (
    <SidebarMenuItem>
      <NavLink
        to="/requests"
        className={({ isActive: linkIsActive }) =>
          `ring-sidebar-ring hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:bg-sidebar-accent active:text-sidebar-accent-foreground data-active:bg-sidebar-accent data-active:text-sidebar-accent-foreground gap-2 rounded-md p-2 text-left text-sm transition-[width,height,padding] group-has-data-[sidebar=menu-action]/menu-item:pr-8 group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:p-2! focus-visible:ring-2 data-active:font-medium peer/menu-button flex w-full items-center overflow-hidden outline-hidden group/menu-button disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50 [&>span:last-child]:truncate [&_svg]:size-4 [&_svg]:shrink-0 h-12 text-sm group-data-[collapsible=icon]:p-0! relative overflow-hidden min-w-8 duration-200 ease-linear ${isActive || linkIsActive ? 'bg-sidebar-accent text-sidebar-accent-foreground font-medium' : ''}`
        }
      >
        {/* Marquee 背景动画 (仅在有 streaming 请求且未激活时显示) */}
        {total > 0 && !isActive && (
          <div
            className="absolute inset-0 animate-marquee pointer-events-none opacity-40"
            style={{ backgroundColor: color }}
          />
        )}
        <Activity className="relative z-10" />
        <span className="relative z-10 group-data-[collapsible=icon]:hidden">
          Requests
        </span>
      </NavLink>
      {total > 0 && (
        <SidebarMenuBadge>
          <StreamingBadge count={total} color={color} />
        </SidebarMenuBadge>
      )}
    </SidebarMenuItem>
  )
}

export function SidebarNav() {
  const versionDisplay =
    `v${__APP_VERSION__}` +
    (__APP_COMMIT__ !== 'unknown' ? ` (${__APP_COMMIT__})` : '')

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <NavProxyStatus />
      </SidebarHeader>

      <SidebarContent>
        <NavMain items={mainNavItems}>
          <RequestsNavItem />
        </NavMain>
        <NavRoutes />
        <NavManagement items={managementItems} title="MANAGEMENT" />
        <NavManagement items={configItems} title="CONFIG" />
      </SidebarContent>

      <SidebarFooter>
        <p className="text-caption text-text-muted group-data-[collapsible=icon]:hidden mb-2">
          {versionDisplay}
        </p>
        <div className="flex items-center gap-2 group-data-[collapsible=icon]:flex-col group-data-[collapsible=icon]:w-full group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:items-stretch">
          <SidebarTrigger />
          <ThemeToggle />
        </div>
      </SidebarFooter>
    </Sidebar>
  )
}
