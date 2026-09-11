import { Fragment } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { SidebarConfig, MenuItem } from '@/types/sidebar';
import { useAuth } from '@/lib/auth-context';
import { usePublicSettings } from '@/hooks/queries';
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar';

interface SidebarRendererProps {
  config: SidebarConfig;
}

export function shouldRenderSidebarItem(
  item: MenuItem,
  {
    settings,
    isAdmin,
    authEnabled,
  }: { settings?: Record<string, string>; isAdmin: boolean; authEnabled: boolean },
) {
  const multiTenantUIEnabled = settings?.ui_multitenant_enabled === 'true';
  const testFieldTabEnabled = settings?.ui_test_field_tab_enabled === 'true';
  const externalModelsTabEnabled = settings?.external_model_list_enabled === 'true';
  const proxyManagementTabEnabled = settings?.ui_proxy_management_enabled === 'true';

  if (
    !multiTenantUIEnabled &&
    item.type === 'standard' &&
    (item.key === 'invite-codes' || item.key === 'users')
  ) {
    return false;
  }
  if (item.type === 'standard' && item.key === 'test-field' && !testFieldTabEnabled) {
    return false;
  }
  if (item.type === 'standard' && item.key === 'external-models' && !externalModelsTabEnabled) {
    return false;
  }
  if (item.type === 'standard' && item.key === 'proxies' && !proxyManagementTabEnabled) {
    return false;
  }
  if (item.type === 'standard' && item.adminOnly && !isAdmin) {
    return false;
  }
  if (item.type === 'standard' && item.authOnly && !authEnabled) {
    return false;
  }
  return true;
}

/**
 * Renders a single menu item based on its type
 */
function MenuItemRenderer({ item }: { item: MenuItem }) {
  const location = useLocation();
  const { t } = useTranslation();

  switch (item.type) {
    case 'standard': {
      const Icon = item.icon;
      const isActive =
        item.activeMatch === 'exact'
          ? location.pathname === item.to
          : location.pathname.startsWith(item.to);

      return (
        <SidebarMenuItem key={item.key}>
          <SidebarMenuButton
            render={<NavLink to={item.to} />}
            isActive={isActive}
            tooltip={t(item.labelKey)}
          >
            <Icon />
            <span>{t(item.labelKey)}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      );
    }

    case 'custom': {
      const Component = item.component;
      return <Component key={item.key} />;
    }

    case 'dynamic-section': {
      return <Fragment key={item.key}>{item.generator()}</Fragment>;
    }

    default:
      return null;
  }
}

/**
 * Unified sidebar renderer that handles all menu item types
 */
export function SidebarRenderer({ config }: SidebarRendererProps) {
  const { t } = useTranslation();
  const { user, authEnabled } = useAuth();
  const publicSettings = usePublicSettings();
  const isAdmin = !user || user.role === 'admin';

  return (
    <>
      {config.sections.map((section) => {
        const filteredItems = section.items.filter((item) =>
          shouldRenderSidebarItem(item, { settings: publicSettings.data, isAdmin, authEnabled }),
        );

        if (filteredItems.length === 0) return null;

        return (
          <SidebarGroup key={section.key}>
            {section.titleKey && <SidebarGroupLabel>{t(section.titleKey)}</SidebarGroupLabel>}
            <SidebarGroupContent>
              <SidebarMenu>
                {filteredItems.map((item) => (
                  <MenuItemRenderer key={item.key} item={item} />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        );
      })}
    </>
  );
}
