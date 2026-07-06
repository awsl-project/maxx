import { useState } from 'react';
import { Clock3, Copy, KeyRound, LogOut, Server, ShieldCheck, UserRound } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle } from '@/components/ui';
import { useAuth } from '@/lib/auth-context';
import {
  useCreateUserPanelAPIToken,
  useProxyStatus,
  usePublicSettings,
  useRegenerateUserPanelAPIToken,
  useUserPanelAPIToken,
} from '@/hooks/queries';
import type { APIToken } from '@/lib/transport';
import { cn } from '@/lib/utils';

function maskNumericIdentity(value?: number) {
  if (!value || value <= 0) return '••';
  const raw = String(value);
  if (raw.length <= 2) return `••${raw}`;
  return `${'•'.repeat(Math.max(2, raw.length - 2))}${raw.slice(-2)}`;
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value || 0);
}

function formatDateTime(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function getTokenStatus(token: APIToken) {
  if (!token.isEnabled) return 'disabled';
  if (token.expiresAt && new Date(token.expiresAt).getTime() <= Date.now()) return 'expired';
  return 'active';
}

export function UserPanelPage() {
  const { t } = useTranslation();
  const { user, logout } = useAuth();
  const { data: proxyStatus } = useProxyStatus();
  const { data: settings } = usePublicSettings();
  const { data: userPanelTokenResponse, isLoading: tokenLoading } = useUserPanelAPIToken();
  const createUserPanelToken = useCreateUserPanelAPIToken();
  const regenerateUserPanelToken = useRegenerateUserPanelAPIToken();
  const [copiedEndpointId, setCopiedEndpointId] = useState('');
  const [exampleCopied, setExampleCopied] = useState(false);
  const [keyCopied, setKeyCopied] = useState(false);
  const [oneTimeToken, setOneTimeToken] = useState('');

  const userPanelToken = userPanelTokenResponse?.apiToken ?? undefined;

  const tenantLabel = user?.tenantName?.trim()
    ? user.tenantName.trim()
    : user?.tenantID
      ? t('nav.tenantFallback', { id: user.tenantID })
      : t('nav.tenantUnknown');
  const authProtected = settings?.api_token_auth_enabled === 'true';
  const proxyOnline = proxyStatus?.running ?? true;
  const origin = typeof window === 'undefined' ? '' : window.location.origin;
  const openAICodexBaseURL = `${origin}/v1`;
  const claudeBaseURL = origin;
  const geminiEndpoint = `${origin}/v1beta/models/{model}:generateContent`;
  const endpointHints = [
    { id: 'openai-codex', label: t('userPanel.routeOpenAICodex'), url: openAICodexBaseURL },
    { id: 'claude', label: t('userPanel.routeClaude'), url: claudeBaseURL },
    { id: 'gemini', label: t('userPanel.routeGemini'), url: geminiEndpoint },
  ];
  const curlExample = `curl ${openAICodexBaseURL}/chat/completions \
  -H "Authorization: Bearer <${t('userPanel.yourKey')}>" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'`;

  const handleCopyEndpoint = async (endpointId: string, url: string) => {
    if (!url || typeof navigator === 'undefined' || !navigator.clipboard) return;
    await navigator.clipboard.writeText(url);
    setCopiedEndpointId(endpointId);
    window.setTimeout(() => setCopiedEndpointId(''), 1600);
  };

  const handleCopyOneTimeToken = async () => {
    if (!oneTimeToken || typeof navigator === 'undefined' || !navigator.clipboard) return;
    await navigator.clipboard.writeText(oneTimeToken);
    setKeyCopied(true);
    window.setTimeout(() => setKeyCopied(false), 1600);
  };

  const handleCopyExample = async () => {
    if (!curlExample || typeof navigator === 'undefined' || !navigator.clipboard) return;
    await navigator.clipboard.writeText(curlExample);
    setExampleCopied(true);
    window.setTimeout(() => setExampleCopied(false), 1600);
  };

  const handleCreateUserPanelToken = async () => {
    const result = await createUserPanelToken.mutateAsync();
    setOneTimeToken(result.token);
  };

  const handleRegenerateUserPanelToken = async () => {
    if (typeof window !== 'undefined' && !window.confirm(t('userPanel.regenerateConfirm'))) return;
    const result = await regenerateUserPanelToken.mutateAsync();
    setOneTimeToken(result.token);
    setKeyCopied(false);
  };

  const tokenActionPending = createUserPanelToken.isPending || regenerateUserPanelToken.isPending;

  return (
    <main className="min-h-svh bg-muted/30 px-4 py-6 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-5">
        <header className="flex flex-col gap-4 rounded-2xl border border-border bg-card/95 p-5 shadow-sm sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-11 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm">
              <UserRound className="size-5" />
            </div>
            <div>
              <h1 className="text-xl font-semibold tracking-tight">{t('userPanel.title')}</h1>
              <p className="text-sm text-muted-foreground">{t('userPanel.description')}</p>
            </div>
          </div>
          <div className="flex flex-col gap-3 sm:items-end">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={proxyOnline ? 'success' : 'danger'}>
                {proxyOnline ? t('userPanel.online') : t('userPanel.offline')}
              </Badge>
              <Badge variant="outline">{tenantLabel}</Badge>
            </div>
            <Button
              variant="outline"
              className="gap-2 border-destructive/30 text-destructive hover:bg-destructive/10"
              onClick={logout}
            >
              <LogOut className="size-4" />
              {t('nav.logout')}
            </Button>
          </div>
        </header>

        <section className="grid gap-5 lg:grid-cols-[1.2fr_0.8fr]">
          <Card className="border-border bg-card shadow-sm">
            <CardHeader className="border-b border-border">
              <CardTitle className="flex items-center gap-2 text-base font-medium">
                <KeyRound className="size-4 text-muted-foreground" />
                {t('userPanel.myKey')}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 p-5">
              {oneTimeToken ? (
                <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div>
                      <p className="font-medium text-emerald-700 dark:text-emerald-300">
                        {t('userPanel.oneTimeKeyTitle')}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t('userPanel.oneTimeKeyDescription')}
                      </p>
                    </div>
                    <Button size="sm" className="gap-2" onClick={handleCopyOneTimeToken}>
                      <Copy className="size-3.5" />
                      {keyCopied ? t('common.copied') : t('userPanel.copyKey')}
                    </Button>
                  </div>
                  <p className="mt-3 break-all rounded-lg bg-background px-3 py-2 font-mono text-xs">
                    {oneTimeToken}
                  </p>
                </div>
              ) : null}

              {tokenLoading ? (
                <div className="text-sm text-muted-foreground">{t('common.loading')}</div>
              ) : userPanelToken ? (
                <div className="rounded-xl border border-border bg-muted/25 p-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="truncate font-medium">{userPanelToken.name}</p>
                        <Badge
                          variant={
                            getTokenStatus(userPanelToken) === 'active'
                              ? 'success'
                              : getTokenStatus(userPanelToken) === 'expired'
                                ? 'warning'
                                : 'danger'
                          }
                        >
                          {t(`userPanel.keyStatus.${getTokenStatus(userPanelToken)}`)}
                        </Badge>
                      </div>
                      <p className="mt-2 font-mono text-xs text-muted-foreground">
                        {userPanelToken.tokenPrefix || 'maxx_••••'}••••
                      </p>
                      <p className="mt-2 text-xs text-muted-foreground">
                        {t('userPanel.existingKeyHint')}
                      </p>
                    </div>
                    <Button
                      variant="outline"
                      className="gap-2 border-destructive/30 text-destructive hover:bg-destructive/10"
                      disabled={tokenActionPending}
                      onClick={handleRegenerateUserPanelToken}
                    >
                      <KeyRound className="size-4" />
                      {tokenActionPending ? t('common.loading') : t('userPanel.regenerateKey')}
                    </Button>
                  </div>
                  <div className="mt-4 grid gap-3 text-xs text-muted-foreground sm:grid-cols-3">
                    <div>
                      <p>{t('userPanel.useCount')}</p>
                      <p className="mt-1 font-medium text-foreground">
                        {formatNumber(userPanelToken.useCount)}
                      </p>
                    </div>
                    <div>
                      <p>{t('userPanel.lastUsed')}</p>
                      <p className="mt-1 font-medium text-foreground">
                        {formatDateTime(userPanelToken.lastUsedAt)}
                      </p>
                    </div>
                    <div>
                      <p>{t('userPanel.expiresAt')}</p>
                      <p className="mt-1 font-medium text-foreground">
                        {formatDateTime(userPanelToken.expiresAt)}
                      </p>
                    </div>
                  </div>
                </div>
              ) : (
                <div className="rounded-xl border border-dashed border-border bg-muted/20 p-5 text-center">
                  <KeyRound className="mx-auto size-9 text-muted-foreground" />
                  <p className="mt-3 font-medium">{t('userPanel.noDedicatedKey')}</p>
                  <p className="mx-auto mt-2 max-w-md text-sm text-muted-foreground">
                    {t('userPanel.noDedicatedKeyDescription')}
                  </p>
                  <Button
                    className="mt-4 gap-2"
                    disabled={tokenActionPending}
                    onClick={handleCreateUserPanelToken}
                  >
                    <KeyRound className="size-4" />
                    {tokenActionPending ? t('common.loading') : t('userPanel.createKey')}
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          <Card className="border-border bg-card shadow-sm">
            <CardHeader className="border-b border-border">
              <CardTitle className="flex items-center gap-2 text-base font-medium">
                <Server className="size-4 text-muted-foreground" />
                {t('userPanel.apiAccess')}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 pt-5">
              <div className="rounded-xl border border-border bg-muted/25 p-4">
                <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {t('userPanel.baseURL')}
                </p>
                <div className="mt-3 space-y-2">
                  {endpointHints.map((endpoint) => (
                    <div
                      key={endpoint.id}
                      className="grid gap-2 rounded-lg bg-background px-3 py-2 text-xs sm:grid-cols-[104px_1fr_auto] sm:items-center"
                    >
                      <span className="font-medium text-foreground">{endpoint.label}</span>
                      <code className="break-all text-muted-foreground">{endpoint.url || '—'}</code>
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-8 gap-2"
                        aria-label={`${t('common.copy')} ${endpoint.label}`}
                        onClick={() => handleCopyEndpoint(endpoint.id, endpoint.url)}
                      >
                        <Copy className="size-3.5" />
                        {copiedEndpointId === endpoint.id ? t('common.copied') : t('common.copy')}
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
              <div className="rounded-xl border border-border bg-muted/25 p-4">
                <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {t('userPanel.authMethod')}
                </p>
                <code className="mt-3 block rounded-lg bg-background px-3 py-2 text-xs text-muted-foreground">
                  Authorization: Bearer {'<'}
                  {t('userPanel.yourKey')}
                  {'>'}
                </code>
              </div>
              <div className="rounded-xl border border-border bg-muted/25 p-4">
                <div className="flex items-center justify-between gap-3">
                  <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t('userPanel.quickStart')}
                  </p>
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 gap-2"
                    onClick={handleCopyExample}
                  >
                    <Copy className="size-3.5" />
                    {exampleCopied ? t('common.copied') : t('common.copy')}
                  </Button>
                </div>
                <pre className="mt-3 overflow-x-auto rounded-lg bg-background px-3 py-2 text-xs text-muted-foreground">
                  <code>{curlExample}</code>
                </pre>
              </div>
              <div className="grid gap-3 text-sm text-muted-foreground">
                <div className="flex items-center justify-between gap-3">
                  <span>{t('userPanel.version')}</span>
                  <span className="font-mono text-foreground">{proxyStatus?.version || '—'}</span>
                </div>
                <div className="flex items-center justify-between gap-3">
                  <span>{t('userPanel.auth')}</span>
                  <Badge variant={authProtected ? 'info' : 'secondary'}>
                    {authProtected ? t('nav.accountStatusProtected') : t('nav.accountStatusLocal')}
                  </Badge>
                </div>
              </div>
            </CardContent>
          </Card>
        </section>

        <section className="grid gap-5 lg:grid-cols-3">
          <Card className="border-border bg-card shadow-sm lg:col-start-3">
            <CardHeader className="border-b border-border">
              <CardTitle className="flex items-center gap-2 text-base font-medium">
                <ShieldCheck className="size-4 text-muted-foreground" />
                {t('userPanel.accountCard')}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 pt-5 text-sm">
              {[
                [t('userPanel.username'), user?.username || t('nav.accountFallback')],
                [
                  t('userPanel.role'),
                  user?.role === 'admin' ? t('users.roleAdmin') : t('users.roleMember'),
                ],
                [t('userPanel.workspace'), tenantLabel],
                [t('userPanel.userId'), maskNumericIdentity(user?.id)],
                [t('userPanel.tenantId'), maskNumericIdentity(user?.tenantID)],
              ].map(([label, value]) => (
                <div key={label} className="flex items-center justify-between gap-3">
                  <span className="text-muted-foreground">{label}</span>
                  <span
                    className={cn(
                      'truncate text-right font-medium',
                      label === t('userPanel.userId') && 'font-mono',
                    )}
                  >
                    {value}
                  </span>
                </div>
              ))}
            </CardContent>
          </Card>
        </section>

        <footer className="flex items-center justify-center gap-2 text-xs text-muted-foreground">
          <Clock3 className="size-3.5" />
          <span>{t('userPanel.securityHint')}</span>
        </footer>
      </div>
    </main>
  );
}
