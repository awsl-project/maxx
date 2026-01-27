import {
  Card,
  CardContent,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui';
import { useSessions } from '@/hooks/queries';
import type { Project } from '@/lib/transport';
import { Loader2, Users } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface SessionsTabProps {
  project: Project;
}

export function SessionsTab({ project }: SessionsTabProps) {
  const { t } = useTranslation();
  const { data: allSessions, isLoading } = useSessions();

  // Filter sessions for this project
  const projectSessions = allSessions?.filter((session) => session.projectID === project.id) ?? [];

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-accent" />
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      <div>
        <h3 className="text-lg font-medium text-text-primary">{t('sessions.projectSessions')}</h3>
        <p className="text-sm text-text-secondary">{t('sessions.description')}</p>
      </div>

      <Card className="border-border bg-card">
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent border-border">
                <TableHead className="w-[100px] text-text-secondary">{t('sessions.id')}</TableHead>
                <TableHead className="text-text-secondary">{t('sessions.sessionId')}</TableHead>
                <TableHead className="text-text-secondary">{t('sessions.clientType')}</TableHead>
                <TableHead className="text-text-secondary">{t('sessions.created')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {projectSessions.map((session) => (
                <TableRow key={session.id} className="border-border hover:bg-accent">
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {session.id}
                  </TableCell>
                  <TableCell>
                    <code className="text-xs bg-muted px-2 py-1 rounded">{session.sessionID}</code>
                  </TableCell>
                  <TableCell>
                    <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-accent/10 text-accent">
                      {session.clientType}
                    </span>
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {new Date(session.createdAt).toLocaleString()}
                  </TableCell>
                </TableRow>
              ))}
              {projectSessions.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={4}
                    className="h-32 text-center text-muted-foreground border-border"
                  >
                    <div className="flex flex-col items-center justify-center gap-2">
                      <Users className="h-8 w-8 opacity-20" />
                      <p>{t('sessions.noProjectSessions')}</p>
                    </div>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
