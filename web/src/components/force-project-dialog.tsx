/**
 * Force Project Dialog
 * Shows when a session requires project binding
 */

import { useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
} from '@/components/ui/dialog';
import { FolderOpen, AlertCircle, Loader2, Clock, X } from 'lucide-react';
import { useProjects, useUpdateSessionProject, useRejectSession } from '@/hooks/queries';
import type { NewSessionPendingEvent } from '@/lib/transport/types';
import { cn } from '@/lib/utils';
import { getClientName, getClientColor } from '@/components/icons/client-icons';

interface ForceProjectDialogProps {
  event: NewSessionPendingEvent | null
  onClose: () => void
  timeoutSeconds: number
}

export function ForceProjectDialog({
  event,
  onClose,
  timeoutSeconds,
}: ForceProjectDialogProps) {
  const { data: projects, isLoading } = useProjects()
  const updateSessionProject = useUpdateSessionProject()
  const rejectSession = useRejectSession()
  const [selectedProjectId, setSelectedProjectId] = useState<number>(0)
  const [remainingTime, setRemainingTime] = useState(timeoutSeconds)

  // Reset state when event changes
  useEffect(() => {
    if (event) {
      setSelectedProjectId(0)
      setRemainingTime(timeoutSeconds)
    }
  }, [event, timeoutSeconds])

  // Countdown timer
  useEffect(() => {
    if (!event) return

    const interval = setInterval(() => {
      setRemainingTime(prev => {
        if (prev <= 1) {
          clearInterval(interval)
          return 0
        }
        return prev - 1
      })
    }, 1000)

    return () => clearInterval(interval)
  }, [event])

  // 超时后关闭弹窗
  useEffect(() => {
    if (remainingTime === 0 && event) {
      onClose()
    }
  }, [remainingTime, event, onClose])

  const handleConfirm = async () => {
    if (!event || selectedProjectId === 0) return

    try {
      await updateSessionProject.mutateAsync({
        sessionID: event.sessionID,
        projectID: selectedProjectId,
      })
      onClose()
    } catch (error) {
      console.error('Failed to bind project:', error)
    }
  }

  const handleReject = async () => {
    if (!event) return

    try {
      await rejectSession.mutateAsync(event.sessionID)
      onClose()
    } catch (error) {
      console.error('Failed to reject session:', error)
    }
  }

  if (!event) return null

  const clientColor = getClientColor(event.clientType)

  return (
    <Dialog open={!!event} onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        showCloseButton={false}
        className="overflow-hidden p-0 w-full max-w-[28rem] bg-surface-primary"
      >
        {/* Header with Gradient */}
        <div className="relative bg-gradient-to-b from-amber-900/20 to-transparent p-6 pb-4">
          <div className="flex flex-col items-center text-center space-y-3">
            <div className="p-3 rounded-2xl bg-amber-500/10 border border-amber-400/20 shadow-[0_0_15px_-3px_rgba(245,158,11,0.2)]">
              <AlertCircle size={28} className="text-amber-400" />
            </div>
            <div>
              <h2 className="text-xl font-bold text-text-primary">选择项目</h2>
              <p className="text-xs text-amber-500/80 font-medium uppercase tracking-wider mt-1">
                Project Selection Required
              </p>
            </div>
          </div>
        </div>

        {/* Body Content */}
        <div className="px-6 pb-6 space-y-5">
          {/* Session Info */}
          <div className="flex items-center gap-4 p-3 rounded-xl bg-surface-secondary border border-border">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-[10px] font-bold text-text-muted uppercase tracking-wider">
                  Session
                </span>
                <span
                  className="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium"
                  style={{
                    backgroundColor: `${clientColor}20`,
                    color: clientColor,
                  }}
                >
                  {getClientName(event.clientType)}
                </span>
              </div>
              <div className="font-mono text-xs text-text-secondary truncate">
                {event.sessionID}
              </div>
            </div>
          </div>

          {/* Countdown Section */}
          <div
            className={cn(
              'relative overflow-hidden rounded-xl border p-5 flex flex-col items-center justify-center group',
              remainingTime <= 10
                ? 'bg-linear-to-br from-red-950/30 to-transparent border-red-500/20'
                : 'bg-linear-to-br from-amber-950/30 to-transparent border-amber-500/20'
            )}
          >
            <div
              className={cn(
                'absolute inset-0 opacity-50 group-hover:opacity-100 transition-opacity',
                remainingTime <= 10 ? 'bg-red-400/5' : 'bg-amber-400/5'
              )}
            />
            <div
              className={cn(
                'relative flex items-center gap-1.5 mb-1',
                remainingTime <= 10 ? 'text-red-500' : 'text-amber-500'
              )}
            >
              <Clock size={14} />
              <span className="text-[10px] font-bold uppercase tracking-widest">
                Remaining
              </span>
            </div>
            <div
              className={cn(
                'relative font-mono text-4xl font-bold tracking-widest tabular-nums',
                remainingTime <= 10
                  ? 'text-red-400 drop-shadow-[0_0_8px_rgba(248,113,113,0.3)]'
                  : 'text-amber-400 drop-shadow-[0_0_8px_rgba(251,191,36,0.3)]'
              )}
            >
              {remainingTime}s
            </div>
          </div>

          {/* Project Selection */}
          {isLoading ? (
            <div className="flex items-center justify-center p-8">
              <Loader2 className="h-6 w-6 animate-spin text-accent" />
            </div>
          ) : (
            <div className="space-y-3">
              <label className="text-[10px] font-bold text-text-muted uppercase tracking-wider">
                选择项目
              </label>
              {projects && projects.length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {projects.map(project => (
                    <button
                      key={project.id}
                      type="button"
                      onClick={() => setSelectedProjectId(project.id)}
                      className={cn(
                        'flex items-center gap-2 px-3 py-2 rounded-lg border text-sm font-medium transition-all',
                        selectedProjectId === project.id
                          ? 'border-amber-500 bg-amber-500 text-white shadow-lg shadow-amber-500/25'
                          : 'border-border bg-surface-secondary text-text-primary hover:bg-surface-hover hover:border-amber-500/50'
                      )}
                    >
                      <FolderOpen size={14} />
                      <span>{project.name}</span>
                    </button>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-text-muted text-center py-4">
                  没有可用的项目，请先创建项目
                </p>
              )}
            </div>
          )}

          {/* Actions */}
          <div className="space-y-3 pt-2">
            <div className="flex gap-2">
              {/* Reject Button */}
              <button
                onClick={handleReject}
                disabled={
                  rejectSession.isPending || updateSessionProject.isPending
                }
                className="flex-1 flex items-center justify-center gap-2 px-4 py-3 rounded-xl border border-red-500/30 bg-red-500/10 text-red-400 hover:bg-red-500/20 hover:border-red-500/50 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {rejectSession.isPending ? (
                  <>
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-red-400/30 border-t-red-400" />
                    <span className="text-sm font-bold">拒绝中...</span>
                  </>
                ) : (
                  <>
                    <X size={16} />
                    <span className="text-sm font-bold">拒绝</span>
                  </>
                )}
              </button>

              {/* Confirm Button */}
              <button
                onClick={handleConfirm}
                disabled={
                  selectedProjectId === 0 ||
                  updateSessionProject.isPending ||
                  rejectSession.isPending
                }
                className="flex-1 relative overflow-hidden rounded-xl p-[1px] group disabled:opacity-50 disabled:cursor-not-allowed transition-all hover:scale-[1.01] active:scale-[0.99]"
              >
                <span className="absolute inset-0 bg-gradient-to-r from-amber-500 to-orange-600 rounded-xl" />
                <div className="relative flex items-center justify-center gap-2 rounded-[11px] bg-surface-primary group-hover:bg-transparent px-4 py-3 transition-colors">
                  {updateSessionProject.isPending ? (
                    <>
                      <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                      <span className="text-sm font-bold text-white">
                        绑定中...
                      </span>
                    </>
                  ) : (
                    <>
                      <FolderOpen
                        size={16}
                        className="text-amber-400 group-hover:text-white transition-colors"
                      />
                      <span className="text-sm font-bold text-amber-400 group-hover:text-white transition-colors">
                        确认绑定
                      </span>
                    </>
                  )}
                </div>
              </button>
            </div>

            <div className="flex items-start gap-2 rounded-lg bg-surface-secondary/50 p-2.5 text-[11px] text-text-muted">
              <AlertCircle size={12} className="mt-0.5 shrink-0" />
              <p>如果未在规定时间内选择项目，请求将被拒绝。</p>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
