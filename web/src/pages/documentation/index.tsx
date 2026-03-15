import { useMemo, useState } from 'react';
import {
  BookOpen,
  Copy,
  Check,
  AlertTriangle,
  Terminal,
  Rocket,
  Stethoscope,
  CircleCheck,
  CircleAlert,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  CardContent,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Input,
  Badge,
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui';
import { ClientIcon } from '@/components/icons/client-icons';
import { PageHeader } from '@/components/layout/page-header';
import {
  useProxyStatus,
  useProviders,
  useRoutes,
  useSettings,
  useAPITokens,
} from '@/hooks/queries';
import { buildCodexConfigBundle, buildProxyBaseUrl } from '@/lib/codex-config';

interface CodeBlockProps {
  code: string;
  id: string;
  copiedCode: string | null;
  onCopy: (text: string, id: string) => void;
}

function CodeBlock({ code, id, copiedCode, onCopy }: CodeBlockProps) {
  return (
    <div className="relative group">
      <pre className="bg-muted/50 border border-border rounded-md p-4 overflow-x-auto text-xs font-mono">
        <code>{code}</code>
      </pre>
      <button
        onClick={() => onCopy(code, id)}
        className="absolute top-2 right-2 p-2 rounded-md bg-background/80 border border-border opacity-0 group-hover:opacity-100 transition-opacity hover:bg-muted"
      >
        {copiedCode === id ? (
          <Check className="h-3 w-3 text-green-500" />
        ) : (
          <Copy className="h-3 w-3" />
        )}
      </button>
    </div>
  );
}

type QuickstartClient = 'claude' | 'openai' | 'codex' | 'gemini';
type DocumentationPageTab = 'quickstart' | 'diagnostics';

interface QuickstartBundle {
  primaryLabel: string;
  primaryCode: string;
  secondaryLabel?: string;
  secondaryCode?: string;
  verifyCode: string;
  oneliner: string;
}


function shellQuote(value: string): string {
  return `'${value.replace(/'/g, "'\\''")}'`;
}

function isQuickstartClient(value: string): value is QuickstartClient {
  return value === 'claude' || value === 'openai' || value === 'codex' || value === 'gemini';
}

function isDocumentationPageTab(value: string): value is DocumentationPageTab {
  return value === 'quickstart' || value === 'diagnostics';
}

function buildQuickstartBundle(params: {
  client: QuickstartClient;
  token: string;
  baseUrl: string;
  projectSlug: string;
}): QuickstartBundle {
  const token = params.token.trim() || 'maxx_your_token_here';
  const projectSlug = params.projectSlug.trim();
  const projectPrefix = projectSlug ? `/project/${projectSlug}` : '';

  const baseUrl = params.baseUrl;

  switch (params.client) {
    case 'claude': {
      const settingsJson = JSON.stringify({ env: { ANTHROPIC_AUTH_TOKEN: token, ANTHROPIC_BASE_URL: baseUrl } });
      return {
        primaryLabel: 'settings.json',
        primaryCode: `{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "${token}",
    "ANTHROPIC_BASE_URL": "${baseUrl}"
  }
}`,
        verifyCode: `ANTHROPIC_BASE_URL="${baseUrl}" ANTHROPIC_AUTH_TOKEN="${token}" claude`,
        oneliner: `mkdir -p ~/.claude && printf '%s\\n' ${shellQuote(settingsJson)} > ~/.claude/settings.json`,
      };
    }
    case 'openai':
      return {
        primaryLabel: '.env',
        primaryCode: `OPENAI_BASE_URL=${baseUrl}${projectPrefix}/v1
OPENAI_API_KEY=${token}`,
        verifyCode: `curl -X POST ${baseUrl}${projectPrefix}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${token}" \\
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}'`,
        oneliner: `printf '%s\\n' ${shellQuote(`OPENAI_BASE_URL=${baseUrl}${projectPrefix}/v1\nOPENAI_API_KEY=${token}`)} > .env`,
      };
    case 'codex': {
      const codexBaseUrl = `${baseUrl}${projectPrefix || ''}`;
      const bundle = buildCodexConfigBundle({ token, baseUrl: codexBaseUrl });
      const authJson = JSON.stringify({ OPENAI_API_KEY: token });
      return {
        primaryLabel: 'config.toml',
        primaryCode: bundle.configToml,
        secondaryLabel: 'auth.json',
        secondaryCode: bundle.authJson,
        verifyCode: 'codex',
        oneliner: `mkdir -p ~/.codex && printf '%s\\n' ${shellQuote(bundle.configToml)} > ~/.codex/config.toml && printf '%s\\n' ${shellQuote(authJson)} > ~/.codex/auth.json`,
      };
    }
    case 'gemini':
      return {
        primaryLabel: 'curl',
        primaryCode: `curl -X POST ${baseUrl}${projectPrefix}/v1beta/models/gemini-pro:generateContent \\
  -H "Content-Type: application/json" \\
  -H "x-goog-api-key: ${token}" \\
  -d '{"contents":[{"parts":[{"text":"hello"}]}]}'`,
        verifyCode: `curl -X POST ${baseUrl}${projectPrefix}/v1beta/models/gemini-pro:generateContent \\
  -H "Content-Type: application/json" \\
  -H "x-goog-api-key: ${token}" \\
  -d '{"contents":[{"parts":[{"text":"diagnose"}]}]}'`,
        oneliner: `curl -X POST ${baseUrl}${projectPrefix}/v1beta/models/gemini-pro:generateContent -H "Content-Type: application/json" -H "x-goog-api-key: ${token}" -d '{"contents":[{"parts":[{"text":"hello"}]}]}'`,
      };
  }
}

export function DocumentationPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={BookOpen}
        iconClassName="text-blue-500"
        title={t('documentation.title')}
        description={t('documentation.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="max-w-7xl mx-auto">
          <DocumentationSection />
        </div>
      </div>
    </div>
  );
}

function DocumentationSection() {
  const { t } = useTranslation();
  const [copiedCode, setCopiedCode] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<DocumentationPageTab>('quickstart');
  const [quickstartClient, setQuickstartClient] = useState<QuickstartClient>('claude');
  const [selectedTokenId, setSelectedTokenId] = useState<string | null>(null);
  const [quickstartTokenInput, setQuickstartTokenInput] = useState('');
  const [quickstartProjectSlug, setQuickstartProjectSlug] = useState('');
  const { data: proxyStatus } = useProxyStatus();
  const { data: settings } = useSettings();
  const { data: providers } = useProviders();
  const { data: routes } = useRoutes();
  const { data: apiTokens = [] } = useAPITokens();
  const baseUrl = buildProxyBaseUrl(proxyStatus);
  const tokenAuthEnabled = settings?.api_token_auth_enabled === 'true';
  const enabledTokens = apiTokens.filter(
    (t) => t.isEnabled && (!t.expiresAt || new Date(t.expiresAt) > new Date()),
  );

  const activeTokenId = useMemo(() => {
    if (enabledTokens.length === 0) return null;
    if (selectedTokenId !== null && enabledTokens.some((t) => String(t.id) === selectedTokenId)) {
      return selectedTokenId;
    }
    return String(enabledTokens[0].id);
  }, [selectedTokenId, enabledTokens]);

  // Use manually pasted token if provided; otherwise fall back to empty
  // (buildQuickstartBundle will use "maxx_your_token_here" placeholder).
  const effectiveToken = quickstartTokenInput.trim();

  const quickstartBundle = useMemo(
    () =>
      buildQuickstartBundle({
        client: quickstartClient,
        token: effectiveToken,
        baseUrl,
        projectSlug: quickstartProjectSlug,
      }),
    [quickstartClient, effectiveToken, baseUrl, quickstartProjectSlug],
  );

  const MAXX_TOKEN_PATTERN = /^maxx_[A-Za-z0-9_-]{8,}$/;
  const tokenFormatOk =
    !tokenAuthEnabled || MAXX_TOKEN_PATTERN.test(quickstartTokenInput.trim());

  const diagnostics = useMemo(
    () => [
      {
        key: 'proxy-running',
        label: t('documentation.diagnosticProxyRunning'),
        ok: !!proxyStatus?.running,
        hint: t('documentation.diagnosticProxyRunningHint'),
        detail: proxyStatus?.running
          ? `${proxyStatus.address || 'localhost'}:${proxyStatus.port || 9880}`
          : '',
      },
      {
        key: 'token-auth',
        label: t('documentation.diagnosticTokenAuth'),
        ok: settings?.api_token_auth_enabled !== undefined,
        hint: t('documentation.diagnosticTokenAuthHint'),
        detail:
          settings?.api_token_auth_enabled !== undefined
            ? tokenAuthEnabled
              ? t('common.enabled')
              : t('common.disabled')
            : '',
      },
      {
        key: 'provider-ready',
        label: t('documentation.diagnosticProviderReady'),
        ok: (providers?.length || 0) > 0,
        hint: t('documentation.diagnosticProviderReadyHint'),
        detail: t('documentation.diagnosticCount', { count: providers?.length || 0 }),
      },
      {
        key: 'route-ready',
        label: t('documentation.diagnosticRouteReady'),
        ok: (routes?.filter((route) => route.isEnabled).length || 0) > 0,
        hint: t('documentation.diagnosticRouteReadyHint'),
        detail: t('documentation.diagnosticCount', {
          count: routes?.filter((route) => route.isEnabled).length || 0,
        }),
      },
      {
        key: 'token-format',
        label: t('documentation.diagnosticTokenFormat'),
        ok: tokenFormatOk,
        hint: t('documentation.diagnosticTokenFormatHint'),
        detail: tokenAuthEnabled
          ? quickstartTokenInput.trim()
            ? t('documentation.diagnosticTokenProvided')
            : t('documentation.diagnosticTokenRequired')
          : t('documentation.diagnosticTokenOptional'),
      },
    ],
    [
      t,
      proxyStatus?.running,
      proxyStatus?.address,
      proxyStatus?.port,
      settings?.api_token_auth_enabled,
      tokenAuthEnabled,
      providers,
      routes,
      tokenFormatOk,
      quickstartTokenInput,
    ],
  );

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedCode(id);
    setTimeout(() => setCopiedCode(null), 2000);
  };

  const handleDocumentationTabChange = (value: string) => {
    if (isDocumentationPageTab(value)) {
      setActiveTab(value);
    }
  };

  const handleQuickstartClientChange = (value: string) => {
    if (isQuickstartClient(value)) {
      setQuickstartClient(value);
    }
  };

  const documentationTabs = [
    {
      value: 'quickstart' as const,
      icon: Rocket,
      iconClassName: 'text-emerald-500',
      label: t('documentation.pageTabQuickStart'),
      description: t('documentation.pageTabQuickStartDesc'),
    },
    {
      value: 'diagnostics' as const,
      icon: Stethoscope,
      iconClassName: 'text-cyan-500',
      label: t('documentation.pageTabDiagnostics'),
      description: t('documentation.pageTabDiagnosticsDesc'),
    },
  ];

  return (
    <Tabs value={activeTab} onValueChange={handleDocumentationTabChange} className="w-full">
      <TabsList
        variant="line"
        data-testid="documentation-page-tabs"
        className="grid w-full grid-cols-2 gap-3 bg-transparent p-0 group-data-horizontal/tabs:!h-auto"
      >
        {documentationTabs.map((tab) => {
          const Icon = tab.icon;

          return (
            <TabsTrigger
              key={tab.value}
              value={tab.value}
              data-testid={`documentation-page-tab-${tab.value}`}
              className="!h-auto min-h-[96px] min-w-0 whitespace-normal flex-col items-start justify-start gap-3 rounded-xl border border-border/70 bg-card px-4 py-4 text-left shadow-none after:hidden data-active:border-primary/30 data-active:bg-primary/5 data-active:shadow-none"
            >
              <Icon className={`h-4 w-4 ${tab.iconClassName}`} />
              <div className="min-w-0 w-full space-y-1">
                <p className="text-sm font-semibold text-foreground break-words">{tab.label}</p>
                <p className="text-xs leading-5 text-muted-foreground whitespace-normal break-words">
                  {tab.description}
                </p>
              </div>
            </TabsTrigger>
          );
        })}
      </TabsList>

      <TabsContent value="quickstart" data-testid="documentation-quickstart-content" className="mt-6">
        <Card className="border-border bg-card">
          <CardContent className="space-y-5 pt-6">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Rocket className="h-4 w-4 text-emerald-500" />
                <h2 className="text-base font-semibold">{t('documentation.quickStartTitle')}</h2>
              </div>
              <p className="text-xs text-muted-foreground">{t('documentation.quickStartDesc')}</p>
            </div>

            <div className="grid gap-3 md:grid-cols-3">
              <div className="rounded-md border border-border/70 bg-muted/20 p-3">
                <p className="text-[11px] font-medium text-muted-foreground uppercase">
                  {t('documentation.quickStartStepClient')}
                </p>
              </div>
              <div className="rounded-md border border-border/70 bg-muted/20 p-3">
                <p className="text-[11px] font-medium text-muted-foreground uppercase">
                  {t('documentation.quickStartStepToken')}
                </p>
              </div>
              <div className="rounded-md border border-border/70 bg-muted/20 p-3">
                <p className="text-[11px] font-medium text-muted-foreground uppercase">
                  {t('documentation.quickStartStepCopy')}
                </p>
              </div>
            </div>

            <Tabs
              value={quickstartClient}
              onValueChange={handleQuickstartClientChange}
              data-testid="documentation-quickstart-client-tabs"
              className="w-full"
            >
              <TabsList className="grid w-full grid-cols-4 h-12 p-1 bg-muted">
                <TabsTrigger value="claude">
                  <div className="flex items-center justify-center gap-2">
                    <ClientIcon type="claude" size={16} className="shrink-0" />
                    <span className="leading-none">Claude</span>
                  </div>
                </TabsTrigger>
                <TabsTrigger value="openai">
                  <div className="flex items-center justify-center gap-2">
                    <ClientIcon type="openai" size={16} className="shrink-0" />
                    <span className="leading-none">OpenAI</span>
                  </div>
                </TabsTrigger>
                <TabsTrigger value="codex">
                  <div className="flex items-center justify-center gap-2">
                    <ClientIcon type="codex" size={16} className="shrink-0" />
                    <span className="leading-none">Codex</span>
                  </div>
                </TabsTrigger>
                <TabsTrigger value="gemini">
                  <div className="flex items-center justify-center gap-2">
                    <ClientIcon type="gemini" size={16} className="shrink-0" />
                    <span className="leading-none">Gemini</span>
                  </div>
                </TabsTrigger>
              </TabsList>
            </Tabs>

            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-2">
                <label className="text-xs font-semibold">
                  {t('documentation.tokenInputLabel')}
                </label>
                {enabledTokens.length > 0 && activeTokenId !== null && (
                  <Select
                    value={activeTokenId}
                    onValueChange={(value) => {
                      if (value !== null) setSelectedTokenId(value);
                    }}
                  >
                    <SelectTrigger data-testid="documentation-quickstart-token-select" className="w-full">
                      <SelectValue>
                        {(() => {
                          const tk = enabledTokens.find((t) => String(t.id) === activeTokenId);
                          return tk ? `${tk.name} (${tk.tokenPrefix})` : '';
                        })()}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {enabledTokens.map((token) => (
                        <SelectItem key={token.id} value={String(token.id)}>
                          {token.name} ({token.tokenPrefix})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                <Input
                  data-testid="documentation-quickstart-token-input"
                  value={quickstartTokenInput}
                  onChange={(event) => setQuickstartTokenInput(event.target.value)}
                  placeholder={t('documentation.tokenInputPlaceholder')}
                />
              </div>
              <div className="space-y-2">
                <label className="text-xs font-semibold">
                  {t('documentation.projectSlugLabel')}
                </label>
                <Input
                  data-testid="documentation-quickstart-project-slug-input"
                  value={quickstartProjectSlug}
                  onChange={(event) => setQuickstartProjectSlug(event.target.value.trim())}
                  placeholder={t('documentation.projectSlugPlaceholder')}
                />
              </div>
            </div>

            {quickstartProjectSlug && (
              <p className="text-xs text-muted-foreground">
                {t('documentation.wizardProjectHint', { slug: quickstartProjectSlug })}
              </p>
            )}

            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Terminal className="h-4 w-4 text-emerald-500" />
                <h3 className="text-sm font-semibold">{t('documentation.onelinerTitle')}</h3>
              </div>
              <CodeBlock
                code={quickstartBundle.oneliner}
                id={`quickstart-${quickstartClient}-oneliner`}
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-semibold">{t('documentation.wizardGenerated')}</h3>
                <Badge variant="outline">{quickstartBundle.primaryLabel}</Badge>
              </div>
              {quickstartClient === 'claude' && (
                <p className="text-xs text-muted-foreground">
                  {t('documentation.settingsJsonDesc')}
                </p>
              )}
              {quickstartClient === 'codex' && (
                <p className="text-xs text-muted-foreground">
                  {t('documentation.configTomlDesc')}
                </p>
              )}
              <CodeBlock
                code={quickstartBundle.primaryCode}
                id={`quickstart-${quickstartClient}-primary`}
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            {quickstartBundle.secondaryCode && quickstartBundle.secondaryLabel && (
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-semibold">{t('documentation.wizardGenerated')}</h3>
                  <Badge variant="outline">{quickstartBundle.secondaryLabel}</Badge>
                </div>
                {quickstartClient === 'codex' && (
                  <p className="text-xs text-muted-foreground">
                    {t('documentation.authJsonDesc')}
                  </p>
                )}
                <CodeBlock
                  code={quickstartBundle.secondaryCode}
                  id={`quickstart-${quickstartClient}-secondary`}
                  copiedCode={copiedCode}
                  onCopy={copyToClipboard}
                />
              </div>
            )}

            {/* Claude: shell function alternative */}
            {quickstartClient === 'claude' && (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <Terminal className="h-4 w-4 text-muted-foreground" />
                  <h3 className="text-sm font-semibold">{t('documentation.shellFunction')}</h3>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.shellFunctionDesc')}
                </p>
                <CodeBlock
                  code={`claude_maxx() {
    export ANTHROPIC_BASE_URL="${baseUrl}"
    export ANTHROPIC_AUTH_TOKEN="${effectiveToken.trim() || 'maxx_your_token_here'}"
    claude "$@"
}`}
                  id="quickstart-claude-shell"
                  copiedCode={copiedCode}
                  onCopy={copyToClipboard}
                />
              </div>
            )}

            {/* OpenAI / Gemini: project proxy endpoint */}
            {(quickstartClient === 'openai' || quickstartClient === 'gemini') && (
              <div className="space-y-3">
                <h3 className="text-sm font-semibold">{t('documentation.projectProxy')}</h3>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.projectProxyDesc')}
                </p>
                <CodeBlock
                  code={
                    quickstartClient === 'openai'
                      ? `POST ${baseUrl}/project/{project-slug}/v1/chat/completions`
                      : `POST ${baseUrl}/project/{project-slug}/v1beta/models/{model}:generateContent`
                  }
                  id={`quickstart-${quickstartClient}-project-proxy`}
                  copiedCode={copiedCode}
                  onCopy={copyToClipboard}
                />
              </div>
            )}

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.wizardVerify')}</h3>
              <CodeBlock
                code={quickstartBundle.verifyCode}
                id={`quickstart-${quickstartClient}-verify`}
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            {/* Token Authentication (shared) */}
            <div className="pt-4 border-t border-border space-y-3">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-amber-500" />
                <h3 className="text-sm font-semibold">
                  {t('documentation.tokenAuthentication')}
                </h3>
              </div>

              <div className="p-4 rounded-md bg-muted/30 border border-border space-y-2">
                <p className="text-sm font-medium">{t('documentation.tokenEnabled')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.tokenEnabledDesc')}
                </p>
              </div>

              <div className="p-4 rounded-md bg-muted/30 border border-border space-y-2">
                <p className="text-sm font-medium">{t('documentation.tokenDisabled')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.tokenDisabledDesc')}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.tokenDisabledNote')}
                </p>
              </div>

              <div className="flex items-start gap-2 p-3 rounded-md bg-blue-500/10 border border-blue-500/20">
                <AlertTriangle className="h-4 w-4 text-blue-500 mt-0.5 shrink-0" />
                <div className="text-xs text-blue-600 dark:text-blue-400 space-y-1">
                  <p className="font-medium">{t('documentation.tokenManagement')}</p>
                  <p>{t('documentation.tokenManagementDesc')}</p>
                </div>
              </div>
            </div>

            <div className="flex flex-col gap-3 rounded-md border border-cyan-500/20 bg-cyan-500/5 p-4 md:flex-row md:items-center md:justify-between">
              <p className="text-xs leading-5 text-cyan-700 dark:text-cyan-300">
                {t('documentation.quickStartDiagnosticHint')}
              </p>
              <Button
                variant="outline"
                data-testid="documentation-open-diagnostics-button"
                onClick={() => setActiveTab('diagnostics')}
              >
                {t('documentation.quickStartDiagnosticAction')}
              </Button>
            </div>
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="diagnostics" data-testid="documentation-diagnostics-content" className="mt-6">
        <Card className="border-border bg-card">
          <CardContent className="space-y-4 pt-6">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Stethoscope className="h-4 w-4 text-cyan-500" />
                <h2 className="text-base font-semibold">{t('documentation.diagnosticsTitle')}</h2>
              </div>
              <p className="text-xs text-muted-foreground">{t('documentation.diagnosticsDesc')}</p>
            </div>

            <div data-testid="documentation-diagnostics-list" className="space-y-2">
              {diagnostics.map((item) => (
                <div
                  key={item.key}
                  data-testid={`documentation-diagnostic-${item.key}`}
                  className="flex items-start justify-between gap-4 rounded-md border border-border/70 p-3"
                >
                  <div className="space-y-1">
                    <p className="text-sm font-medium">{item.label}</p>
                    <p className="text-xs text-muted-foreground">
                      {item.ok ? item.detail : item.hint}
                    </p>
                  </div>
                  <Badge
                    variant="outline"
                    className={
                      item.ok
                        ? 'text-emerald-600 border-emerald-500/30 bg-emerald-500/5'
                        : 'text-amber-600 border-amber-500/30 bg-amber-500/5'
                    }
                  >
                    {item.ok ? (
                      <CircleCheck className="h-3 w-3 mr-1" />
                    ) : (
                      <CircleAlert className="h-3 w-3 mr-1" />
                    )}
                    {item.ok ? t('documentation.statusPass') : t('documentation.statusFail')}
                  </Badge>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </TabsContent>
    </Tabs>
  );
}

export default DocumentationPage;
