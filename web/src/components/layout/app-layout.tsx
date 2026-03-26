import { Outlet } from 'react-router-dom';
import { AppSidebar } from './app-sidebar';
import { SidebarProvider, SidebarInset } from '@/components/ui/sidebar';
import { ForceProjectDialog } from '@/components/force-project-dialog';
import { usePendingSession } from '@/hooks/use-pending-session';
import { usePublicSettings } from '@/hooks/queries';

export function AppLayout() {
  const { pendingSession, clearPendingSession } = usePendingSession();
  const { data: settings } = usePublicSettings();

  const forceProjectEnabled = settings?.force_project_binding === 'true';
  const timeoutSeconds = parseInt(settings?.force_project_timeout || '30', 10);

  return (
    <>
      <SidebarProvider className="h-svh! min-h-0! overflow-hidden">
        <AppSidebar />
        <SidebarInset className="flex flex-col">
          <div className="@container/main flex-1 min-h-0 overflow-hidden">
            <Outlet />
          </div>
        </SidebarInset>
      </SidebarProvider>

      {/* Force Project Dialog - render outside SidebarProvider to avoid z-index issues */}
      {forceProjectEnabled && (
        <ForceProjectDialog
          event={pendingSession}
          onClose={clearPendingSession}
          timeoutSeconds={timeoutSeconds}
        />
      )}
    </>
  );
}
