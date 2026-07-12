import { useEffect, useState } from 'react';
import {
  Clock3,
  Copy,
  Eye,
  EyeOff,
  KeyRound,
  ListChecks,
  LogOut,
  Server,
  UserRound,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui';
import { LanguageToggle } from '@/components/language-toggle';
import { MarqueeBackground } from '@/components/ui/marquee-background';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { useAuth } from '@/lib/auth-context';
import {
  useCreateUserPanelAPIToken,
  useProxyRequests,
  useProxyStatus,
  useRegenerateUserPanelAPIToken,
  useUserPanelAPIToken,
  useProxyRequestUpdates,
} from '@/hooks/queries';
import type { APIToken, ProxyRequest } from '@/lib/transport';
import {
  buildUserPanelChatCompletionsExample,
  buildUserPanelEndpointHints,
} from '@/lib/user-panel-endpoints';
import {
  getUserPanelTabStorageKey,
  resolveUserPanelTab,
  updateUserPanelTabSearch,
  type UserPanelTab,
} from '@/lib/user-panel-tabs';

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

function formatDuration(value?: number) {
  if (!value || value <= 0) return '—';
  const milliseconds = value / 1_000_000;
  if (milliseconds < 1000) return `${Math.round(milliseconds)}ms`;
  return `${(milliseconds / 1000).toFixed(2)}s`;
}

function getTokenStatus(token: APIToken) {
  if (!token.isEnabled) return 'disabled';
  if (token.expiresAt && new Date(token.expiresAt).getTime() <= Date.now()) return 'expired';
  return 'active';
}

function isActiveUserPanelRequest(request: ProxyRequest) {
  return request.status === 'PENDING' || request.status === 'IN_PROGRESS';
}

function getRequestStatusVariant(request: ProxyRequest) {
  if (request.status === 'COMPLETED' && request.statusCode < 400) return 'success';
  if (request.status === 'PENDING' || request.status === 'IN_PROGRESS') return 'warning';
  if (
    request.status === 'FAILED' ||
    request.status === 'CANCELLED' ||
    request.status === 'REJECTED'
  ) {
    return 'danger';
  }
  if (request.statusCode >= 400) return 'danger';
  return 'secondary';
}

function UserPanelRequestsTab() {
  const { t } = useTranslation();
  const { data, isLoading, isError } = useProxyRequests({ limit: 25 });
  const requests = data?.items ?? [];

  return (
    <Card className="min-h-[820px] border-border bg-card shadow-sm">
      <CardHeader className="border-b border-border">
        <CardTitle className="flex items-center gap-2 text-base font-medium">
          <ListChecks className="size-4 text-muted-foreground" />
          {t('userPanel.requestsTab')}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex min-h-[744px] flex-col p-5">
        {isLoading ? (
          <div className="flex flex-1 items-center justify-center text-center text-sm text-muted-foreground">
            {t('common.loading')}
          </div>
        ) : isError ? (
          <div className="flex flex-1 items-center justify-center">
            <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
              {t('userPanel.requestsLoadFailed')}
            </div>
          </div>
        ) : requests.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center text-center">
            <ListChecks className="size-9 text-muted-foreground" />
            <p className="mt-3 font-medium">{t('userPanel.noRequests')}</p>
            <p className="mt-2 max-w-md text-sm text-muted-foreground">
              {t('userPanel.noRequestsDescription')}
            </p>
          </div>
        ) : (
          <div className="divide-y divide-border rounded-xl border border-border">
            {requests.map((request) => (
              <div key={request.id} className="space-y-3 p-4">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0">
                    <p className="truncate font-mono text-sm font-medium">
                      {request.requestID || `#${request.id}`}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {formatDateTime(request.createdAt)} · {request.clientType || '—'} ·{' '}
                      {request.requestModel || '—'}
                    </p>
                  </div>
                  <Badge variant={getRequestStatusVariant(request)}>
                    {request.statusCode || '—'} · {request.status || '—'}
                  </Badge>
                </div>
                <div className="grid gap-2 text-xs text-muted-foreground sm:grid-cols-3">
                  <div className="rounded-lg bg-muted/25 px-3 py-2">
                    <span className="block">{t('userPanel.requestDuration')}</span>
                    <span className="mt-1 block font-mono text-foreground">
                      {formatDuration(request.duration)}
                    </span>
                  </div>
                  <div className="rounded-lg bg-muted/25 px-3 py-2">
                    <span className="block">{t('userPanel.requestInputTokens')}</span>
                    <span className="mt-1 block font-mono text-foreground">
                      {formatNumber(request.inputTokenCount)}
                    </span>
                  </div>
                  <div className="rounded-lg bg-muted/25 px-3 py-2">
                    <span className="block">{t('userPanel.requestOutputTokens')}</span>
                    <span className="mt-1 block font-mono text-foreground">
                      {formatNumber(request.outputTokenCount)}
                    </span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export function UserPanelPage() {
  const { t } = useTranslation();
  const { logout, user } = useAuth();
  const { data: proxyStatus } = useProxyStatus();
  const { data: userPanelTokenResponse, isLoading: tokenLoading } = useUserPanelAPIToken();
  const { data: userPanelRequests } = useProxyRequests({ limit: 25 });
  useProxyRequestUpdates();
  const createUserPanelToken = useCreateUserPanelAPIToken();
  const regenerateUserPanelToken = useRegenerateUserPanelAPIToken();
  const [copiedEndpointId, setCopiedEndpointId] = useState('');
  const [exampleCopied, setExampleCopied] = useState(false);
  const [keyCopied, setKeyCopied] = useState(false);
  const [oneTimeToken, setOneTimeToken] = useState('');
  const [showKeyMasked, setShowKeyMasked] = useState(true);
  const tabStorageKey = getUserPanelTabStorageKey(user?.id);
  const [activeTab, setActiveTab] = useState<UserPanelTab>(() => {
    if (typeof window === 'undefined') return 'main';
    const params = new URLSearchParams(window.location.search);
    const navigationEntry = window.performance.getEntriesByType('navigation')[0] as
      | PerformanceNavigationTiming
      | undefined;
    const allowStoredTab = navigationEntry?.type === 'reload';
    return resolveUserPanelTab({
      urlTab: params.get('tab'),
      storedTab: window.localStorage.getItem(getUserPanelTabStorageKey(user?.id)),
      allowStoredTab,
    });
  });

  const activeRequestCount =
    userPanelRequests?.items.filter((request) => isActiveUserPanelRequest(request)).length ?? 0;
  const userPanelToken = userPanelTokenResponse?.apiToken ?? undefined;
  const origin = typeof window === 'undefined' ? '' : window.location.origin;
  const endpointHints = buildUserPanelEndpointHints(origin).map((endpoint) => ({
    ...endpoint,
    label:
      endpoint.id === 'openai-codex'
        ? t('userPanel.routeOpenAICodex')
        : endpoint.id === 'claude'
          ? t('userPanel.routeClaude')
          : t('userPanel.routeGemini'),
  }));
  const curlExample = buildUserPanelChatCompletionsExample({ origin });

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const urlTab = new URLSearchParams(window.location.search).get('tab');
    const storedTab = window.localStorage.getItem(tabStorageKey);
    const navigationEntry = window.performance.getEntriesByType('navigation')[0] as
      | PerformanceNavigationTiming
      | undefined;
    const allowStoredTab = navigationEntry?.type === 'reload';
    setActiveTab(resolveUserPanelTab({ urlTab, storedTab, allowStoredTab }));
  }, [tabStorageKey]);

  const handleTabChange = (value: string | null) => {
    const nextTab = resolveUserPanelTab({ urlTab: value });
    setActiveTab(nextTab);
    if (typeof window === 'undefined') {
      return;
    }
    window.localStorage.setItem(tabStorageKey, nextTab);
    const nextSearch = updateUserPanelTabSearch(window.location.search, nextTab);
    window.history.replaceState(null, '', `${window.location.pathname}${nextSearch}`);
  };

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
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <header className="flex flex-col gap-4 rounded-2xl border border-border bg-card p-5 shadow-sm sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-11 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm">
              <UserRound className="size-5" />
            </div>
            <div>
              <h1 className="text-xl font-semibold tracking-tight">{t('userPanel.title')}</h1>
              <p className="text-sm text-muted-foreground">{t('userPanel.description')}</p>
            </div>
          </div>
          <div className="flex items-center gap-2 self-start sm:self-center">
            <LanguageToggle />
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

        <Tabs value={activeTab} onValueChange={handleTabChange} className="space-y-5">
          <TabsList className="grid w-full grid-cols-2 rounded-xl p-1">
            <TabsTrigger value="main">{t('userPanel.mainTab')}</TabsTrigger>
            <TabsTrigger value="requests" className="relative overflow-hidden">
              <MarqueeBackground
                show={activeRequestCount > 0}
                color="var(--color-success)"
                opacity={0.3}
              />
              <span className="relative z-10">{t('userPanel.requestsTab')}</span>
              <span className="relative z-10">
                <StreamingBadge count={activeRequestCount} color="var(--color-success)" />
              </span>
            </TabsTrigger>
          </TabsList>

          <TabsContent value="main" className="space-y-5">
            <section className="grid gap-5 lg:grid-cols-[1fr_1fr]">
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
                    <div className="space-y-4">
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

                      <div className="relative">
                        <Input
                          readOnly
                          type={showKeyMasked ? 'password' : 'text'}
                          value={userPanelToken.tokenPrefix || 'maxx_••••'}
                          className="h-9 pr-9 font-mono text-xs"
                          aria-label={t('userPanel.myKey')}
                        />
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="absolute right-1 top-1/2 size-7 -translate-y-1/2"
                          aria-label={
                            showKeyMasked ? t('userPanel.showKey') : t('userPanel.hideKey')
                          }
                          onClick={() => setShowKeyMasked((prev) => !prev)}
                        >
                          {showKeyMasked ? (
                            <Eye className="size-3.5" />
                          ) : (
                            <EyeOff className="size-3.5" />
                          )}
                        </Button>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {t('userPanel.existingKeyHint')}
                      </p>
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
                    {t('userPanel.usageStats')}
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-4 p-5">
                  {userPanelToken ? (
                    <>
                      <div className="grid gap-3">
                        <div className="rounded-lg border border-border bg-muted/25 px-3 py-2.5">
                          <p className="text-xs text-muted-foreground">{t('userPanel.useCount')}</p>
                          <p className="mt-1 text-xl font-bold tabular-nums text-foreground">
                            {formatNumber(userPanelToken.useCount)}
                          </p>
                        </div>
                        <div className="rounded-lg border border-border bg-muted/25 px-3 py-2.5">
                          <p className="text-xs text-muted-foreground">{t('userPanel.lastUsed')}</p>
                          <p className="mt-1 font-mono text-sm tabular-nums text-foreground">
                            {formatDateTime(userPanelToken.lastUsedAt)}
                          </p>
                        </div>
                        <div className="rounded-lg border border-border bg-muted/25 px-3 py-2.5">
                          <p className="text-xs text-muted-foreground">
                            {t('userPanel.expiresAt')}
                          </p>
                          <p className="mt-1 font-mono text-sm tabular-nums text-foreground">
                            {formatDateTime(userPanelToken.expiresAt)}
                          </p>
                        </div>
                      </div>
                      <div className="grid gap-3 border-t border-border pt-4 text-sm text-muted-foreground">
                        <div className="flex items-center justify-between gap-3">
                          <span>{t('userPanel.version')}</span>
                          <span className="font-mono text-foreground">
                            {proxyStatus?.version || '—'}
                          </span>
                        </div>
                      </div>
                    </>
                  ) : (
                    <div className="flex flex-col items-center justify-center py-8 text-center">
                      <Server className="size-9 text-muted-foreground" />
                      <p className="mt-3 text-sm text-muted-foreground">
                        {t('userPanel.noUsageData')}
                      </p>
                    </div>
                  )}
                </CardContent>
              </Card>
            </section>

            <Card className="border-border bg-card shadow-sm">
              <CardHeader className="border-b border-border">
                <CardTitle className="flex items-center gap-2 text-base font-medium">
                  <Server className="size-4 text-muted-foreground" />
                  {t('userPanel.apiAccess')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 p-5">
                <div className="rounded-xl border border-border bg-muted/25 p-4">
                  <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t('userPanel.baseURL')}
                  </p>
                  <div className="mt-3 space-y-2">
                    {endpointHints.map((endpoint) => (
                      <div
                        key={endpoint.id}
                        className="flex items-center gap-3 rounded-lg bg-background px-3 py-2 text-xs"
                      >
                        <span className="w-20 shrink-0 font-medium text-foreground">
                          {endpoint.label}
                        </span>
                        <code className="min-w-0 flex-1 break-all text-muted-foreground">
                          {endpoint.url || '—'}
                        </code>
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-8 shrink-0 gap-2"
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
                  <pre className="mt-3 whitespace-pre-wrap break-words rounded-lg bg-background px-3 py-2 text-xs text-muted-foreground">
                    <code>{curlExample}</code>
                  </pre>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="requests">
            <UserPanelRequestsTab />
          </TabsContent>
        </Tabs>

        <footer className="flex items-center justify-center gap-2 text-xs text-muted-foreground">
          <Clock3 className="size-3.5" />
          <span>{t('userPanel.securityHint')}</span>
        </footer>
      </div>
    </main>
  );
}
