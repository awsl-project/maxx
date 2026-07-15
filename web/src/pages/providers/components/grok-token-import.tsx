import { useState } from 'react';
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

export function GrokTokenImport() {
  const navigate = useNavigate();
  const createProvider = useCreateProvider();
  const [jsonText, setJsonText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async () => {
    setError(null);
    setSubmitting(true);
    try {
      const raw = JSON.parse(jsonText) as CPAxAIExportJSON;
      const grok = normalizeGrokConfig(raw);
      const data: CreateProviderData = {
        type: 'grok',
        name: grok.email ? `Grok (${grok.email})` : 'Grok',
        config: { grok },
        supportedClientTypes: ['openai'],
      };
      await createProvider.mutateAsync(data);
      navigate('/providers');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-4 p-6">
      <div>
        <h1 className="text-2xl font-semibold">Import Grok CPA JSON</h1>
        <p className="text-sm text-muted-foreground">
          Paste the CLIProxyAPI xAI OAuth JSON export. Token fields stay in provider config and are not logged by this form.
        </p>
      </div>
      <Textarea
        value={jsonText}
        onChange={(event) => setJsonText(event.target.value)}
        placeholder='{"type":"xai","auth_kind":"oauth",...}'
        className="min-h-80 font-mono text-xs"
      />
      {error && <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">{error}</div>}
      <div className="flex gap-2">
        <Button onClick={submit} disabled={submitting || !jsonText.trim()}>
          {submitting ? 'Importing…' : 'Import Grok provider'}
        </Button>
        <Button type="button" variant="outline" onClick={() => navigate('/providers/create')}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
