import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

interface PageHeaderProps {
  icon?: LucideIcon
  iconClassName?: string
  title: string
  description?: string
  actions?: ReactNode
  children?: ReactNode
}

export function PageHeader({
  icon: Icon,
  iconClassName = 'text-blue-500',
  title,
  description,
  actions,
  children,
}: PageHeaderProps) {
  return (
    <header className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary shrink-0">
      <div className="flex items-center gap-3">
        {Icon && (
          <div className="p-2 bg-gradient-to-br from-surface-secondary/80 to-surface-secondary/40 rounded-lg shadow-sm">
            <Icon size={20} className={iconClassName} />
          </div>
        )}
        <div>
          <h1 className="text-lg font-semibold text-text-primary leading-tight">
            {title}
          </h1>
          {description && (
            <p className="text-xs text-text-secondary">{description}</p>
          )}
        </div>
      </div>
      {(actions || children) && (
        <div className="flex items-center gap-2">
          {actions}
          {children}
        </div>
      )}
    </header>
  )
}
