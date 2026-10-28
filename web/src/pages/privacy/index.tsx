import { useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { ShieldCheck, Loader2 } from 'lucide-react';
import { PageHeader } from '@/components/layout';
import { Button, Card, CardContent, CardHeader, CardTitle, Input, Switch } from '@/components/ui';
import { projectKeys, useProjects } from '@/hooks/queries';
import { getTransport, type Project } from '@/lib/transport';

export function PrivacyPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState('');
  const { data: projects = [], isLoading, error, refetch } = useProjects();
  const selectedProject = Number(params.get('project')) || 0;
  const update = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      getTransport().updateProjectPrivacy(id, enabled),
    onSuccess: async (updated) => {
      queryClient.setQueryData<Project[]>(projectKeys.list(), (current) =>
        current?.map((project) =>
          project.id === updated.id ? { ...project, blockSecrets: updated.blockSecrets } : project,
        ),
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: projectKeys.lists() }),
        queryClient.invalidateQueries({ queryKey: projectKeys.detail(updated.id) }),
      ]);
    },
  });
  const visible = projects.filter(
    (project) =>
      (!selectedProject || project.id === selectedProject) &&
      `${project.name} ${project.slug}`.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader
        icon={ShieldCheck}
        title={t('privacy.title')}
        description={t('privacy.description')}
      />
      <div className="flex-1 overflow-auto p-4 sm:p-6 space-y-6">
        <Card>
          <CardHeader className="flex items-center justify-between gap-4">
            <CardTitle className="text-base">{t('privacy.archiveTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-text-secondary">{t('privacy.archiveDescription')}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('privacy.projectTitle')}</CardTitle>
            <p id="privacy-project-description" className="text-sm text-text-secondary">
              {t('privacy.blockSecretsDesc')}
            </p>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-center gap-3">
              <Input
                aria-label={t('privacy.searchProjects')}
                placeholder={t('privacy.searchProjects')}
                className="max-w-sm"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
              {selectedProject > 0 && (
                <Button variant="outline" onClick={() => setParams({})}>
                  {t('privacy.allProjects')}
                </Button>
              )}
            </div>
            {update.error && (
              <p role="alert" className="text-sm text-destructive">
                {t('privacy.saveFailed')}
              </p>
            )}
            {isLoading ? (
              <p className="text-sm text-text-secondary">{t('common.loading')}</p>
            ) : error ? (
              <div role="alert" className="space-y-2">
                <p className="text-sm text-destructive">{t('privacy.loadFailed')}</p>
                <Button variant="outline" onClick={() => void refetch()}>
                  {t('common.refresh')}
                </Button>
              </div>
            ) : visible.length === 0 ? (
              <p className="text-sm text-text-secondary">{t('privacy.noProjects')}</p>
            ) : (
              <div className="divide-y divide-border rounded-lg border border-border">
                {visible.map((project) => (
                  <div
                    key={project.id}
                    data-project-id={project.id}
                    className="flex items-center justify-between gap-4 p-4"
                  >
                    <div className="min-w-0 space-y-1">
                      <Link
                        to={`/projects/${project.id}`}
                        className="block truncate text-sm font-medium text-text-primary hover:underline"
                      >
                        {project.name}
                      </Link>
                      <p className="truncate text-xs text-text-muted">{project.slug}</p>
                    </div>
                    <div className="flex shrink-0 items-center gap-3">
                      {update.isPending && update.variables?.id === project.id && (
                        <Loader2 aria-label={t('common.saving')} className="h-4 w-4 animate-spin" />
                      )}
                      <Switch
                        aria-label={t('privacy.blockSecretsFor', { project: project.name })}
                        aria-describedby="privacy-project-description"
                        checked={project.blockSecrets ?? false}
                        disabled={update.isPending}
                        onCheckedChange={(enabled) => update.mutate({ id: project.id, enabled })}
                      />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
