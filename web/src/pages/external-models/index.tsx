import { useEffect, useMemo, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Eye, Save } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle } from '@/components/ui';
import { Textarea } from '@/components/ui/textarea';
import {
  settingsKeys,
  useProjects,
  useProviders,
  useRoutes,
  useSettings,
  useUpdateSetting,
} from '@/hooks/queries';
import type { Project, Provider, Route } from '@/lib/transport/types';

const EXTERNAL_MODEL_LIST_SETTING_KEY = 'external_model_list';
const UNCATEGORIZED_KEY = '__uncategorized__';

type CategorizedExternalModelListSetting = {
  version?: number;
  routes?: Record<string, string[]>;
  uncategorized?: string[];
};

type RouteGroup = {
  key: string;
  route: Route;
  provider: Provider;
  project?: Project;
};

function parseModelText(value: string) {
  return Array.from(
    new Set(
      value
        .split(/[\n,]+/)
        .map((model) => model.trim())
        .filter(Boolean),
    ),
  ).sort((a, b) => a.localeCompare(b));
}

function parseStoredExternalModels(value: string): Record<string, string> {
  const trimmed = value.trim();
  if (!trimmed) return { [UNCATEGORIZED_KEY]: '' };

  try {
    const parsed = JSON.parse(trimmed) as unknown;
    if (Array.isArray(parsed)) {
      return { [UNCATEGORIZED_KEY]: parseModelText(parsed.join('\n')).join('\n') };
    }
    if (parsed && typeof parsed === 'object') {
      const setting = parsed as CategorizedExternalModelListSetting;
      const next: Record<string, string> = {};
      for (const [routeID, models] of Object.entries(setting.routes ?? {})) {
        next[routeID] = parseModelText(Array.isArray(models) ? models.join('\n') : '').join('\n');
      }
      next[UNCATEGORIZED_KEY] = parseModelText((setting.uncategorized ?? []).join('\n')).join('\n');
      return next;
    }
  } catch {
    // Legacy newline/comma-separated values are intentionally kept unassigned.
  }

  return { [UNCATEGORIZED_KEY]: parseModelText(trimmed).join('\n') };
}

function buildRouteGroups({
  routes,
  providers,
  projects,
}: {
  routes: Route[];
  providers: Provider[];
  projects: Project[];
}): RouteGroup[] {
  const providerByID = new Map(providers.map((provider) => [provider.id, provider]));
  const projectByID = new Map(projects.map((project) => [project.id, project]));

  return routes
    .filter((route) => route.isEnabled)
    .sort((a, b) => {
      if (a.projectID !== b.projectID) return a.projectID - b.projectID;
      if (a.clientType !== b.clientType) return a.clientType.localeCompare(b.clientType);
      return a.position - b.position;
    })
    .flatMap((route) => {
      const provider = providerByID.get(route.providerID);
      if (!provider) return [];
      return [
        {
          key: String(route.id),
          route,
          provider,
          project: route.projectID ? projectByID.get(route.projectID) : undefined,
        },
      ];
    });
}

function normalizeDraftForSave(draft: Record<string, string>, groups: RouteGroup[]) {
  const routes: Record<string, string[]> = {};
  const usedKeys = new Set(groups.map((group) => group.key));

  for (const group of groups) {
    const models = parseModelText(draft[group.key] ?? '');
    if (models.length > 0) routes[group.key] = models;
  }

  const uncategorized = parseModelText(
    Object.entries(draft)
      .filter(([key]) => key === UNCATEGORIZED_KEY || !usedKeys.has(key))
      .map(([, value]) => value)
      .join('\n'),
  );

  return JSON.stringify(
    {
      version: 1,
      routes,
      uncategorized,
    },
    null,
    2,
  );
}

function countModels(draft: Record<string, string>) {
  const models = new Set<string>();
  for (const value of Object.values(draft)) {
    for (const model of parseModelText(value)) models.add(model);
  }
  return models.size;
}

export function ExternalModelsPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data: settings, isLoading } = useSettings();
  const { data: providers = [] } = useProviders();
  const { data: routes = [] } = useRoutes();
  const { data: projects = [] } = useProjects();
  const updateSetting = useUpdateSetting();
  const storedList = settings?.[EXTERNAL_MODEL_LIST_SETTING_KEY] ?? '';
  const routeGroups = useMemo(
    () => buildRouteGroups({ routes, providers, projects }),
    [routes, providers, projects],
  );
  const [draft, setDraft] = useState<Record<string, string>>(() =>
    parseStoredExternalModels(storedList),
  );
  const [saved, setSaved] = useState(false);
  const normalizedValue = useMemo(
    () => normalizeDraftForSave(draft, routeGroups),
    [draft, routeGroups],
  );
  const storedNormalizedValue = useMemo(() => {
    const parsed = parseStoredExternalModels(storedList);
    return normalizeDraftForSave(parsed, routeGroups);
  }, [storedList, routeGroups]);
  const modelCount = useMemo(() => countModels(draft), [draft]);
  const hasChanges = normalizedValue !== storedNormalizedValue;

  useEffect(() => {
    setDraft(parseStoredExternalModels(storedList));
  }, [storedList]);

  const setGroupValue = (key: string, value: string) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const handleSave = async () => {
    await updateSetting.mutateAsync({
      key: EXTERNAL_MODEL_LIST_SETTING_KEY,
      value: normalizedValue,
    });
    queryClient.setQueryData<Record<string, string>>(settingsKeys.all, {
      ...(settings ?? {}),
      [EXTERNAL_MODEL_LIST_SETTING_KEY]: normalizedValue,
    });
    setDraft(parseStoredExternalModels(normalizedValue));
    setSaved(true);
    window.setTimeout(() => setSaved(false), 1600);
  };

  return (
    <div className="flex h-full flex-col bg-background">
      <PageHeader
        icon={Eye}
        iconClassName="text-zinc-500"
        title={t('externalModels.title')}
        description={t('externalModels.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <Card className="border-border bg-card">
          <CardHeader className="border-b border-border">
            <CardTitle className="flex items-center justify-between gap-3 text-base font-medium">
              <span>{t('externalModels.editorTitle')}</span>
              <Badge variant="secondary">
                {t('externalModels.modelCount', { count: modelCount })}
              </Badge>
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 p-5">
            <div className="space-y-1">
              <p className="text-sm text-muted-foreground">{t('externalModels.editorDesc')}</p>
              <p className="text-xs text-muted-foreground">{t('externalModels.saveHint')}</p>
            </div>

            {routeGroups.length > 0 ? (
              <div className="space-y-3">
                {routeGroups.map((group) => {
                  const models = parseModelText(draft[group.key] ?? '');
                  return (
                    <div
                      key={group.key}
                      className="rounded-lg border border-border bg-background p-4"
                    >
                      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium text-foreground">
                            {group.provider.name}
                          </div>
                          <div className="mt-1 flex flex-wrap gap-2 text-xs text-muted-foreground">
                            <span>{group.project?.name ?? t('externalModels.globalScope')}</span>
                            <span>·</span>
                            <span>{group.route.clientType}</span>
                            <span>·</span>
                            <span>route #{group.route.id}</span>
                          </div>
                        </div>
                        <Badge variant="secondary">
                          {t('externalModels.modelCount', { count: models.length })}
                        </Badge>
                      </div>
                      <Textarea
                        className="min-h-28 font-mono text-sm"
                        value={draft[group.key] ?? ''}
                        onChange={(event) => setGroupValue(group.key, event.target.value)}
                        disabled={isLoading || updateSetting.isPending}
                        placeholder={t('externalModels.groupPlaceholder')}
                      />
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-border bg-muted/20 p-4 text-sm text-muted-foreground">
                {t('externalModels.noRoutes')}
              </div>
            )}

            <div className="rounded-lg border border-dashed border-border bg-muted/20 p-4">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div className="text-sm font-medium text-foreground">
                    {t('externalModels.uncategorizedTitle')}
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('externalModels.uncategorizedDesc')}
                  </p>
                </div>
                <Badge variant="outline">
                  {t('externalModels.modelCount', {
                    count: parseModelText(draft[UNCATEGORIZED_KEY] ?? '').length,
                  })}
                </Badge>
              </div>
              <Textarea
                className="min-h-24 font-mono text-sm"
                value={draft[UNCATEGORIZED_KEY] ?? ''}
                onChange={(event) => setGroupValue(UNCATEGORIZED_KEY, event.target.value)}
                disabled={isLoading || updateSetting.isPending}
                placeholder={t('externalModels.uncategorizedPlaceholder')}
              />
            </div>

            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-xs text-muted-foreground">{t('externalModels.saveFormatHint')}</p>
              <Button
                className="gap-2"
                onClick={handleSave}
                disabled={isLoading || updateSetting.isPending || !hasChanges}
              >
                <Save className="size-4" />
                {updateSetting.isPending
                  ? t('common.loading')
                  : saved
                    ? t('common.saved')
                    : t('common.save')}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
