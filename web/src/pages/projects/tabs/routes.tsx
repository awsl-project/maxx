/**
 * Project Routes Tab
 * 显示项目特定的路由配置 - 左侧 ClientType Sidebar + 右侧拖拽卡片布局
 */

import { useState, useMemo } from 'react';
import { useRoutes, useUpdateProject, projectKeys } from '@/hooks/queries';
import { useStreamingRequests } from '@/hooks/use-streaming';
import { ClientIcon, getClientName, getClientColor } from '@/components/icons/client-icons';
import { cn } from '@/lib/utils';
import { Switch } from '@/components/ui';
import type { Project, ClientType, Route } from '@/lib/transport';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { useQueryClient } from '@tanstack/react-query';
import { ClientTypeRoutesContent } from '@/components/routes/ClientTypeRoutesContent';
import { useTranslation } from 'react-i18next';

// 支持的客户端类型列表
const CLIENT_TYPES: ClientType[] = ['claude', 'openai', 'codex', 'gemini'];

interface RoutesTabProps {
  project: Project;
}

// 项目路由内容包装器 - 包含自定义路由开关逻辑
interface ProjectClientTypeWrapperProps {
  clientType: ClientType;
  project: Project;
  isCustomRoutesEnabled: boolean;
  onToggleCustomRoutes: (enabled: boolean) => void;
  projectRoutes: Route[];
}

function ProjectClientTypeWrapper({
  clientType,
  project,
  isCustomRoutesEnabled,
  onToggleCustomRoutes,
  projectRoutes,
}: ProjectClientTypeWrapperProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col h-full">
      {/* Header with Toggle */}
      <div className="flex items-center justify-between px-lg py-4 border-b border-border bg-card">
        <div className="flex items-center gap-4">
          <ClientIcon type={clientType} size={32} />
          <div>
            <h2 className="text-lg font-semibold text-foreground">{getClientName(clientType)}</h2>
            <p className="text-xs text-muted-foreground">
              {isCustomRoutesEnabled
                ? t('routes.routesConfigured', {
                    count: projectRoutes.filter((r) => r.clientType === clientType).length,
                  })
                : t('routes.usingGlobalRoutes')}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm text-text-secondary">{t('routes.customRoutes')}</span>
          <Switch checked={isCustomRoutesEnabled} onCheckedChange={onToggleCustomRoutes} />
        </div>
      </div>

      {/* Content */}
      {!isCustomRoutesEnabled ? (
        <div className="flex-1 flex flex-col items-center justify-center py-24 text-center space-y-6 px-lg">
          <div className="p-6 rounded-full bg-muted/50">
            <ClientIcon type={clientType} size={48} className="opacity-30" />
          </div>
          <div className="space-y-3">
            <h3 className="text-lg font-semibold text-text-primary">
              {t('routes.customRoutesDisabled')}
            </h3>
            <p className="text-sm text-text-secondary leading-relaxed">
              {t('routes.usingGlobalRoutesDesc', {
                client: getClientName(clientType),
              })}
            </p>
          </div>
        </div>
      ) : (
        <div className="flex-1 overflow-hidden">
          <ClientTypeRoutesContent clientType={clientType} projectID={project.id} />
        </div>
      )}
    </div>
  );
}

export function RoutesTab({ project }: RoutesTabProps) {
  const { t } = useTranslation();
  const [activeClientType, setActiveClientType] = useState<ClientType>('claude');
  const queryClient = useQueryClient();

  const { data: allRoutes, isLoading: routesLoading } = useRoutes();
  const { countsByRoute } = useStreamingRequests();

  const updateProject = useUpdateProject();

  const loading = routesLoading;

  // 获取项目的路由
  const projectRoutes = useMemo(() => {
    return allRoutes?.filter((r) => r.projectID === project.id) || [];
  }, [allRoutes, project.id]);

  // 获取每个 ClientType 的路由数量
  const getRouteCount = (clientType: ClientType) => {
    return projectRoutes.filter((r) => r.clientType === clientType).length;
  };

  // 获取每个 ClientType 的 streaming 请求数（只统计当前项目的路由）
  const getStreamingCount = (clientType: ClientType) => {
    const clientRoutes = projectRoutes.filter((r) => r.clientType === clientType);
    let count = 0;
    for (const route of clientRoutes) {
      count += countsByRoute.get(route.id) || 0;
    }
    return count;
  };

  // 检查某个 ClientType 是否启用了自定义路由
  const isCustomRoutesEnabled = (clientType: ClientType): boolean => {
    return (project.enabledCustomRoutes || []).includes(clientType);
  };

  // 切换 ClientType 的自定义路由开关
  const handleToggleCustomRoutes = (clientType: ClientType, enabled: boolean) => {
    const currentEnabled = project.enabledCustomRoutes || [];
    let newEnabled: ClientType[];

    if (enabled) {
      // 添加到列表
      newEnabled = [...currentEnabled, clientType];
    } else {
      // 从列表移除
      newEnabled = currentEnabled.filter((ct) => ct !== clientType);
    }

    updateProject.mutate(
      {
        id: project.id,
        data: {
          name: project.name,
          slug: project.slug,
          enabledCustomRoutes: newEnabled,
        },
      },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
          queryClient.invalidateQueries({
            queryKey: projectKeys.slug(project.slug),
          });
        },
      },
    );
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full p-6">
        <div className="text-muted-foreground">{t('common.loading')}</div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* ClientType Tabs */}
      <div className="flex items-center gap-2 px-lg py-4 border-b border-border bg-card shrink-0">
        {CLIENT_TYPES.map((clientType) => {
          const isActive = activeClientType === clientType;
          const routeCount = getRouteCount(clientType);
          const streamingCount = getStreamingCount(clientType);
          const color = getClientColor(clientType);

          return (
            <button
              key={clientType}
              onClick={() => setActiveClientType(clientType)}
              className={cn(
                'relative flex items-center gap-2 px-4 py-2 rounded-lg transition-all',
                isActive
                  ? 'bg-accent/10 text-accent border border-accent/20'
                  : 'bg-muted text-muted-foreground hover:bg-accent border border-transparent',
              )}
            >
              <ClientIcon type={clientType} size={16} />
              <span className="text-sm font-medium">{getClientName(clientType)}</span>
              {routeCount > 0 && (
                <span className="text-[10px] font-mono text-muted-foreground">{routeCount}</span>
              )}
              {streamingCount > 0 && <StreamingBadge count={streamingCount} color={color} />}
            </button>
          );
        })}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0 overflow-hidden">
        <ProjectClientTypeWrapper
          clientType={activeClientType}
          project={project}
          projectRoutes={projectRoutes}
          isCustomRoutesEnabled={isCustomRoutesEnabled(activeClientType)}
          onToggleCustomRoutes={(enabled) => handleToggleCustomRoutes(activeClientType, enabled)}
        />
      </div>
    </div>
  );
}
