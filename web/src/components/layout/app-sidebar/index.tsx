import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarTrigger,
} from '@/components/ui/sidebar';
import { NavProxyStatus } from '../nav-proxy-status';
import { ThemeToggle } from '@/components/theme-toggle';
import { SidebarRenderer } from './sidebar-renderer';
import { sidebarConfig } from './sidebar-config';

export function AppSidebar() {
  return (
    <Sidebar collapsible="icon" className="border-border">
      <SidebarHeader className="h-[73px] border-b border-border justify-center">
        <NavProxyStatus />
      </SidebarHeader>

      <SidebarContent>
        <SidebarRenderer config={sidebarConfig} />
      </SidebarContent>

      <SidebarFooter className="border-t border-border">
        <div className="flex items-center justify-between gap-2 group-data-[collapsible=icon]:flex-col group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:gap-1">
          <ThemeToggle />
          <SidebarTrigger />
        </div>
      </SidebarFooter>
    </Sidebar>
  );
}
