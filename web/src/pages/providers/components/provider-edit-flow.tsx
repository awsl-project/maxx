import { useState, useMemo } from 'react'
import { Globe, ChevronLeft, Key, Check, Trash2, Plus, ArrowRight, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  useUpdateProvider,
  useDeleteProvider,
  useModelMappings,
  useCreateModelMapping,
  useUpdateModelMapping,
  useDeleteModelMapping,
} from '@/hooks/queries'
import type { Provider, ClientType, CreateProviderData, ModelMapping, ModelMappingInput } from '@/lib/transport'
import { defaultClients, type ClientConfig } from '../types'
import { ClientsConfigSection } from './clients-config-section'
import { AntigravityProviderView } from './antigravity-provider-view'
import { KiroProviderView } from './kiro-provider-view'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ModelInput } from '@/components/ui/model-input'

// Provider Model Mappings Section for Custom Providers
function ProviderModelMappings({ provider }: { provider: Provider }) {
  const { t } = useTranslation()
  const { data: allMappings } = useModelMappings()
  const createMapping = useCreateModelMapping()
  const updateMapping = useUpdateModelMapping()
  const deleteMapping = useDeleteModelMapping()
  const [newPattern, setNewPattern] = useState('')
  const [newTarget, setNewTarget] = useState('')

  // Filter mappings for this provider
  const providerMappings = useMemo(() => {
    return (allMappings || []).filter(
      m => m.scope === 'provider' && m.providerID === provider.id
    )
  }, [allMappings, provider.id])

  const isPending = createMapping.isPending || updateMapping.isPending || deleteMapping.isPending

  const handleAddMapping = async () => {
    if (!newPattern.trim() || !newTarget.trim()) return

    await createMapping.mutateAsync({
      pattern: newPattern.trim(),
      target: newTarget.trim(),
      scope: 'provider',
      providerID: provider.id,
      providerType: 'custom',
      priority: providerMappings.length * 10 + 1000,
      isEnabled: true,
    })
    setNewPattern('')
    setNewTarget('')
  }

  const handleUpdateMapping = async (mapping: ModelMapping, data: Partial<ModelMappingInput>) => {
    await updateMapping.mutateAsync({
      id: mapping.id,
      data: {
        pattern: data.pattern ?? mapping.pattern,
        target: data.target ?? mapping.target,
        scope: 'provider',
        providerID: provider.id,
        providerType: 'custom',
        priority: mapping.priority,
        isEnabled: mapping.isEnabled,
      },
    })
  }

  const handleDeleteMapping = async (id: number) => {
    await deleteMapping.mutateAsync(id)
  }

  return (
    <div>
      <div className="flex items-center gap-2 mb-4 border-b border-border pb-2">
        <Zap size={18} className="text-yellow-500" />
        <h4 className="text-lg font-semibold text-foreground">
          {t('modelMappings.title')}
        </h4>
        <span className="text-sm text-muted-foreground">
          ({providerMappings.length})
        </span>
      </div>

      <div className="bg-card border border-border rounded-xl p-4">
        <p className="text-xs text-muted-foreground mb-4">
          {t('modelMappings.pageDesc')}
        </p>

        {providerMappings.length > 0 && (
          <div className="space-y-2 mb-4">
            {providerMappings.map((mapping, index) => (
              <div key={mapping.id} className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground w-6 shrink-0">{index + 1}.</span>
                <ModelInput
                  value={mapping.pattern}
                  onChange={pattern => handleUpdateMapping(mapping, { pattern })}
                  placeholder={t('modelMappings.matchPattern')}
                  disabled={isPending}
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
                <ModelInput
                  value={mapping.target}
                  onChange={target => handleUpdateMapping(mapping, { target })}
                  placeholder={t('modelMappings.targetModel')}
                  disabled={isPending}
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleDeleteMapping(mapping.id)}
                  disabled={isPending}
                >
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>
            ))}
          </div>
        )}

        {providerMappings.length === 0 && (
          <div className="text-center py-6 mb-4">
            <p className="text-muted-foreground text-sm">{t('modelMappings.noMappings')}</p>
          </div>
        )}

        <div className="flex items-center gap-2 pt-4 border-t border-border">
          <ModelInput
            value={newPattern}
            onChange={setNewPattern}
            placeholder={t('modelMappings.matchPattern')}
            disabled={isPending}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
          <ModelInput
            value={newTarget}
            onChange={setNewTarget}
            placeholder={t('modelMappings.targetModel')}
            disabled={isPending}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddMapping}
            disabled={!newPattern.trim() || !newTarget.trim() || isPending}
          >
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  )
}

interface ProviderEditFlowProps {
  provider: Provider;
  onClose: () => void;
}

type EditFormData = {
  name: string;
  baseURL: string;
  apiKey: string;
  clients: ClientConfig[];
};

export function ProviderEditFlow({ provider, onClose }: ProviderEditFlowProps) {
  const { t } = useTranslation();
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const updateProvider = useUpdateProvider();
  const deleteProvider = useDeleteProvider();

  const initClients = (): ClientConfig[] => {
    const supportedTypes = provider.supportedClientTypes || [];
    return defaultClients.map((client) => {
      const isEnabled = supportedTypes.includes(client.id);
      const urlOverride = provider.config?.custom?.clientBaseURL?.[client.id] || '';
      return { ...client, enabled: isEnabled, urlOverride };
    });
  };

  const [formData, setFormData] = useState<EditFormData>({
    name: provider.name,
    baseURL: provider.config?.custom?.baseURL || '',
    apiKey: provider.config?.custom?.apiKey || '',
    clients: initClients(),
  });

  const updateClient = (clientId: ClientType, updates: Partial<ClientConfig>) => {
    setFormData((prev) => ({
      ...prev,
      clients: prev.clients.map((c) => (c.id === clientId ? { ...c, ...updates } : c)),
    }));
  };

  const isValid = () => {
    if (!formData.name.trim()) return false;
    const hasEnabledClient = formData.clients.some((c) => c.enabled);
    const hasUrl =
      formData.baseURL.trim() || formData.clients.some((c) => c.enabled && c.urlOverride.trim());
    return hasEnabledClient && hasUrl;
  };

  const handleSave = async () => {
    if (!isValid()) return;

    setSaving(true);
    setSaveStatus('idle');

    try {
      const supportedClientTypes = formData.clients.filter((c) => c.enabled).map((c) => c.id);
      const clientBaseURL: Partial<Record<ClientType, string>> = {};
      formData.clients.forEach((c) => {
        if (c.enabled && c.urlOverride) {
          clientBaseURL[c.id] = c.urlOverride;
        }
      });

      const data: Partial<CreateProviderData> = {
        name: formData.name,
        type: provider.type || 'custom', // Preserve the provider type
        config: {
          custom: {
            baseURL: formData.baseURL,
            apiKey: formData.apiKey || provider.config?.custom?.apiKey || '',
            clientBaseURL: Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
          },
        },
        supportedClientTypes,
      };

      await updateProvider.mutateAsync({ id: Number(provider.id), data });
      setSaveStatus('success');
      setTimeout(() => onClose(), 500);
    } catch (error) {
      console.error('Failed to update provider:', error);
      setSaveStatus('error');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      await deleteProvider.mutateAsync(Number(provider.id));
      onClose();
    } catch (error) {
      console.error('Failed to delete provider:', error);
    } finally {
      setDeleting(false);
      setShowDeleteConfirm(false);
    }
  };

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
    );
  }

  // Kiro provider
  if (provider.type === 'kiro') {
    return (
      <>
        <KiroProviderView
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
    );
  }

  // Custom provider edit form
  return (
    <div className="flex flex-col h-full">
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-card">
        <div className="flex items-center gap-4">
          <Button onClick={onClose} variant={'ghost'}>
            <ChevronLeft size={20} />
          </Button>
          <div>
            <h2 className="text-headline font-semibold text-foreground">Edit Provider</h2>
            <p className="text-caption text-muted-foreground">
              Update your custom provider settings
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button onClick={() => setShowDeleteConfirm(true)} variant={'destructive'}>
            <Trash2 size={14} />
            Delete
          </Button>
          <Button onClick={onClose} variant={'secondary'}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving || !isValid()} variant={'default'}>
            {saving ? (
              t('common.saving')
            ) : saveStatus === 'success' ? (
              <>
                <Check size={14} /> {t('common.saved')}
              </>
            ) : (
              t('provider.saveChanges')
            )}
          </Button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-foreground border-b border-border pb-2">
              1. Basic Information
            </h3>

            <div className="grid gap-6">
              <div>
                <label className="text-sm font-medium text-foreground block mb-2">
                  Display Name
                </label>
                <Input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                  placeholder={t('provider.namePlaceholder')}
                  className="w-full"
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <label className="text-sm font-medium text-foreground block mb-2">
                    <div className="flex items-center gap-2">
                      <Globe size={14} />
                      <span>API Endpoint</span>
                    </div>
                  </label>
                  <Input
                    type="text"
                    value={formData.baseURL}
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        baseURL: e.target.value,
                      }))
                    }
                    placeholder={t('provider.endpointPlaceholder')}
                    className="w-full"
                  />
                  <p className="text-xs text-muted-foreground mt-1">
                    Optional if client-specific URLs are set below.
                  </p>
                </div>

                <div>
                  <label className="text-sm font-medium text-foreground block mb-2">
                    <div className="flex items-center gap-2">
                      <Key size={14} />
                      <span>API Key (leave empty to keep current)</span>
                    </div>
                  </label>
                  <Input
                    type="password"
                    value={formData.apiKey}
                    onChange={(e) => setFormData((prev) => ({ ...prev, apiKey: e.target.value }))}
                    placeholder={t('provider.keyPlaceholder')}
                    className="w-full"
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-6">
            <h3 className="text-lg font-semibold text-foreground border-b border-border pb-2">
              2. Client Configuration
            </h3>
            <ClientsConfigSection clients={formData.clients} onUpdateClient={updateClient} />
          </div>

          {/* Provider Model Mappings */}
          <ProviderModelMappings provider={provider} />

          {saveStatus === 'error' && (
            <div className="p-4 bg-error/10 border border-error/30 rounded-lg text-sm text-error flex items-center gap-2">
              <div className="w-1.5 h-1.5 rounded-full bg-error" />
              Failed to update provider. Please check your connection and try again.
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
  );
}

function DeleteConfirmModal({
  providerName,
  deleting,
  open,
  onConfirm,
  onCancel,
}: {
  providerName: string;
  deleting: boolean;
  open: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={(isOpen) => !isOpen && onCancel()}>
      <DialogContent className="w-[400px]">
        <DialogHeader>
          <DialogTitle>Delete Provider?</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete{' '}
            <span className="font-medium text-foreground">"{providerName}"</span>? This action
            cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={onCancel} variant={'secondary'} className="px-4">
            Cancel
          </Button>
          <Button onClick={onConfirm} disabled={deleting} variant={'destructive'} className="px-4">
            {deleting ? t('common.deleting') : t('common.delete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
