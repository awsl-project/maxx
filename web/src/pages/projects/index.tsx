import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  CardFooter,
} from '@/components/ui';
import { useProjects, useCreateProject } from '@/hooks/queries';
import { Plus, X, FolderKanban, Loader2, Calendar, Hash } from 'lucide-react';
import { PageHeader } from '@/components/layout';

export function ProjectsPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { data: projects, isLoading } = useProjects();
  const createProject = useCreateProject();
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createProject.mutate(
      { name, enabledCustomRoutes: [] },
      {
        onSuccess: (project) => {
          setShowForm(false);
          setName('');
          // 创建后自动跳转到详情页
          navigate(`/projects/${project.id}`);
        },
      },
    );
  };

  const handleRowClick = (id: number) => {
    navigate(`/projects/${id}`);
  };

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={FolderKanban}
        iconClassName="text-amber-500"
        title={t('projects.title')}
        description={t('projects.description')}
      >
        <Button onClick={() => setShowForm(!showForm)} variant={showForm ? 'secondary' : 'default'}>
          {showForm ? <X className="mr-2 h-4 w-4" /> : <Plus className="mr-2 h-4 w-4" />}
          {showForm ? t('common.cancel') : t('projects.addProject')}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-auto p-6 space-y-6">
        {showForm && (
          <Card className="border-border bg-card animate-in slide-in-from-top-4 duration-200">
            <CardContent className="pt-6">
              <form onSubmit={handleSubmit} className="flex gap-4 items-end">
                <div className="flex-1 space-y-2">
                  <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                    {t('projects.projectName')}
                  </label>
                  <Input
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder={t('projects.projectNamePlaceholder')}
                    required
                    className="bg-muted border-border"
                    autoFocus
                  />
                </div>
                <Button type="submit" disabled={createProject.isPending}>
                  {createProject.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    t('projects.createProject')
                  )}
                </Button>
              </form>
            </CardContent>
          </Card>
        )}

        {isLoading ? (
          <div className="flex items-center justify-center p-12">
            <Loader2 className="h-8 w-8 animate-spin text-accent" />
          </div>
        ) : projects && projects.length > 0 ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {projects.map((project) => (
              <Card
                key={project.id}
                className="group border-border bg-surface-primary cursor-pointer hover:border-accent/50 hover:shadow-card-hover transition-all duration-200 flex flex-col"
                onClick={() => handleRowClick(project.id)}
              >
                <CardHeader className="pb-4">
                  <div className="flex items-center gap-3 mb-2">
                    <div className="p-2 rounded-lg text-amber-500 bg-amber-500/10 group-hover:bg-amber-500/20 transition-colors">
                      <FolderKanban size={18} />
                    </div>
                    <div className="flex items-center gap-1.5 text-xs font-mono px-2 py-1 rounded bg-muted text-muted-foreground">
                      <Hash size={10} />
                      <span className="truncate">{project.slug}</span>
                    </div>
                  </div>
                  <CardTitle className="text-base font-semibold leading-tight truncate">
                    {project.name}
                  </CardTitle>
                </CardHeader>
                <CardContent className="pt-0 flex-1" />
                <CardFooter className="pt-0 pb-4 text-xs flex justify-between items-center text-muted-foreground">
                  <div className="flex items-center gap-1.5">
                    <Calendar size={12} />
                    <span>
                      {new Date(project.createdAt).toLocaleDateString(
                        i18n.resolvedLanguage ?? i18n.language,
                        {
                          month: 'short',
                          day: 'numeric',
                          year: 'numeric',
                        },
                      )}
                    </span>
                  </div>
                </CardFooter>
              </Card>
            ))}
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center h-64 text-muted-foreground border-2 border-dashed border-border rounded-lg bg-card/50">
            <Calendar className="h-12 w-12 opacity-20 mb-4" />
            <p className="text-lg font-medium">{t('projects.noProjects')}</p>
            <p className="text-sm">{t('projects.noProjectsHint')}</p>
          </div>
        )}
      </div>
    </div>
  );
}
