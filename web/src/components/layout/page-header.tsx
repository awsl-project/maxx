import type { LucideIcon } from 'lucide-react';
import type { ReactNode, ReactElement } from 'react';
import { isValidElement } from 'react';
import { SidebarTrigger } from '@/components/ui/sidebar';
import { useAuth } from '@/lib/auth-context';
import { useTranslation } from 'react-i18next';

interface PageHeaderProps {
  icon?: LucideIcon | ReactElement;
  iconClassName?: string;
  title: string;
  description?: string;
  actions?: ReactNode;
  children?: ReactNode;
}

export function PageHeader({
  icon: Icon,
  iconClassName = 'text-blue-500',
  title,
  description,
  actions,
  children,
}: PageHeaderProps) {
  const { user } = useAuth();
  const { t } = useTranslation();
  const accountName = user?.username?.trim() || '';
  const shouldShowAccount = accountName.length > 0;
  const hasActions = actions !== null && actions !== undefined && actions !== false;
  const hasChildren = children !== null && children !== undefined && children !== false;

  return (
    <header className="min-h-[73px] flex items-center justify-between px-4 md:px-6 gap-2 py-2 md:py-0 flex-wrap border-b border-border bg-card shrink-0">
      <div className="flex items-center gap-3">
        <SidebarTrigger className="-ml-2" />
        {Icon && (
          <div className="p-2 bg-secondary/50 rounded-lg border border-border/50">
            {isValidElement(Icon) ? Icon : <Icon size={20} className={iconClassName} />}
          </div>
        )}
        <div>
          <h1 className="text-lg font-semibold text-foreground leading-tight">{title}</h1>
          {description && <p className="text-xs text-muted-foreground">{description}</p>}
        </div>
      </div>
      {(shouldShowAccount || hasActions || hasChildren) && (
        <div className="flex items-center gap-2 flex-wrap">
          {shouldShowAccount && (
            <div className="inline-flex h-8 max-w-full items-center gap-1 rounded-full border border-border bg-secondary/40 px-3 text-xs font-medium text-foreground">
              <span className="text-muted-foreground">{t('nav.account')}:</span>
              <span className="max-w-[12rem] truncate">{accountName}</span>
            </div>
          )}
          {actions}
          {children}
        </div>
      )}
    </header>
  );
}
