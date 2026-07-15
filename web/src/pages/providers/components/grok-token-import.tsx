import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowRight, Check, ChevronLeft, FileJson, Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui';
import { Textarea } from '@/components/ui/textarea';
import { ModelInput } from '@/components/ui/model-input';
import { PageHeader } from '@/components/layout/page-header';
import { useCreateModelMapping, useCreateProvider } from '@/hooks/queries';
import type { ClientType, CreateProviderData } from '@/lib/transport';
import type { ProviderConfigGrok } from '@/lib/transport/types';

export interface CPAxAIExportJSON {
  type?: string;
  auth_kind?: string;
  email?: string;
  sub?: string;
  access_token?: string;
  refresh_token?: string;
  id_token?: string;
  token_type?: string;
  expires_in?: number;
  expired?: string;
  last_refresh?: string;
  redirect_uri?: string;
  token_endpoint?: string;
  base_url?: string;
  disabled?: boolean;
  headers?: Record<string, string>;
}

interface MaxxGrokProviderExport {
  type?: string;
  name?: string;
  config?: {
    grok?: ProviderConfigGrok;
  };
}

interface CPAAuthRecordExport {
  provider?: string;
  disabled?: boolean;
  attributes?: Record<string, string>;
  metadata?: CPAxAIExportJSON;
}

interface GrokImportItem {
  source: string;
  raw: CPAxAIExportJSON | MaxxGrokProviderExport | CPAAuthRecordExport;
}

const GROK_CLIENT_TYPES = ['openai'] as const satisfies readonly ClientType[];

type ModelMappingsEditorProps = {
  mappings: Record<string, string>;
  onChange: (mappings: Record<string, string>) => void;
};

function ModelMappingsEditor({ mappings, onChange }: ModelMappingsEditorProps) {
  const { t } = useTranslation();
  const [newPattern, setNewPattern] = useState('');
  const [newTarget, setNewTarget] = useState('');
  const entries = useMemo(() => Object.entries(mappings), [mappings]);

  const updateEntry = (oldPattern: string, pattern: string, target: string) => {
    const next: Record<string, string> = {};
    for (const [key, value] of entries) {
      if (key !== oldPattern) next[key] = value;
    }
    const nextPattern = pattern.trim();
    const nextTarget = target.trim();
    if (nextPattern && nextTarget) next[nextPattern] = nextTarget;
    onChange(next);
  };

  const addEntry = () => {
    const pattern = newPattern.trim();
    const target = newTarget.trim();
    if (!pattern || !target) return;
    onChange({ ...mappings, [pattern]: target });
    setNewPattern('');
    setNewTarget('');
  };

  const removeEntry = (pattern: string) => {
    const next: Record<string, string> = {};
    for (const [key, value] of entries) {
      if (key !== pattern) next[key] = value;
    }
    onChange(next);
  };

  return (
    <div className="space-y-4">
      <div className="border-b border-border pb-2">
        <h3 className="text-lg font-semibold text-foreground">
          {t('addProvider.grok.modelMappingTitle')}
        </h3>
        <p className="mt-1 text-xs text-muted-foreground">
          {t('addProvider.grok.modelMappingDesc')}
        </p>
      </div>
      <div className="rounded-xl border border-border bg-card p-4">
        {entries.length > 0 ? (
          <div className="mb-4 space-y-2">
            {entries.map(([pattern, target], index) => (
              <div key={pattern} className="flex items-center gap-2">
                <span className="w-6 shrink-0 text-xs text-muted-foreground">{index + 1}.</span>
                <ModelInput
                  value={pattern}
                  onChange={(value) => updateEntry(pattern, value, target)}
                  placeholder={t('addProvider.grok.modelMappingFrom')}
                  className="h-8 min-w-0 flex-1 text-sm"
                />
                <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                <Input
                  value={target}
                  onChange={(event) => updateEntry(pattern, pattern, event.target.value)}
                  placeholder={t('addProvider.grok.modelMappingTo')}
                  className="h-8 min-w-0 flex-1 font-mono text-sm"
                />
                <Button variant="ghost" size="sm" onClick={() => removeEntry(pattern)}>
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>
            ))}
          </div>
        ) : (
          <div className="mb-4 py-6 text-center text-sm text-muted-foreground">
            {t('addProvider.grok.modelMappingEmpty')}
          </div>
        )}

        <div className="flex items-center gap-2 border-t border-border pt-4">
          <ModelInput
            value={newPattern}
            onChange={setNewPattern}
            placeholder={t('addProvider.grok.modelMappingFrom')}
            className="h-8 min-w-0 flex-1 text-sm"
          />
          <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground" />
          <Input
            value={newTarget}
            onChange={(event) => setNewTarget(event.target.value)}
            placeholder={t('addProvider.grok.modelMappingTo')}
            className="h-8 min-w-0 flex-1 font-mono text-sm"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={addEntry}
            disabled={!newPattern.trim() || !newTarget.trim()}
          >
            <Plus className="mr-1 h-4 w-4" />
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === 'number' ? value : undefined;
}

function booleanValue(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined;
}

function stringRecord(value: unknown): Record<string, string> | undefined {
  if (!isRecord(value)) return undefined;
  const out: Record<string, string> = {};
  for (const [key, val] of Object.entries(value)) {
    if (typeof val === 'string') out[key] = val;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

function normalizeSnakeCaseCredential(raw: Record<string, unknown>): ProviderConfigGrok {
  const type = stringValue(raw.type);
  if (type !== 'xai') {
    throw new Error(`Expected CPA xai credential JSON, got type=${type || '(empty)'}`);
  }
  const authKind = stringValue(raw.auth_kind) || 'oauth';
  if (authKind !== 'oauth') {
    throw new Error(`Expected oauth credential JSON, got auth_kind=${authKind || '(empty)'}`);
  }
  const accessToken = stringValue(raw.access_token);
  const refreshToken = stringValue(raw.refresh_token);
  if (!accessToken && !refreshToken) {
    throw new Error('access_token or refresh_token is required');
  }
  return {
    type: 'xai',
    authKind: 'oauth',
    email: stringValue(raw.email),
    sub: stringValue(raw.sub),
    accessToken,
    refreshToken,
    idToken: stringValue(raw.id_token),
    tokenType: stringValue(raw.token_type),
    expiresIn: numberValue(raw.expires_in),
    expired: stringValue(raw.expired),
    lastRefresh: stringValue(raw.last_refresh),
    redirectURI: stringValue(raw.redirect_uri),
    tokenEndpoint: stringValue(raw.token_endpoint),
    baseURL: stringValue(raw.base_url),
    disabled: booleanValue(raw.disabled),
    headers: stringRecord(raw.headers),
  };
}

function normalizeMaxxGrokConfig(raw: ProviderConfigGrok): ProviderConfigGrok {
  if ((raw.type || 'xai') !== 'xai') {
    throw new Error(`Expected Maxx Grok xai config, got type=${raw.type || '(empty)'}`);
  }
  if ((raw.authKind || 'oauth') !== 'oauth') {
    throw new Error(`Expected Maxx Grok oauth config, got authKind=${raw.authKind || '(empty)'}`);
  }
  if (!raw.accessToken && !raw.refreshToken) {
    throw new Error('accessToken or refreshToken is required');
  }
  return {
    ...raw,
    type: 'xai',
    authKind: 'oauth',
  };
}

export function normalizeGrokConfig(
  raw: CPAxAIExportJSON | MaxxGrokProviderExport | CPAAuthRecordExport,
): ProviderConfigGrok {
  if (!isRecord(raw)) {
    throw new Error('Expected CPA xai credential JSON object');
  }

  // Maxx provider export/import payload: { type: 'grok', config: { grok: { ...camelCase } } }.
  const config = raw.config;
  if (isRecord(config) && isRecord(config.grok)) {
    return normalizeMaxxGrokConfig(config.grok as ProviderConfigGrok);
  }

  // CLIProxyAPI auth record export: { provider: 'xai', attributes: {...}, metadata: { type: 'xai', ... } }.
  if (isRecord(raw.metadata)) {
    const provider = stringValue(raw.provider);
    if (provider && provider !== 'xai' && provider !== 'grok') {
      throw new Error(`Expected CPA xai auth record, got provider=${provider}`);
    }
    const merged: Record<string, unknown> = { ...raw.metadata };
    if (!merged.auth_kind && isRecord(raw.attributes)) merged.auth_kind = raw.attributes.auth_kind;
    if (merged.disabled === undefined && typeof raw.disabled === 'boolean')
      merged.disabled = raw.disabled;
    return normalizeSnakeCaseCredential(merged);
  }

  return normalizeSnakeCaseCredential(raw);
}

function providerName(grok: ProviderConfigGrok, source: string): string {
  if (grok.email) return `Grok (${grok.email})`;
  const stem = source.replace(/\.json$/i, '').trim();
  return stem ? `Grok (${stem})` : 'Grok';
}

function parseImportItemsFromText(jsonText: string): GrokImportItem[] {
  const trimmed = jsonText.trim();
  if (!trimmed) return [];
  const parsed = JSON.parse(trimmed) as GrokImportItem['raw'] | GrokImportItem['raw'][];
  if (Array.isArray(parsed)) {
    return parsed.map((raw, index) => ({ source: `pasted item ${index + 1}`, raw }));
  }
  return [{ source: 'pasted JSON', raw: parsed }];
}

async function parseImportItemsFromFiles(files: FileList): Promise<GrokImportItem[]> {
  const items: GrokImportItem[] = [];
  for (const file of Array.from(files)) {
    const text = await file.text();
    const parsed = JSON.parse(text) as GrokImportItem['raw'] | GrokImportItem['raw'][];
    if (Array.isArray(parsed)) {
      parsed.forEach((raw, index) => items.push({ source: `${file.name}#${index + 1}`, raw }));
    } else {
      items.push({ source: file.name, raw: parsed });
    }
  }
  return items;
}

export function GrokTokenImport() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const createProvider = useCreateProvider();
  const createModelMapping = useCreateModelMapping();
  const [jsonText, setJsonText] = useState('');
  const [fileItems, setFileItems] = useState<GrokImportItem[]>([]);
  const [modelMapping, setModelMapping] = useState<Record<string, string>>({});
  const [disableErrorCooldown, setDisableErrorCooldown] = useState(false);
  const [blackBox, setBlackBox] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');

  const handleFilesSelected = async (files: FileList | null) => {
    setError(null);
    if (!files || files.length === 0) {
      setFileItems([]);
      return;
    }
    try {
      setFileItems(await parseImportItemsFromFiles(files));
    } catch (err) {
      setFileItems([]);
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const submit = async () => {
    setError(null);
    setSaveStatus('idle');
    setSubmitting(true);
    try {
      const items = fileItems.length > 0 ? fileItems : parseImportItemsFromText(jsonText);
      if (items.length === 0) throw new Error('Paste JSON or select files');

      const mappingEntries = Object.entries(modelMapping);
      for (const item of items) {
        const grok = normalizeGrokConfig(item.raw);
        const data: CreateProviderData = {
          type: 'grok',
          name: providerName(grok, item.source),
          config: { disableErrorCooldown, grok },
          supportedClientTypes: [...GROK_CLIENT_TYPES],
          excludeFromExport: blackBox,
          blackBox,
        };
        const provider = await createProvider.mutateAsync(data);
        for (const [pattern, target] of mappingEntries) {
          await createModelMapping.mutateAsync({
            scope: 'provider',
            providerID: provider.id,
            pattern,
            target,
          });
        }
      }
      setSaveStatus('success');
      navigate('/providers');
    } catch (err) {
      setSaveStatus('error');
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  const pastedItemCount = useMemo(() => {
    try {
      return parseImportItemsFromText(jsonText).length;
    } catch {
      return jsonText.trim() ? 1 : 0;
    }
  }, [jsonText]);
  const itemCount = fileItems.length > 0 ? fileItems.length : pastedItemCount;

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={
          <ChevronLeft className="cursor-pointer" onClick={() => navigate('/providers/create')} />
        }
        title={t('addProvider.grok.name')}
        description={t('addProvider.grok.description')}
      >
        <Button type="button" variant="secondary" onClick={() => navigate('/providers')}>
          {t('common.cancel')}
        </Button>
        <Button onClick={submit} disabled={submitting || itemCount === 0}>
          {submitting ? (
            t('common.saving')
          ) : saveStatus === 'success' ? (
            <>
              <Check size={14} /> {t('common.saved')}
            </>
          ) : itemCount > 1 ? (
            t('addProvider.grok.importMany', { count: itemCount })
          ) : (
            t('addProvider.grok.importOne')
          )}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          <div className="space-y-6">
            <h3 className="border-b border-border pb-2 text-lg font-semibold text-foreground">
              {t('provider.basicInfo')}
            </h3>
            <div className="rounded-lg border bg-card p-4">
              <label
                className="block text-sm font-medium text-foreground"
                htmlFor="grok-json-files"
              >
                <span className="inline-flex items-center gap-2">
                  <FileJson size={14} /> {t('addProvider.grok.jsonFiles')}
                </span>
              </label>
              <input
                id="grok-json-files"
                type="file"
                accept="application/json,.json"
                multiple
                className="mt-2 block w-full text-sm text-muted-foreground file:mr-4 file:rounded-md file:border-0 file:bg-primary file:px-3 file:py-2 file:text-sm file:font-medium file:text-primary-foreground hover:file:bg-primary/90"
                onChange={(event) => void handleFilesSelected(event.target.files)}
              />
              {fileItems.length > 0 && (
                <div className="mt-3 text-xs text-muted-foreground">
                  {t('addProvider.grok.selectedCount', { count: fileItems.length })}
                </div>
              )}
            </div>
            <Textarea
              value={jsonText}
              onChange={(event) => {
                setJsonText(event.target.value);
                if (event.target.value.trim()) setFileItems([]);
              }}
              placeholder='{"type":"xai","auth_kind":"oauth",...}'
              className="min-h-64 font-mono text-xs"
            />
          </div>

          <div className="space-y-6">
            <h3 className="border-b border-border pb-2 text-lg font-semibold text-foreground">
              {t('addProvider.grok.clientsTitle')}
            </h3>
            <p className="text-xs text-muted-foreground">{t('addProvider.grok.clientsDesc')}</p>
            <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">OpenAI</div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('addProvider.grok.clientOpenAIDesc')}
                </p>
              </div>
              <Switch checked disabled />
            </div>
          </div>

          <ModelMappingsEditor mappings={modelMapping} onChange={setModelMapping} />

          <div className="space-y-6">
            <h3 className="border-b border-border pb-2 text-lg font-semibold text-foreground">
              {t('provider.errorCooldownTitle')}
            </h3>
            <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">
                  {t('provider.disableErrorCooldown')}
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('provider.disableErrorCooldownDesc')}
                </p>
              </div>
              <Switch checked={disableErrorCooldown} onCheckedChange={setDisableErrorCooldown} />
            </div>
          </div>

          <div className="space-y-6">
            <h3 className="border-b border-border pb-2 text-lg font-semibold text-foreground">
              {t('provider.visibilityAndExport')}
            </h3>
            <div className="flex items-center justify-between rounded-xl border border-border bg-card p-4">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">{t('provider.blackBox')}</div>
                <p className="mt-1 text-xs text-muted-foreground">{t('provider.blackBoxDesc')}</p>
              </div>
              <Switch checked={blackBox} onCheckedChange={setBlackBox} />
            </div>
          </div>

          {error && (
            <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
