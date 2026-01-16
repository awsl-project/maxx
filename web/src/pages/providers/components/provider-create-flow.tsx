import { useState } from 'react'
import { Globe, ChevronLeft, Key, Check } from 'lucide-react'
import { useCreateProvider } from '@/hooks/queries'
import type { ClientType, CreateProviderData } from '@/lib/transport'
import {
  quickTemplates,
  defaultClients,
  type ClientConfig,
  type ProviderFormData,
  type CreateStep,
} from '../types'
import { ClientsConfigSection } from './clients-config-section'
import { SelectTypeStep } from './select-type-step'
import { AntigravityTokenImport } from './antigravity-token-import'
import { KiroTokenImport } from './kiro-token-import'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

interface ProviderCreateFlowProps {
  onClose: () => void
}

export function ProviderCreateFlow({ onClose }: ProviderCreateFlowProps) {
  const [step, setStep] = useState<CreateStep>('select-type')
  const [saving, setSaving] = useState(false)
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>(
    'idle'
  )
  const createProvider = useCreateProvider()

  const [formData, setFormData] = useState<ProviderFormData>({
    type: 'custom',
    name: '',
    selectedTemplate: null,
    baseURL: '',
    apiKey: '',
    clients: [...defaultClients],
  })

  const selectType = (type: 'custom' | 'antigravity' | 'kiro') => {
    setFormData(prev => ({ ...prev, type }))
    if (type === 'antigravity') {
      setStep('antigravity-import')
    } else if (type === 'kiro') {
      setStep('kiro-import')
    }
  }

  const applyTemplate = (templateId: string) => {
    const template = quickTemplates.find(t => t.id === templateId)
    if (template) {
      const updatedClients = defaultClients.map(client => {
        const isSupported = template.supportedClients.includes(client.id)
        const baseURL = template.clientBaseURLs[client.id] || ''
        return { ...client, enabled: isSupported, urlOverride: baseURL }
      })

      setFormData(prev => ({
        ...prev,
        selectedTemplate: templateId,
        name: template.name,
        clients: updatedClients,
      }))

      setStep('custom-config')
    }
  }

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
    if (!formData.apiKey.trim()) return false
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

      const data: CreateProviderData = {
        type: 'custom',
        name: formData.name,
        config: {
          custom: {
            baseURL: formData.baseURL,
            apiKey: formData.apiKey,
            clientBaseURL:
              Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
          },
        },
        supportedClientTypes,
      }

      await createProvider.mutateAsync(data)
      setSaveStatus('success')
      setTimeout(() => onClose(), 500)
    } catch (error) {
      console.error('Failed to create provider:', error)
      setSaveStatus('error')
    } finally {
      setSaving(false)
    }
  }

  const handleBack = () => {
    if (step === 'custom-config' || step === 'antigravity-import' || step === 'kiro-import') {
      setStep('select-type')
    } else {
      onClose()
    }
  }

  if (step === 'select-type') {
    return (
      <SelectTypeStep
        formData={formData}
        onSelectType={selectType}
        onApplyTemplate={applyTemplate}
        onSkipToConfig={() => setStep('custom-config')}
        onBack={handleBack}
      />
    )
  }

  if (step === 'antigravity-import') {
    const handleCreateAntigravityProvider = async (
      data: CreateProviderData
    ) => {
      await createProvider.mutateAsync(data)
      onClose()
    }
    return (
      <AntigravityTokenImport
        onBack={handleBack}
        onCreateProvider={handleCreateAntigravityProvider}
      />
    )
  }

  if (step === 'kiro-import') {
    const handleCreateKiroProvider = async (data: CreateProviderData) => {
      await createProvider.mutateAsync(data)
      onClose()
    }
    return (
      <KiroTokenImport
        onBack={handleBack}
        onCreateProvider={handleCreateKiroProvider}
      />
    )
  }

  // Custom: Configuration
  return (
    <div className="flex flex-col h-full">
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-card">
        <div className="flex items-center gap-4">
          <Button onClick={handleBack} variant="ghost" size="sm">
            <ChevronLeft size={20} />
          </Button>
          <div>
            <h2 className="text-headline font-semibold text-foreground">
              Configure Provider
            </h2>
            <p className="text-caption text-muted-foreground">
              Set up your custom provider connection
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
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
              'Create Provider'
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
                  placeholder="e.g. Production OpenAI"
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
                    placeholder="https://api.openai.com/v1"
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
                      <span>API Key</span>
                    </div>
                  </label>
                  <Input
                    type="password"
                    value={formData.apiKey}
                    onChange={e =>
                      setFormData(prev => ({ ...prev, apiKey: e.target.value }))
                    }
                    placeholder="sk-..."
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
              Failed to create provider. Please check your connection and try
              again.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
