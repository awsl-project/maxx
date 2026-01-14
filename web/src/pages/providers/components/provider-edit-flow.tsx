import { useState } from 'react'
import { Globe, ChevronLeft, Key, Check, Trash2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { useUpdateProvider, useDeleteProvider } from '@/hooks/queries'
import type { Provider, ClientType, CreateProviderData } from '@/lib/transport'
import { defaultClients, type ClientConfig } from '../types'
import { ClientsConfigSection } from './clients-config-section'
import { AntigravityProviderView } from './antigravity-provider-view'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

interface ProviderEditFlowProps {
  provider: Provider
  onClose: () => void
}

type EditFormData = {
  name: string
  baseURL: string
  apiKey: string
  clients: ClientConfig[]
}

export function ProviderEditFlow({ provider, onClose }: ProviderEditFlowProps) {
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>(
    'idle'
  )
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const updateProvider = useUpdateProvider()
  const deleteProvider = useDeleteProvider()

  const initClients = (): ClientConfig[] => {
    const supportedTypes = provider.supportedClientTypes || []
    return defaultClients.map(client => {
      const isEnabled = supportedTypes.includes(client.id)
      const urlOverride =
        provider.config?.custom?.clientBaseURL?.[client.id] || ''
      return { ...client, enabled: isEnabled, urlOverride }
    })
  }

  const [formData, setFormData] = useState<EditFormData>({
    name: provider.name,
    baseURL: provider.config?.custom?.baseURL || '',
    apiKey: provider.config?.custom?.apiKey || '',
    clients: initClients(),
  })

  const updateClient = (
    clientId: ClientType,
    updates: Partial<ClientConfig>
  ) => {
    setFormData(prev => ({
      ...prev,
      clients: prev.clients.map(c =>
        c.id === clientId ? { ...c, ...updates } : c
      ),
    }))
  }

  const isValid = () => {
    if (!formData.name.trim()) return false
    const hasEnabledClient = formData.clients.some(c => c.enabled)
    const hasUrl =
      formData.baseURL.trim() ||
      formData.clients.some(c => c.enabled && c.urlOverride.trim())
    return hasEnabledClient && hasUrl
  }

  const handleSave = async () => {
    if (!isValid()) return

    setSaving(true)
    setSaveStatus('idle')

    try {
      const supportedClientTypes = formData.clients
        .filter(c => c.enabled)
        .map(c => c.id)
      const clientBaseURL: Partial<Record<ClientType, string>> = {}
      formData.clients.forEach(c => {
        if (c.enabled && c.urlOverride) {
          clientBaseURL[c.id] = c.urlOverride
        }
      })

      const data: Partial<CreateProviderData> = {
        name: formData.name,
        type: provider.type || 'custom', // Preserve the provider type
        config: {
          custom: {
            baseURL: formData.baseURL,
            apiKey: formData.apiKey || provider.config?.custom?.apiKey || '',
            clientBaseURL:
              Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
          },
        },
        supportedClientTypes,
      }

      await updateProvider.mutateAsync({ id: Number(provider.id), data })
      setSaveStatus('success')
      setTimeout(() => onClose(), 500)
    } catch (error) {
      console.error('Failed to update provider:', error)
      setSaveStatus('error')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    setDeleting(true)
    try {
      await deleteProvider.mutateAsync(Number(provider.id))
      onClose()
    } catch (error) {
      console.error('Failed to delete provider:', error)
    } finally {
      setDeleting(false)
      setShowDeleteConfirm(false)
    }
  }

  // Antigravity provider (read-only for now)
  if (provider.type === 'antigravity') {
    return (
      <>
        <AntigravityProviderView
          provider={provider}
          onDelete={() => setShowDeleteConfirm(true)}
          onClose={onClose}
        />
        <DeleteConfirmModal
          providerName={provider.name}
          deleting={deleting}
          open={showDeleteConfirm}
          onConfirm={handleDelete}
          onCancel={() => setShowDeleteConfirm(false)}
        />
      </>
    )
  }

  // Custom provider edit form
  return (
    <div className="flex flex-col h-full">
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary">
        <div className="flex items-center gap-4">
          <Button onClick={onClose} variant={'ghost'}>
            <ChevronLeft size={20} />
          </Button>
          <div>
            <h2 className="text-headline font-semibold text-text-primary">
              Edit Provider
            </h2>
            <p className="text-caption text-text-secondary">
              Update your custom provider settings
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            onClick={() => setShowDeleteConfirm(true)}
            variant={'destructive'}
          >
            <Trash2 size={14} />
            Delete
          </Button>
          <Button onClick={onClose} variant={'secondary'}>
            Cancel
          </Button>
          <Button
            onClick={handleSave}
            disabled={saving || !isValid()}
            variant={'default'}
          >
            {saving ? (
              'Saving...'
            ) : saveStatus === 'success' ? (
              <>
                <Check size={14} /> Saved
              </>
            ) : (
              'Save Changes'
            )}
          </Button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              1. Basic Information
            </h3>

            <div className="grid gap-6">
              <div>
                <label className="text-sm font-medium text-text-primary block mb-2">
                  Display Name
                </label>
                <Input
                  type="text"
                  value={formData.name}
                  onChange={e =>
                    setFormData(prev => ({ ...prev, name: e.target.value }))
                  }
                  placeholder="My Provider"
                  className="w-full"
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <label className="text-sm font-medium text-text-primary block mb-2">
                    <div className="flex items-center gap-2">
                      <Globe size={14} />
                      <span>API Endpoint</span>
                    </div>
                  </label>
                  <Input
                    type="text"
                    value={formData.baseURL}
                    onChange={e =>
                      setFormData(prev => ({
                        ...prev,
                        baseURL: e.target.value,
                      }))
                    }
                    placeholder="https://api.example.com/v1"
                    className="w-full"
                  />
                  <p className="text-xs text-text-secondary mt-1">
                    Optional if client-specific URLs are set below.
                  </p>
                </div>

                <div>
                  <label className="text-sm font-medium text-text-primary block mb-2">
                    <div className="flex items-center gap-2">
                      <Key size={14} />
                      <span>API Key (leave empty to keep current)</span>
                    </div>
                  </label>
                  <Input
                    type="password"
                    value={formData.apiKey}
                    onChange={e =>
                      setFormData(prev => ({ ...prev, apiKey: e.target.value }))
                    }
                    placeholder="••••••••"
                    className="w-full"
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              2. Client Configuration
            </h3>
            <ClientsConfigSection
              clients={formData.clients}
              onUpdateClient={updateClient}
            />
          </div>

          {saveStatus === 'error' && (
            <div className="p-4 bg-error/10 border border-error/30 rounded-lg text-sm text-error flex items-center gap-2">
              <div className="w-1.5 h-1.5 rounded-full bg-error" />
              Failed to update provider. Please check your connection and try
              again.
            </div>
          )}
        </div>
      </div>

      <DeleteConfirmModal
        providerName={provider.name}
        deleting={deleting}
        open={showDeleteConfirm}
        onConfirm={handleDelete}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </div>
  )
}

function DeleteConfirmModal({
  providerName,
  deleting,
  open,
  onConfirm,
  onCancel,
}: {
  providerName: string
  deleting: boolean
  open: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <Dialog open={open} onOpenChange={(isOpen) => !isOpen && onCancel()}>
      <DialogContent className="w-[400px]">
        <DialogHeader>
          <DialogTitle>Delete Provider?</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete{' '}
            <span className="font-medium text-text-primary">
              "{providerName}"
            </span>
            ? This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={onCancel} variant={'secondary'} className="px-4">
            Cancel
          </Button>
          <Button
            onClick={onConfirm}
            disabled={deleting}
            variant={'destructive'}
            className="px-4"
          >
            {deleting ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
