import { useState } from 'react'
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Badge,
} from '@/components/ui'
import {
  useRoutingStrategies,
  useCreateRoutingStrategy,
  useUpdateRoutingStrategy,
  useDeleteRoutingStrategy,
  useProjects,
} from '@/hooks/queries'
import { Plus, Trash2, Pencil } from 'lucide-react'
import type { RoutingStrategy, RoutingStrategyType } from '@/lib/transport'

export function RoutingStrategiesPage() {
  const { data: strategies, isLoading } = useRoutingStrategies()
  const { data: projects } = useProjects()
  const createStrategy = useCreateRoutingStrategy()
  const updateStrategy = useUpdateRoutingStrategy()
  const deleteStrategy = useDeleteRoutingStrategy()
  const [showForm, setShowForm] = useState(false)
  const [editingStrategy, setEditingStrategy] = useState<
    RoutingStrategy | undefined
  >()

  const [projectID, setProjectID] = useState('0')
  const [type, setType] = useState<RoutingStrategyType>('priority')

  const resetForm = () => {
    setProjectID('0')
    setType('priority')
  }

  const handleEdit = (strategy: RoutingStrategy) => {
    setEditingStrategy(strategy)
    setProjectID(String(strategy.projectID))
    setType(strategy.type)
    setShowForm(true)
  }

  const handleCloseForm = () => {
    setShowForm(false)
    setEditingStrategy(undefined)
    resetForm()
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const data = {
      projectID: Number(projectID),
      type,
      config: null,
    }

    if (editingStrategy) {
      updateStrategy.mutate(
        { id: editingStrategy.id, data },
        { onSuccess: handleCloseForm }
      )
    } else {
      createStrategy.mutate(data, { onSuccess: handleCloseForm })
    }
  }

  const handleDelete = (id: number) => {
    if (confirm('Are you sure you want to delete this strategy?')) {
      deleteStrategy.mutate(id)
    }
  }

  const getProjectName = (pid: number) => {
    if (pid === 0) return 'Global'
    return projects?.find(p => p.id === pid)?.name ?? `#${pid}`
  }

  const isPending = createStrategy.isPending || updateStrategy.isPending

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold">Routing Strategies</h2>
        <Button onClick={() => setShowForm(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Add Strategy
        </Button>
      </div>

      {showForm && (
        <Card>
          <CardHeader>
            <CardTitle>
              {editingStrategy
                ? 'Edit Routing Strategy'
                : 'New Routing Strategy'}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid gap-4 md:grid-cols-2">
                <div>
                  <label className="mb-1 block text-sm font-medium">
                    Project
                  </label>
                  <select
                    value={projectID}
                    onChange={e => setProjectID(e.target.value)}
                    className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs focus:border-ring focus:ring-2 focus:ring-ring/50 outline-none"
                  >
                    <option value="0">Global</option>
                    {projects?.map(p => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium">Type</label>
                  <select
                    value={type}
                    onChange={e =>
                      setType(e.target.value as RoutingStrategyType)
                    }
                    className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs focus:border-ring focus:ring-2 focus:ring-ring/50 outline-none"
                  >
                    <option value="priority">Priority (by position)</option>
                    <option value="weighted_random">Weighted Random</option>
                  </select>
                </div>
              </div>
              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleCloseForm}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={isPending}>
                  {isPending
                    ? 'Saving...'
                    : editingStrategy
                      ? 'Update'
                      : 'Create'}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>All Strategies</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-gray-500">Loading...</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>Project</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {strategies?.map(strategy => (
                  <TableRow key={strategy.id}>
                    <TableCell className="font-mono">{strategy.id}</TableCell>
                    <TableCell>
                      <span
                        className={
                          strategy.projectID === 0 ? 'text-gray-400' : ''
                        }
                      >
                        {getProjectName(strategy.projectID)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          strategy.type === 'priority' ? 'info' : 'warning'
                        }
                      >
                        {strategy.type === 'priority'
                          ? 'Priority'
                          : 'Weighted Random'}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleEdit(strategy)}
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDelete(strategy.id)}
                          disabled={deleteStrategy.isPending}
                        >
                          <Trash2 className="h-4 w-4 text-red-500" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
                {(!strategies || strategies.length === 0) && (
                  <TableRow>
                    <TableCell
                      colSpan={4}
                      className="text-center text-gray-500"
                    >
                      No strategies
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
