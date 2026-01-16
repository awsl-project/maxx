import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { AppLayout } from '@/components/layout';
import { OverviewPage } from '@/pages/overview';
import { RequestsPage } from '@/pages/requests';
import { RequestDetailPage } from '@/pages/requests/detail';
import { ProvidersPage } from '@/pages/providers';
import { RoutesPage } from '@/pages/routes';
import { ClientRoutesPage } from '@/pages/client-routes';
import { ProjectsPage } from '@/pages/projects';
import { ProjectDetailPage } from '@/pages/projects/detail';
import { SessionsPage } from '@/pages/sessions';
import { RetryConfigsPage } from '@/pages/retry-configs';
import { RoutingStrategiesPage } from '@/pages/routing-strategies';
import { ConsolePage } from '@/pages/console';
import { SettingsPage } from '@/pages/settings';
import { LoginPage } from '@/pages/login';
import { AuthProvider, useAuth } from '@/lib/auth-context';

function AppRoutes() {
  const { isAuthenticated, isLoading, login } = useAuth();

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <span className="text-muted-foreground">Loading...</span>
      </div>
    );
  }

  if (!isAuthenticated) {
    return <LoginPage onSuccess={login} />;
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<AppLayout />}>
          <Route index element={<OverviewPage />} />
          <Route path="requests" element={<RequestsPage />} />
          <Route path="requests/:id" element={<RequestDetailPage />} />
          <Route path="console" element={<ConsolePage />} />
          <Route path="providers" element={<ProvidersPage />} />
          <Route path="routes" element={<RoutesPage />} />
          <Route path="routes/:clientType" element={<ClientRoutesPage />} />
          <Route path="projects" element={<ProjectsPage />} />
          <Route path="projects/:slug" element={<ProjectDetailPage />} />
          <Route path="sessions" element={<SessionsPage />} />
          <Route path="retry-configs" element={<RetryConfigsPage />} />
          <Route path="routing-strategies" element={<RoutingStrategiesPage />} />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

function App() {
  return (
    <AuthProvider>
      <AppRoutes />
    </AuthProvider>
  );
}

export default App;
