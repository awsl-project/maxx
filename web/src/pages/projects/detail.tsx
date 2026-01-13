import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Button,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from '@/components/ui'
import {
  useProjectBySlug,
  useDeleteProject,
  projectKeys,
} from '@/hooks/queries'
import {
  ArrowLeft,
  Trash2,
  Loader2,
  FolderKanban,
  LayoutGrid,
  Route,
  Users,
  FileText,
} from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { OverviewTab } from './tabs/overview'
import { RoutesTab } from './tabs/routes'
import { SessionsTab } from './tabs/sessions'
import { RequestsTab } from './tabs/requests'

type TabId = 'overview' | 'routes' | 'sessions' | 'requests'

export function ProjectDetailPage() {
  const { slug } = useParams<{ slug: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data: project, isLoading, error } = useProjectBySlug(slug ?? '')
  const deleteProject = useDeleteProject()
  const [activeTab, setActiveTab] = useState<TabId>('overview')

  const handleDelete = () => {
    if (!project) return
    if (confirm(`Are you sure you want to delete project "${project.name}"?`)) {
      deleteProject.mutate(project.id, {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: projectKeys.lists() })
          navigate('/projects')
        },
      })
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-8 w-8 animate-spin text-accent" />
      </div>
    )
  }

  if (error || !project) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4">
        <p className="text-text-muted">Project not found</p>
        <Button variant="secondary" onClick={() => navigate('/projects')}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Projects
        </Button>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary shrink-0 z-10">
        <div className="flex items-center gap-4">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate('/projects')}
            className="h-8 w-8 p-0 hover:bg-surface-hover rounded-full"
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="flex items-center gap-3">
            <div className="p-2 bg-accent/10 rounded-lg">
              <FolderKanban size={20} className="text-accent" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-text-primary leading-tight">
                {project.name}
              </h2>
              <p className="text-xs text-text-secondary font-mono mt-0.5">
                {project.slug}
              </p>
            </div>
          </div>
        </div>
        <Button
          variant="destructive"
          size="sm"
          onClick={handleDelete}
          disabled={deleteProject.isPending}
          className="opacity-80 hover:opacity-100"
        >
          {deleteProject.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <>
              <Trash2 className="mr-2 h-4 w-4" />
              Delete Project
            </>
          )}
        </Button>
      </div>

      {/* Tabs */}
      <Tabs
        value={activeTab}
        onValueChange={(v: string) => setActiveTab(v as TabId)}
        className="flex-1 flex flex-col overflow-hidden"
      >
        <div className="px-6 py-4 border-b border-border bg-surface-primary/50">
          <TabsList>
            <TabsTrigger value="overview">
              <LayoutGrid className="h-4 w-4 mr-2" />
              Overview
            </TabsTrigger>
            <TabsTrigger value="routes">
              <Route className="h-4 w-4 mr-2" />
              Routes
            </TabsTrigger>
            <TabsTrigger value="sessions">
              <Users className="h-4 w-4 mr-2" />
              Sessions
            </TabsTrigger>
            <TabsTrigger value="requests">
              <FileText className="h-4 w-4 mr-2" />
              Requests
            </TabsTrigger>
          </TabsList>
        </div>

        <div className="flex-1 overflow-hidden bg-background">
          <TabsContent
            value="overview"
            className="h-full overflow-auto mt-0 animate-in fade-in-50 duration-200"
          >
            <OverviewTab project={project} />
          </TabsContent>
          <TabsContent
            value="routes"
            className="h-full overflow-hidden mt-0 animate-in fade-in-50 duration-200"
          >
            <RoutesTab project={project} />
          </TabsContent>
          <TabsContent
            value="sessions"
            className="h-full overflow-auto mt-0 animate-in fade-in-50 duration-200"
          >
            <SessionsTab project={project} />
          </TabsContent>
          <TabsContent
            value="requests"
            className="h-full overflow-auto mt-0 animate-in fade-in-50 duration-200"
          >
            <RequestsTab project={project} />
          </TabsContent>
        </div>
      </Tabs>
    </div>
  )
}
