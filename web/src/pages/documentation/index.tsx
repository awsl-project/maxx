import { useState } from 'react';
import { BookOpen, Code, Copy, Check, AlertTriangle, Terminal } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  CardContent,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from '@/components/ui';
import { ClientIcon } from '@/components/icons/client-icons';

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

export function DocumentationPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-card shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent/10 rounded-lg">
            <BookOpen size={20} className="text-accent" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-foreground leading-tight">
              {t('documentation.title')}
            </h2>
            <p className="text-xs text-muted-foreground">{t('documentation.description')}</p>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
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

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedCode(id);
    setTimeout(() => setCopiedCode(null), 2000);
  };

  return (
    <Card className="border-border bg-card">
      <CardContent className="space-y-6 pt-6">
        <Tabs defaultValue="claude" className="w-full">
          <TabsList className="grid w-full grid-cols-4 h-12 p-1 bg-muted">
            <TabsTrigger
              value="claude"
              className="data-active:!bg-background data-active:!shadow-lg data-active:!border-2 data-active:!border-border font-semibold h-full"
            >
              <div className="flex items-center justify-center gap-2">
                <ClientIcon type="claude" size={16} className="shrink-0" />
                <span className="leading-none">Claude Code</span>
              </div>
            </TabsTrigger>
            <TabsTrigger
              value="openai"
              className="data-active:!bg-background data-active:!shadow-lg data-active:!border-2 data-active:!border-border font-semibold h-full"
            >
              <div className="flex items-center justify-center gap-2">
                <ClientIcon type="openai" size={16} className="shrink-0" />
                <span className="leading-none">OpenAI</span>
              </div>
            </TabsTrigger>
            <TabsTrigger
              value="codex"
              className="data-active:!bg-background data-active:!shadow-lg data-active:!border-2 data-active:!border-border font-semibold h-full"
            >
              <div className="flex items-center justify-center gap-2">
                <ClientIcon type="codex" size={16} className="shrink-0" />
                <span className="leading-none">Codex CLI</span>
              </div>
            </TabsTrigger>
            <TabsTrigger
              value="gemini"
              className="data-active:!bg-background data-active:!shadow-lg data-active:!border-2 data-active:!border-border font-semibold h-full"
            >
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
    "ANTHROPIC_BASE_URL": "http://localhost:9880"
  }
}`}
                id="claude-settings"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.shellFunction')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.shellFunctionDesc')}</p>
              <CodeBlock
                code={`claude_maxx() {
    export ANTHROPIC_BASE_URL="http://localhost:9880"
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
                <p className="text-xs text-muted-foreground">{t('documentation.tokenEnabledDesc')}</p>
                <div className="text-xs text-muted-foreground space-y-1 pl-2">
                  <p>
                    {t('documentation.claudeTokenEnabledNote')}
                  </p>
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
                code="POST http://localhost:9880/v1/chat/completions"
                id="openai-endpoint"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.requestExample')}</h3>
              <CodeBlock
                code={`curl -X POST http://localhost:9880/v1/chat/completions \\
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
                code="POST http://localhost:9880/project/{project-slug}/v1/chat/completions"
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
                <p className="text-xs text-muted-foreground">{t('documentation.tokenEnabledDesc')}</p>
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
                code={`# 可选：设置为默认 provider
model_provider = "maxx"

[model_providers.maxx]
name = "maxx"
base_url = "http://localhost:9880"
wire_api = "responses"
request_max_retries = 4
stream_max_retries = 10
stream_idle_timeout_ms = 300000`}
                id="codex-config"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.authJson')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.authJsonDesc')}</p>
              <CodeBlock
                code={`{
  "maxx": {
    "api_key": "maxx_your_token_here"
  }
}`}
                id="codex-auth"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.usage')}</h3>
              <p className="text-xs text-muted-foreground">{t('documentation.codexUsageDesc')}</p>
              <CodeBlock
                code={`# 使用 --provider 参数指定
codex --provider maxx

# 或者设置为默认 provider 后直接使用
codex`}
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
                <p className="text-xs text-muted-foreground">{t('documentation.tokenEnabledDesc')}</p>
                <div className="text-xs text-muted-foreground space-y-1 pl-2">
                  <p>
                    {t('documentation.codexTokenEnabledNote')}
                  </p>
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
                code="POST http://localhost:9880/v1beta/models/{model}:generateContent"
                id="gemini-endpoint"
                copiedCode={copiedCode}
                onCopy={copyToClipboard}
              />
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">{t('documentation.requestExample')}</h3>
              <CodeBlock
                code={`curl -X POST http://localhost:9880/v1beta/models/gemini-pro:generateContent \\
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
                code="POST http://localhost:9880/{project-slug}/v1beta/models/{model}:generateContent"
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
                <p className="text-xs text-muted-foreground">{t('documentation.tokenEnabledDesc')}</p>
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
  );
}

export default DocumentationPage;
