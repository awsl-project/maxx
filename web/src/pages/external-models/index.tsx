import { useEffect, useMemo, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Eye, Save } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/layout/page-header';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle } from '@/components/ui';
import { Textarea } from '@/components/ui/textarea';
import {
  settingsKeys,
  useModelMappings,
  useProjects,
  useProviders,
  useRoutes,
  useSettings,
  useUpdateSetting,
} from '@/hooks/queries';
import type { ModelMapping, Project, Provider, Route } from '@/lib/transport/types';

const EXTERNAL_MODEL_LIST_SETTING_KEY = 'external_model_list';

function parseModelList(value: string) {
  return Array.from(
    new Set(
      value
        .split(/[\n,]+/)
        .map((model) => model.trim())
        .filter(Boolean),
    ),
  ).sort((a, b) => a.localeCompare(b));
}

type ExternalModelRouteGroup = {
  route: Route;
  provider: Provider;
  project?: Project;
  models: string[];
};

function isConcretePublicModel(model: string): boolean {
  const trimmed = model.trim();
  return trimmed.length > 0 && !trimmed.includes('*');
}

function mappingAppliesToRoute(mapping: ModelMapping, route: Route, provider: Provider): boolean {
  if (!mapping.isEnabled) return false;
  if (mapping.routeID && mapping.routeID !== route.id) return false;
  if (mapping.providerID && mapping.providerID !== provider.id) return false;
  if (mapping.projectID && mapping.projectID !== route.projectID) return false;
  if (mapping.clientType && mapping.clientType !== route.clientType) return false;
  if (mapping.providerType && mapping.providerType !== provider.type) return false;
  return true;
}

function routePublicModels(route: Route, provider: Provider, mappings: ModelMapping[]): string[] {
  const models = new Set<string>();

  const configuredModels = provider.exposedModelsEnabled
    ? (provider.exposedModels ?? [])
    : (provider.supportModels ?? []);
  for (const model of configuredModels) {
    if (isConcretePublicModel(model)) models.add(model.trim());
  }

  for (const mapping of mappings) {
    if (!mappingAppliesToRoute(mapping, route, provider)) continue;
    if (isConcretePublicModel(mapping.pattern)) models.add(mapping.pattern.trim());
  }

  return Array.from(models).sort((a, b) => a.localeCompare(b));
}

function buildExternalModelRouteGroups({
  models,
  routes,
  providers,
  projects,
  mappings,
}: {
  models: string[];
  routes: Route[];
  providers: Provider[];
  projects: Project[];
  mappings: ModelMapping[];
}): { groups: ExternalModelRouteGroup[]; uncategorized: string[] } {
  const providerByID = new Map(providers.map((provider) => [provider.id, provider]));
  const projectByID = new Map(projects.map((project) => [project.id, project]));
  const remaining = new Set(models);

  const groups = routes
    .filter((route) => route.isEnabled)
    .sort((a, b) => {
      if (a.projectID !== b.projectID) return a.projectID - b.projectID;
      if (a.clientType !== b.clientType) return a.clientType.localeCompare(b.clientType);
      return a.position - b.position;
    })
    .flatMap((route) => {
      const provider = providerByID.get(route.providerID);
      if (!provider) return [];
      const routeModels = new Set(routePublicModels(route, provider, mappings));
      const matched = models.filter((model) => routeModels.has(model));
      if (matched.length === 0) return [];
      for (const model of matched) remaining.delete(model);
      return [
        {
          route,
          provider,
          project: route.projectID ? projectByID.get(route.projectID) : undefined,
          models: matched,
        },
      ];
    });

  return { groups, uncategorized: Array.from(remaining).sort((a, b) => a.localeCompare(b)) };
}

export function ExternalModelsPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data: settings, isLoading } = useSettings();
  const { data: providers = [] } = useProviders();
  const { data: routes = [] } = useRoutes();
  const { data: projects = [] } = useProjects();
  const { data: mappings = [] } = useModelMappings();
  const updateSetting = useUpdateSetting();
  const storedList = settings?.[EXTERNAL_MODEL_LIST_SETTING_KEY] ?? '';
  const [value, setValue] = useState(storedList);
  const [saved, setSaved] = useState(false);
  const models = useMemo(() => parseModelList(value), [value]);
  const { groups, uncategorized } = useMemo(
    () => buildExternalModelRouteGroups({ models, routes, providers, projects, mappings }),
    [models, routes, providers, projects, mappings],
  );
  const hasChanges = value !== storedList;

  useEffect(() => {
    setValue(storedList);
  }, [storedList]);

  const handleSave = async () => {
    const normalized = models.join('\n');
    await updateSetting.mutateAsync({ key: EXTERNAL_MODEL_LIST_SETTING_KEY, value: normalized });
    queryClient.setQueryData<Record<string, string>>(settingsKeys.all, {
      ...(settings ?? {}),
      [EXTERNAL_MODEL_LIST_SETTING_KEY]: normalized,
    });
    setValue(normalized);
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
                {t('externalModels.modelCount', { count: models.length })}
              </Badge>
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 p-5">
            <p className="text-sm text-muted-foreground">{t('externalModels.editorDesc')}</p>
            <Textarea
              className="min-h-72 font-mono text-sm"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              disabled={isLoading || updateSetting.isPending}
              placeholder={t('externalModels.placeholder')}
            />
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-xs text-muted-foreground">{t('externalModels.saveHint')}</p>
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
            {models.length > 0 ? (
              <div className="space-y-3 rounded-xl border border-border bg-muted/25 p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <div className="text-sm font-medium text-foreground">
                      {t('externalModels.groupedTitle')}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('externalModels.groupedDesc')}
                    </p>
                  </div>
                  <Badge variant="outline">
                    {t('externalModels.groupCount', { count: groups.length })}
                  </Badge>
                </div>

                {groups.map((group) => (
                  <div
                    key={group.route.id}
                    className="rounded-lg border border-border bg-background p-3"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
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
                        {t('externalModels.modelCount', { count: group.models.length })}
                      </Badge>
                    </div>
                    <div className="mt-3 flex flex-wrap gap-2">
                      {group.models.map((model) => (
                        <Badge
                          key={model}
                          variant="outline"
                          className="max-w-full truncate font-mono"
                        >
                          {model}
                        </Badge>
                      ))}
                    </div>
                  </div>
                ))}

                {uncategorized.length > 0 && (
                  <div className="rounded-lg border border-dashed border-border bg-background p-3">
                    <div className="text-sm font-medium text-foreground">
                      {t('externalModels.uncategorizedTitle')}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('externalModels.uncategorizedDesc')}
                    </p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      {uncategorized.map((model) => (
                        <Badge
                          key={model}
                          variant="outline"
                          className="max-w-full truncate font-mono"
                        >
                          {model}
                        </Badge>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-border bg-muted/20 p-4 text-sm text-muted-foreground">
                {t('externalModels.empty')}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
