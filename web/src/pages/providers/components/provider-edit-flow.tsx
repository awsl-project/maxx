import { useState, useMemo, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Globe,
  ChevronLeft,
  Key,
  Check,
  Trash2,
  Copy,
  Plus,
  ArrowRight,
  Zap,
  Filter,
  Eye,
  EyeOff,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  useCreateProvider,
  useUpdateProvider,
  useDeleteProvider,
  useModelMappings,
  useCreateModelMapping,
} from '@/hooks/queries';
import type {
  Provider,
  ClientType,
  CreateProviderData,
  ProviderRuntimeModelsPreviewRequest,
} from '@/lib/transport';
import { defaultClients, type ClientConfig, type CustomBackend } from '../types';
import { buildDisguisePayload } from '../utils/disguise';
import { ClientsConfigSection } from './clients-config-section';
import { AntigravityProviderView } from './antigravity-provider-view';
import { BedrockProviderView } from './bedrock-provider-view';
import { KiroProviderView } from './kiro-provider-view';
import { CodexProviderView } from './codex-provider-view';
import { ClaudeProviderView } from './claude-provider-view';
import { OpenRouterProviderView } from './openrouter-provider-view';
import { ZaiProviderView } from './zai-provider-view';
import { FalProviderView } from './fal-provider-view';
import { NewApiProviderView } from './newapi-provider-view';
import { OllamaProviderView } from './ollama-provider-view';
import { GrokProviderView } from './grok-provider-view';
import { ProviderModelMappings } from './provider-model-mappings';
import { SmartMappingRetrySettings } from './smart-mapping-retry-settings';
import { ReasoningPolicySettings } from './reasoning-policy-settings';
import {
  normalizeMaxConcurrency,
  ProviderMaxConcurrencyField,
} from './provider-max-concurrency-field';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ModelInput } from '@/components/ui/model-input';
import { PageHeader } from '@/components/layout/page-header';
import { ProviderProxyURLCard } from './provider-proxy-url-card';
import { normalizeProviderArrayField } from '../utils/provider-normalize';

function ProviderEditSection({
  id,
  title,
  description,
  icon,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  icon?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section
      id={id}
      className="scroll-mt-28 space-y-4 rounded-2xl border border-border bg-card/60 p-5 shadow-sm"
    >
      <div className="flex items-start gap-3 border-b border-border pb-4">
        {icon && (
          <div className="mt-0.5 rounded-lg border border-border bg-background p-2 text-muted-foreground">
            {icon}
          </div>
        )}
        <div className="min-w-0">
          <h3 className="text-lg font-semibold text-foreground">{title}</h3>
          {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
        </div>
      </div>
      <div className="space-y-6">{children}</div>
    </section>
  );
}

function ProviderEditSectionNav({
  title,
  sections,
}: {
  title: string;
  sections: Array<{ id: string; label: string; description: string }>;
}) {
  return (
    <aside className="hidden xl:block">
      <div className="sticky top-28 rounded-2xl border border-border bg-card/70 p-3 shadow-sm">
        <div className="px-3 pb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {title}
        </div>
        <nav className="space-y-1">
          {sections.map((section) => (
            <a
              key={section.id}
              href={`#${section.id}`}
              className="block rounded-xl px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <span className="block font-medium text-foreground">{section.label}</span>
              <span className="mt-0.5 block text-xs text-muted-foreground">
                {section.description}
              </span>
            </a>
          ))}
        </nav>
      </div>
    </aside>
  );
}

function ResponseModelMappings({
  mappings,
  onChange,
  disabled,
}: {
  mappings: Record<string, string>;
  onChange: (mappings: Record<string, string>) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');
  const entries = useMemo(() => Object.entries(mappings), [mappings]);
  const isValidTarget = (target: string) => !target.trim().includes('*');

  const handleAddMapping = () => {
    const pattern = newPattern.trim();
    const target = newTarget.trim();
    if (!pattern || !target || !isValidTarget(target)) return;

    onChange({ ...mappings, [pattern]: target });
    setNewPattern('');
    setNewTarget('');
  };

  const handleUpdateMapping = (oldPattern: string, pattern: string, target: string) => {
    const next: Record<string, string> = {};
    for (const [key, value] of entries) {
      if (key === oldPattern) continue;
      next[key] = value;
    }

    const nextPattern = pattern.trim();
    const nextTarget = target.trim();
    if (nextTarget && !isValidTarget(nextTarget)) return;
    if (nextPattern && nextTarget) {
      next[nextPattern] = nextTarget;
    }
    onChange(next);
  };

  const handleDeleteMapping = (pattern: string) => {
    const next: Record<string, string> = {};
    for (const [key, value] of entries) {
      if (key === pattern) continue;
      next[key] = value;
    }
    onChange(next);
  };

  return (
    <div>
      <div className="flex items-center gap-2 mb-4 border-b border-border pb-2">
        <Zap size={18} className="text-yellow-500" />
        <h4 className="text-lg font-semibold text-foreground">
          {t('responseModelMappings.title')}
        </h4>
        <span className="text-sm text-muted-foreground">({entries.length})</span>
      </div>

      <div className="bg-card border border-border rounded-xl p-4">
        <p className="text-xs text-muted-foreground mb-4">{t('responseModelMappings.pageDesc')}</p>

        {entries.length > 0 && (
          <div className="space-y-2 mb-4">
            {entries.map(([pattern, target], index) => (
              <div key={pattern} className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground w-6 shrink-0">{index + 1}.</span>
                <ModelInput
                  value={pattern}
                  onChange={(value) => handleUpdateMapping(pattern, value, target)}
                  placeholder={t('responseModelMappings.matchPattern')}
                  disabled={disabled}
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
                <Input
                  value={target}
                  onChange={(event) => handleUpdateMapping(pattern, pattern, event.target.value)}
                  placeholder={t('responseModelMappings.targetModel')}
                  disabled={disabled}
                  className="flex-1 min-w-0 h-8 text-sm"
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleDeleteMapping(pattern)}
                  disabled={disabled}
                >
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>
            ))}
          </div>
        )}

        {entries.length === 0 && (
          <div className="text-center py-6 mb-4">
            <p className="text-muted-foreground text-sm">{t('responseModelMappings.noMappings')}</p>
          </div>
        )}

        <div className="flex items-center gap-2 pt-4 border-t border-border">
          <ModelInput
            value={newPattern}
            onChange={setNewPattern}
            placeholder={t('responseModelMappings.matchPattern')}
            disabled={disabled}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
          <Input
            value={newTarget}
            onChange={(event) => setNewTarget(event.target.value)}
            placeholder={t('responseModelMappings.targetModel')}
            disabled={disabled}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddMapping}
            disabled={
              !newPattern.trim() || !newTarget.trim() || !isValidTarget(newTarget) || disabled
            }
          >
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  );
}

// Provider Supported Models Section
function ProviderSupportModels({
  supportModels,
  onChange,
}: {
  supportModels: string[];
  onChange: (models: string[]) => void;
}) {
  const { t } = useTranslation();
  const [newModel, setNewModel] = useState('');

  const handleAddModel = () => {
    if (!newModel.trim()) return;
    const trimmedModel = newModel.trim();
    if (!supportModels.includes(trimmedModel)) {
      onChange([...supportModels, trimmedModel]);
    }
    setNewModel('');
  };

  const handleRemoveModel = (model: string) => {
    onChange(supportModels.filter((m) => m !== model));
  };

  return (
    <div>
      <div className="flex items-center gap-2 mb-4 border-b border-border pb-2">
        <Filter size={18} className="text-blue-500" />
        <h4 className="text-lg font-semibold text-foreground">
          {t('providers.supportModels.title')}
        </h4>
        <span className="text-sm text-muted-foreground">({supportModels.length})</span>
      </div>

      <div className="bg-card border border-border rounded-xl p-4">
        <p className="text-xs text-muted-foreground mb-4">{t('providers.supportModels.desc')}</p>

        {supportModels.length > 0 && (
          <div className="flex flex-wrap gap-2 mb-4">
            {supportModels.map((model) => (
              <div
                key={model}
                className="flex items-center gap-1 bg-muted/50 border border-border rounded-lg px-3 py-1.5"
              >
                <span className="text-sm">{model}</span>
                <button
                  type="button"
                  onClick={() => handleRemoveModel(model)}
                  className="text-muted-foreground hover:text-destructive ml-1"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}

        {supportModels.length === 0 && (
          <div className="text-center py-6 mb-4">
            <p className="text-muted-foreground text-sm">{t('providers.supportModels.empty')}</p>
          </div>
        )}

        <div className="flex items-center gap-2 pt-4 border-t border-border">
          <ModelInput
            value={newModel}
            onChange={setNewModel}
            placeholder={t('providers.supportModels.placeholder')}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <Button variant="outline" size="sm" onClick={handleAddModel} disabled={!newModel.trim()}>
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  );
}

// Provider Exposed Models Section
function ProviderExposedModels({
  enabled,
  exposedModels,
  onEnabledChange,
  onChange,
}: {
  enabled: boolean;
  exposedModels: string[];
  onEnabledChange: (enabled: boolean) => void;
  onChange: (models: string[]) => void;
}) {
  const { t } = useTranslation();
  const [newModel, setNewModel] = useState('');

  const handleAddModel = () => {
    if (!newModel.trim()) return;
    const trimmedModel = newModel.trim();
    if (!exposedModels.includes(trimmedModel)) {
      onChange([...exposedModels, trimmedModel]);
    }
    setNewModel('');
  };

  const handleRemoveModel = (model: string) => {
    onChange(exposedModels.filter((m) => m !== model));
  };

  return (
    <div>
      <div className="flex items-center gap-2 mb-4 border-b border-border pb-2">
        <Eye size={18} className="text-emerald-500" />
        <h4 className="text-lg font-semibold text-foreground">
          {t('providers.exposedModels.title')}
        </h4>
        <span className="text-sm text-muted-foreground">({exposedModels.length})</span>
      </div>

      <div
        className={`rounded-xl border bg-card p-4 shadow-sm transition-colors ${
          enabled ? 'border-emerald-500/40' : 'border-border'
        }`}
      >
        <div className="flex items-center justify-between gap-4 rounded-lg border border-border bg-muted/30 p-3">
          <div className="flex min-w-0 gap-3">
            <div
              className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${
                enabled ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground'
              }`}
            >
              {enabled ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <div className="text-sm font-medium text-foreground">
                  {t('providers.exposedModels.enable')}
                </div>
                <span
                  className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
                    enabled
                      ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                      : 'bg-muted text-muted-foreground'
                  }`}
                >
                  {enabled
                    ? t('providers.exposedModels.statusOn')
                    : t('providers.exposedModels.statusOff')}
                </span>
              </div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('providers.exposedModels.enableDesc')}
              </p>
            </div>
          </div>
          <Switch
            checked={enabled}
            onCheckedChange={onEnabledChange}
            className="shrink-0 self-center"
          />
        </div>

        <div className="mt-4 rounded-lg bg-muted/20 p-3">
          <p className="text-xs leading-5 text-muted-foreground">
            {t('providers.exposedModels.desc')}
          </p>
        </div>

        {enabled && exposedModels.length === 0 && (
          <div className="mt-4 rounded-lg border border-warning/40 bg-warning/10 p-3 text-xs leading-5 text-warning">
            {t('providers.exposedModels.emptyEnabled')}
          </div>
        )}

        <div className="mt-4">
          {exposedModels.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {exposedModels.map((model) => (
                <div
                  key={model}
                  className="group flex items-center gap-1.5 rounded-full border border-border bg-background px-3 py-1.5 shadow-sm"
                >
                  <span className="text-sm text-foreground">{model}</span>
                  <button
                    type="button"
                    onClick={() => handleRemoveModel(model)}
                    className="text-muted-foreground transition-colors hover:text-destructive group-hover:text-foreground"
                    aria-label={t('providers.exposedModels.remove', { model })}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <div className="rounded-lg border border-dashed border-border py-6 text-center">
              <p className="text-sm text-muted-foreground">{t('providers.exposedModels.empty')}</p>
            </div>
          )}
        </div>

        <div className="mt-4 flex items-center gap-2 border-t border-border pt-4">
          <ModelInput
            value={newModel}
            onChange={setNewModel}
            placeholder={t('providers.exposedModels.placeholder')}
            className="flex-1 min-w-0 h-8 text-sm"
          />
          <Button variant="outline" size="sm" onClick={handleAddModel} disabled={!newModel.trim()}>
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  );
}

interface ProviderEditFlowProps {
  provider: Provider;
  onClose: () => void;
}

type EditFormData = {
  name: string;
  baseURL: string;
  backend: CustomBackend;
  apiKey: string;
  clients: ClientConfig[];
  supportModels: string[];
  exposedModelsEnabled: boolean;
  exposedModels: string[];
  disguiseType?: 'none' | 'claude-code' | 'bedrock' | 'x-api-key';
  cloakMode?: 'auto' | 'always' | 'never';
  cloakStrictMode?: boolean;
  cloakSensitiveWords?: string;
  responseModelMapping: Record<string, string>;
  quotaEnabled?: boolean;
  disableErrorCooldown?: boolean;
  smartMappingRetryEnabled?: boolean;
  smartMappingRetryLimit?: number;
  reasoning?: NonNullable<Provider['config']>['reasoning'];
  maxConcurrency: number;
  excludeFromExport: boolean;
  blackBox: boolean;
  // undefined = 默认透传;false = 旧的硬编码 /responses。
  responsesPassthrough?: boolean;
  // false/undefined = 不启用 Codex Responses WebSocket；true = 允许 WS 上游。
  responsesWebSocket?: boolean;
};

export function ProviderEditFlow({ provider, onClose }: ProviderEditFlowProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [saving, setSaving] = useState(false);
  const [cloning, setCloning] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');
  const [cloneError, setCloneError] = useState<string | null>(null);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const createProvider = useCreateProvider();
  const updateProvider = useUpdateProvider();
  const deleteProvider = useDeleteProvider();
  const createModelMapping = useCreateModelMapping();
  const { data: allMappings } = useModelMappings();

  const initClients = (): ClientConfig[] => {
    const supportedTypes = normalizeProviderArrayField(provider.supportedClientTypes);
    return defaultClients.map((client) => {
      const isEnabled = supportedTypes.includes(client.id);
      const urlOverride = provider.excludeFromExport
        ? ''
        : provider.config?.custom?.clientBaseURL?.[client.id] || '';
      const multiplier = provider.config?.custom?.clientMultiplier?.[client.id] || 10000;
      return { ...client, enabled: isEnabled, urlOverride, multiplier };
    });
  };

  const [showApiKey, setShowApiKey] = useState(false);
  const [formData, setFormData] = useState<EditFormData>(() => {
    const supportModels = normalizeProviderArrayField(provider.supportModels);
    const exposedModels = normalizeProviderArrayField(provider.exposedModels);
    const exposedModelsEnabled = provider.exposedModelsEnabled === true;
    // Read effective claude-code sub-options, preferring the new `disguise`
    // shape but falling back to the legacy `cloak` field so providers saved
    // before the migration continue to display their previous settings.
    const customCfg = provider.config?.custom as
      | (NonNullable<typeof provider.config>['custom'] & {
          cloak?: {
            mode?: 'auto' | 'always' | 'never';
            strictMode?: boolean;
            sensitiveWords?: string[];
          };
        })
      | undefined;
    const disguise = customCfg?.disguise;
    const legacyCloak = customCfg?.cloak;
    // Default disguiseType: 'claude-code' (preserves legacy auto-cloak behavior).
    const disguiseType = (disguise?.type ?? 'claude-code') as
      | 'none'
      | 'claude-code'
      | 'bedrock'
      | 'x-api-key';
    const cc = disguise?.claudeCode ?? legacyCloak;
    return {
      name: provider.name,
      baseURL: provider.excludeFromExport ? '' : provider.config?.custom?.baseURL || '',
      backend: provider.config?.custom?.backend === 'ollama' ? 'ollama' : 'http',
      apiKey: provider.excludeFromExport ? '' : provider.config?.custom?.apiKey || '',
      clients: initClients(),
      supportModels,
      exposedModelsEnabled,
      exposedModels,
      disguiseType,
      cloakMode: cc?.mode || 'auto',
      cloakStrictMode: cc?.strictMode || false,
      cloakSensitiveWords: (cc?.sensitiveWords || []).join('\n'),
      responseModelMapping: provider.config?.custom?.responseModelMapping || {},
      quotaEnabled: provider.config?.quotaEnabled ?? false,
      disableErrorCooldown: provider.config?.disableErrorCooldown ?? false,
      smartMappingRetryEnabled: provider.config?.smartMappingRetryEnabled ?? false,
      smartMappingRetryLimit: provider.config?.smartMappingRetryLimit ?? 1,
      reasoning: provider.config?.reasoning,
      maxConcurrency: normalizeMaxConcurrency(provider.maxConcurrency),
      excludeFromExport: !!provider.excludeFromExport || !!provider.blackBox,
      blackBox: !!provider.blackBox,
      responsesPassthrough: provider.config?.custom?.responsesPassthrough,
      responsesWebSocket: provider.config?.custom?.responsesWebSocket === true,
    };
  });
  const providerConfigIsWriteOnly = !!provider.excludeFromExport;
  const excludeFromExportLocked = providerConfigIsWriteOnly;
  const effectiveExcludeFromExport =
    excludeFromExportLocked || !!formData.blackBox || !!formData.excludeFromExport;
  const runtimeModelsPreview = useMemo<ProviderRuntimeModelsPreviewRequest | undefined>(() => {
    const clientBaseURL: Partial<Record<ClientType, string>> = {};
    formData.clients.forEach((client) => {
      const url = client.urlOverride.trim();
      if (client.enabled && url) {
        clientBaseURL[client.id] = url;
      }
    });

    const baseURL = formData.baseURL.trim();
    const apiKey = formData.apiKey.trim();
    if (!baseURL && !clientBaseURL.openai) return undefined;

    return {
      type: provider.type || 'custom',
      config: {
        custom: {
          baseURL,
          backend: formData.backend === 'ollama' ? 'ollama' : undefined,
          apiKey,
          clientBaseURL: Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
        },
      },
    };
  }, [formData.apiKey, formData.backend, formData.baseURL, formData.clients, provider.type]);

  const providerMappingTargetCount = useMemo(
    () =>
      new Set(
        (allMappings ?? [])
          .filter(
            (mapping) => mapping.providerID === Number(provider.id) && mapping.isEnabled !== false,
          )
          .map((mapping) => mapping.target.trim())
          .filter(Boolean),
      ).size,
    [allMappings, provider.id],
  );
  const smartMappingRetryEnabled =
    !!formData.disableErrorCooldown &&
    providerMappingTargetCount > 1 &&
    !!formData.smartMappingRetryEnabled;

  const updateClient = (clientId: ClientType, updates: Partial<ClientConfig>) => {
    setFormData((prev) => ({
      ...prev,
      clients: prev.clients.map((c) => (c.id === clientId ? { ...c, ...updates } : c)),
    }));
  };

  const hasVisibleURL = () =>
    formData.baseURL.trim() || formData.clients.some((c) => c.enabled && c.urlOverride.trim());

  const isSaveValid = () => {
    if (!formData.name.trim()) return false;
    const hasEnabledClient = formData.clients.some((c) => c.enabled);
    return hasEnabledClient && (providerConfigIsWriteOnly || !!hasVisibleURL());
  };

  const isCloneValid = () => {
    if (!formData.name.trim()) return false;
    const hasEnabledClient = formData.clients.some((c) => c.enabled);
    return hasEnabledClient && !!hasVisibleURL();
  };

  // Build the disguise payload from current form state. Delegates to the
  // shared util in ../utils/disguise so the create flow and the edit flow
  // produce identical payloads.
  const currentDisguisePayload = () =>
    buildDisguisePayload(
      formData.disguiseType,
      formData.cloakMode,
      !!formData.cloakStrictMode,
      formData.cloakSensitiveWords || '',
    );

  const handleSave = async () => {
    if (!isSaveValid()) return;

    setSaving(true);
    setSaveStatus('idle');

    try {
      const supportedClientTypes = formData.clients.filter((c) => c.enabled).map((c) => c.id);
      const clientBaseURL: Partial<Record<ClientType, string>> = {};
      const clientMultiplier: Partial<Record<ClientType, number>> = {};
      formData.clients.forEach((c) => {
        if (c.enabled && c.urlOverride) {
          clientBaseURL[c.id] = c.urlOverride;
        }
        if (c.enabled && c.multiplier !== 10000) {
          clientMultiplier[c.id] = c.multiplier;
        }
      });

      const data: Partial<CreateProviderData> = {
        name: formData.name,
        type: provider.type || 'custom', // Preserve the provider type
        maxConcurrency: normalizeMaxConcurrency(formData.maxConcurrency),
        config: {
          quotaEnabled: !!formData.quotaEnabled,
          disableErrorCooldown: !!formData.disableErrorCooldown,
          smartMappingRetryEnabled,
          smartMappingRetryLimit: formData.smartMappingRetryLimit ?? 1,
          reasoning: formData.reasoning,
          custom: {
            baseURL: formData.baseURL,
            backend: formData.backend === 'ollama' ? 'ollama' : undefined,
            apiKey: formData.apiKey.trim() || '',
            responsesPassthrough: formData.responsesPassthrough,
            responsesWebSocket: formData.responsesWebSocket === true,
            clientBaseURL: Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
            clientMultiplier:
              Object.keys(clientMultiplier).length > 0 ? clientMultiplier : undefined,
            disguise: currentDisguisePayload(),
            responseModelMapping:
              Object.keys(formData.responseModelMapping).length > 0
                ? formData.responseModelMapping
                : undefined,
          },
        },
        supportedClientTypes,
        supportModels: formData.supportModels.length > 0 ? formData.supportModels : undefined,
        exposedModelsEnabled: formData.exposedModelsEnabled,
        exposedModels: formData.exposedModels.length > 0 ? formData.exposedModels : undefined,
        excludeFromExport: effectiveExcludeFromExport,
        blackBox: !!formData.blackBox,
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

  const handleClone = async () => {
    if (!isCloneValid() || cloning) return;

    setCloneError(null);
    if (providerConfigIsWriteOnly && !formData.apiKey.trim()) {
      setCloneError(t('provider.cloneWriteOnlyRequiresKey'));
      return;
    }

    setCloning(true);

    try {
      const supportedClientTypes = formData.clients.filter((c) => c.enabled).map((c) => c.id);
      const clientBaseURL: Partial<Record<ClientType, string>> = {};
      const clientMultiplier: Partial<Record<ClientType, number>> = {};
      formData.clients.forEach((c) => {
        if (c.enabled && c.urlOverride) {
          clientBaseURL[c.id] = c.urlOverride;
        }
        if (c.enabled && c.multiplier !== 10000) {
          clientMultiplier[c.id] = c.multiplier;
        }
      });

      const baseName = formData.name.trim() || provider.name;
      const suffix = t('provider.cloneSuffix');
      const cloneName = baseName.endsWith(suffix) ? baseName : `${baseName}${suffix}`;

      const data: CreateProviderData = {
        type: provider.type || 'custom',
        name: cloneName,
        logo: provider.logo,
        maxConcurrency: normalizeMaxConcurrency(formData.maxConcurrency),
        config: {
          quotaEnabled: !!formData.quotaEnabled,
          disableErrorCooldown: !!formData.disableErrorCooldown,
          smartMappingRetryEnabled,
          smartMappingRetryLimit: formData.smartMappingRetryLimit ?? 1,
          reasoning: formData.reasoning,
          custom: {
            baseURL: formData.baseURL,
            backend: formData.backend === 'ollama' ? 'ollama' : undefined,
            apiKey:
              formData.apiKey.trim() ||
              (providerConfigIsWriteOnly ? '' : provider.config?.custom?.apiKey) ||
              '',
            responsesPassthrough: formData.responsesPassthrough,
            responsesWebSocket: formData.responsesWebSocket === true,
            clientBaseURL: Object.keys(clientBaseURL).length > 0 ? clientBaseURL : undefined,
            clientMultiplier:
              Object.keys(clientMultiplier).length > 0 ? clientMultiplier : undefined,
            disguise: currentDisguisePayload(),
            responseModelMapping:
              Object.keys(formData.responseModelMapping).length > 0
                ? formData.responseModelMapping
                : undefined,
          },
        },
        supportedClientTypes,
        supportModels: formData.supportModels.length > 0 ? formData.supportModels : undefined,
        exposedModelsEnabled: formData.exposedModelsEnabled,
        exposedModels: formData.exposedModels.length > 0 ? formData.exposedModels : undefined,
        excludeFromExport: effectiveExcludeFromExport,
        blackBox: !!formData.blackBox,
      };

      const newProvider = await createProvider.mutateAsync(data);

      const providerMappings = (allMappings || []).filter(
        (mapping) => mapping.scope === 'provider' && mapping.providerID === provider.id,
      );

      if (providerMappings.length > 0) {
        for (const mapping of providerMappings) {
          await createModelMapping.mutateAsync({
            scope: mapping.scope,
            clientType: mapping.clientType,
            providerType: mapping.providerType,
            providerID: newProvider.id,
            projectID: mapping.projectID,
            routeID: mapping.routeID,
            apiTokenID: mapping.apiTokenID,
            pattern: mapping.pattern,
            target: mapping.target,
            priority: mapping.priority,
            isEnabled: mapping.isEnabled,
          });
        }
      }

      navigate(`/providers/${newProvider.id}/edit`, { replace: true });
    } catch (error) {
      console.error('Failed to clone provider:', error);
    } finally {
      setCloning(false);
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

  // Bedrock provider
  if (provider.type === 'bedrock') {
    return (
      <>
        <BedrockProviderView
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

  // Codex provider
  if (provider.type === 'codex') {
    return (
      <>
        <CodexProviderView
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

  // Claude provider
  if (provider.type === 'claude') {
    return (
      <>
        <ClaudeProviderView
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

  // OpenRouter provider
  if (provider.type === 'openrouter') {
    return (
      <>
        <OpenRouterProviderView
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

  // z.ai (智谱 GLM) provider
  if (provider.type === 'zai') {
    return (
      <>
        <ZaiProviderView
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

  // fal (fal.ai) provider
  if (provider.type === 'fal') {
    return (
      <>
        <FalProviderView
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

  // New API provider
  if (provider.type === 'newapi') {
    return (
      <>
        <NewApiProviderView
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

  // Ollama provider
  if (provider.type === 'ollama') {
    return (
      <>
        <OllamaProviderView
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

  // Grok provider
  if (provider.type === 'grok') {
    return (
      <>
        <GrokProviderView
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
      <PageHeader
        icon={<ChevronLeft className="cursor-pointer" onClick={onClose} />}
        title={t('provider.edit')}
        description={t('provider.editDescription')}
      />

      <div className="flex-1 overflow-y-auto">
        <div className="sticky top-0 z-40 border-b border-border bg-background/95 px-4 py-3 shadow-sm backdrop-blur supports-[backdrop-filter]:bg-background/80 md:px-6">
          <div className="mx-auto flex max-w-7xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold text-foreground">
                {formData.name || provider.name}
              </div>
              <div className="text-xs text-muted-foreground">{t('provider.editDescription')}</div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button onClick={() => setShowDeleteConfirm(true)} variant={'destructive'}>
                <Trash2 size={14} />
                {t('provider.delete')}
              </Button>
              <Button
                onClick={handleClone}
                disabled={cloning || saving || !isCloneValid()}
                variant={'outline'}
              >
                <Copy size={14} />
                {cloning ? t('provider.cloning') : t('provider.clone')}
              </Button>
              <Button onClick={onClose} variant={'secondary'}>
                {t('provider.cancel')}
              </Button>
              <Button onClick={handleSave} disabled={saving || !isSaveValid()} variant={'default'}>
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
        </div>
        <div className="p-6">
          <div className="mx-auto grid max-w-7xl gap-6 xl:grid-cols-[240px_minmax(0,1fr)]">
            <ProviderEditSectionNav
              title={t('provider.infoSections.navigation')}
              sections={[
                {
                  id: 'provider-overview',
                  label: t('provider.infoSections.overview'),
                  description: t('provider.infoSections.overviewDesc'),
                },
                {
                  id: 'provider-connection',
                  label: t('provider.infoSections.connection'),
                  description: t('provider.infoSections.connectionDesc'),
                },
                {
                  id: 'provider-clients',
                  label: t('provider.infoSections.clients'),
                  description: t('provider.infoSections.clientsDesc'),
                },
                {
                  id: 'provider-models',
                  label: t('provider.infoSections.models'),
                  description: t('provider.infoSections.modelsDesc'),
                },
                {
                  id: 'provider-policies',
                  label: t('provider.infoSections.policies'),
                  description: t('provider.infoSections.policiesDesc'),
                },
                {
                  id: 'provider-danger',
                  label: t('provider.infoSections.danger'),
                  description: t('provider.infoSections.dangerDesc'),
                },
              ]}
            />

            <div className="min-w-0 space-y-6">
              <ProviderEditSection
                id="provider-overview"
                title={t('provider.infoSections.overview')}
                description={t('provider.infoSections.overviewDesc')}
                icon={<Globe size={18} />}
              >
                <ProviderProxyURLCard provider={provider} />

                <div className="grid gap-6 md:grid-cols-2">
                  <div>
                    <label className="mb-2 block text-sm font-medium text-foreground">
                      {t('provider.displayName')}
                    </label>
                    <Input
                      type="text"
                      value={formData.name}
                      onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                      placeholder={t('provider.namePlaceholder')}
                      className="w-full"
                    />
                  </div>

                  <div>
                    <label className="mb-2 block text-sm font-medium text-foreground">
                      {t('provider.customBackend')}
                    </label>
                    <Select
                      value={formData.backend}
                      onValueChange={(backend) =>
                        setFormData((prev) => ({
                          ...prev,
                          backend: backend === 'ollama' ? 'ollama' : 'http',
                        }))
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="http">{t('provider.customBackendHttp')}</SelectItem>
                        <SelectItem value="ollama">{t('provider.customBackendOllama')}</SelectItem>
                      </SelectContent>
                    </Select>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {formData.backend === 'ollama'
                        ? t('provider.customBackendOllamaDesc')
                        : t('provider.customBackendHttpDesc')}
                    </p>
                  </div>
                </div>
              </ProviderEditSection>

              <ProviderEditSection
                id="provider-connection"
                title={t('provider.infoSections.connection')}
                description={t('provider.infoSections.connectionDesc')}
                icon={<Key size={18} />}
              >
                <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                  <div>
                    <label className="mb-2 block text-sm font-medium text-foreground">
                      <div className="flex items-center gap-2">
                        <Globe size={14} />
                        <span>{t('provider.apiEndpoint')}</span>
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
                      placeholder={
                        providerConfigIsWriteOnly
                          ? t('provider.endpointPlaceholderWriteOnly')
                          : t('provider.endpointPlaceholder')
                      }
                      className="w-full"
                    />
                    <p className="mt-1 text-xs text-muted-foreground">
                      {providerConfigIsWriteOnly
                        ? t('provider.urlExcludedHint')
                        : t('provider.optionalUrlNote')}
                    </p>
                  </div>

                  <div>
                    <label className="mb-2 block text-sm font-medium text-foreground">
                      <div className="flex items-center gap-2">
                        <Key size={14} />
                        <span>
                          {formData.backend === 'ollama'
                            ? t('provider.apiKeyOptional')
                            : t('provider.apiKeyEdit')}
                        </span>
                      </div>
                    </label>
                    <div className="relative">
                      <Input
                        type={showApiKey && !providerConfigIsWriteOnly ? 'text' : 'password'}
                        value={formData.apiKey}
                        onChange={(e) => {
                          setCloneError(null);
                          setFormData((prev) => ({ ...prev, apiKey: e.target.value }));
                        }}
                        placeholder={
                          providerConfigIsWriteOnly
                            ? t('provider.keyPlaceholderWriteOnly')
                            : formData.backend === 'ollama'
                              ? t('provider.keyPlaceholderOptional')
                              : t('provider.keyPlaceholder')
                        }
                        className="w-full pr-10"
                      />
                      {!providerConfigIsWriteOnly && (
                        <button
                          type="button"
                          onClick={() => setShowApiKey(!showApiKey)}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
                          aria-label={showApiKey ? t('common.hide') : t('common.show')}
                        >
                          {showApiKey ? (
                            <EyeOff className="h-4 w-4" />
                          ) : (
                            <Eye className="h-4 w-4" />
                          )}
                        </button>
                      )}
                    </div>
                    {providerConfigIsWriteOnly && (
                      <div className="mt-2 rounded-lg border border-border bg-muted/50 p-3 text-xs text-muted-foreground">
                        {t('provider.apiKeyExcludedHint')}
                      </div>
                    )}
                    {cloneError && (
                      <div className="mt-2 rounded-lg border border-error/30 bg-error/10 p-3 text-xs text-error">
                        {cloneError}
                      </div>
                    )}
                  </div>
                </div>
              </ProviderEditSection>

              <ProviderEditSection
                id="provider-clients"
                title={t('provider.infoSections.clients')}
                description={t('provider.infoSections.clientsDesc')}
                icon={<Zap size={18} />}
              >
                <ClientsConfigSection
                  clients={formData.clients}
                  onUpdateClient={updateClient}
                  disguise={{
                    type: formData.disguiseType ?? 'claude-code',
                    claudeCodeMode: formData.cloakMode ?? 'auto',
                    claudeCodeStrictMode: !!formData.cloakStrictMode,
                    claudeCodeSensitiveWords: formData.cloakSensitiveWords ?? '',
                  }}
                  onUpdateDisguise={(updates) =>
                    setFormData((prev) => ({
                      ...prev,
                      disguiseType: updates?.type ?? prev.disguiseType,
                      cloakMode: updates?.claudeCodeMode ?? prev.cloakMode,
                      cloakStrictMode: updates?.claudeCodeStrictMode ?? prev.cloakStrictMode,
                      cloakSensitiveWords:
                        updates?.claudeCodeSensitiveWords ?? prev.cloakSensitiveWords,
                    }))
                  }
                  responsesWebSocket={formData.responsesWebSocket === true}
                  onUpdateResponsesWebSocket={(checked) =>
                    setFormData((prev) => ({ ...prev, responsesWebSocket: checked }))
                  }
                />

                <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
                  <div className="pr-4">
                    <div className="text-sm font-medium text-foreground">
                      {t('provider.responsesPassthrough')}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('provider.responsesPassthroughDesc')}
                    </p>
                  </div>
                  <Switch
                    checked={formData.responsesPassthrough !== false}
                    onCheckedChange={(checked) =>
                      setFormData((prev) => ({ ...prev, responsesPassthrough: checked }))
                    }
                  />
                </div>
              </ProviderEditSection>

              <ProviderEditSection
                id="provider-models"
                title={t('provider.infoSections.models')}
                description={t('provider.infoSections.modelsDesc')}
                icon={<Filter size={18} />}
              >
                <ProviderSupportModels
                  supportModels={formData.supportModels}
                  onChange={(models) => setFormData((prev) => ({ ...prev, supportModels: models }))}
                />

                <ProviderExposedModels
                  enabled={formData.exposedModelsEnabled}
                  exposedModels={formData.exposedModels}
                  onEnabledChange={(enabled) =>
                    setFormData((prev) => ({ ...prev, exposedModelsEnabled: enabled }))
                  }
                  onChange={(models) => setFormData((prev) => ({ ...prev, exposedModels: models }))}
                />

                <ProviderModelMappings
                  provider={provider}
                  runtimeModelsPreview={runtimeModelsPreview}
                />

                <ResponseModelMappings
                  mappings={formData.responseModelMapping}
                  onChange={(mappings) =>
                    setFormData((prev) => ({ ...prev, responseModelMapping: mappings }))
                  }
                  disabled={saving}
                />
              </ProviderEditSection>

              <ProviderEditSection
                id="provider-policies"
                title={t('provider.infoSections.policies')}
                description={t('provider.infoSections.policiesDesc')}
                icon={<Zap size={18} />}
              >
                <ProviderMaxConcurrencyField
                  value={formData.maxConcurrency}
                  onChange={(maxConcurrency) =>
                    setFormData((prev) => ({ ...prev, maxConcurrency }))
                  }
                />

                <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
                  <div className="pr-4">
                    <div className="text-sm font-medium text-foreground">
                      {t('provider.quotaEnabled')}
                    </div>
                  </div>
                  <Switch
                    checked={!!formData.quotaEnabled}
                    onCheckedChange={(checked) =>
                      setFormData((prev) => ({ ...prev, quotaEnabled: checked }))
                    }
                  />
                </div>

                <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
                  <div className="pr-4">
                    <div className="text-sm font-medium text-foreground">
                      {t('provider.disableErrorCooldown')}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('provider.disableErrorCooldownDesc')}
                    </p>
                  </div>
                  <Switch
                    checked={!!formData.disableErrorCooldown}
                    onCheckedChange={(checked) =>
                      setFormData((prev) => ({
                        ...prev,
                        disableErrorCooldown: checked,
                        smartMappingRetryEnabled: checked ? prev.smartMappingRetryEnabled : false,
                      }))
                    }
                  />
                </div>

                <SmartMappingRetrySettings
                  disableErrorCooldown={!!formData.disableErrorCooldown}
                  enabled={formData.smartMappingRetryEnabled}
                  retryLimit={formData.smartMappingRetryLimit}
                  mappingTargetCount={providerMappingTargetCount}
                  onEnabledChange={(checked) =>
                    setFormData((prev) => ({ ...prev, smartMappingRetryEnabled: checked }))
                  }
                  onRetryLimitChange={(limit) =>
                    setFormData((prev) => ({ ...prev, smartMappingRetryLimit: limit }))
                  }
                />
                <ReasoningPolicySettings
                  value={formData.reasoning}
                  onChange={(reasoning) => setFormData((prev) => ({ ...prev, reasoning }))}
                />
              </ProviderEditSection>

              <ProviderEditSection
                id="provider-danger"
                title={t('provider.infoSections.danger')}
                description={t('provider.infoSections.dangerDesc')}
                icon={<Trash2 size={18} />}
              >
                <div className="space-y-3">
                  <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
                    <div className="pr-4">
                      <div className="text-sm font-medium text-foreground">
                        {t('provider.blackBox')}
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t('provider.blackBoxDesc')}
                      </p>
                    </div>
                    <Switch
                      checked={!!formData.blackBox}
                      onCheckedChange={(checked) =>
                        setFormData((prev) => ({
                          ...prev,
                          blackBox: checked,
                          excludeFromExport: checked ? true : prev.excludeFromExport,
                        }))
                      }
                    />
                  </div>

                  <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
                    <div className="pr-4">
                      <div className="text-sm font-medium text-foreground">
                        {t('provider.excludeFromExport')}
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {excludeFromExportLocked
                          ? t('provider.excludeFromExportLockedDesc')
                          : t('provider.excludeFromExportDesc')}
                      </p>
                    </div>
                    <Switch
                      checked={effectiveExcludeFromExport}
                      disabled={excludeFromExportLocked || !!formData.blackBox}
                      onCheckedChange={(checked) =>
                        setFormData((prev) => ({ ...prev, excludeFromExport: checked }))
                      }
                    />
                  </div>
                </div>
              </ProviderEditSection>

              {saveStatus === 'error' && (
                <div className="p-4 bg-error/10 border border-error/30 rounded-lg text-sm text-error flex items-center gap-2">
                  <div className="w-1.5 h-1.5 rounded-full bg-error" />
                  {t('provider.updateError')}
                </div>
              )}
            </div>
          </div>
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
          <DialogTitle>{t('providers.deleteConfirm.title')}</DialogTitle>
          <DialogDescription>
            {t('providers.deleteConfirm.description', { name: providerName })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={onCancel} variant={'secondary'} className="px-4">
            {t('provider.cancel')}
          </Button>
          <Button onClick={onConfirm} disabled={deleting} variant={'destructive'} className="px-4">
            {deleting ? t('common.deleting') : t('common.delete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
