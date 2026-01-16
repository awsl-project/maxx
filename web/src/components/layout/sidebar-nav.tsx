import { NavLink, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  LayoutDashboard,
  Activity,
  Server,
  FolderKanban,
  Users,
  RefreshCw,
  Terminal,
  Settings,
  Key,
  BarChart3,
} from 'lucide-react'
import { StreamingBadge } from '@/components/ui/streaming-badge'
import { useStreamingRequests } from '@/hooks/use-streaming'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarTrigger,
} from '@/components/ui/sidebar'
import { NavMain } from './nav-main'
import { NavRoutes } from './nav-routes'
import { NavManagement } from './nav-management'
import { NavProxyStatus } from './nav-proxy-status'
import { ThemeToggle } from '@/components/theme-toggle'

const mainNavItems = [
  { to: '/', icon: LayoutDashboard, labelKey: 'nav.dashboard' },
  { to: '/console', icon: Terminal, labelKey: 'nav.console' },
]

const managementItems = [
  { to: '/providers', icon: Server, labelKey: 'nav.providers' },
  { to: '/projects', icon: FolderKanban, labelKey: 'nav.projects' },
  { to: '/sessions', icon: Users, labelKey: 'nav.sessions' },
  { to: '/api-tokens', icon: Key, labelKey: 'nav.apiTokens' },
]

const configItems = [
  { to: '/retry-configs', icon: RefreshCw, labelKey: 'nav.retryConfigs' },
  { to: '/settings', icon: Settings, labelKey: 'nav.settings' },
]

/**
 * Requests 导航项 - 带 Streaming Badge
 */
function RequestsNavItem() {
  const location = useLocation()
  const { total } = useStreamingRequests()
  const { t } = useTranslation()
  const isActive = location.pathname.startsWith('/requests')
  const color = 'var(--color-success)' // emerald-500

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<NavLink to="/requests" />}
        isActive={isActive}
        tooltip={t('requests.title')}
        className="relative"
      >
        {/* Marquee 背景动画 (仅在有 streaming 请求且未激活时显示) */}
        {total > 0 && !isActive && (
          <div
            className="absolute inset-0 animate-marquee pointer-events-none opacity-40"
            style={{ backgroundColor: color }}
          />
        )}
        <Activity className="relative z-10" />
        <span className="relative z-10">{t('requests.title')}</span>
      </SidebarMenuButton>
      {total > 0 && (
        <SidebarMenuBadge>
          <StreamingBadge count={total} color={color} />
        </SidebarMenuBadge>
      )}
    </SidebarMenuItem>
  )
}

export function SidebarNav() {
  const { t } = useTranslation()
  const location = useLocation()
  const versionDisplay = `v${__APP_VERSION__}`
  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <NavProxyStatus />
      </SidebarHeader>

      <SidebarContent>
        <NavMain items={mainNavItems}>
          <RequestsNavItem />
          <SidebarMenuItem>
            <SidebarMenuButton
              render={<NavLink to="/stats" />}
              isActive={location.pathname.startsWith('/stats')}
              tooltip={t('nav.stats')}
            >
              <BarChart3 />
              <span>{t('nav.stats')}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </NavMain>
        <NavRoutes />
        <NavManagement items={managementItems} title={t('nav.management')} />
        <NavManagement items={configItems} title={t('nav.config')} />
      </SidebarContent>

      <SidebarFooter>
        <p className="text-caption text-muted-foreground group-data-[collapsible=icon]:hidden mb-2">
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
