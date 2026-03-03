import { Fragment } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { SidebarConfig, MenuItem } from '@/types/sidebar';
import { useAuth } from '@/lib/auth-context';
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
  const { user } = useAuth();
  const isAdmin = !user || user.role === 'admin';

  return (
    <>
      {config.sections.map((section) => {
        const filteredItems = section.items.filter((item) => {
          if (item.type === 'standard' && item.adminOnly && !isAdmin) {
            return false;
          }
          return true;
        });

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
