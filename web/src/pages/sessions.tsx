import { useState, useEffect, useCallback } from 'react'
import { createPortal } from 'react-dom'
import {
  Badge,
  Button,
  Card,
  CardContent,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui'
import {
  useSessions,
  useProjects,
  useUpdateSessionProject,
} from '@/hooks/queries'
import {
  LayoutDashboard,
  Loader2,
  Calendar,
  X,
  Link2,
  Check,
  AlertCircle,
  FolderOpen,
} from 'lucide-react'
import type { Session } from '@/lib/transport'
import { cn } from '@/lib/utils'
import { ClientIcon } from '@/components/icons/client-icons'

export function SessionsPage() {
  const { data: sessions, isLoading } = useSessions()
  const { data: projects } = useProjects()
  const [selectedSession, setSelectedSession] = useState<Session | null>(null)

  // Create project ID to name mapping
  const projectMap = new Map(projects?.map(p => [p.id, p.name]) ?? [])

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent rounded-lg">
            <LayoutDashboard size={20} />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-text-primary leading-tight">
              Sessions
            </h2>
            <p className="text-xs text-text-secondary">
              {sessions?.length ?? 0} active sessions
            </p>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-auto p-6">
        <Card className="border-border bg-surface-primary">
          <CardContent className="p-0">
            {isLoading ? (
              <div className="flex items-center justify-center p-12">
                <Loader2 className="h-8 w-8 animate-spin text-accent" />
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent border-border">
                    <TableHead className="w-[60px] text-text-secondary">
                      Client
                    </TableHead>
                    <TableHead className="text-text-secondary">
                      Session ID
                    </TableHead>
                    <TableHead className="w-[150px] text-text-secondary">
                      Project
                    </TableHead>
                    <TableHead className="w-[180px] text-right text-text-secondary">
                      Created
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sessions?.map(session => (
                    <TableRow
                      key={session.id}
                      className="border-border hover:bg-surface-hover cursor-pointer transition-colors"
                      onClick={() => setSelectedSession(session)}
                    >
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <div className="p-1 rounded bg-surface-secondary">
                            <ClientIcon type={session.clientType} size={16} />
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="font-mono text-xs text-text-primary">
                        <span
                          className="truncate max-w-[300px] block"
                          title={session.sessionID}
                        >
                          {session.sessionID}
                        </span>
                      </TableCell>
                      <TableCell>
                        {session.projectID === 0 ? (
                          <span className="text-text-muted text-xs italic">
                            Unassigned
                          </span>
                        ) : (
                          <Badge variant="default" className="text-xs">
                            {projectMap.get(session.projectID) ??
                              `#${session.projectID}`}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-right text-xs text-text-secondary font-mono">
                        {new Date(session.createdAt).toLocaleString()}
                      </TableCell>
                    </TableRow>
                  ))}
                  {(!sessions || sessions.length === 0) && (
                    <TableRow>
                      <TableCell
                        colSpan={4}
                        className="h-32 text-center text-text-muted"
                      >
                        <div className="flex flex-col items-center justify-center gap-2">
                          <Calendar className="h-8 w-8 opacity-20" />
                          <p>No active sessions</p>
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Session Detail Modal */}
      {selectedSession && (
        <SessionDetailModal
          session={selectedSession}
          projects={projects ?? []}
          onClose={() => setSelectedSession(null)}
        />
      )}
    </div>
  )
}

interface SessionDetailModalProps {
  session: Session
  projects: { id: number; name: string }[]
  onClose: () => void
}

function SessionDetailModal({
  session,
  projects,
  onClose,
}: SessionDetailModalProps) {
  const [selectedProjectId, setSelectedProjectId] = useState<number>(
    session.projectID
  )
  const updateSessionProject = useUpdateSessionProject()

  // Handle ESC key
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
      }
    },
    [onClose]
  )

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      document.body.style.overflow = ''
    }
  }, [handleKeyDown])

  const handleSave = async () => {
    try {
      await updateSessionProject.mutateAsync({
        sessionID: session.sessionID,
        projectID: selectedProjectId,
      })
      onClose()
    } catch (error) {
      console.error('Failed to update session project:', error)
    }
  }

  const hasChanges = selectedProjectId !== session.projectID

  return createPortal(
    <>
      {/* Overlay */}
      <div
        className="dialog-overlay"
        onClick={onClose}
        style={{ zIndex: 99998 }}
      />

      {/* Content */}
      <div
        className="dialog-content overflow-hidden"
        style={{
          zIndex: 99999,
          width: '100%',
          maxWidth: '32rem',
          padding: 0,
          background: 'var(--color-surface-primary)',
        }}
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-accent/10">
              <ClientIcon type={session.clientType} size={20} />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-text-primary">
                Session Details
              </h3>
              <p className="text-xs text-text-muted capitalize">
                {session.clientType} client
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 rounded-full hover:bg-surface-hover text-text-muted hover:text-text-primary transition-colors"
          >
            <X size={16} />
          </button>
        </div>

        {/* Content */}
        <div className="px-6 py-4 space-y-4">
          {/* Session ID */}
          <div>
            <label className="text-xs font-medium text-text-secondary uppercase tracking-wider block mb-1.5">
              Session ID
            </label>
            <div className="font-mono text-xs text-text-primary bg-surface-secondary px-3 py-2 rounded-md select-all break-all">
              {session.sessionID}
            </div>
          </div>

          {/* Created At */}
          <div>
            <label className="text-xs font-medium text-text-secondary uppercase tracking-wider block mb-1.5">
              Created
            </label>
            <div className="text-sm text-text-primary">
              {new Date(session.createdAt).toLocaleString()}
            </div>
          </div>

          {/* Project Binding */}
          <div>
            <label className="text-xs font-medium text-text-secondary uppercase tracking-wider flex items-center gap-2 mb-2">
              <Link2 size={12} /> Project Binding
            </label>
            <div className="flex flex-wrap gap-2">
              {/* Unassigned option */}
              <button
                type="button"
                onClick={() => setSelectedProjectId(0)}
                className={cn(
                  'flex items-center gap-2 px-3 py-2 rounded-lg border text-sm font-medium transition-all',
                  selectedProjectId === 0
                    ? 'border-blue-500 bg-blue-500 text-white shadow-lg shadow-blue-500/25'
                    : 'border-border bg-surface-secondary text-text-secondary hover:bg-surface-hover'
                )}
              >
                <X size={14} />
                <span>Unassigned</span>
              </button>
              {/* Project options */}
              {projects.map(project => (
                <button
                  key={project.id}
                  type="button"
                  onClick={() => setSelectedProjectId(project.id)}
                  className={cn(
                    'flex items-center gap-2 px-3 py-2 rounded-lg border text-sm font-medium transition-all',
                    selectedProjectId === project.id
                      ? 'border-blue-500 bg-blue-500 text-white shadow-lg shadow-blue-500/25'
                      : 'border-border bg-surface-secondary text-text-primary hover:bg-surface-hover'
                  )}
                >
                  <FolderOpen size={14} />
                  <span>{project.name}</span>
                </button>
              ))}
            </div>
            <p className="text-[10px] text-text-muted mt-2">
              Changing the project will also update all requests associated with
              this session.
            </p>
          </div>

          {/* Update result info */}
          {updateSessionProject.isSuccess && updateSessionProject.data && (
            <div className="flex items-center gap-2 text-xs text-emerald-400 bg-emerald-400/10 px-3 py-2 rounded-md">
              <Check size={14} />
              <span>
                Updated {updateSessionProject.data.updatedRequests} requests
              </span>
            </div>
          )}

          {updateSessionProject.isError && (
            <div className="flex items-center gap-2 text-xs text-red-400 bg-red-400/10 px-3 py-2 rounded-md">
              <AlertCircle size={14} />
              <span>Failed to update session project</span>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2 px-6 py-4 border-t border-border bg-surface-secondary/30">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={handleSave}
            disabled={!hasChanges || updateSessionProject.isPending}
            className={cn(
              'min-w-[100px]',
              hasChanges && 'bg-accent hover:bg-accent-hover'
            )}
          >
            {updateSessionProject.isPending ? (
              <Loader2 size={14} className="animate-spin" />
            ) : (
              'Save'
            )}
          </Button>
        </div>
      </div>
    </>,
    document.body
  )
}
