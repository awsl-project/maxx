import { ProviderSupportModels } from './provider-support-models';
import { ProviderModelMappings } from './provider-model-mappings';
import { ModelMappingEditor } from './model-mapping-editor';
import { useRef, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { PageHeader } from '@/components/layout/page-header';
import { useCreateModelMapping, useCreateProvider, useUpdateProvider } from '@/hooks/queries';
import { getTransport } from '@/lib/transport';
import type { Provider, CreateProviderData } from '@/lib/transport';
import type { WorkBuddyAIConfig, WorkBuddyAIAuthStartResult } from '@/lib/transport/types';

export function safeWorkBuddyLoginURL(value: string): string | null {
  try {
    const url = new URL(value);
    return url.protocol === 'https:' && !url.username && !url.password ? url.href : null;
  } catch {
    return null;
  }
}

export function resolveWorkBuddyConfig(
  existing: WorkBuddyAIConfig | undefined,
  current: WorkBuddyAIConfig,
  replaced: boolean,
): WorkBuddyAIConfig {
  return replaced ? { ...current } : { ...existing, ...current };
}

export function WorkBuddyAIForm({
  provider,
  onClose,
}: {
  provider?: Provider;
  onClose?: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const create = useCreateProvider();
  const update = useUpdateProvider();
  const createMapping = useCreateModelMapping();
  const [savedProvider, setSavedProvider] = useState(provider);
  const [supportModels, setSupportModels] = useState<string[]>(provider?.supportModels || []);
  const [mappings, setMappings] = useState<Record<string, string>>({});
  const [name, setName] = useState(provider?.name || 'WorkBuddy AI');
  // Existing secrets are write-only: omit unchanged credentials on update.
  const [config, setConfig] = useState<WorkBuddyAIConfig>({});
  const [replaced, setReplaced] = useState(false);
  const effectiveConfig = resolveWorkBuddyConfig(provider?.config?.workbuddyAI, config, replaced);
  const [session, setSession] = useState<WorkBuddyAIAuthStartResult | null>(null);
  const [status, setStatus] = useState('');
  const [busy, setBusy] = useState(false);
  const [models, setModels] = useState<string[]>([]);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const close = () => (onClose ? onClose() : navigate('/providers'));
  const key = (s: string) => t('addProvider.workbuddyAI.' + s);
  const start = async () => {
    setBusy(true);
    setStatus('');
    setSession(null);
    try {
      const result = await getTransport().startWorkBuddyAIAuth();
      const url = safeWorkBuddyLoginURL(result.url);
      if (!url) throw new Error('invalid URL');
      if (active.current) {
        setSession({ ...result, url });
        setStatus('pending');
      }
    } catch {
      if (active.current) setStatus('error');
    } finally {
      if (active.current) setBusy(false);
    }
  };
  const poll = async () => {
    if (!session) return;
    setBusy(true);
    try {
      const result = await getTransport().pollWorkBuddyAIAuth(session.state);
      if (!active.current) return;
      if (result.status === 'authorized' && result.config) {
        setConfig(result.config);
        setReplaced(true);
        setSession(null);
        setStatus('authorized');
      } else {
        setStatus(result.status === 'authorized' ? 'error' : result.status);
        if (result.status !== 'pending') setSession(null);
      }
    } catch {
      if (active.current) setStatus('error');
    } finally {
      if (active.current) setBusy(false);
    }
  };
  const discover = async () => {
    setBusy(true);
    try {
      const result = await getTransport().getWorkBuddyAIModels({
        ...effectiveConfig,
        providerID: !config.accessToken ? provider?.id : undefined,
      });
      if (!active.current) return;
      setModels(result.models);
      if ('config' in result && result.config) setConfig(result.config as WorkBuddyAIConfig);
    } catch {
      if (active.current) setStatus('error');
    } finally {
      if (active.current) setBusy(false);
    }
  };
  const save = async () => {
    setBusy(true);
    try {
      const data: CreateProviderData = {
        ...savedProvider,
        supportModels,
        type: 'workbuddy-ai',
        name: name.trim(),
        config: {
          ...provider?.config,
          workbuddyAI: effectiveConfig,
        },
      };
      let saved = savedProvider;
      if (saved) await update.mutateAsync({ id: saved.id, data });
      else {
        saved = await create.mutateAsync(data);
        setSavedProvider(saved);
      }
      for (const [pattern, target] of Object.entries(mappings)) {
        // Re-read persisted rules on every attempt: a failed response may have
        // committed the previous create. Match the complete scope, not just pattern.
        const existing = (await getTransport().getModelMappings()).find(
          (rule) =>
            rule.scope === 'provider' &&
            rule.providerID === saved.id &&
            rule.providerType === 'workbuddy-ai' &&
            !rule.clientType &&
            !rule.projectID &&
            !rule.routeID &&
            !rule.apiTokenID &&
            rule.pattern === pattern,
        );
        const input = {
          scope: 'provider' as const,
          providerID: saved.id,
          providerType: 'workbuddy-ai',
          pattern,
          target,
          isEnabled: true,
        };
        if (existing) {
          await getTransport().updateModelMapping(existing.id, {
            ...input,
            priority: existing.priority,
          });
        } else await createMapping.mutateAsync(input);
        setMappings((pending) => {
          const remaining = { ...pending };
          delete remaining[pattern];
          return remaining;
        });
      }
      close();
    } catch {
      setStatus('error');
    } finally {
      if (active.current) setBusy(false);
    }
  };
  return (
    <div className="flex h-full flex-col">
      <PageHeader title={key('name')} description={key('description')}>
        <Button variant="secondary" onClick={close}>
          {t('common.cancel')}
        </Button>
        <Button
          onClick={save}
          disabled={
            busy || !name.trim() || (!provider && !config.accessToken && !config.refreshToken)
          }
        >
          {t('common.save')}
        </Button>
      </PageHeader>
      <div className="mx-auto w-full max-w-3xl space-y-6 overflow-y-auto p-6">
        <div className="space-y-2">
          <label htmlFor="workbuddy-name">{t('provider.name')}</label>
          <Input id="workbuddy-name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="space-y-3 rounded-xl border border-border p-4">
          <Button onClick={start} disabled={busy}>
            {key('login')}
          </Button>
          {session && (
            <div className="space-y-3">
              <a
                className="block break-all text-primary underline"
                href={session.url}
                target="_blank"
                rel="noopener noreferrer"
              >
                {key('openLogin')}
              </a>
              <Button onClick={poll} disabled={busy}>
                {key('check')}
              </Button>
            </div>
          )}
          {status && <p role="status">{key(status)}</p>}
        </div>
        <div className="space-y-2">
          <Button onClick={discover} disabled={busy || (!provider && !config.accessToken)}>
            {key('discoverModels')}
          </Button>
          {models.length > 0 && <p role="status">{models.join(', ')}</p>}
        </div>
        <ProviderSupportModels supportModels={supportModels} onChange={setSupportModels} />
        {savedProvider && Object.keys(mappings).length === 0 ? (
          <ProviderModelMappings provider={{ ...savedProvider, supportModels }} />
        ) : (
          <section aria-labelledby="workbuddy-mappings">
            <h4 id="workbuddy-mappings">{t('modelMappings.title')}</h4>
            <ModelMappingEditor value={mappings} onChange={setMappings} disabled={busy} />
          </section>
        )}
        <p className="text-sm text-muted-foreground">{key('modelPolicyHint')}</p>
        <p className="text-sm text-muted-foreground">{key('secretHint')}</p>
        {(
          ['accessToken', 'refreshToken', 'domain', 'uid', 'enterpriseId', 'tokenType'] as const
        ).map((field) => (
          <div className="space-y-2" key={field}>
            <label htmlFor={'workbuddy-' + field}>{key(field)}</label>
            <Input
              id={'workbuddy-' + field}
              type={field.endsWith('Token') ? 'password' : 'text'}
              autoComplete="off"
              value={config[field] ?? (field.endsWith('Token') ? '' : effectiveConfig[field] || '')}
              onChange={(e) => setConfig({ ...config, [field]: e.target.value || undefined })}
            />
          </div>
        ))}
        {(['expiresAt', 'refreshExpiresAt'] as const).map((field) => (
          <div className="space-y-2" key={field}>
            <label htmlFor={'workbuddy-' + field}>{key(field)}</label>
            <Input
              id={'workbuddy-' + field}
              type="number"
              min="0"
              value={effectiveConfig[field] ?? ''}
              onChange={(e) =>
                setConfig({
                  ...config,
                  [field]: e.target.value ? Number(e.target.value) : undefined,
                })
              }
            />
          </div>
        ))}
      </div>
    </div>
  );
}
