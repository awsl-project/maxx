/**
 * Client Routes Page (Global Routes)
 * 全局路由配置页面 - 显示当前 ClientType 的路由
 */

import { useState } from 'react';
import { useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Search } from 'lucide-react';
import { ClientIcon, getClientName } from '@/components/icons/client-icons';
import type { ClientType } from '@/lib/transport';
import { ClientTypeRoutesContent } from '@/components/routes/ClientTypeRoutesContent';
import { Input } from '@/components/ui/input';

export function ClientRoutesPage() {
  const { t } = useTranslation();
  const { clientType } = useParams<{ clientType: string }>();
  const activeClientType = (clientType as ClientType) || 'claude';
  const [searchQuery, setSearchQuery] = useState('');

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-card shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent/10 rounded-lg">
            <ClientIcon type={activeClientType} size={20} className="text-accent" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-foreground leading-tight">
              {getClientName(activeClientType)} Routes
            </h2>
            <p className="text-xs text-muted-foreground">
              Configure default routing for all projects
            </p>
          </div>
        </div>
        <div className="relative">
          <Search
            size={14}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            placeholder={t('common.searchProviders')}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9 w-48"
          />
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0 overflow-hidden">
        <ClientTypeRoutesContent
          clientType={activeClientType}
          projectID={0}
          searchQuery={searchQuery}
        />
      </div>
    </div>
  );
}
