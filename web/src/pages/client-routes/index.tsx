/**
 * Client Routes Page (Global Routes)
 * 全局路由配置页面 - 显示当前 ClientType 的路由
 */

import { useState } from 'react';
import { useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Search, Globe, FolderKanban } from 'lucide-react';
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
import { useProjects, useUpdateProject } from '@/hooks/queries';

export function ClientRoutesPage() {
  const { t } = useTranslation();
  const { clientType } = useParams<{ clientType: string }>();
  const activeClientType = (clientType as ClientType) || 'claude';
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedProjectId, setSelectedProjectId] = useState<string>('0'); // '0' = Global
  const { data: projects } = useProjects();
  const updateProject = useUpdateProject();

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
      <Tabs value={selectedProjectId} onValueChange={setSelectedProjectId} className="flex-1 flex flex-col overflow-hidden">
        <div className="px-6 py-4 border-b border-border bg-card shrink-0">
          <TabsList>
            {/* Global Tab */}
            <TabsTrigger value="0" className="flex items-center gap-2">
              <Globe className="h-4 w-4" />
              <span>Global</span>
            </TabsTrigger>

            {/* Separator between Global and Projects */}
            {projects && projects.length > 0 && (
              <div className="h-6 w-px bg-border mx-2" />
            )}

            {/* Project Tabs - show all projects */}
            {projects?.map((project) => (
              <TabsTrigger key={project.id} value={String(project.id)} className="flex items-center gap-2">
                <FolderKanban className="h-4 w-4" />
                <span>{project.name}</span>
              </TabsTrigger>
            ))}
          </TabsList>
        </div>

        {/* Global Tab Content */}
        <TabsContent value="0" className="flex-1 overflow-y-auto m-0">
          <ClientTypeRoutesContent
            clientType={activeClientType}
            projectID={0}
            searchQuery={searchQuery}
          />
        </TabsContent>

        {/* Project Tab Contents */}
        {projects?.map((project) => {
          const isCustomRoutesEnabled = (project.enabledCustomRoutes ?? []).includes(activeClientType);

          return (
            <TabsContent key={project.id} value={String(project.id)} className="flex-1 overflow-y-auto m-0 flex flex-col">
              {/* Custom Routes Toggle Bar */}
              <div className="h-12 px-6 border-b border-border bg-card flex items-center justify-between sticky top-0 z-10">
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
                <div className="flex-1">
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
