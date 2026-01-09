import { useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { cn } from '@/lib/utils';
import {
  LayoutDashboard,
  Activity,
  Server,
  FolderKanban,
  Users,
  RefreshCw,
  Shuffle,
  Radio,
  Check,
  Copy,
  Terminal,
  Settings,
} from 'lucide-react';
import { ClientIcon, allClientTypes, getClientName, getClientColor } from '@/components/icons/client-icons';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { useStreamingRequests } from '@/hooks/use-streaming';
import { useProxyStatus } from '@/hooks/queries';
import type { ClientType } from '@/lib/transport';

interface NavItem {
  to?: string;
  icon?: React.ComponentType<{ className?: string }>;
  label?: string;
  type?: 'divider' | 'section';
  sectionTitle?: string;
}

const mainNavItems: NavItem[] = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/console', icon: Terminal, label: 'Console' },
];

const managementItems: NavItem[] = [
  { to: '/providers', icon: Server, label: 'Providers' },
  { to: '/projects', icon: FolderKanban, label: 'Projects' },
  { to: '/sessions', icon: Users, label: 'Sessions' },
];

const configItems: NavItem[] = [
  { to: '/retry-configs', icon: RefreshCw, label: 'Retry Configs' },
  { to: '/routing-strategies', icon: Shuffle, label: 'Strategies' },
  { to: '/settings', icon: Settings, label: 'Settings' },
];

/**
 * Requests 导航项 - 带 Streaming Badge
 */
function RequestsNavItem() {
  const location = useLocation();
  const { total } = useStreamingRequests();
  const isActive = location.pathname === '/requests';
  const color = '#10B981'; // emerald-500

  return (
    <NavLink
      to="/requests"
      className={({ isActive }) =>
        cn(
          'sidebar-item relative overflow-hidden',
          isActive && 'sidebar-item-active'
        )
      }
    >
      {/* Marquee 背景动画 (仅在有 streaming 请求且未激活时显示) */}
      {total > 0 && !isActive && (
        <div
          className="absolute inset-0 animate-marquee pointer-events-none opacity-50"
          style={{ backgroundColor: `${color}10` }}
        />
      )}
      <Activity className="h-4 w-4 relative z-10" />
      <span className="flex-1 relative z-10 text-body">Requests</span>
      <StreamingBadge count={total} color={color} />
    </NavLink>
  );
}

/**
 * 客户端路由项 - 带 Streaming Badge
 */
function ClientNavItem({ clientType }: { clientType: ClientType }) {
  const location = useLocation();
  const { countsByClient } = useStreamingRequests();
  const streamingCount = countsByClient.get(clientType) || 0;
  const color = getClientColor(clientType);
  const isActive = location.pathname === `/routes/${clientType}`;

  return (
    <NavLink
      to={`/routes/${clientType}`}
      className={({ isActive }) =>
        cn(
          'sidebar-item relative overflow-hidden',
          isActive && 'sidebar-item-active'
        )
      }
    >
      {/* Marquee 背景动画 (仅在有 streaming 请求且未激活时显示) */}
      {streamingCount > 0 && !isActive && (
        <div
          className="absolute inset-0 animate-marquee pointer-events-none opacity-50"
          style={{ backgroundColor: `${color}10` }}
        />
      )}
      <ClientIcon type={clientType} size={18} className="relative z-10" />
      <span className="flex-1 relative z-10 text-body">{getClientName(clientType)}</span>
      <StreamingBadge count={streamingCount} color={color} />
    </NavLink>
  );
}

/**
 * 导航项组件
 */
function NavItemComponent({ item }: { item: NavItem }) {
  const Icon = item.icon!;
  return (
    <NavLink
      to={item.to!}
      end={item.to === '/'}
      className={({ isActive }) =>
        cn(
          'sidebar-item',
          isActive && 'sidebar-item-active'
        )
      }
    >
      <Icon className="h-4 w-4" />
      <span className="text-body">{item.label}</span>
    </NavLink>
  );
}

export function SidebarNav() {
  const { data: proxyStatus } = useProxyStatus();
  const [copied, setCopied] = useState(false);

  const proxyAddress = proxyStatus?.address ?? '...';
  const fullUrl = `http://${proxyAddress}`;

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(fullUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <aside className="sidebar">
      {/* Proxy Status - Click to copy */}
      <div className="h-[73px] p-lg border-b border-border flex items-center">
        <button
          onClick={handleCopy}
          className="flex items-center gap-sm group w-full rounded-lg p-1 -m-1 hover:bg-surface-hover transition-colors"
          title={`Click to copy: ${fullUrl}`}
        >
          <div className="w-8 h-8 rounded-lg bg-emerald-400/10 flex items-center justify-center flex-shrink-0">
            <Radio size={16} className="text-emerald-400" />
          </div>
          <div className="flex flex-col items-start flex-1 min-w-0">
            <span className="text-caption text-text-muted">Listening on</span>
            <span className="text-body font-mono font-medium text-text-primary truncate">{proxyAddress}</span>
          </div>
          <div className="flex-shrink-0 text-text-muted">
            {copied ? (
              <Check size={14} className="text-emerald-400" />
            ) : (
              <Copy size={14} className="opacity-0 group-hover:opacity-100 transition-opacity" />
            )}
          </div>
        </button>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto py-md">
        {/* Main Navigation */}
        {mainNavItems.map((item) => (
          <NavItemComponent key={item.to} item={item} />
        ))}
        <RequestsNavItem />

        {/* Routes Section - Dynamic Client List */}
        <div className="sidebar-section-title">ROUTES</div>
        {allClientTypes.map((clientType) => (
          <ClientNavItem key={clientType} clientType={clientType} />
        ))}

        {/* Management Section */}
        <div className="sidebar-section-title">MANAGEMENT</div>
        {managementItems.map((item) => (
          <NavItemComponent key={item.to} item={item} />
        ))}

        {/* Config Section */}
        <div className="sidebar-section-title">CONFIG</div>
        {configItems.map((item) => (
          <NavItemComponent key={item.to} item={item} />
        ))}
      </nav>

      {/* Footer */}
      <div className="px-lg py-md border-t border-border">
        <p className="text-caption text-text-muted">v0.1.0</p>
      </div>
    </aside>
  );
}
