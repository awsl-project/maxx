import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Input } from '@/components/ui';
import { useCreateRoute, useUpdateRoute, useProviders, useProjects } from '@/hooks/queries';
import type { ClientType, Route, Provider } from '@/lib/transport';
import { ModelMappingEditor } from '@/pages/providers/components/model-mapping-editor';

type ProviderTypeKey = 'antigravity' | 'kiro' | 'custom';

const PROVIDER_TYPE_ORDER: ProviderTypeKey[] = ['antigravity', 'kiro', 'custom'];

const PROVIDER_TYPE_LABELS: Record<ProviderTypeKey, string> = {
  antigravity: 'Antigravity',
  kiro: 'Kiro',
  custom: 'Custom',
};

interface RouteFormProps {
  route?: Route;
  onClose: () => void;
  isGlobal?: boolean;
  projectId?: number;
}

export function RouteForm({ route, onClose, isGlobal, projectId }: RouteFormProps) {
  const { t } = useTranslation();
  const createRoute = useCreateRoute();
  const updateRoute = useUpdateRoute();
  const { data: providers } = useProviders();
  const { data: projects } = useProjects();
  const isEditing = !!route;

  const [clientType, setClientType] = useState<ClientType>('openai');
  const [providerID, setProviderID] = useState('');
  const [projectID, setProjectID] = useState(projectId !== undefined ? String(projectId) : '0');
  const [position, setPosition] = useState('1');
  const [isEnabled, setIsEnabled] = useState(true);
  const [modelMapping, setModelMapping] = useState<Record<string, string>>({});

  // Group providers by type and sort alphabetically
  const groupedProviders = useMemo(() => {
    const groups: Record<ProviderTypeKey, Provider[]> = {
      antigravity: [],
      kiro: [],
      custom: [],
    };

    providers?.forEach((p) => {
      const type = p.type as ProviderTypeKey;
      if (groups[type]) {
        groups[type].push(p);
      } else {
        groups.custom.push(p);
      }
    });

    // Sort alphabetically within each group
    for (const key of Object.keys(groups) as ProviderTypeKey[]) {
      groups[key].sort((a, b) => a.name.localeCompare(b.name));
    }

    return groups;
  }, [providers]);

  useEffect(() => {
    if (route) {
      setClientType(route.clientType);
      setProviderID(String(route.providerID));
      setProjectID(String(route.projectID));
      setPosition(String(route.position));
      setIsEnabled(route.isEnabled);
      setModelMapping(route.modelMapping || {});
    }
  }, [route]);

  // Lock projectID for global routes or when projectId is provided
  useEffect(() => {
    if (isGlobal) {
      setProjectID('0');
    } else if (projectId !== undefined) {
      setProjectID(String(projectId));
    }
  }, [isGlobal, projectId]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    const data = {
      clientType,
      providerID: Number(providerID),
      projectID: Number(projectID),
      position: Number(position),
      isEnabled,
      isNative: route?.isNative ?? false, // 手动创建的 Route 默认为转换路由
      retryConfigID: route?.retryConfigID ?? 0,
      modelMapping: Object.keys(modelMapping).length > 0 ? modelMapping : undefined,
    };

    if (isEditing) {
      updateRoute.mutate({ id: route.id, data }, { onSuccess: () => onClose() });
    } else {
      createRoute.mutate(data, { onSuccess: () => onClose() });
    }
  };

  const isPending = createRoute.isPending || updateRoute.isPending;
  const showProjectSelector = !isGlobal && projectId === undefined;

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2">
        <div>
          <label className="mb-1 block text-sm font-medium">{t('routes.form.clientType')}</label>
          <select
            value={clientType}
            onChange={(e) => setClientType(e.target.value as ClientType)}
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
          >
            <option value="claude">{t('clientRoutes.claude')}</option>
            <option value="openai">{t('clientRoutes.openai')}</option>
            <option value="codex">{t('clientRoutes.codex')}</option>
            <option value="gemini">{t('clientRoutes.gemini')}</option>
          </select>
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">{t('routes.form.provider')}</label>
          <select
            value={providerID}
            onChange={(e) => setProviderID(e.target.value)}
            required
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
          >
            <option value="">{t('routes.form.selectProvider')}</option>
            {PROVIDER_TYPE_ORDER.map((typeKey) => {
              const typeProviders = groupedProviders[typeKey];
              if (typeProviders.length === 0) return null;
              return (
                <optgroup key={typeKey} label={PROVIDER_TYPE_LABELS[typeKey]}>
                  {typeProviders.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </optgroup>
              );
            })}
          </select>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {showProjectSelector && (
          <div>
            <label className="mb-1 block text-sm font-medium">{t('routes.form.project')}</label>
            <select
              value={projectID}
              onChange={(e) => setProjectID(e.target.value)}
              className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs focus:border-ring focus:ring-2 focus:ring-ring/50 outline-none"
            >
              <option value="0">{t('routes.form.globalProjects')}</option>
              {projects?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
        )}
        <div>
          <label className="mb-1 block text-sm font-medium">{t('routes.form.position')}</label>
          <Input
            type="number"
            value={position}
            onChange={(e) => setPosition(e.target.value)}
            min="1"
            required
          />
        </div>
      </div>

      {/* Model Mapping (route-level override) */}
      <div>
        <label className="mb-1 block text-sm font-medium">{t('routes.form.modelMapping')}</label>
        <p className="mb-2 text-xs text-text-secondary">{t('routes.form.modelMappingHelp')}</p>
        <ModelMappingEditor value={modelMapping} onChange={setModelMapping} disabled={isPending} />
      </div>

      <div className="flex items-center gap-2">
        <input
          type="checkbox"
          id="isEnabled"
          checked={isEnabled}
          onChange={(e) => setIsEnabled(e.target.checked)}
          className="h-4 w-4 rounded border-gray-300"
        />
        <label htmlFor="isEnabled" className="text-sm font-medium">
          {t('routes.form.enabled')}
        </label>
      </div>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" onClick={onClose}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" disabled={isPending}>
          {isPending ? t('common.saving') : isEditing ? t('routes.update') : t('routes.create')}
        </Button>
      </div>
    </form>
  );
}
