import { Outlet } from 'react-router-dom'
import { SidebarNav } from './sidebar-nav'
import { SidebarProvider, SidebarInset } from '@/components/ui/sidebar'

export function AppLayout() {
  return (
    <SidebarProvider>
      <SidebarNav />
      <SidebarInset>
        <div className="@container/main">
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
