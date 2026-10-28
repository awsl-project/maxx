import { Textarea } from '@/components/ui/textarea';
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@/components/ui';
import { getTransport, type Project } from '@/lib/transport';
import type { PrivacyAction, PrivacyConfig, PrivacyRule } from '@/lib/transport/privacy-types';
import { Choice, Field, Toggle } from './controls';
import { PrivacyModels } from './models';
import { PrivacyPreview } from './preview';
import { PrivacyHistory } from './history';
import { projectKeys } from '@/hooks/queries';

export function ProjectPrivacySettings({ project }: { project: Project }) {
  const { t } = useTranslation();
  const config = useQuery({
    queryKey: ['privacy', project.id, 'config'],
    queryFn: () => getTransport().getPrivacyConfig(project.id),
  });
  if (config.isLoading) return <p>{t('common.loading')}</p>;
  if (!config.data)
    return (
      <div role="alert">
        <p>{t('privacy.loadFailed')}</p>
        <Button variant="outline" onClick={() => void config.refetch()}>
          {t('common.refresh')}
        </Button>
      </div>
    );
  return (
    <PrivacyEditor
      key={`${project.id}:${config.dataUpdatedAt}`}
      project={project}
      initial={config.data}
    />
  );
}

function PrivacyEditor({ project, initial }: { project: Project; initial: PrivacyConfig }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [config, setConfig] = useState(initial);
  const [tab, setTab] = useState<'settings' | 'history'>('settings');
  const [actionKind, setActionKind] = useState('');
  const categories = useQuery({
    queryKey: ['privacy', project.id, 'categories'],
    queryFn: () => getTransport().getPrivacyCategories(project.id),
  });
  const providers = useQuery({
    queryKey: ['privacy', project.id, 'providers'],
    queryFn: () => getTransport().getProviders(),
  });
  const set = <K extends keyof PrivacyConfig>(key: K, value: PrivacyConfig[K]) =>
    setConfig((c) => ({ ...c, [key]: value }));
  const save = useMutation({
    mutationFn: () => getTransport().savePrivacyConfig(project.id, config),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ['privacy', project.id, 'config'] }),
        client.invalidateQueries({ queryKey: projectKeys.all }),
      ]);
    },
  });
  const updateRule = (index: number, patch: Partial<PrivacyRule>) =>
    set(
      'rules',
      config.rules?.map((rule, at) => (at === index ? { ...rule, ...patch } : rule)),
    );
  const actions = ['mask', 'warn', 'block'] as const;
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {project.name} · {t('privacy.settings')}
        </CardTitle>
        <div className="flex gap-2">
          <Button
            type="button"
            variant={tab === 'settings' ? 'default' : 'outline'}
            onClick={() => setTab('settings')}
          >
            {t('privacy.settings')}
          </Button>
          <Button
            type="button"
            variant={tab === 'history' ? 'default' : 'outline'}
            onClick={() => setTab('history')}
          >
            {t('privacy.history')}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {tab === 'history' ? (
          <PrivacyHistory id={project.id} />
        ) : (
          <form
            className="space-y-6"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            <Toggle
              label={t('privacy.enabled')}
              checked={config.enabled}
              onChange={(value) => set('enabled', value)}
              description={t('privacy.offHelp')}
            />
            <div className="grid gap-6 lg:grid-cols-2">
              <div className="space-y-4">
                <Field label={t('privacy.defaultAction')}>
                  <Choice
                    value={config.mode}
                    onChange={(e) => set('mode', e.target.value as PrivacyAction)}
                  >
                    {actions.map((a) => (
                      <option key={a} value={a}>
                        {t(`privacy.actionName.${a}`)}
                      </option>
                    ))}
                  </Choice>
                </Field>
                <p className="text-xs text-text-secondary">{t('privacy.actionHelp')}</p>
                <Toggle
                  label={t('privacy.secrets')}
                  checked={config.secrets}
                  onChange={(value) => set('secrets', value)}
                />
                <Toggle
                  label={t('privacy.personal')}
                  checked={config.personal}
                  onChange={(value) => set('personal', value)}
                  description={t('privacy.personalHelp')}
                />
                <Toggle
                  label={t('privacy.shape')}
                  checked={config.shape}
                  onChange={(value) => set('shape', value)}
                  description={t('privacy.shapeHelp')}
                />
              </div>
              <div className="rounded-lg border border-border p-4">
                <p className="mb-2 text-sm font-medium">{t('privacy.categories')}</p>
                {categories.error && <p role="alert">{t('privacy.loadFailed')}</p>}
                {categories.data?.map((category) => (
                  <Toggle
                    key={category.ID}
                    label={t(`privacy.category.${category.ID}`)}
                    checked={config.kinds?.[category.ID] ?? category.On}
                    onChange={(value) => set('kinds', { ...config.kinds, [category.ID]: value })}
                  />
                ))}
              </div>
            </div>
            <details className="rounded-lg border border-border p-4" open>
              <summary className="cursor-pointer text-sm font-semibold">
                {t('privacy.custom')}
              </summary>
              <div className="mt-4 grid gap-5 lg:grid-cols-2">
                <div className="space-y-3">
                  <Field label={t('privacy.words')} description={t('privacy.onePerLine')}>
                    <Textarea
                      aria-label={t('privacy.words')}
                      value={config.words?.join('\n') ?? ''}
                      onChange={(e) => set('words', e.target.value.split('\n'))}
                      rows={4}
                    />
                  </Field>
                  <Toggle
                    label={t('privacy.ignoreCase')}
                    checked={config.wordsAnyCase}
                    onChange={(value) => set('wordsAnyCase', value)}
                  />
                  <Toggle
                    label={t('privacy.wholeWords')}
                    checked={config.wordsWhole}
                    onChange={(value) => set('wordsWhole', value)}
                  />
                </div>
                <Field label={t('privacy.allowlist')} description={t('privacy.allowHelp')}>
                  <Textarea
                    aria-label={t('privacy.allowlist')}
                    value={config.allow?.join('\n') ?? ''}
                    onChange={(e) => set('allow', e.target.value.split('\n'))}
                    rows={6}
                  />
                </Field>
              </div>
              <div className="mt-5 space-y-3">
                <p className="text-sm font-medium">{t('privacy.rules')}</p>
                <p className="text-xs text-text-secondary">{t('privacy.rulesHelp')}</p>
                {config.rules?.map((rule, index) => (
                  <div
                    key={index}
                    className="grid gap-2 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1fr_2fr_auto]"
                  >
                    <Input
                      aria-label={`${t('privacy.kind')} ${index + 1}`}
                      placeholder="CUSTOM"
                      value={rule.kind}
                      onChange={(e) => updateRule(index, { kind: e.target.value.toUpperCase() })}
                      maxLength={24}
                    />
                    <Input
                      aria-label={`${t('privacy.prefix')} ${index + 1}`}
                      placeholder="my-key-"
                      value={rule.prefix ?? ''}
                      onChange={(e) => updateRule(index, { prefix: e.target.value })}
                      maxLength={64}
                    />
                    <Input
                      aria-label={`${t('privacy.regex')} ${index + 1}`}
                      placeholder="token=([A-Za-z0-9]{8,})"
                      value={rule.regex ?? ''}
                      onChange={(e) => updateRule(index, { regex: e.target.value })}
                      maxLength={300}
                    />
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() =>
                        set(
                          'rules',
                          config.rules?.filter((_, at) => at !== index),
                        )
                      }
                    >
                      {t('common.delete')}
                    </Button>
                  </div>
                ))}
                <Button
                  type="button"
                  variant="outline"
                  disabled={(config.rules?.length ?? 0) >= 32}
                  onClick={() =>
                    set('rules', [...(config.rules ?? []), { kind: 'CUSTOM', prefix: '' }])
                  }
                >
                  {t('privacy.addRule')}
                </Button>
              </div>
            </details>
            <details className="rounded-lg border border-border p-4">
              <summary className="cursor-pointer text-sm font-semibold">
                {t('privacy.kindActions')}
              </summary>
              <div className="mt-4 space-y-3">
                {Object.entries(config.actions ?? {}).map(([kind, action]) => (
                  <div key={kind} className="flex flex-wrap items-center gap-3">
                    <span className="w-40 break-all text-sm">{kind}</span>
                    <Choice
                      aria-label={`${kind} ${t('privacy.action')}`}
                      value={action}
                      onChange={(e) =>
                        set('actions', {
                          ...config.actions,
                          [kind]: e.target.value as PrivacyAction,
                        })
                      }
                    >
                      {actions.map((a) => (
                        <option key={a} value={a}>
                          {t(`privacy.actionName.${a}`)}
                        </option>
                      ))}
                    </Choice>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => {
                        const next = { ...config.actions };
                        delete next[kind];
                        set('actions', next);
                      }}
                    >
                      {t('common.delete')}
                    </Button>
                  </div>
                ))}
                <div className="flex gap-2">
                  <Input
                    aria-label={t('privacy.newKind')}
                    value={actionKind}
                    onChange={(e) => setActionKind(e.target.value.toUpperCase())}
                    placeholder="EMAIL / API_KEY / CUSTOM"
                    maxLength={24}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    disabled={!/^[A-Z][A-Z0-9_]{0,23}$/.test(actionKind)}
                    onClick={() => {
                      set('actions', { ...config.actions, [actionKind]: config.mode });
                      setActionKind('');
                    }}
                  >
                    {t('privacy.addAction')}
                  </Button>
                </div>
              </div>
            </details>
            <details className="rounded-lg border border-border p-4">
              <summary className="cursor-pointer text-sm font-semibold">
                {t('privacy.models')}
              </summary>
              <div className="mt-4">
                <PrivacyModels
                  id={project.id}
                  model={config.model}
                  setModel={(value) => set('model', value)}
                />
              </div>
            </details>
            <details className="rounded-lg border border-border p-4">
              <summary className="cursor-pointer text-sm font-semibold">
                {t('privacy.providerExceptions')}
              </summary>
              <p className="mt-3 text-xs text-text-secondary">{t('privacy.bypassHelp')}</p>
              {providers.error && <p role="alert">{t('privacy.loadFailed')}</p>}
              {providers.data?.map((provider) => (
                <Toggle
                  key={provider.id}
                  label={provider.name}
                  checked={config.bypassProviders?.includes(provider.id) ?? false}
                  onChange={(value) =>
                    set(
                      'bypassProviders',
                      value
                        ? [...(config.bypassProviders ?? []), provider.id]
                        : config.bypassProviders?.filter((id) => id !== provider.id),
                    )
                  }
                />
              ))}
            </details>
            {save.error && (
              <p role="alert" className="text-sm text-destructive">
                {t('privacy.saveFailed')}
              </p>
            )}
            <Button type="submit" disabled={save.isPending}>
              {t(save.isPending ? 'common.saving' : 'privacy.savePolicy')}
            </Button>
            <details className="rounded-lg border border-border p-4">
              <summary className="cursor-pointer text-sm font-semibold">
                {t('privacy.preview')}
              </summary>
              <div className="mt-4">
                <PrivacyPreview id={project.id} config={config} />
              </div>
            </details>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
