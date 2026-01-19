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

      <SidebarFooter>
        <div className="flex items-center gap-2 group-data-[collapsible=icon]:flex-col group-data-[collapsible=icon]:w-full group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:items-stretch">
          <SidebarTrigger />
          <ThemeToggle />
        </div>
      </SidebarFooter>
    </Sidebar>
  );
}
