import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
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
import { Dialog, DialogContent } from '@/components/ui/dialog'
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
  const { t } = useTranslation()
  const { data: sessions, isLoading } = useSessions()
  const { data: projects } = useProjects()
  const [selectedSession, setSelectedSession] = useState<Session | null>(null)

  // Create project ID to name mapping
  const projectMap = new Map(projects?.map(p => [p.id, p.name]) ?? [])

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-card shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent rounded-lg">
            <LayoutDashboard size={20} />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-text-primary leading-tight">
              {t('sessions.title')}
            </h2>
            <p className="text-xs text-text-secondary">
              {t('sessions.activeCount', { count: sessions?.length ?? 0 })}
            </p>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-auto p-6">
        <Card className="border-border bg-card">
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
                      {t('sessions.client')}
                    </TableHead>
                    <TableHead className="text-text-secondary">
                      {t('sessions.sessionId')}
                    </TableHead>
                    <TableHead className="w-[150px] text-text-secondary">
                      {t('sessions.project')}
                    </TableHead>
                    <TableHead className="w-[180px] text-right text-text-secondary">
                      {t('common.created')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sessions?.map(session => (
                    <TableRow
                      key={session.id}
                      className="border-border hover:bg-accent cursor-pointer transition-colors"
                      onClick={() => setSelectedSession(session)}
                    >
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <div className="p-1 rounded bg-muted">
                            <ClientIcon type={session.clientType} size={16} />
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="font-mono text-xs text-foreground">
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
                            {t('sessions.unassigned')}
                          </span>
                        ) : (
                          <Badge variant="default" className="text-xs">
                            {projectMap.get(session.projectID) ??
                              `#${session.projectID}`}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-right text-xs text-muted-foreground font-mono">
                        {new Date(session.createdAt).toLocaleString()}
                      </TableCell>
                    </TableRow>
                  ))}
                  {(!sessions || sessions.length === 0) && (
                    <TableRow>
                      <TableCell
                        colSpan={4}
                        className="h-32 text-center text-muted-foreground"
                      >
                        <div className="flex flex-col items-center justify-center gap-2">
                          <Calendar className="h-8 w-8 opacity-20" />
                          <p>{t('sessions.noSessions')}</p>
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
      <SessionDetailModal
        session={selectedSession}
        projects={projects ?? []}
        onClose={() => setSelectedSession(null)}
      />
    </div>
  )
}

interface SessionDetailModalProps {
  session: Session | null
  projects: { id: number; name: string }[]
  onClose: () => void
}

function SessionDetailModal({
  session,
  projects,
  onClose,
}: SessionDetailModalProps) {
  const { t } = useTranslation()
  const [selectedProjectId, setSelectedProjectId] = useState<number>(0)
  const updateSessionProject = useUpdateSessionProject()

  // Reset selected project when session changes
  useEffect(() => {
    if (session) {
      setSelectedProjectId(session.projectID)
    }
  }, [session])

  const handleSave = async () => {
    if (!session) return
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

  const hasChanges = session ? selectedProjectId !== session.projectID : false

  if (!session) return null

  return (
    <Dialog open={!!session} onOpenChange={open => !open && onClose()}>
      <DialogContent
        showCloseButton={false}
        className="overflow-hidden p-0 w-full max-w-[32rem] bg-card"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-accent/10">
              <ClientIcon type={session.clientType} size={20} />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-text-primary">
                {t('sessions.sessionDetails')}
              </h3>
              <p className="text-xs text-text-muted capitalize">
                {session.clientType} {t('sessions.client')}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 rounded-full hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
          >
            <X size={16} />
          </button>
        </div>

        {/* Content */}
        <div className="px-6 py-4 space-y-4">
          {/* Session ID */}
          <div>
            <label className="text-xs font-medium text-text-secondary uppercase tracking-wider block mb-1.5">
              {t('sessions.sessionId')}
            </label>
            <div className="font-mono text-xs text-text-primary bg-muted px-3 py-2 rounded-md select-all break-all">
              {session.sessionID}
            </div>
          </div>

          {/* Created At */}
          <div>
            <label className="text-xs font-medium text-text-secondary uppercase tracking-wider block mb-1.5">
              {t('common.created')}
            </label>
            <div className="text-sm text-foreground">
              {new Date(session.createdAt).toLocaleString()}
            </div>
          </div>

          {/* Project Binding */}
          <div>
            <label className="text-xs font-medium text-text-secondary uppercase tracking-wider flex items-center gap-2 mb-2">
              <Link2 size={12} /> {t('sessions.projectBinding')}
            </label>
            <div className="flex flex-wrap gap-2">
              {/* Unassigned option */}
              <button
                type="button"
                onClick={() => setSelectedProjectId(0)}
                className={cn(
                  'flex items-center gap-2 px-3 py-2 rounded-lg border text-sm font-medium transition-all',
                  selectedProjectId === 0
                    ? 'border-primary bg-primary text-primary-foreground shadow-lg shadow-primary/25'
                    : 'border-border bg-muted text-text-secondary hover:bg-accent'
                )}
              >
                <X size={14} />
                <span>{t('sessions.unassigned')}</span>
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
                      ? 'border-primary bg-primary text-primary-foreground shadow-lg shadow-primary/25'
                      : 'border-border bg-muted text-text-primary hover:bg-accent'
                  )}
                >
                  <FolderOpen size={14} />
                  <span>{project.name}</span>
                </button>
              ))}
            </div>
            <p className="text-[10px] text-text-muted mt-2">
              {t('sessions.projectBindingHint')}
            </p>
          </div>

          {/* Update result info */}
          {updateSessionProject.isSuccess && updateSessionProject.data && (
            <div className="flex items-center gap-2 text-xs text-emerald-400 bg-emerald-400/10 px-3 py-2 rounded-md">
              <Check size={14} />
              <span>
                {t('sessions.updatedRequests', { count: updateSessionProject.data.updatedRequests })}
              </span>
            </div>
          )}

          {updateSessionProject.isError && (
            <div className="flex items-center gap-2 text-xs text-red-400 bg-red-400/10 px-3 py-2 rounded-md">
              <AlertCircle size={14} />
              <span>{t('sessions.updateFailed')}</span>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2 px-6 py-4 border-t border-border bg-muted/30">
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
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
              t('common.save')
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
