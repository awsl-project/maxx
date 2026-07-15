import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { useCreateProvider } from '@/hooks/queries';
import type { CreateProviderData } from '@/lib/transport';
import type { ProviderConfigGrok } from '@/lib/transport/types';

interface CPAxAIExportJSON {
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

interface GrokImportItem {
  source: string;
  raw: CPAxAIExportJSON;
}

function normalizeGrokConfig(raw: CPAxAIExportJSON): ProviderConfigGrok {
  if (raw.type !== 'xai') {
    throw new Error(`Expected CPA xai credential JSON, got type=${raw.type || '(empty)'}`);
  }
  if ((raw.auth_kind || 'oauth') !== 'oauth') {
    throw new Error(`Expected oauth credential JSON, got auth_kind=${raw.auth_kind || '(empty)'}`);
  }
  if (!raw.access_token && !raw.refresh_token) {
    throw new Error('access_token or refresh_token is required');
  }
  return {
    type: 'xai',
    authKind: 'oauth',
    email: raw.email,
    sub: raw.sub,
    accessToken: raw.access_token,
    refreshToken: raw.refresh_token,
    idToken: raw.id_token,
    tokenType: raw.token_type,
    expiresIn: raw.expires_in,
    expired: raw.expired,
    lastRefresh: raw.last_refresh,
    redirectURI: raw.redirect_uri,
    tokenEndpoint: raw.token_endpoint,
    baseURL: raw.base_url,
    disabled: raw.disabled,
    headers: raw.headers,
  };
}

function providerName(grok: ProviderConfigGrok, source: string): string {
  if (grok.email) {
    return `Grok (${grok.email})`;
  }
  const stem = source.replace(/\.json$/i, '').trim();
  return stem ? `Grok (${stem})` : 'Grok';
}

function parseImportItemsFromText(jsonText: string): GrokImportItem[] {
  const trimmed = jsonText.trim();
  if (!trimmed) {
    return [];
  }
  const parsed = JSON.parse(trimmed) as CPAxAIExportJSON | CPAxAIExportJSON[];
  if (Array.isArray(parsed)) {
    return parsed.map((raw, index) => ({ source: `pasted item ${index + 1}`, raw }));
  }
  return [{ source: 'pasted JSON', raw: parsed }];
}

async function parseImportItemsFromFiles(files: FileList): Promise<GrokImportItem[]> {
  const items: GrokImportItem[] = [];
  for (const file of Array.from(files)) {
    const text = await file.text();
    const parsed = JSON.parse(text) as CPAxAIExportJSON | CPAxAIExportJSON[];
    if (Array.isArray(parsed)) {
      parsed.forEach((raw, index) => items.push({ source: `${file.name}#${index + 1}`, raw }));
    } else {
      items.push({ source: file.name, raw: parsed });
    }
  }
  return items;
}

export function GrokTokenImport() {
  const navigate = useNavigate();
  const createProvider = useCreateProvider();
  const [jsonText, setJsonText] = useState('');
  const [fileItems, setFileItems] = useState<GrokImportItem[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

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
    setSubmitting(true);
    try {
      const items = fileItems.length > 0 ? fileItems : parseImportItemsFromText(jsonText);
      if (items.length === 0) {
        throw new Error('Paste JSON or select files');
      }
      for (const item of items) {
        const grok = normalizeGrokConfig(item.raw);
        const data: CreateProviderData = {
          type: 'grok',
          name: providerName(grok, item.source),
          config: { grok },
          supportedClientTypes: ['openai'],
        };
        await createProvider.mutateAsync(data);
      }
      navigate('/providers');
    } catch (err) {
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
    <div className="mx-auto flex max-w-3xl flex-col gap-4 p-6">
      <div>
        <h1 className="text-2xl font-semibold">Import Grok JSON</h1>
      </div>
      <div className="rounded-lg border bg-card p-4">
        <label className="block text-sm font-medium text-foreground" htmlFor="grok-json-files">
          JSON files
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
            {fileItems.length} credential{fileItems.length === 1 ? '' : 's'} selected.
          </div>
        )}
      </div>
      <Textarea
        value={jsonText}
        onChange={(event) => {
          setJsonText(event.target.value);
          if (event.target.value.trim()) {
            setFileItems([]);
          }
        }}
        placeholder='{"type":"xai","auth_kind":"oauth",...}'
        className="min-h-80 font-mono text-xs"
      />
      {error && <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">{error}</div>}
      <div className="flex gap-2">
        <Button onClick={submit} disabled={submitting || itemCount === 0}>
          {submitting ? 'Importing…' : itemCount > 1 ? `Import ${itemCount} Grok providers` : 'Import Grok provider'}
        </Button>
        <Button type="button" variant="outline" onClick={() => navigate('/providers/create')}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
