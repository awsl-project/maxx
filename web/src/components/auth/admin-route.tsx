import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useAuth } from '@/lib/auth-context';

interface AdminRouteProps {
  children: ReactNode;
}

export function AdminRoute({ children }: AdminRouteProps) {
  const { t } = useTranslation();
  const { authEnabled, user, isLoading, isAuthenticated } = useAuth();

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <span className="text-muted-foreground">{t('common.loading')}</span>
      </div>
    );
  }

  if (authEnabled) {
    if (!isAuthenticated || !user) {
      return <Navigate to="/" replace />;
    }
    if (user.role !== 'admin') {
      return <Navigate to="/" replace />;
    }
  }

  return <>{children}</>;
}
