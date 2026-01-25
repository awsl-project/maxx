/**
 * Force Project Dialog
 * Shows when a session requires project binding
 */

import { useEffect, useState, useCallback } from 'react';
import { Dialog, DialogContent } from '@/components/ui/dialog';
import { FolderOpen, AlertCircle, Loader2, Clock, X } from 'lucide-react';
import { useProjects, useUpdateSessionProject, useRejectSession } from '@/hooks/queries';
import type { NewSessionPendingEvent, Project, ClientType } from '@/lib/transport/types';
import { cn } from '@/lib/utils';
import { getClientName, getClientColor } from '@/components/icons/client-icons';
import { useTranslation } from 'react-i18next';
import { useCountdown } from '@/hooks/use-countdown';

// ============================================================================
// Types
// ============================================================================

interface ForceProjectDialogProps {
  event: NewSessionPendingEvent | null;
  onClose: () => void;
  timeoutSeconds: number;
}

// ============================================================================
// Sub-components
// ============================================================================

interface SessionInfoProps {
  sessionID: string;
  clientType: ClientType;
}

function SessionInfo({ sessionID, clientType }: SessionInfoProps) {
  const { t } = useTranslation();
  const clientColor = getClientColor(clientType);

  return (
    <div className="flex items-center gap-4 p-3 rounded-xl bg-muted border border-border">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-[10px] font-bold text-text-muted uppercase tracking-wider">
            {t('sessions.session')}
          </span>
          <span
            className="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium"
            style={{
              backgroundColor: `${clientColor}20`,
              color: clientColor,
            }}
          >
            {getClientName(clientType)}
          </span>
        </div>
        <div className="font-mono text-xs text-muted-foreground truncate">{sessionID}</div>
      </div>
    </div>
  );
}

interface CountdownTimerProps {
  remainingTime: number;
}

function CountdownTimer({ remainingTime }: CountdownTimerProps) {
  const { t } = useTranslation();
  const isUrgent = remainingTime <= 10;

  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-xl border p-5 flex flex-col items-center justify-center group',
        isUrgent
          ? 'bg-linear-to-br from-red-950/30 to-transparent border-red-500/20'
          : 'bg-linear-to-br from-amber-950/30 to-transparent border-amber-500/20',
      )}
    >
      <div
        className={cn(
          'absolute inset-0 opacity-50 group-hover:opacity-100 transition-opacity',
          isUrgent ? 'bg-red-400/5' : 'bg-amber-400/5',
        )}
      />
      <div
        className={cn(
          'relative flex items-center gap-1.5 mb-1',
          isUrgent ? 'text-red-500' : 'text-amber-500',
        )}
      >
        <Clock size={14} />
        <span className="text-[10px] font-bold uppercase tracking-widest">
          {t('sessions.remaining')}
        </span>
      </div>
      <div
        className={cn(
          'relative font-mono text-4xl font-bold tracking-widest tabular-nums',
          isUrgent
            ? 'text-red-400 drop-shadow-[0_0_8px_rgba(248,113,113,0.3)]'
            : 'text-amber-400 drop-shadow-[0_0_8px_rgba(251,191,36,0.3)]',
        )}
      >
        {remainingTime}s
      </div>
    </div>
  );
}

interface ProjectSelectorProps {
  projects: Project[] | undefined;
  isLoading: boolean;
  selectedProjectId: number;
  onSelect: (id: number) => void;
  disabled?: boolean;
}

function ProjectSelector({ projects, isLoading, selectedProjectId, onSelect, disabled }: ProjectSelectorProps) {
  const { t } = useTranslation();

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-8">
        <Loader2 className="h-6 w-6 animate-spin text-accent" />
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <label className="text-[10px] font-bold text-text-muted uppercase tracking-wider">
        {t('sessions.selectProject')}
      </label>
      {projects && projects.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {projects.map((project) => (
            <button
              key={project.id}
              type="button"
              disabled={disabled}
              onClick={() => onSelect(project.id)}
              className={cn(
                'flex items-center gap-2 px-4 py-2.5 rounded-xl border-2 text-sm font-bold transition-all',
                'hover:scale-[1.02] active:scale-[0.98]',
                disabled && 'opacity-50 cursor-not-allowed hover:scale-100',
                selectedProjectId === project.id
                  ? 'border-amber-500 bg-amber-500 text-white shadow-lg shadow-amber-500/30'
                  : 'border-amber-500/40 bg-amber-500/10 text-amber-400 hover:bg-amber-500/20 hover:border-amber-500/60',
              )}
            >
              <FolderOpen size={16} />
              <span>{project.name}</span>
            </button>
          ))}
        </div>
      ) : (
        <p className="text-sm text-text-muted text-center py-4">
          {t('sessions.noProjectsAvailable')}
        </p>
      )}
    </div>
  );
}

// ============================================================================
// Main Component
// ============================================================================

export function ForceProjectDialog({ event, onClose, timeoutSeconds }: ForceProjectDialogProps) {
  const { t } = useTranslation();
  const { data: projects, isLoading } = useProjects();
  const updateSessionProject = useUpdateSessionProject();
  const rejectSession = useRejectSession();

  const [selectedProjectId, setSelectedProjectId] = useState(0);
  const [eventId, setEventId] = useState<string | null>(null);

  const handleTimeout = useCallback(() => {
    if (event) {
      onClose();
    }
  }, [event, onClose]);

  const { remainingTime, reset: resetCountdown } = useCountdown({
    initialSeconds: timeoutSeconds,
    onComplete: handleTimeout,
    autoStart: !!event,
  });

  // Reset state when event changes
  useEffect(() => {
    if (event && event.sessionID !== eventId) {
      setEventId(event.sessionID);
      setSelectedProjectId(0);
      resetCountdown(timeoutSeconds);
    }
  }, [event, eventId, timeoutSeconds, resetCountdown]);

  const handleConfirm = async (projectId: number) => {
    if (!event || projectId === 0) return;

    setSelectedProjectId(projectId);
    try {
      await updateSessionProject.mutateAsync({
        sessionID: event.sessionID,
        projectID: projectId,
      });
      onClose();
    } catch (error) {
      console.error('Failed to bind project:', error);
      setSelectedProjectId(0);
    }
  };

  const handleReject = async () => {
    if (!event) return;

    try {
      await rejectSession.mutateAsync(event.sessionID);
      onClose();
    } catch (error) {
      console.error('Failed to reject session:', error);
    }
  };

  if (!event) return null;

  return (
    <Dialog open={!!event} onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        showCloseButton={false}
        className="p-0 w-full max-w-[28rem] bg-card border border-amber-500/30 shadow-[0_0_30px_-5px_rgba(245,158,11,0.3)]"
      >
        {/* Header */}
        <div className="relative bg-gradient-to-b from-amber-900/20 to-transparent p-6 pb-4 rounded-t-lg">
          <div className="flex flex-col items-center text-center space-y-3">
            <div className="p-3 rounded-2xl bg-amber-500/10 border border-amber-400/20 shadow-[0_0_15px_-3px_rgba(245,158,11,0.2)]">
              <AlertCircle size={28} className="text-amber-400" />
            </div>
            <div>
              <h2 className="text-xl font-bold text-text-primary">{t('sessions.selectProject')}</h2>
              <p className="text-xs text-amber-500/80 font-medium uppercase tracking-wider mt-1">
                {t('sessions.projectSelectionRequired')}
              </p>
            </div>
          </div>
        </div>

        {/* Body */}
        <div className="px-6 pb-6 space-y-5">
          <SessionInfo sessionID={event.sessionID} clientType={event.clientType} />

          <CountdownTimer remainingTime={remainingTime} />

          <ProjectSelector
            projects={projects}
            isLoading={isLoading}
            selectedProjectId={selectedProjectId}
            onSelect={handleConfirm}
            disabled={updateSessionProject.isPending || rejectSession.isPending}
          />

          {/* Reject Button */}
          <div className="space-y-3 pt-2">
            <button
              onClick={handleReject}
              disabled={rejectSession.isPending || updateSessionProject.isPending}
              className="w-full flex items-center justify-center gap-2 px-4 py-3 rounded-xl border border-red-500/30 bg-red-500/10 text-red-400 hover:bg-red-500/20 hover:border-red-500/50 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {rejectSession.isPending ? (
                <>
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-red-400/30 border-t-red-400" />
                  <span className="text-sm font-bold">{t('sessions.rejecting')}</span>
                </>
              ) : (
                <>
                  <X size={16} />
                  <span className="text-sm font-bold">{t('sessions.reject')}</span>
                </>
              )}
            </button>

            <div className="flex items-start gap-2 rounded-lg bg-muted/50 p-2.5 text-[11px] text-muted-foreground">
              <AlertCircle size={12} className="mt-0.5 shrink-0" />
              <p>{t('sessions.timeoutWarning')}</p>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
