import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  CardFooter,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui';
import { useProjects, useCreateProject } from '@/hooks/queries';
import {
  Plus,
  X,
  FolderKanban,
  Loader2,
  Calendar,
  Hash,
  Activity,
  Archive,
  Clock3,
} from 'lucide-react';
import { PageHeader } from '@/components/layout';
import { getProjectActivityState } from '@/lib/project-activity';
import { cn } from '@/lib/utils';

const INACTIVE_THRESHOLDS = [30, 60, 90] as const;

function formatDate(value: string | null | undefined, locale: string) {
  if (!value) {
    return null;
  }
  const time = new Date(value);
  if (!Number.isFinite(time.getTime())) {
    return null;
  }
  return time.toLocaleDateString(locale, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

export function ProjectsPage() {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const navigate = useNavigate();
  const { data: projects, isLoading } = useProjects();
  const createProject = useCreateProject();
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [inactiveThresholdDays, setInactiveThresholdDays] = useState(90);
  const [showInactiveOnly, setShowInactiveOnly] = useState(false);
  const [activityNow] = useState(() => Date.now());

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

  const activityByProject = useMemo(() => {
    return new Map(
      (projects ?? []).map((project) => [
        project.id,
        getProjectActivityState(project, inactiveThresholdDays, activityNow),
      ]),
    );
  }, [activityNow, inactiveThresholdDays, projects]);

  const summary = useMemo(() => {
    const states = Array.from(activityByProject.values());
    return {
      inactive: states.filter((state) => state.status === 'inactive').length,
      neverUsed: states.filter((state) => state.status === 'never-used').length,
      active: states.filter((state) => state.status === 'active').length,
    };
  }, [activityByProject]);

  const sortedProjects = useMemo(() => {
    if (!projects) {
      return undefined;
    }
    return projects
      .filter((project) => !showInactiveOnly || activityByProject.get(project.id)?.inactive)
      .slice()
      .sort((a, b) => {
        const activityA = activityByProject.get(a.id);
        const activityB = activityByProject.get(b.id);
        if (activityA?.inactive !== activityB?.inactive) {
          return activityA?.inactive ? -1 : 1;
        }

        const lastUsedA = activityA?.lastUsedAt ? new Date(activityA.lastUsedAt).getTime() : 0;
        const lastUsedB = activityB?.lastUsedAt ? new Date(activityB.lastUsedAt).getTime() : 0;
        if (lastUsedA !== lastUsedB) {
          return lastUsedA - lastUsedB;
        }

        const timeA = Number.isFinite(new Date(a.createdAt).getTime())
          ? new Date(a.createdAt).getTime()
          : 0;
        const timeB = Number.isFinite(new Date(b.createdAt).getTime())
          ? new Date(b.createdAt).getTime()
          : 0;
        if (timeA !== timeB) {
          return timeA - timeB;
        }
        return a.id - b.id;
      });
  }, [activityByProject, projects, showInactiveOnly]);

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

        <Card className="border-border bg-card/70">
          <CardContent className="p-4">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 flex-1">
                <div className="rounded-lg border border-border bg-muted/30 p-3">
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Archive className="h-3.5 w-3.5 text-amber-500" />
                    {t('projects.cleanupCandidates')}
                  </div>
                  <div className="mt-1 text-2xl font-semibold text-foreground">
                    {summary.inactive + summary.neverUsed}
                  </div>
                </div>
                <div className="rounded-lg border border-border bg-muted/30 p-3">
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Clock3 className="h-3.5 w-3.5 text-red-500" />
                    {t('projects.neverUsed')}
                  </div>
                  <div className="mt-1 text-2xl font-semibold text-foreground">
                    {summary.neverUsed}
                  </div>
                </div>
                <div className="rounded-lg border border-border bg-muted/30 p-3">
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Activity className="h-3.5 w-3.5 text-emerald-500" />
                    {t('projects.activeProjects')}
                  </div>
                  <div className="mt-1 text-2xl font-semibold text-foreground">
                    {summary.active}
                  </div>
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <span>{t('projects.inactiveThreshold')}</span>
                  <Select
                    value={String(inactiveThresholdDays)}
                    onValueChange={(value) => setInactiveThresholdDays(Number(value))}
                  >
                    <SelectTrigger className="h-9 w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {INACTIVE_THRESHOLDS.map((days) => (
                        <SelectItem key={days} value={String(days)}>
                          {t('projects.daysThreshold', { count: days })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <Button
                  type="button"
                  variant={showInactiveOnly ? 'default' : 'outline'}
                  size="sm"
                  aria-pressed={showInactiveOnly}
                  onClick={() => setShowInactiveOnly((value) => !value)}
                >
                  {t('projects.showInactiveOnly')}
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        {isLoading ? (
          <div className="flex items-center justify-center p-12">
            <Loader2 className="h-8 w-8 animate-spin text-accent" />
          </div>
        ) : sortedProjects && sortedProjects.length > 0 ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {sortedProjects.map((project) => {
              const activity = activityByProject.get(project.id);
              const lastUsed = formatDate(activity?.lastUsedAt, locale);
              const lastSuccess = formatDate(activity?.lastSuccessfulRequestAt, locale);
              const badgeLabel =
                activity?.status === 'never-used'
                  ? t('projects.neverUsed')
                  : activity?.status === 'inactive'
                    ? t('projects.inactiveBadge', { days: activity.unusedDays })
                    : t('projects.activeBadge');

              return (
                <Card
                  key={project.id}
                  className={cn(
                    'group border-border bg-surface-primary cursor-pointer hover:border-accent/50 hover:shadow-card-hover transition-all duration-200 flex flex-col',
                    activity?.inactive && 'border-amber-500/40 bg-amber-500/5',
                  )}
                  role="button"
                  tabIndex={0}
                  onClick={() => handleRowClick(project.id)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      handleRowClick(project.id);
                    }
                  }}
                >
                  <CardHeader className="pb-4">
                    <div className="flex items-center gap-3 mb-2">
                      <div className="p-2 rounded-lg text-amber-500 bg-amber-500/10 group-hover:bg-amber-500/20 transition-colors">
                        <FolderKanban size={18} />
                      </div>
                      <div className="flex min-w-0 flex-1 items-center gap-1.5 text-xs font-mono px-2 py-1 rounded bg-muted text-muted-foreground">
                        <Hash size={10} className="shrink-0" />
                        <span className="truncate">{project.slug}</span>
                      </div>
                    </div>
                    <div className="flex flex-col gap-2">
                      <CardTitle className="text-base font-semibold leading-tight truncate">
                        {project.name}
                      </CardTitle>
                      <Badge
                        variant={activity?.inactive ? 'warning' : 'outline'}
                        className="h-auto min-h-5 max-w-full whitespace-normal break-words text-left leading-tight"
                      >
                        {badgeLabel}
                      </Badge>
                    </div>
                  </CardHeader>
                  <CardContent className="pt-0 flex-1 space-y-2 text-xs text-muted-foreground">
                    <div className="flex items-center justify-between gap-3">
                      <span>{t('projects.lastUsed')}</span>
                      <span className="font-medium text-foreground">
                        {lastUsed ?? t('projects.noUsage')}
                      </span>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <span>{t('projects.lastSuccess')}</span>
                      <span className="font-medium text-foreground">
                        {lastSuccess ?? t('projects.noSuccess')}
                      </span>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <span>{t('projects.recentRequests')}</span>
                      <span className="font-mono text-foreground">
                        {activity?.requestCount30d ?? 0} /{' '}
                        {activity?.successfulRequestCount30d ?? 0}
                      </span>
                    </div>
                  </CardContent>
                  <CardFooter className="pt-4 pb-4 text-xs flex justify-between items-center text-muted-foreground">
                    <div className="flex items-center gap-1.5">
                      <Calendar size={12} />
                      <span>{formatDate(project.createdAt, locale)}</span>
                    </div>
                    <span className="font-mono">
                      {t('projects.totalRequests', { count: activity?.totalRequestCount ?? 0 })}
                    </span>
                  </CardFooter>
                </Card>
              );
            })}
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
