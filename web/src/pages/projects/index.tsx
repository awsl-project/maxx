import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  CardFooter,
} from '@/components/ui'
import { useProjects, useCreateProject } from '@/hooks/queries'
import {
  Plus,
  X,
  FolderKanban,
  Loader2,
  Calendar,
  ArrowRight,
  Hash,
} from 'lucide-react'
import { PageHeader } from '@/components/layout'

export function ProjectsPage() {
  const navigate = useNavigate()
  const { data: projects, isLoading } = useProjects()
  const createProject = useCreateProject()
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createProject.mutate(
      { name, enabledCustomRoutes: [] },
      {
        onSuccess: project => {
          setShowForm(false)
          setName('')
          // 创建后自动跳转到详情页
          navigate(`/projects/${project.slug}`)
        },
      }
    )
  }

  const handleRowClick = (slug: string) => {
    navigate(`/projects/${slug}`)
  }

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={FolderKanban}
        iconClassName="text-amber-500"
        title="Projects"
        description="Manage your projects and environments"
      >
        <Button
          onClick={() => setShowForm(!showForm)}
          variant={showForm ? 'secondary' : 'default'}
        >
          {showForm ? (
            <X className="mr-2 h-4 w-4" />
          ) : (
            <Plus className="mr-2 h-4 w-4" />
          )}
          {showForm ? 'Cancel' : 'Add Project'}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-auto p-6 space-y-6">
        {showForm && (
          <Card className="border-border bg-surface-primary animate-in slide-in-from-top-4 duration-200">
            <CardContent className="pt-6">
              <form onSubmit={handleSubmit} className="flex gap-4 items-end">
                <div className="flex-1 space-y-2">
                  <label className="text-xs font-medium text-text-secondary uppercase tracking-wider">
                    Project Name
                  </label>
                  <Input
                    value={name}
                    onChange={e => setName(e.target.value)}
                    placeholder="e.g., Production, Staging, Experiment-A"
                    required
                    className="bg-surface-secondary border-border"
                    autoFocus
                  />
                </div>
                <Button type="submit" disabled={createProject.isPending}>
                  {createProject.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    'Create Project'
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
            {projects.map(project => (
              <Card
                key={project.id}
                className="group border-border bg-surface-primary cursor-pointer hover:border-accent/50 hover:shadow-md transition-all duration-200 flex flex-col"
                onClick={() => handleRowClick(project.slug)}
              >
                <CardHeader className="pb-3 space-y-0">
                  <div className="flex justify-between items-start">
                    <div className="p-2 bg-surface-secondary rounded-md text-text-secondary group-hover:text-accent group-hover:bg-accent/10 transition-colors">
                      <FolderKanban size={20} />
                    </div>
                  </div>
                  <CardTitle className="pt-3 text-base font-semibold text-text-primary leading-tight truncate">
                    {project.name}
                  </CardTitle>
                </CardHeader>
                <CardContent className="pb-3 flex-1">
                  <div className="flex items-center gap-2 text-xs text-text-secondary bg-surface-secondary/50 p-2 rounded border border-transparent group-hover:border-border transition-colors">
                    <Hash size={12} className="opacity-50" />
                    <span className="font-mono truncate">{project.slug}</span>
                  </div>
                </CardContent>
                <CardFooter className="pt-0 text-xs text-text-muted flex justify-between items-center border-t border-border/50 mt-auto p-4 bg-surface-secondary/20">
                  <span>
                    Created{' '}
                    {new Date(project.createdAt).toLocaleDateString(undefined, {
                      month: 'short',
                      day: 'numeric',
                      year: 'numeric',
                    })}
                  </span>
                  <div className="flex items-center text-accent opacity-0 -translate-x-2 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-200 font-medium">
                    Manage <ArrowRight size={12} className="ml-1" />
                  </div>
                </CardFooter>
              </Card>
            ))}
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center h-64 text-text-muted border-2 border-dashed border-border rounded-lg bg-surface-primary/50">
            <Calendar className="h-12 w-12 opacity-20 mb-4" />
            <p className="text-lg font-medium">No projects found</p>
            <p className="text-sm">Create a new project to get started</p>
          </div>
        )}
      </div>
    </div>
  )
}
