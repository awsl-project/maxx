import { Server } from 'lucide-react'

interface EmptyStateProps {
  message: string
  icon?: React.ReactNode
}

export function EmptyState({ message, icon }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center h-full text-text-muted p-12 text-center select-none">
      {icon || <Server className="h-12 w-12 mb-3 opacity-10" />}
      <p className="text-sm font-medium">{message}</p>
    </div>
  )
}
