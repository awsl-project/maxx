import { Card, CardContent } from '@/components/ui';
import type { Project } from '@/lib/transport';
import { FileText } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface RequestsTabProps {
  project: Project;
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
export function RequestsTab({ project: _project }: RequestsTabProps) {
  const { t } = useTranslation();
  return (
    <div className="p-6 space-y-6">
      <div>
        <h3 className="text-lg font-medium text-foreground">{t('projects.requests.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('projects.requests.description')}</p>
      </div>

      <Card className="border-border bg-card">
        <CardContent className="p-12">
          <div className="flex flex-col items-center justify-center gap-4 text-center">
            <FileText className="h-12 w-12 text-muted-foreground opacity-20" />
            <div>
              <p className="text-muted-foreground">{t('projects.requests.comingSoon')}</p>
              <p className="text-xs text-muted-foreground mt-1">
                {t('projects.requests.comingSoonNote')}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
