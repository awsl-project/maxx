import { useState, useMemo, useRef } from 'react'
import { Plus, Layers, Wand2, Server, Download, Upload } from 'lucide-react'
import { useProviders, useAllProviderStats } from '@/hooks/queries'
import { useStreamingRequests } from '@/hooks/use-streaming'
import type { Provider, ImportResult } from '@/lib/transport'
import { getTransport } from '@/lib/transport'
import { ProviderRow } from './components/provider-row'
import { ProviderCreateFlow } from './components/provider-create-flow'
import { ProviderEditFlow } from './components/provider-edit-flow'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/layout/page-header'

export function ProvidersPage() {
  const { data: providers, isLoading } = useProviders()
  const { data: providerStats = {} } = useAllProviderStats()
  const { countsByProvider } = useStreamingRequests()
  const [showCreateFlow, setShowCreateFlow] = useState(false)
  const [editingProvider, setEditingProvider] = useState<Provider | null>(null)
  const [importStatus, setImportStatus] = useState<ImportResult | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const queryClient = useQueryClient()

  const groupedProviders = useMemo(() => {
    const antigravity: Provider[] = []
    const custom: Provider[] = []

    providers?.forEach(p => {
      if (p.type === 'antigravity') {
        antigravity.push(p)
      } else {
        custom.push(p)
      }
    })

    return { antigravity, custom }
  }, [providers])

  // Export providers as JSON file
  const handleExport = async () => {
    try {
      const transport = getTransport()
      const data = await transport.exportProviders()
      const blob = new Blob([JSON.stringify(data, null, 2)], {
        type: 'application/json',
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `providers-${new Date().toISOString().split('T')[0]}.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch (error) {
      console.error('Export failed:', error)
    }
  }

  // Import providers from JSON file
  const handleImport = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return

    try {
      const text = await file.text()
      const data = JSON.parse(text) as Provider[]
      const transport = getTransport()
      const result = await transport.importProviders(data)
      setImportStatus(result)
      queryClient.invalidateQueries({ queryKey: ['providers'] })
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      // Clear file input
      if (fileInputRef.current) {
        fileInputRef.current.value = ''
      }
      // Auto-hide status after 5 seconds
      setTimeout(() => setImportStatus(null), 5000)
    } catch (error) {
      console.error('Import failed:', error)
      setImportStatus({
        imported: 0,
        skipped: 0,
        errors: [`Import failed: ${error}`],
      })
      setTimeout(() => setImportStatus(null), 5000)
    }
  }

  // Show edit flow
  if (editingProvider) {
    return (
      <ProviderEditFlow
        provider={editingProvider}
        onClose={() => setEditingProvider(null)}
      />
    )
  }

  // Show create flow
  if (showCreateFlow) {
    return <ProviderCreateFlow onClose={() => setShowCreateFlow(false)} />
  }

  // Provider list
  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Layers}
        iconClassName="text-blue-500"
        title="Providers"
        description={`${providers?.length || 0} configured`}
      >
        <input
          type="file"
          ref={fileInputRef}
          onChange={handleImport}
          accept=".json"
          className="hidden"
        />
        <Button
          onClick={() => fileInputRef.current?.click()}
          className="flex items-center gap-2"
          title="Import Providers"
          variant={'outline'}
        >
          <Upload size={14} />
          <span>Import</span>
        </Button>
        <Button
          onClick={handleExport}
          className="flex items-center gap-2"
          disabled={!providers?.length}
          title="Export Providers"
          variant={'outline'}
        >
          <Download size={14} />
          <span>Export</span>
        </Button>
        <Button onClick={() => setShowCreateFlow(true)}>
          <Plus size={14} />
          <span>Add Provider</span>
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl">
          {isLoading ? (
            <div className="flex items-center justify-center h-full">
              <div className="text-text-muted">Loading...</div>
            </div>
          ) : providers?.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-text-muted">
              <Layers size={48} className="mb-4 opacity-50" />
              <p className="text-body">No providers configured</p>
              <p className="text-caption mt-2">
                Click "Add Provider" to create one
              </p>
              <Button
                onClick={() => setShowCreateFlow(true)}
                className=" mt-6 flex items-center gap-2"
              >
                <Plus size={14} />
                <span>Add Provider</span>
              </Button>
            </div>
          ) : (
            <div className="space-y-8">
              {/* Antigravity Section */}
              {groupedProviders.antigravity.length > 0 && (
                <section className="space-y-3">
                  <div className="flex items-center gap-2 px-1">
                    <Wand2 size={16} className="text-purple-400" />
                    <h3 className="text-sm font-semibold text-text-secondary uppercase tracking-wider">
                      Antigravity Cloud
                    </h3>
                    <div className="h-px flex-1 bg-border/50 ml-2" />
                  </div>
                  <div className="space-y-3">
                    {groupedProviders.antigravity.map(provider => (
                      <ProviderRow
                        key={provider.id}
                        provider={provider}
                        stats={providerStats[provider.id]}
                        streamingCount={countsByProvider.get(provider.id) || 0}
                        onClick={() => setEditingProvider(provider)}
                      />
                    ))}
                  </div>
                </section>
              )}

              {/* Custom Section */}
              {groupedProviders.custom.length > 0 && (
                <section className="space-y-3">
                  <div className="flex items-center gap-2 px-1">
                    <Server size={16} className="text-blue-400" />
                    <h3 className="text-sm font-semibold text-text-secondary uppercase tracking-wider">
                      Custom Providers
                    </h3>
                    <div className="h-px flex-1 bg-border/50 ml-2" />
                  </div>
                  <div className="space-y-3">
                    {groupedProviders.custom.map(provider => (
                      <ProviderRow
                        key={provider.id}
                        provider={provider}
                        stats={providerStats[provider.id]}
                        streamingCount={countsByProvider.get(provider.id) || 0}
                        onClick={() => setEditingProvider(provider)}
                      />
                    ))}
                  </div>
                </section>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Import Status Toast */}
      {importStatus && (
        <div className="fixed bottom-6 right-6 bg-surface-primary border border-border rounded-lg shadow-lg p-4">
          <div className="space-y-2">
            <div className="text-sm font-medium text-text-primary">
              Import completed: {importStatus.imported} imported,{' '}
              {importStatus.skipped} skipped
            </div>
            {importStatus.errors.length > 0 && (
              <div className="text-xs text-red-400 space-y-1">
                {importStatus.errors.map((error, i) => (
                  <div key={i}>• {error}</div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
