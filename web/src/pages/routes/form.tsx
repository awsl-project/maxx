import { useState, useEffect } from 'react'
import { Button, Input } from '@/components/ui'
import {
  useCreateRoute,
  useUpdateRoute,
  useProviders,
  useProjects,
} from '@/hooks/queries'
import type { ClientType, Route } from '@/lib/transport'

interface RouteFormProps {
  route?: Route
  onClose: () => void
  isGlobal?: boolean
  projectId?: number
}

export function RouteForm({
  route,
  onClose,
  isGlobal,
  projectId,
}: RouteFormProps) {
  const createRoute = useCreateRoute()
  const updateRoute = useUpdateRoute()
  const { data: providers } = useProviders()
  const { data: projects } = useProjects()
  const isEditing = !!route

  const [clientType, setClientType] = useState<ClientType>('openai')
  const [providerID, setProviderID] = useState('')
  const [projectID, setProjectID] = useState(
    projectId !== undefined ? String(projectId) : '0'
  )
  const [position, setPosition] = useState('1')
  const [isEnabled, setIsEnabled] = useState(true)

  useEffect(() => {
    if (route) {
      setClientType(route.clientType)
      setProviderID(String(route.providerID))
      setProjectID(String(route.projectID))
      setPosition(String(route.position))
      setIsEnabled(route.isEnabled)
    }
  }, [route])

  // Lock projectID for global routes or when projectId is provided
  useEffect(() => {
    if (isGlobal) {
      setProjectID('0')
    } else if (projectId !== undefined) {
      setProjectID(String(projectId))
    }
  }, [isGlobal, projectId])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()

    const data = {
      clientType,
      providerID: Number(providerID),
      projectID: Number(projectID),
      position: Number(position),
      isEnabled,
      isNative: route?.isNative ?? false, // 手动创建的 Route 默认为转换路由
      retryConfigID: route?.retryConfigID ?? 0,
    }

    if (isEditing) {
      updateRoute.mutate({ id: route.id, data }, { onSuccess: () => onClose() })
    } else {
      createRoute.mutate(data, { onSuccess: () => onClose() })
    }
  }

  const isPending = createRoute.isPending || updateRoute.isPending
  const showProjectSelector = !isGlobal && projectId === undefined

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2">
        <div>
          <label className="mb-1 block text-sm font-medium">Client Type</label>
          <select
            value={clientType}
            onChange={e => setClientType(e.target.value as ClientType)}
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
          >
            <option value="claude">Claude</option>
            <option value="openai">OpenAI</option>
            <option value="codex">Codex</option>
            <option value="gemini">Gemini</option>
          </select>
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">Provider</label>
          <select
            value={providerID}
            onChange={e => setProviderID(e.target.value)}
            required
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
          >
            <option value="">Select provider...</option>
            {providers?.map(p => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {showProjectSelector && (
          <div>
            <label className="mb-1 block text-sm font-medium">Project</label>
            <select
              value={projectID}
              onChange={e => setProjectID(e.target.value)}
              className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs focus:border-ring focus:ring-2 focus:ring-ring/50 outline-none"
            >
              <option value="0">Global (All Projects)</option>
              {projects?.map(p => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
        )}
        <div>
          <label className="mb-1 block text-sm font-medium">
            Position (lower = higher priority)
          </label>
          <Input
            type="number"
            value={position}
            onChange={e => setPosition(e.target.value)}
            min="1"
            required
          />
        </div>
      </div>

      <div className="flex items-center gap-2">
        <input
          type="checkbox"
          id="isEnabled"
          checked={isEnabled}
          onChange={e => setIsEnabled(e.target.checked)}
          className="h-4 w-4 rounded border-gray-300"
        />
        <label htmlFor="isEnabled" className="text-sm font-medium">
          Enabled
        </label>
      </div>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={isPending}>
          {isPending ? 'Saving...' : isEditing ? 'Update' : 'Create'}
        </Button>
      </div>
    </form>
  )
}
