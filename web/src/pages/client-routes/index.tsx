/**
 * Client Routes Page (Global Routes)
 * 全局路由配置页面 - 显示当前 ClientType 的路由
 */

import { useState, useMemo } from 'react';
import { useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Search, Globe, FolderKanban, ArrowUpDown, Zap, Code2 } from 'lucide-react';
import { ClientIcon, getClientName } from '@/components/icons/client-icons';
import type { ClientType } from '@/lib/transport';
import { ClientTypeRoutesContent } from '@/components/routes/ClientTypeRoutesContent';
import { Input } from '@/components/ui/input';
import {
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Switch,
  Button,
} from '@/components/ui';
import { useProjects, useUpdateProject, useRoutes, useProviders, routeKeys } from '@/hooks/queries';
import { useTransport } from '@/lib/transport/context';
import { useQueryClient } from '@tanstack/react-query';

export function ClientRoutesPage() {
  const { t } = useTranslation();
  const { clientType } = useParams<{ clientType: string }>();
  const activeClientType = (clientType as ClientType) || 'claude';
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedProjectId, setSelectedProjectId] = useState<string>('0'); // '0' = Global
  const [isSorting, setIsSorting] = useState(false);

  const { data: projects } = useProjects();
  const { data: allRoutes } = useRoutes();
  const { data: providers = [] } = useProviders();
  const sortedProjects = projects?.slice().sort((a, b) => a.id - b.id);
  const updateProject = useUpdateProject();
  const { transport } = useTransport();
  const queryClient = useQueryClient();

  // Check if there are any Antigravity/Codex routes in the current scope (Global routes, projectID=0)
  const { hasAntigravityRoutes, hasCodexRoutes } = useMemo(() => {
    const globalRoutes = allRoutes?.filter(
      (r) => r.clientType === activeClientType && r.projectID === 0,
    ) || [];

    let hasAntigravity = false;
    let hasCodex = false;

    for (const route of globalRoutes) {
      const provider = providers.find((p) => p.id === route.providerID);
      if (provider?.type === 'antigravity') hasAntigravity = true;
      if (provider?.type === 'codex') hasCodex = true;
      if (hasAntigravity && hasCodex) break;
    }

    return { hasAntigravityRoutes: hasAntigravity, hasCodexRoutes: hasCodex };
  }, [allRoutes, providers, activeClientType]);

  const handleSortAntigravity = async () => {
    setIsSorting(true);
    try {
      await transport.sortAntigravityRoutes();
      queryClient.invalidateQueries({ queryKey: routeKeys.list() });
    } catch (error) {
      console.error('Failed to sort Antigravity routes:', error);
    } finally {
      setIsSorting(false);
    }
  };

  const handleSortCodex = async () => {
    setIsSorting(true);
    try {
      await transport.sortCodexRoutes();
      queryClient.invalidateQueries({ queryKey: routeKeys.list() });
    } catch (error) {
      console.error('Failed to sort Codex routes:', error);
    } finally {
      setIsSorting(false);
    }
  };

  const handleToggleCustomRoutes = (projectId: number, enabled: boolean) => {
    const project = projects?.find((p) => p.id === projectId);
    if (!project) return;

    const currentEnabledRoutes = project.enabledCustomRoutes ?? [];
    const updatedEnabledCustomRoutes = enabled
      ? [...currentEnabledRoutes, activeClientType]
      : currentEnabledRoutes.filter((type) => type !== activeClientType);

    updateProject.mutate({
      id: projectId,
      data: {
        name: project.name,
        slug: project.slug,
        enabledCustomRoutes: updatedEnabledCustomRoutes,
      },
    });
  };

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-card shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent/10 rounded-lg">
            <ClientIcon type={activeClientType} size={20} className="text-accent" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-foreground leading-tight">
              {getClientName(activeClientType)} Routes
            </h2>
            <p className="text-xs text-muted-foreground">
              Configure routing for {getClientName(activeClientType)}
            </p>
          </div>
        </div>
        <div className="relative">
          <Search
            size={14}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            placeholder={t('common.searchProviders')}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9 w-48"
          />
        </div>
      </div>

      {/* Tabs for Global / Projects */}
      <Tabs value={selectedProjectId} onValueChange={setSelectedProjectId} className="flex-1 min-h-0 flex flex-col">
        {/* Only show tab bar when there are projects */}
        {sortedProjects && sortedProjects.length > 0 && (
          <div className="px-6 py-3 border-b border-border bg-card">
            <div className="mx-auto max-w-[1400px] flex items-center justify-between gap-6">
              <div className="flex items-center gap-6">
                {/* Global Group */}
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Global</span>
                  <TabsList className="h-8">
                    <TabsTrigger value="0" className="h-7 px-3 text-xs flex items-center gap-1.5">
                      <Globe className="h-3.5 w-3.5" />
                      <span>Default</span>
                    </TabsTrigger>
                  </TabsList>
                </div>

                {/* Projects Group */}
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Projects</span>
                  <TabsList className="h-8">
                    {sortedProjects.map((project) => (
                      <TabsTrigger
                        key={project.id}
                        value={String(project.id)}
                        className="h-7 px-3 text-xs flex items-center gap-1.5"
                      >
                        <FolderKanban className="h-3.5 w-3.5" />
                        <span>{project.name}</span>
                      </TabsTrigger>
                    ))}
                  </TabsList>
                </div>
              </div>

              {/* Sort Buttons - Only show when viewing Global routes */}
              {selectedProjectId === '0' && (hasAntigravityRoutes || hasCodexRoutes) && (
                <div className="flex items-center gap-2">
                  {hasAntigravityRoutes && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleSortAntigravity}
                      disabled={isSorting}
                      className="h-8 text-xs"
                    >
                      <Zap className="h-3.5 w-3.5 mr-1.5" />
                      {t('routes.sortAntigravity')}
                      {isSorting && <ArrowUpDown className="h-3.5 w-3.5 ml-1.5 animate-pulse" />}
                    </Button>
                  )}
                  {hasCodexRoutes && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleSortCodex}
                      disabled={isSorting}
                      className="h-8 text-xs"
                    >
                      <Code2 className="h-3.5 w-3.5 mr-1.5" />
                      {t('routes.sortCodex')}
                      {isSorting && <ArrowUpDown className="h-3.5 w-3.5 ml-1.5 animate-pulse" />}
                    </Button>
                  )}
                </div>
              )}
            </div>
          </div>
        )}

        {/* Global Tab Content */}
        <TabsContent value="0" className="flex-1 min-h-0 overflow-hidden m-0">
          <ClientTypeRoutesContent
            clientType={activeClientType}
            projectID={0}
            searchQuery={searchQuery}
          />
        </TabsContent>

        {/* Project Tab Contents */}
        {sortedProjects?.map((project) => {
          const isCustomRoutesEnabled = (project.enabledCustomRoutes ?? []).includes(activeClientType);

          return (
            <TabsContent key={project.id} value={String(project.id)} className="flex-1 min-h-0 overflow-hidden m-0 flex flex-col">
              {/* Custom Routes Toggle Bar */}
              <div className="h-12 px-6 border-b border-border bg-card flex items-center justify-between shrink-0">
                <div className="flex items-center gap-3">
                  <p className="text-sm font-medium">Custom Routes</p>
                  {isCustomRoutesEnabled && (
                    <span className="text-xs px-2 py-0.5 bg-green-500/10 text-green-600 dark:text-green-400 rounded-full">
                      Enabled
                    </span>
                  )}
                  <p className="text-xs text-muted-foreground">
                    {isCustomRoutesEnabled
                      ? 'Using project-specific routes'
                      : 'Using global routes'}
                  </p>
                </div>
                <Switch
                  checked={isCustomRoutesEnabled}
                  onCheckedChange={(checked) => handleToggleCustomRoutes(project.id, checked)}
                  disabled={updateProject.isPending}
                />
              </div>

              {/* Content Area */}
              {isCustomRoutesEnabled ? (
                <div className="flex-1 min-h-0 overflow-hidden">
                  <ClientTypeRoutesContent
                    clientType={activeClientType}
                    projectID={project.id}
                    searchQuery={searchQuery}
                  />
                </div>
              ) : (
                <div className="flex-1 flex items-center justify-center">
                  <div className="text-center space-y-4 max-w-md">
                    <div className="p-4 bg-muted/50 rounded-full w-16 h-16 mx-auto flex items-center justify-center">
                      <FolderKanban className="h-8 w-8 text-muted-foreground" />
                    </div>
                    <div>
                      <h3 className="text-lg font-semibold mb-2">
                        Custom Routes Not Enabled
                      </h3>
                      <p className="text-sm text-muted-foreground">
                        This project is currently using global routes for {getClientName(activeClientType)}.
                      </p>
                    </div>
                    <Button
                      onClick={() => handleToggleCustomRoutes(project.id, true)}
                      disabled={updateProject.isPending}
                    >
                      Enable Custom Routes
                    </Button>
                  </div>
                </div>
              )}
            </TabsContent>
          );
        })}
      </Tabs>
    </div>
  );
}
