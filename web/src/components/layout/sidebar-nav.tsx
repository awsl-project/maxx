import { NavLink, useLocation } from 'react-router-dom';
import {
  LayoutDashboard,
  Activity,
  Server,
  FolderKanban,
  Users,
  RefreshCw,
  Terminal,
  Settings,
} from 'lucide-react';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { useStreamingRequests } from '@/hooks/use-streaming';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar';
import { NavMain } from './nav-main';
import { NavRoutes } from './nav-routes';
import { NavManagement } from './nav-management';
import { NavProxyStatus } from './nav-proxy-status';

const mainNavItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/console', icon: Terminal, label: 'Console' },
];

const managementItems = [
  { to: '/providers', icon: Server, label: 'Providers' },
  { to: '/projects', icon: FolderKanban, label: 'Projects' },
  { to: '/sessions', icon: Users, label: 'Sessions' },
];

const configItems = [
  { to: '/retry-configs', icon: RefreshCw, label: 'Retry Configs' },
  { to: '/settings', icon: Settings, label: 'Settings' },
];

/**
 * Requests 导航项 - 带 Streaming Badge
 */
function RequestsNavItem() {
  const location = useLocation();
  const { total } = useStreamingRequests();
  const isActive = location.pathname.startsWith('/requests');
  const color = 'var(--color-success)'; // emerald-500

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={(props) => <NavLink to="/requests" {...props} />}
        isActive={isActive}
        size="lg"
        className="min-w-8 duration-200 ease-linear"
      >
        {/* Marquee 背景动画 (仅在有 streaming 请求且未激活时显示) */}
        {total > 0 && !isActive && (
          <div
            className="absolute inset-0 animate-marquee pointer-events-none opacity-50"
            style={{ backgroundColor: `${color}10` }}
          />
        )}
        <Activity className="relative z-10" />
        <span className="relative z-10">Requests</span>
      </SidebarMenuButton>
      {total > 0 && (
        <SidebarMenuBadge>
          <StreamingBadge count={total} color={color} />
        </SidebarMenuBadge>
      )}
    </SidebarMenuItem>
  );
}

export function SidebarNav() {
  const versionDisplay =
    `v${__APP_VERSION__}` + (__APP_COMMIT__ !== 'unknown' ? ` (${__APP_COMMIT__})` : '');

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
        <p className="text-caption text-text-muted px-2">{versionDisplay}</p>
      </SidebarFooter>
    </Sidebar>
  );
}
