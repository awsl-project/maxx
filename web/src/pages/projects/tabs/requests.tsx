import { Card, CardContent } from '@/components/ui'
import type { Project } from '@/lib/transport'
import { FileText } from 'lucide-react'

interface RequestsTabProps {
  project: Project
}

export function RequestsTab({ project: _project }: RequestsTabProps) {
  return (
    <div className="p-6 space-y-6">
      <div>
        <h3 className="text-lg font-medium text-text-primary">
          Project Requests
        </h3>
        <p className="text-sm text-text-secondary">
          Request history for this project
        </p>
      </div>

      <Card className="border-border bg-surface-primary">
        <CardContent className="p-12">
          <div className="flex flex-col items-center justify-center gap-4 text-center">
            <FileText className="h-12 w-12 text-text-muted opacity-20" />
            <div>
              <p className="text-text-muted">
                Request tracking by project coming soon
              </p>
              <p className="text-xs text-text-muted mt-1">
                This feature requires adding projectID to ProxyRequest records.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
