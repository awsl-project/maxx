import { useMemo, useState } from 'react';
import {
  BookOpen,
  Code,
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
} from '@/components/ui';
import { ClientIcon } from '@/components/icons/client-icons';
import { PageHeader } from '@/components/layout/page-header';
import { useProxyStatus, useProviders, useRoutes, useSettings } from '@/hooks/queries';
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

interface QuickstartBundle {
  primaryLabel: string;
  primaryCode: string;
  secondaryLabel?: string;
  secondaryCode?: string;
  verifyCode: string;
}

const MAXX_TOKEN_PATTERN = /^maxx_[A-Za-z0-9_-]{8,}$/;

function buildQuickstartBundle(params: {
  client: QuickstartClient;
  token: string;
  baseUrl: string;
  projectSlug: string;
}): QuickstartBundle {
  const token = params.token.trim() || 'maxx_your_token_here';
  const projectSlug = params.projectSlug.trim();
  const projectPrefix = projectSlug ? `/project/${projectSlug}` : '';

  switch (params.client) {
    case 'claude':
      return {
        primaryLabel: 'settings.json',
        primaryCode: `{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "${token}",
    "ANTHROPIC_BASE_URL": "${params.baseUrl}"
  }
}`,
        verifyCode: `ANTHROPIC_BASE_URL="${params.baseUrl}" ANTHROPIC_AUTH_TOKEN="${token}" claude`,
      };
    case 'openai':
      return {
        primaryLabel: '.env',
        primaryCode: `OPENAI_BASE_URL=${params.baseUrl}${projectPrefix}/v1
OPENAI_API_KEY=${token}`,
        verifyCode: `curl -X POST ${params.baseUrl}${projectPrefix}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${token}" \\
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}'`,
      };
    case 'codex': {
      const bundle = buildCodexConfigBundle({
        token,
        baseUrl: `${params.baseUrl}${projectPrefix || ''}`,
      });
      return {
        primaryLabel: 'config.toml',
        primaryCode: bundle.configToml,
        secondaryLabel: 'auth.json',
        secondaryCode: bundle.authJson,
        verifyCode: 'codex',
      };
    }
    case 'gemini':
      return {
        primaryLabel: 'curl',
        primaryCode: `curl -X POST ${params.baseUrl}${projectPrefix}/v1beta/models/gemini-pro:generateContent \\
  -H "Content-Type: application/json" \\
  -H "x-goog-api-key: ${token}" \\
  -d '{"contents":[{"parts":[{"text":"hello"}]}]}'`,
        verifyCode: `curl -X POST ${params.baseUrl}${projectPrefix}/v1beta/models/gemini-pro:generateContent \\
  -H "Content-Type: application/json" \\
  -H "x-goog-api-key: ${token}" \\
  -d '{"contents":[{"parts":[{"text":"diagnose"}]}]}'`,
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
  const [quickstartClient, setQuickstartClient] = useState<QuickstartClient>('claude');
  const [quickstartToken, setQuickstartToken] = useState('');
  const [quickstartProjectSlug, setQuickstartProjectSlug] = useState('');
  const { data: proxyStatus } = useProxyStatus();
  const { data: settings } = useSettings();
  const { data: providers } = useProviders();
  const { data: routes } = useRoutes();
  const baseUrl = buildProxyBaseUrl(proxyStatus);
  const codexTemplate = useMemo(
    () => buildCodexConfigBundle({ token: 'maxx_your_token_here', baseUrl }),
    [baseUrl],
  );
  const quickstartBundle = useMemo(
    () =>
      buildQuickstartBundle({
        client: quickstartClient,
        token: quickstartToken,
        baseUrl,
        projectSlug: quickstartProjectSlug,
      }),
    [quickstartClient, quickstartToken, baseUrl, quickstartProjectSlug],
  );
  const tokenAuthEnabled = settings?.api_token_auth_enabled === 'true';
  const tokenFormatOk = !tokenAuthEnabled || MAXX_TOKEN_PATTERN.test(quickstartToken.trim());
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
        detail:
          tokenAuthEnabled
            ? quickstartToken.trim() === ''
              ? t('documentation.diagnosticTokenRequired')
              : t('documentation.diagnosticTokenProvided')
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
      quickstartToken,
    ],
  );

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedCode(id);
    setTimeout(() => setCopiedCode(null), 2000);
  };

  const handleQuickstartClientChange = (value: string) => {
    if (value === 'claude' || value === 'openai' || value === 'codex' || value === 'gemini') {
      setQuickstartClient(value);
    }
  };

  return (
    <div className="space-y-6">
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
            className="w-full"
          >
            <TabsList className="grid w-full grid-cols-4 h-10 p-1 bg-muted">
              <TabsTrigger value="claude">Claude</TabsTrigger>
              <TabsTrigger value="openai">OpenAI</TabsTrigger>
              <TabsTrigger value="codex">Codex</TabsTrigger>
              <TabsTrigger value="gemini">Gemini</TabsTrigger>
            </TabsList>
          </Tabs>

          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-2">
              <label className="text-xs font-semibold">{t('documentation.tokenInputLabel')}</label>
              <Input
                value={quickstartToken}
                onChange={(event) => setQuickstartToken(event.target.value)}
                placeholder={t('documentation.tokenInputPlaceholder')}
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-semibold">
                {t('documentation.projectSlugLabel')}
              </label>
              <Input
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
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold">{t('documentation.wizardGenerated')}</h3>
              <Badge variant="outline">{quickstartBundle.primaryLabel}</Badge>
            </div>
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
              <CodeBlock
                code={quickstartBundle.secondaryCode}
                id={`quickstart-${quickstartClient}-secondary`}
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
        </CardContent>
      </Card>

      <Card className="border-border bg-card">
        <CardContent className="space-y-4 pt-6">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <Stethoscope className="h-4 w-4 text-cyan-500" />
              <h2 className="text-base font-semibold">{t('documentation.diagnosticsTitle')}</h2>
            </div>
            <p className="text-xs text-muted-foreground">{t('documentation.diagnosticsDesc')}</p>
          </div>

          <div className="space-y-2">
            {diagnostics.map((item) => (
              <div
                key={item.key}
                className="flex items-start justify-between gap-4 rounded-md border border-border/70 p-3"
              >
                <div className="space-y-1">
                  <p className="text-sm font-medium">{item.label}</p>
                  <p className="text-xs text-muted-foreground">{item.ok ? item.detail : item.hint}</p>
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

      <Card className="border-border bg-card">
        <CardContent className="space-y-6 pt-6">
          <Tabs defaultValue="claude" className="w-full">
          <TabsList className="grid w-full grid-cols-4 h-12 p-1 bg-muted">
            <TabsTrigger value="claude">
              <div className="flex items-center justify-center gap-2">
                <ClientIcon type="claude" size={16} className="shrink-0" />
                <span className="leading-none">Claude Code</span>
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
                <span className="leading-none">Codex CLI</span>
              </div>
            </TabsTrigger>
            <TabsTrigger value="gemini">
              <div className="flex items-center justify-center gap-2">
                <ClientIcon type="gemini" size={16} className="shrink-0" />
                <span className="leading-none">Gemini</span>
              </div>
            </TabsTrigger>
          </TabsList>

          {/* Claude Code CLI */}
          <TabsContent value="claude" className="space-y-4 mt-6">
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Terminal className="h-4 w-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold">{t('documentation.claudeConfig')}</h3>
              </div>
              <p className="text-xs text-muted-foreground">{t('documentation.claudeConfigDesc')}</p>
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.settingsJson')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.settingsJsonDesc')}</p>
              <CodeBlock
                code={`{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key-here",
    "ANTHROPIC_BASE_URL": "${baseUrl}"
  }
}`}
                id="claude-settings"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.shellFunction')}</h3>
              <p className="text-xs text-muted-foreground">
                {t('documentation.shellFunctionDesc')}
              </p>
              <CodeBlock
                code={`claude_maxx() {
    export ANTHROPIC_BASE_URL="${baseUrl}"
    export ANTHROPIC_AUTH_TOKEN="your-api-key-here"
    claude "$@"
}`}
                id="claude-shell"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            {/* Token Authentication for Claude Code */}
            <div className="pt-4 border-t border-border space-y-3">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-amber-500" />
                <h3 className="text-sm font-semibold">{t('documentation.tokenAuthentication')}</h3>
              </div>

              <div className="p-4 rounded-md bg-muted/30 border border-border space-y-2">
                <p className="text-sm font-medium">{t('documentation.tokenEnabled')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.tokenEnabledDesc')}
                </p>
                <div className="text-xs text-muted-foreground space-y-1 pl-2">
                  <p>{t('documentation.claudeTokenEnabledNote')}</p>
                </div>
              </div>

              <div className="p-4 rounded-md bg-muted/30 border border-border space-y-2">
                <p className="text-sm font-medium">{t('documentation.tokenDisabled')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.claudeTokenDisabledNote')}
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
          </TabsContent>

          {/* OpenAI API */}
          <TabsContent value="openai" className="space-y-4 mt-6">
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Code className="h-4 w-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold">{t('documentation.apiEndpoint')}</h3>
              </div>
              <CodeBlock
                code={`POST ${baseUrl}/v1/chat/completions`}
                id="openai-endpoint"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.requestExample')}</h3>
              <CodeBlock
                code={`curl -X POST ${baseUrl}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer maxx_your_token_here" \\
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello, GPT!"}
    ]
  }'`}
                id="openai-example"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.projectProxy')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.projectProxyDesc')}</p>
              <CodeBlock
                code={`POST ${baseUrl}/project/{project-slug}/v1/chat/completions`}
                id="openai-project"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            {/* Token Authentication */}
            <div className="pt-4 border-t border-border space-y-3">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-amber-500" />
                <h3 className="text-sm font-semibold">{t('documentation.tokenAuthentication')}</h3>
              </div>

              <div className="p-4 rounded-md bg-muted/30 border border-border space-y-2">
                <p className="text-sm font-medium">{t('documentation.tokenEnabled')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.tokenEnabledDesc')}
                </p>
                <div className="text-xs text-muted-foreground space-y-1 pl-2">
                  <p>
                    <strong>{t('documentation.requestHeader')}:</strong>{' '}
                    <code className="text-xs bg-muted px-1 py-0.5 rounded">
                      Authorization: Bearer maxx_your_token_here
                    </code>
                  </p>
                </div>
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
          </TabsContent>

          {/* Codex CLI */}
          <TabsContent value="codex" className="space-y-4 mt-6">
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Terminal className="h-4 w-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold">{t('documentation.codexConfig')}</h3>
              </div>
              <p className="text-xs text-muted-foreground">{t('documentation.codexConfigDesc')}</p>
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.configToml')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.configTomlDesc')}</p>
              <CodeBlock
                code={codexTemplate.configToml}
                id="codex-config"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.authJson')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.authJsonDesc')}</p>
              <CodeBlock
                code={codexTemplate.authJson}
                id="codex-auth"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.usage')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.codexUsageDesc')}</p>
              <CodeBlock
                code={`codex`}
                id="codex-usage"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            {/* Token Authentication for Codex CLI */}
            <div className="pt-4 border-t border-border space-y-3">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-amber-500" />
                <h3 className="text-sm font-semibold">{t('documentation.tokenAuthentication')}</h3>
              </div>

              <div className="p-4 rounded-md bg-muted/30 border border-border space-y-2">
                <p className="text-sm font-medium">{t('documentation.tokenEnabled')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.tokenEnabledDesc')}
                </p>
                <div className="text-xs text-muted-foreground space-y-1 pl-2">
                  <p>{t('documentation.codexTokenEnabledNote')}</p>
                </div>
              </div>

              <div className="p-4 rounded-md bg-muted/30 border border-border space-y-2">
                <p className="text-sm font-medium">{t('documentation.tokenDisabled')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.codexTokenDisabledNote')}
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
          </TabsContent>

          {/* Gemini API */}
          <TabsContent value="gemini" className="space-y-4 mt-6">
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Code className="h-4 w-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold">{t('documentation.apiEndpoint')}</h3>
              </div>
              <CodeBlock
                code={`POST ${baseUrl}/v1beta/models/{model}:generateContent`}
                id="gemini-endpoint"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.requestExample')}</h3>
              <CodeBlock
                code={`curl -X POST ${baseUrl}/v1beta/models/gemini-pro:generateContent \\
  -H "Content-Type: application/json" \\
  -H "x-goog-api-key: maxx_your_token_here" \\
  -d '{
    "contents": [{
      "parts": [{"text": "Hello, Gemini!"}]
    }]
  }'`}
                id="gemini-example"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.projectProxy')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.projectProxyDesc')}</p>
              <CodeBlock
                code={`POST ${baseUrl}/{project-slug}/v1beta/models/{model}:generateContent`}
                id="gemini-project"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            {/* Token Authentication */}
            <div className="pt-4 border-t border-border space-y-3">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-amber-500" />
                <h3 className="text-sm font-semibold">{t('documentation.tokenAuthentication')}</h3>
              </div>

              <div className="p-4 rounded-md bg-muted/30 border border-border space-y-2">
                <p className="text-sm font-medium">{t('documentation.tokenEnabled')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('documentation.tokenEnabledDesc')}
                </p>
                <div className="text-xs text-muted-foreground space-y-1 pl-2">
                  <p>
                    <strong>{t('documentation.requestHeader')}:</strong>{' '}
                    <code className="text-xs bg-muted px-1 py-0.5 rounded">
                      x-goog-api-key: maxx_your_token_here
                    </code>
                  </p>
                </div>
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
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
    </div>
  );
}

export default DocumentationPage;
