import { NavLink, useLocation } from 'react-router-dom';
import type { LucideIcon } from 'lucide-react';
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
} from '@/components/ui/sidebar';

interface NavItem {
  to: string;
  icon: LucideIcon;
  label: string;
}

interface NavManagementProps {
  items: NavItem[];
  title?: string;
}

export function NavManagement({ items, title }: NavManagementProps) {
  const location = useLocation();

  return (
    <SidebarGroup>
      {title && <SidebarGroupLabel>{title}</SidebarGroupLabel>}
      <SidebarGroupContent>
        <SidebarMenu>
          {items.map((item) => {
            const Icon = item.icon;
            const isActive = location.pathname.startsWith(item.to);

            return (
              <SidebarMenuItem key={item.to}>
                <SidebarMenuButton
                  isActive={isActive}
                  size="lg"
                  tooltip={item.label}
                >
                  <NavLink to={item.to} className="flex items-center gap-2 w-full h-full group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:gap-0">
                    <Icon />
                    <span className="group-data-[collapsible=icon]:hidden">{item.label}</span>
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            );
          })}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
