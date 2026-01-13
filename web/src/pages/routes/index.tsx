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
import { useRoutes, useDeleteRoute, useProviders } from '@/hooks/queries'
import { Plus, Trash2, Pencil, Route as RouteIcon } from 'lucide-react'
import { RouteForm } from './form'
import type { Route } from '@/lib/transport'
import { PageHeader } from '@/components/layout/page-header'

export function RoutesPage() {
  const { data: routes, isLoading } = useRoutes()
  const { data: providers } = useProviders()
  const deleteRoute = useDeleteRoute()
  const [showForm, setShowForm] = useState(false)
  const [editingRoute, setEditingRoute] = useState<Route | undefined>()

  // Filter to only show global routes (projectID = 0)
  const globalRoutes = routes?.filter(route => route.projectID === 0) ?? []

  const getProviderName = (providerId: number) => {
    return providers?.find(p => p.id === providerId)?.name ?? `#${providerId}`
  }

  const handleEdit = (route: Route) => {
    setEditingRoute(route)
    setShowForm(true)
  }

  const handleCloseForm = () => {
    setShowForm(false)
    setEditingRoute(undefined)
  }

  const handleDelete = (id: number) => {
    if (confirm('Are you sure you want to delete this route?')) {
      deleteRoute.mutate(id)
    }
  }

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={RouteIcon}
        iconClassName="text-violet-500"
        title="Global Routes"
        description="These routes apply to all projects. Project-specific routes can be configured in each project's settings."
      >
        <Button onClick={() => setShowForm(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Add Route
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-6 space-y-6">
        {showForm && (
          <Card>
            <CardHeader>
              <CardTitle>{editingRoute ? 'Edit Route' : 'New Route'}</CardTitle>
            </CardHeader>
            <CardContent>
              <RouteForm
                route={editingRoute}
                onClose={handleCloseForm}
                isGlobal
              />
            </CardContent>
          </Card>
        )}

        <Card>
          <CardHeader>
            <CardTitle>All Global Routes</CardTitle>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <p className="text-gray-500">Loading...</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>ID</TableHead>
                    <TableHead>Client</TableHead>
                    <TableHead>Provider</TableHead>
                    <TableHead>Position</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {globalRoutes.map(route => (
                    <TableRow key={route.id}>
                      <TableCell className="font-mono">{route.id}</TableCell>
                      <TableCell>
                        <Badge variant="info">{route.clientType}</Badge>
                      </TableCell>
                      <TableCell>{getProviderName(route.providerID)}</TableCell>
                      <TableCell className="font-mono">
                        {route.position}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={route.isEnabled ? 'success' : 'default'}
                        >
                          {route.isEnabled ? 'Enabled' : 'Disabled'}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className="flex gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleEdit(route)}
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleDelete(route.id)}
                            disabled={deleteRoute.isPending}
                          >
                            <Trash2 className="h-4 w-4 text-red-500" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                  {globalRoutes.length === 0 && (
                    <TableRow>
                      <TableCell
                        colSpan={6}
                        className="text-center text-gray-500"
                      >
                        No global routes configured
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
