/**
 * Client Routes Page (Global Routes)
 * 全局路由配置页面 - 显示当前 ClientType 的路由
 */

import { useParams } from 'react-router-dom'
import { ClientIcon, getClientName } from '@/components/icons/client-icons'
import type { ClientType } from '@/lib/transport'
import { ClientTypeRoutesContent } from '@/components/routes/ClientTypeRoutesContent'

export function ClientRoutesPage() {
  const { clientType } = useParams<{ clientType: string }>()
  const activeClientType = (clientType as ClientType) || 'claude'

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary flex-shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent/10 rounded-lg">
            <ClientIcon
              type={activeClientType}
              size={20}
              className="text-accent"
            />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-text-primary leading-tight">
              {getClientName(activeClientType)} Routes
            </h2>
            <p className="text-xs text-text-secondary">
              Configure default routing for all projects
            </p>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0 overflow-hidden">
        <ClientTypeRoutesContent clientType={activeClientType} projectID={0} />
      </div>
    </div>
  )
}
