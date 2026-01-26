import { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle, Input, Button } from '@/components/ui';
import { useUpdateProject, projectKeys } from '@/hooks/queries';
import { useQueryClient } from '@tanstack/react-query';
import type { Project } from '@/lib/transport';
import { Loader2, Save, Copy, Check } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface OverviewTabProps {
  project: Project;
}

export function OverviewTab({ project }: OverviewTabProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const updateProject = useUpdateProject();
  const [name, setName] = useState(project.name);
  const [slug, setSlug] = useState(project.slug);
  const [copied, setCopied] = useState<string | null>(null);

  const hasChanges = name !== project.name || slug !== project.slug;

  const handleSave = () => {
    updateProject.mutate(
      {
        id: project.id,
        data: { name, slug, enabledCustomRoutes: project.enabledCustomRoutes },
      },
      {
        onSuccess: () => {
          // Invalidate queries
          queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
          queryClient.invalidateQueries({
            queryKey: projectKeys.detail(project.id),
          });
        },
      },
    );
  };

  const copyToClipboard = (key: string, text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  };

  // Generate project base URL
  const baseUrl = window.location.origin;
  const projectBaseUrl = `${baseUrl}/project/${project.slug}/`;

  return (
    <div className="p-6 space-y-6">
      {/* Project Info */}
      <Card className="border-border bg-card">
        <CardHeader>
          <CardTitle className="text-base">{t('projects.information')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <label htmlFor="name" className="text-sm font-medium text-text-primary">
                {t('projects.name')}
              </label>
              <Input
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="bg-muted border-border"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="slug" className="text-sm font-medium text-text-primary">
                {t('projects.slug')}
              </label>
              <Input
                id="slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                className="bg-surface-secondary border-border font-mono"
                placeholder={t('projects.slugPlaceholder')}
              />
              <p className="text-xs text-text-muted">{t('projects.slugDesc')}</p>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <span className="text-text-secondary">{t('projects.created')}</span>{' '}
              <span className="text-text-primary">
                {new Date(project.createdAt).toLocaleString()}
              </span>
            </div>
            <div>
              <span className="text-text-secondary">{t('projects.updated')}</span>{' '}
              <span className="text-text-primary">
                {new Date(project.updatedAt).toLocaleString()}
              </span>
            </div>
          </div>

          {hasChanges && (
            <div className="flex justify-end pt-2">
              <Button onClick={handleSave} disabled={updateProject.isPending}>
                {updateProject.isPending ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Save className="mr-2 h-4 w-4" />
                )}
                {t('projects.saveChanges')}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Proxy Configuration */}
      <Card className="border-border bg-card">
        <CardHeader>
          <CardTitle className="text-base">{t('projects.proxyConfig')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-text-secondary">{t('projects.proxyConfigDesc')}</p>

          <div className="flex items-center gap-3">
            <span className="text-sm font-medium text-text-primary w-20">
              {t('projects.baseUrl')}
            </span>
            <code className="flex-1 text-xs bg-surface-secondary px-3 py-2 rounded border border-border text-text-primary font-mono">
              {projectBaseUrl}
            </code>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 w-8 p-0"
              onClick={() => copyToClipboard('base', projectBaseUrl)}
            >
              {copied === 'base' ? (
                <Check className="h-4 w-4 text-success" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
