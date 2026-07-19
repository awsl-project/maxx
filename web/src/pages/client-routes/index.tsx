/**
 * Client Routes Page (Global Routes)
 * 全局路由配置页面 - 显示当前 ClientType 的路由
 */

import { useState, useMemo, useRef, useEffect, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Search,
  Globe,
  FolderKanban,
  ArrowUpDown,
  Zap,
  Code2,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react';
import { ClientIcon, getClientName } from '@/components/icons/client-icons';
import { PageHeader } from '@/components/layout/page-header';
import type { ClientType } from '@/lib/transport';
import { ClientTypeRoutesContent } from '@/components/routes/ClientTypeRoutesContent';
import { Input } from '@/components/ui/input';
import { Tabs, TabsList, TabsTrigger, TabsContent, Switch, Button } from '@/components/ui';
import { useProjects, useUpdateProject, useRoutes, useProviders, routeKeys } from '@/hooks/queries';
import { useTransport } from '@/lib/transport/context';
import { useQueryClient } from '@tanstack/react-query';
import { cn } from '@/lib/utils';

const SCROLL_STEP = 200;

interface ProjectTabBarProps {
  projects: { id: number; name: string }[];
  selectedProjectId: string;
  onHoverStart: () => void;
  onHoverEnd: () => void;
}

function ProjectTabBar({
  projects,
  selectedProjectId,
  onHoverStart,
  onHoverEnd,
}: ProjectTabBarProps) {
  const { t } = useTranslation();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);

  const updateScrollState = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    setCanScrollLeft(el.scrollLeft > 0);
    setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 1);
  }, []);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    updateScrollState();
    el.addEventListener('scroll', updateScrollState, { passive: true });
    const ro = new ResizeObserver(updateScrollState);
    ro.observe(el);
    return () => {
      el.removeEventListener('scroll', updateScrollState);
      ro.disconnect();
    };
  }, [updateScrollState, projects]);

  // Scroll selected tab into view (centered) when selection changes
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const tab = el.querySelector<HTMLElement>(`[data-project-id="${selectedProjectId}"]`);
    if (!tab) return;
    const targetLeft = tab.offsetLeft - (el.clientWidth - tab.offsetWidth) / 2;
    el.scrollTo({ left: Math.max(0, targetLeft), behavior: 'smooth' });
  }, [selectedProjectId]);

  return (
    <div
      className="flex min-w-0 flex-1 items-center gap-1"
      onMouseEnter={onHoverStart}
      onMouseLeave={onHoverEnd}
    >
      {/* "Projects" label */}
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider shrink-0 select-none">
        {t('nav.projects')}
      </span>

      {/* Left arrow */}
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label="Scroll projects left"
        disabled={!canScrollLeft}
        onClick={() => scrollRef.current?.scrollBy({ left: -SCROLL_STEP, behavior: 'smooth' })}
        className={cn('shrink-0 h-7 w-7 transition-opacity', !canScrollLeft && 'opacity-0')}
      >
        <ChevronLeft className="h-3.5 w-3.5" />
      </Button>

      {/* Scrollable tab container */}
      <div ref={scrollRef} className="min-w-0 flex-1 overflow-x-auto no-scrollbar pb-1">
        <TabsList className="h-8 w-max shrink-0">
          {projects.map((project) => (
            <TabsTrigger
              key={project.id}
              value={String(project.id)}
              data-project-id={String(project.id)}
              className="h-7 px-3 text-xs flex items-center gap-1.5"
            >
              <FolderKanban className="h-3.5 w-3.5" />
              <span>{project.name}</span>
            </TabsTrigger>
          ))}
        </TabsList>
      </div>

      {/* Right arrow */}
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label="Scroll projects right"
        disabled={!canScrollRight}
        onClick={() => scrollRef.current?.scrollBy({ left: SCROLL_STEP, behavior: 'smooth' })}
        className={cn('shrink-0 h-7 w-7 transition-opacity', !canScrollRight && 'opacity-0')}
      >
        <ChevronRight className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

export function ClientRoutesPage() {
  const { t } = useTranslation();
  const { clientType } = useParams<{ clientType: string }>();
  const activeClientType = (clientType as ClientType) || 'claude';
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedProjectId, setSelectedProjectId] = useState<string>('0'); // '0' = Global
  const [isSorting, setIsSorting] = useState(false);
  const [showProjectPanel, setShowProjectPanel] = useState(false);
  const panelTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isClaudePage = activeClientType === 'claude';
  const isCodexPage = activeClientType === 'codex';

  const { data: projects } = useProjects();
  const { data: allRoutes } = useRoutes();
  const { data: providers = [] } = useProviders();
  const sortedProjects = useMemo(
    () => (projects ? projects.slice().sort((a, b) => a.id - b.id) : []),
    [projects],
  );
  const updateProject = useUpdateProject();
  const { transport } = useTransport();
  const queryClient = useQueryClient();

  const handleProjectHoverStart = useCallback(() => {
    if (!window.matchMedia('(hover: hover)').matches) return;
    if (panelTimerRef.current) clearTimeout(panelTimerRef.current);
    setShowProjectPanel(true);
  }, []);

  const handleProjectHoverEnd = useCallback(() => {
    if (!window.matchMedia('(hover: hover)').matches) return;
    if (panelTimerRef.current) clearTimeout(panelTimerRef.current);
    panelTimerRef.current = setTimeout(() => setShowProjectPanel(false), 150);
  }, []);

  useEffect(() => {
    return () => {
      if (panelTimerRef.current) clearTimeout(panelTimerRef.current);
    };
  }, []);

  // Check if there are any Antigravity/Codex routes in the current scope (Global routes, projectID=0)
  const { hasAntigravityRoutes, hasCodexRoutes } = useMemo(() => {
    const globalRoutes =
      allRoutes?.filter((r) => r.clientType === activeClientType && r.projectID === 0) || [];

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

  const sortButtons = (
    <div className="flex items-center gap-2">
      {selectedProjectId === '0' && hasAntigravityRoutes && isClaudePage && (
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
      {selectedProjectId === '0' && hasCodexRoutes && isCodexPage && (
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
  );

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={<ClientIcon type={activeClientType} size={20} />}
        title={t('routes.clientRoutesTitle', { client: getClientName(activeClientType) })}
        description={t('routes.configureRoutingFor', { client: getClientName(activeClientType) })}
        actions={
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
        }
      />

      {/* Tabs for Global / Projects */}
      <Tabs
        value={selectedProjectId}
        onValueChange={setSelectedProjectId}
        className="flex-1 min-h-0 flex flex-col"
      >
        {/* Only show tab bar when there are projects */}
        {sortedProjects.length > 0 && (
          <div className="relative px-6 py-3 border-b border-border bg-card">
            <div className="mx-auto max-w-[1400px] flex items-center justify-between gap-6">
              <div className="flex min-w-0 flex-1 items-center gap-6">
                {/* Global Group */}
                <div className="flex items-center gap-2 shrink-0">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {t('common.global')}
                  </span>
                  <TabsList className="h-8 shrink-0">
                    <TabsTrigger value="0" className="h-7 px-3 text-xs flex items-center gap-1.5">
                      <Globe className="h-3.5 w-3.5" />
                      <span>{t('common.default')}</span>
                    </TabsTrigger>
                  </TabsList>
                </div>

                <ProjectTabBar
                  projects={sortedProjects}
                  selectedProjectId={selectedProjectId}
                  onHoverStart={handleProjectHoverStart}
                  onHoverEnd={handleProjectHoverEnd}
                />
              </div>

              {/* Sort Buttons */}
              {sortButtons}
            </div>

            {/* Full-width hover panel: all projects */}
            {showProjectPanel && (
              <div
                className="absolute left-0 right-0 top-full z-50 border-b border-border bg-card shadow-md"
                onMouseEnter={handleProjectHoverStart}
                onMouseLeave={handleProjectHoverEnd}
              >
                <div className="mx-auto max-w-[1400px] px-6 py-3 flex flex-wrap gap-2">
                  {sortedProjects.map((project) => (
                    <button
                      key={project.id}
                      onClick={() => {
                        setSelectedProjectId(String(project.id));
                        setShowProjectPanel(false);
                      }}
                      className={cn(
                        'flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm transition-colors hover:bg-accent hover:text-accent-foreground',
                        selectedProjectId === String(project.id) &&
                          'bg-accent text-accent-foreground',
                      )}
                    >
                      <FolderKanban className="h-3.5 w-3.5 shrink-0" />
                      <span>{project.name}</span>
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
        {sortedProjects.length === 0 && (
          <div className="px-6 py-3 border-b border-border bg-card">
            <div className="mx-auto max-w-[1400px] flex justify-end">{sortButtons}</div>
          </div>
        )}

        {/* Global Tab Content */}
        <TabsContent value="0" className="flex-1 min-h-0 overflow-hidden m-0">
          <ClientTypeRoutesContent
            clientType={activeClientType}
            projectID={0}
            searchQuery={searchQuery}
            isActive={selectedProjectId === '0'}
          />
        </TabsContent>

        {/* Project Tab Contents */}
        {sortedProjects.map((project) => {
          const isCustomRoutesEnabled = (project.enabledCustomRoutes ?? []).includes(
            activeClientType,
          );

          return (
            <TabsContent
              key={project.id}
              value={String(project.id)}
              className="flex-1 min-h-0 overflow-hidden m-0 flex flex-col"
            >
              {/* Custom Routes Toggle Bar */}
              <div className="min-h-12 px-6 py-2 border-b border-border bg-card flex items-center justify-between gap-4 shrink-0">
                <div className="flex flex-1 min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
                  <p className="text-sm font-medium">{t('routes.customRoutes')}</p>
                  {isCustomRoutesEnabled && (
                    <span className="text-xs px-2 py-0.5 bg-green-500/10 text-green-600 dark:text-green-400 rounded-full">
                      {t('common.enabled')}
                    </span>
                  )}
                  <p className="text-xs text-muted-foreground min-w-0 flex-1 break-words">
                    {isCustomRoutesEnabled
                      ? t('routes.usingProjectRoutes')
                      : t('routes.usingGlobalRoutes')}
                  </p>
                </div>
                <div className="shrink-0 self-center">
                  <Switch
                    checked={isCustomRoutesEnabled}
                    onCheckedChange={(checked) => handleToggleCustomRoutes(project.id, checked)}
                    disabled={updateProject.isPending}
                  />
                </div>
              </div>

              {/* Content Area */}
              {isCustomRoutesEnabled ? (
                <div className="flex-1 min-h-0 overflow-hidden">
                  <ClientTypeRoutesContent
                    clientType={activeClientType}
                    projectID={project.id}
                    searchQuery={searchQuery}
                    isActive={selectedProjectId === String(project.id)}
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
                        {t('routes.customRoutesDisabled')}
                      </h3>
                      <p className="text-sm text-muted-foreground">
                        {t('routes.usingGlobalRoutesShort', {
                          client: getClientName(activeClientType),
                        })}
                      </p>
                    </div>
                    <Button
                      onClick={() => handleToggleCustomRoutes(project.id, true)}
                      disabled={updateProject.isPending}
                    >
                      {t('routes.enableCustomRoutes')}
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
