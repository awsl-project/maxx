import {
  LayoutDashboard,
  Server,
  FolderKanban,
  Users,
  RefreshCw,
  Terminal,
  Settings,
  Key,
  Zap,
  BarChart3,
  DollarSign,
} from 'lucide-react';
import type { SidebarConfig } from '@/types/sidebar';
import { RequestsNavItem } from './requests-nav-item';
import { ClientRoutesItems } from './client-routes-items';

/**
 * Unified sidebar configuration
 * All menu items are defined here in a single source of truth
 */
export const sidebarConfig: SidebarConfig = {
  sections: [
    {
      key: 'main',
      items: [
        {
          type: 'standard',
          key: 'dashboard',
          to: '/',
          icon: LayoutDashboard,
          labelKey: 'nav.dashboard',
          activeMatch: 'exact',
        },
        {
          type: 'standard',
          key: 'console',
          to: '/console',
          icon: Terminal,
          labelKey: 'nav.console',
        },
        {
          type: 'standard',
          key: 'stats',
          to: '/stats',
          icon: BarChart3,
          labelKey: 'nav.stats',
        },
        {
          type: 'custom',
          key: 'requests',
          component: RequestsNavItem,
        },
      ],
    },
    {
      key: 'routes',
      titleKey: 'nav.routes',
      items: [
        {
          type: 'dynamic-section',
          key: 'client-routes',
          generator: () => <ClientRoutesItems />,
        },
      ],
    },
    {
      key: 'management',
      titleKey: 'nav.management',
      items: [
        {
          type: 'standard',
          key: 'providers',
          to: '/providers',
          icon: Server,
          labelKey: 'nav.providers',
        },
        {
          type: 'standard',
          key: 'projects',
          to: '/projects',
          icon: FolderKanban,
          labelKey: 'nav.projects',
        },
        {
          type: 'standard',
          key: 'sessions',
          to: '/sessions',
          icon: Users,
          labelKey: 'nav.sessions',
        },
        {
          type: 'standard',
          key: 'api-tokens',
          to: '/api-tokens',
          icon: Key,
          labelKey: 'nav.apiTokens',
        },
      ],
    },
    {
      key: 'config',
      titleKey: 'nav.config',
      items: [
        {
          type: 'standard',
          key: 'model-mappings',
          to: '/model-mappings',
          icon: Zap,
          labelKey: 'nav.modelMappings',
        },
        {
          type: 'standard',
          key: 'model-prices',
          to: '/model-prices',
          icon: DollarSign,
          labelKey: 'nav.modelPrices',
        },
        {
          type: 'standard',
          key: 'retry-configs',
          to: '/retry-configs',
          icon: RefreshCw,
          labelKey: 'nav.retryConfigs',
        },
        {
          type: 'standard',
          key: 'settings',
          to: '/settings',
          icon: Settings,
          labelKey: 'nav.settings',
        },
      ],
    },
  ],
};
