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
                  render={(props) => <NavLink to={item.to} {...props} />}
                  isActive={isActive}
                  size="lg"
                >
                  <Icon />
                  <span>{item.label}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            );
          })}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
