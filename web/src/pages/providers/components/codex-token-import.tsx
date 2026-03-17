import { useState, useEffect, useRef } from 'react';
import {
  Code2,
  ChevronLeft,
  Loader2,
  CheckCircle2,
  AlertCircle,
  Key,
  ExternalLink,
  Mail,
  ShieldCheck,
  Zap,
  Link,
  Copy,
  Check,
} from 'lucide-react';
import { getTransport } from '@/lib/transport';
import type {
  CodexTokenValidationResult,
  CreateProviderData,
  CodexOAuthResult,
} from '@/lib/transport';
import { CODEX_COLOR } from '../types';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { PageHeader } from '@/components/layout/page-header';
import { cn } from '@/lib/utils';
import { useProviderNavigation } from '../hooks/use-provider-navigation';
import { useCreateProvider } from '@/hooks/queries';
import { useTranslation } from 'react-i18next';
import { CLIProxyAPISwitch } from './cliproxyapi-switch';

type ImportMode = 'oauth' | 'token';
type OAuthStatus = 'idle' | 'waiting' | 'success' | 'error';

export function CodexTokenImport() {
  const { t } = useTranslation();
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();
  const [mode, setMode] = useState<ImportMode>('oauth');
  const [email, setEmail] = useState('');
  const [token, setToken] = useState('');
  const [validating, setValidating] = useState(false);
  const [creating, setCreating] = useState(false);
  const [validationResult, setValidationResult] = useState<CodexTokenValidationResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  // CLIProxyAPI 开关
  const [useCLIProxyAPI, setUseCLIProxyAPI] = useState(false);

  // OAuth state
  const [oauthStatus, setOAuthStatus] = useState<OAuthStatus>('idle');
  const [oauthState, setOAuthState] = useState<string | null>(null);
  const [oauthResult, setOAuthResult] = useState<CodexOAuthResult | null>(null);
  const oauthWindowRef = useRef<Window | null>(null);

  // Manual callback URL state
  const [callbackUrl, setCallbackUrl] = useState('');
  const [exchanging, setExchanging] = useState(false);
  const [copied, setCopied] = useState(false);
  const [popupClosed, setPopupClosed] = useState(false);

  // Subscribe to OAuth result messages via WebSocket
  useEffect(() => {
    const transport = getTransport();
    const unsubscribe = transport.subscribe<CodexOAuthResult>('codex_oauth_result', (result) => {
      // Only handle results that match our current OAuth state
      if (result.state === oauthState) {
        // Close the OAuth window if it's still open
        if (oauthWindowRef.current && !oauthWindowRef.current.closed) {
          oauthWindowRef.current.close();
        }

        if (result.success && result.refreshToken) {
          // OAuth succeeded, save result for user confirmation
          setOAuthStatus('success');
          setOAuthResult(result);
        } else {
          // OAuth failed
          setOAuthStatus('error');
          setError(result.error || t('providers.codexTokenImport.errors.oauthFailed'));
        }
      }
    });

    return () => unsubscribe();
  }, [oauthState, t]);

  // Parse callback URL and extract code/state
  const parseCallbackUrl = (url: string): { code: string; state: string } | null => {
    try {
      const urlObj = new URL(url);
      const code = urlObj.searchParams.get('code');
      const state = urlObj.searchParams.get('state');
      if (code && state) {
        return { code, state };
      }
      return null;
    } catch {
      return null;
    }
  };

  // Handle manual callback URL exchange
  const handleExchangeCallback = async () => {
    const parsed = parseCallbackUrl(callbackUrl.trim());
    if (!parsed) {
      setError(t('providers.codexTokenImport.errors.invalidCallbackUrl'));
      return;
    }

    if (parsed.state !== oauthState) {
      setError(t('providers.codexTokenImport.errors.stateMismatch'));
      return;
    }

    setExchanging(true);
    setError(null);

    try {
      const result = await getTransport().exchangeCodexOAuthCallback(parsed.code, parsed.state);
      if (result.success && result.refreshToken) {
        setOAuthStatus('success');
        setOAuthResult(result);
        // Close the OAuth window if it's still open
        if (oauthWindowRef.current && !oauthWindowRef.current.closed) {
          oauthWindowRef.current.close();
        }
      } else {
        setOAuthStatus('error');
        setError(result.error || t('providers.codexTokenImport.errors.oauthFailed'));
      }
    } catch (err) {
      setOAuthStatus('error');
      setError(
        err instanceof Error ? err.message : t('providers.codexTokenImport.errors.exchangeFailed'),
      );
    } finally {
      setExchanging(false);
    }
  };

  // OAuth auth URL (for copy functionality)
  const [authUrl, setAuthUrl] = useState<string | null>(null);
  const handleCopyAuthUrl = async () => {
    if (authUrl) {
      await navigator.clipboard.writeText(authUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  // Handle OAuth flow
  const handleOAuth = async () => {
    setOAuthStatus('waiting');
    setError(null);
    setCallbackUrl('');
    setPopupClosed(false);

    try {
      // Request OAuth URL from backend
      const { authURL, state } = await getTransport().startCodexOAuth();
      setOAuthState(state);
      setAuthUrl(authURL);

      // Open OAuth window
      const width = 600;
      const height = 700;
      const left = window.screenX + (window.outerWidth - width) / 2;
      const top = window.screenY + (window.outerHeight - height) / 2;

      oauthWindowRef.current = window.open(
        authURL,
        'Codex OAuth',
        `width=${width},height=${height},left=${left},top=${top},resizable=yes,scrollbars=yes`,
      );

      // Monitor window closure
      const checkWindowClosed = setInterval(() => {
        if (oauthWindowRef.current?.closed) {
          clearInterval(checkWindowClosed);
          // Window closed, but keep the state so user can still paste callback URL manually
          // Don't reset to idle - user can click Cancel if they want to restart
          setPopupClosed(true);
        }
      }, 500);
    } catch (err) {
      setOAuthStatus('error');
      setError(
        err instanceof Error
          ? err.message
          : t('providers.codexTokenImport.errors.startOAuthFailed'),
      );
    }
  };

  // Validate token
  const handleValidate = async () => {
    if (token.trim() === '') {
      setError(t('providers.codexTokenImport.errors.invalidRefreshToken'));
      return;
    }

    setValidating(true);
    setError(null);
    setValidationResult(null);

    try {
      const result = await getTransport().validateCodexToken(token.trim());
      setValidationResult(result);
      if (!result.valid) {
        setError(result.error || t('providers.codexTokenImport.errors.tokenValidationFailed'));
      }
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t('providers.codexTokenImport.errors.validationFailed'),
      );
    } finally {
      setValidating(false);
    }
  };

  // Create provider from OAuth result
  const handleCreateFromOAuth = async () => {
    if (!oauthResult?.refreshToken) {
      setError(t('providers.codexTokenImport.errors.oauthResultMissing'));
      return;
    }

    setCreating(true);
    setError(null);

    try {
      const providerData: CreateProviderData = {
        type: 'codex',
        name: oauthResult.email || 'Codex Account',
        config: {
          codex: {
            email: oauthResult.email || '',
            name: oauthResult.name,
            picture: oauthResult.picture,
            refreshToken: oauthResult.refreshToken,
            accessToken: oauthResult.accessToken,
            expiresAt: oauthResult.expiresAt,
            accountId: oauthResult.accountId,
            userId: oauthResult.userId,
            planType: oauthResult.planType,
            subscriptionStart: oauthResult.subscriptionStart,
            subscriptionEnd: oauthResult.subscriptionEnd,
            useCLIProxyAPI,
          },
        },
      };
      await createProvider.mutateAsync(providerData);
      goToProviders();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t('providers.codexTokenImport.errors.createFailed'),
      );
    } finally {
      setCreating(false);
    }
  };

  // Create provider from token validation
  const handleCreate = async () => {
    if (!validationResult?.valid) {
      setError(t('providers.codexTokenImport.errors.validateFirst'));
      return;
    }

    setCreating(true);
    setError(null);

    try {
      const finalEmail = validationResult.email || email.trim() || '';
      const providerData: CreateProviderData = {
        type: 'codex',
        name: finalEmail || 'Codex Account',
        config: {
          codex: {
            email: finalEmail,
            name: validationResult.name,
            picture: validationResult.picture,
            refreshToken: validationResult.refreshToken || token.trim(),
            accessToken: validationResult.accessToken,
            expiresAt: validationResult.expiresAt,
            accountId: validationResult.accountId,
            userId: validationResult.userId,
            planType: validationResult.planType,
            subscriptionStart: validationResult.subscriptionStart,
            subscriptionEnd: validationResult.subscriptionEnd,
            useCLIProxyAPI,
          },
        },
      };
      await createProvider.mutateAsync(providerData);
      goToProviders();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t('providers.codexTokenImport.errors.createFailed'),
      );
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="flex flex-col h-full bg-card">
      <PageHeader
        icon={<ChevronLeft className="cursor-pointer" onClick={goToSelectType} />}
        title={t('providers.codexTokenImport.title')}
      />

      <div className="flex-1 overflow-y-auto">
        <div className="container max-w-2xl mx-auto py-8 px-6 space-y-8">
          {/* Hero Section */}
          <div className="text-center space-y-2 mb-8">
            <div
              className="w-16 h-16 rounded-2xl flex items-center justify-center mx-auto mb-4 shadow-inner"
              style={{ backgroundColor: `${CODEX_COLOR}15` }}
            >
              <Code2 size={32} style={{ color: CODEX_COLOR }} />
            </div>
            <h1 className="text-2xl font-bold text-foreground">
              {t('providers.codexTokenImport.connectTitle')}
            </h1>
            <p className="text-muted-foreground mx-auto max-w-md">
              {t('providers.codexTokenImport.connectDescription')}
            </p>
          </div>

          {/* CLIProxyAPI Switch */}
          <CLIProxyAPISwitch checked={useCLIProxyAPI} onChange={setUseCLIProxyAPI} />

          {/* Mode Tabs */}
          <div className="flex gap-2 p-1 bg-muted rounded-lg">
            <button
              onClick={() => {
                setMode('oauth');
                setError(null);
              }}
              className={cn(
                'flex-1 flex items-center justify-center gap-2 py-2.5 px-4 rounded-md text-sm font-medium transition-all',
                mode === 'oauth'
                  ? 'bg-card text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground',
              )}
            >
              <Zap size={16} />
              {t('providers.codexTokenImport.oauthLogin')}
            </button>
            <button
              onClick={() => {
                setMode('token');
                setError(null);
                setOAuthStatus('idle');
              }}
              className={cn(
                'flex-1 flex items-center justify-center gap-2 py-2.5 px-4 rounded-md text-sm font-medium transition-all',
                mode === 'token'
                  ? 'bg-card text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground',
              )}
            >
              <Key size={16} />
              {t('providers.codexTokenImport.tokenImport')}
            </button>
          </div>

          {/* OAuth Mode */}
          {mode === 'oauth' && (
            <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
              <div className="bg-muted rounded-2xl p-6 border border-border space-y-6 shadow-sm">
                <div className="flex items-center gap-3 pb-4 border-b border-border/50">
                  <div className="p-2 rounded-lg" style={{ backgroundColor: `${CODEX_COLOR}15` }}>
                    <Zap size={18} style={{ color: CODEX_COLOR }} />
                  </div>
                  <div>
                    <h3 className="text-base font-semibold text-foreground">
                      {t('providers.codexTokenImport.openaiOauth')}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      {t('providers.codexTokenImport.openaiOauthDesc')}
                    </p>
                  </div>
                </div>

                {oauthStatus === 'idle' && (
                  <Button
                    onClick={handleOAuth}
                    className="w-full font-medium"
                    style={{ backgroundColor: CODEX_COLOR }}
                  >
                    <ExternalLink size={16} className="mr-2" />
                    {t('providers.codexTokenImport.signInWithOpenAI')}
                  </Button>
                )}

                {oauthStatus === 'waiting' && (
                  <div className="space-y-6">
                    <div className="text-center py-4 space-y-3">
                      {!popupClosed && (
                        <Loader2
                          size={32}
                          className="animate-spin mx-auto"
                          style={{ color: CODEX_COLOR }}
                        />
                      )}
                      <div>
                        <p className="text-sm font-medium text-foreground">
                          {popupClosed
                            ? t('providers.codexTokenImport.popupClosed')
                            : t('providers.codexTokenImport.waitingAuth')}
                        </p>
                        <p className="text-xs text-muted-foreground mt-1">
                          {popupClosed
                            ? t('providers.codexTokenImport.pasteCallbackHint')
                            : t('providers.codexTokenImport.completeSignIn')}
                        </p>
                      </div>
                    </div>

                    {/* Manual callback URL section */}
                    <div className="border-t border-border/50 pt-5 space-y-3">
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <Link size={14} />
                        <span>
                          {popupClosed
                            ? t('providers.codexTokenImport.copyAuthUrlHint')
                            : t('providers.codexTokenImport.popupNotWorkingHint')}
                        </span>
                      </div>

                      {/* Copy Auth URL button */}
                      {authUrl && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="w-full"
                          onClick={handleCopyAuthUrl}
                        >
                          {copied ? (
                            <>
                              <Check size={14} className="mr-2 text-success" />
                              {t('common.copied')}
                            </>
                          ) : (
                            <>
                              <Copy size={14} className="mr-2" />
                              {t('providers.codexTokenImport.copyAuthUrl')}
                            </>
                          )}
                        </Button>
                      )}

                      {/* Callback URL input */}
                      <div className="space-y-2">
                        <label className="text-xs font-medium text-muted-foreground">
                          {t('providers.codexTokenImport.pasteCallbackUrl')}
                        </label>
                        <Input
                          value={callbackUrl}
                          onChange={(e) => setCallbackUrl(e.target.value)}
                          placeholder="http://localhost:1455/auth/callback?code=...&state=..."
                          className="bg-card text-xs font-mono"
                          disabled={exchanging}
                        />
                        <p className="text-[10px] text-muted-foreground">
                          {t('providers.codexTokenImport.callbackNote')}
                        </p>
                      </div>

                      <Button
                        onClick={handleExchangeCallback}
                        disabled={!callbackUrl.trim() || exchanging}
                        className="w-full"
                        variant="secondary"
                      >
                        {exchanging ? (
                          <>
                            <Loader2 size={14} className="animate-spin mr-2" />
                            {t('providers.codexTokenImport.exchanging')}
                          </>
                        ) : (
                          t('providers.codexTokenImport.submitCallbackUrl')
                        )}
                      </Button>
                    </div>

                    <div className="flex justify-center">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setOAuthStatus('idle');
                          setOAuthState(null);
                          setAuthUrl(null);
                          setCallbackUrl('');
                          setPopupClosed(false);
                          if (oauthWindowRef.current && !oauthWindowRef.current.closed) {
                            oauthWindowRef.current.close();
                          }
                        }}
                      >
                        {t('common.cancel')}
                      </Button>
                    </div>
                  </div>
                )}

                {oauthStatus === 'success' && oauthResult && (
                  <div className="space-y-4">
                    <div className="bg-success/5 border border-success/20 rounded-xl p-4 flex items-start gap-3">
                      <CheckCircle2 size={20} className="text-success shrink-0 mt-0.5" />
                      <div>
                        <p className="text-sm font-medium text-foreground">
                          {t('providers.codexTokenImport.authorizationSuccessful')}
                        </p>
                        <p className="text-xs text-muted-foreground mt-0.5">
                          {t('providers.codexTokenImport.signedInAs')}{' '}
                          {oauthResult.email || oauthResult.name || t('common.unknown')}
                        </p>
                      </div>
                    </div>
                    <Button
                      onClick={handleCreateFromOAuth}
                      disabled={creating}
                      className="w-full font-medium"
                      style={{ backgroundColor: CODEX_COLOR }}
                    >
                      {creating ? (
                        <>
                          <Loader2 size={16} className="animate-spin mr-2" />
                          {t('providers.codexTokenImport.creatingProvider')}
                        </>
                      ) : (
                        t('providers.codexTokenImport.completeSetup')
                      )}
                    </Button>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Token Mode */}
          {mode === 'token' && (
            <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
              <div className="bg-muted rounded-2xl p-6 border border-border space-y-6 shadow-sm">
                <div className="flex items-center gap-3 pb-4 border-b border-border/50">
                  <div className="p-2 rounded-lg bg-accent">
                    <ShieldCheck size={18} className="text-foreground" />
                  </div>
                  <div>
                    <h3 className="text-base font-semibold text-foreground">
                      {t('providers.codexTokenImport.credentials')}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      {t('providers.codexTokenImport.importTokenFrom')}{' '}
                      <a
                        href="https://github.com/router-for-me/CLIProxyAPI"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-primary hover:underline"
                      >
                        CLIProxyAPI
                      </a>
                    </p>
                  </div>
                </div>

                {/* Email Input */}
                <div className="space-y-2">
                  <label className="text-sm font-medium text-foreground flex items-center justify-between">
                    <span className="flex items-center gap-2">
                      <Mail size={14} /> {t('providers.codexTokenImport.emailAddress')}
                    </span>
                    <span className="text-[10px] text-muted-foreground bg-accent px-2 py-0.5 rounded-full">
                      {t('providers.codexTokenImport.optional')}
                    </span>
                  </label>
                  <Input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder={t('providers.codexTokenImport.emailPlaceholder')}
                    className="bg-card"
                    disabled={validating || creating}
                  />
                  <p className="text-[11px] text-muted-foreground pl-1">
                    {t('providers.codexTokenImport.displayOnlyNote')}
                  </p>
                </div>

                {/* Token Input */}
                <div className="space-y-2">
                  <label className="text-sm font-medium text-foreground flex items-center gap-2">
                    <Key size={14} /> {t('providers.codexTokenImport.refreshToken')}
                  </label>
                  <div className="relative">
                    <textarea
                      value={token}
                      onChange={(e) => {
                        setToken(e.target.value);
                        setValidationResult(null);
                      }}
                      placeholder={t('providers.codexTokenImport.refreshTokenPlaceholder')}
                      className="w-full h-32 px-4 py-3 rounded-xl border border-border bg-card text-foreground placeholder:text-muted-foreground font-mono text-xs resize-none focus:outline-none focus:ring-2 focus:ring-accent/50 transition-all"
                      disabled={validating || creating}
                    />
                    {token && (
                      <div className="absolute bottom-3 right-3 text-[10px] text-muted-foreground font-mono bg-muted px-2 py-1 rounded border border-border">
                        {token.length} {t('providers.codexTokenImport.chars')}
                      </div>
                    )}
                  </div>
                </div>

                {/* Validate Button */}
                <Button
                  onClick={handleValidate}
                  disabled={!token.trim() || validating || creating}
                  className="w-full font-medium"
                  variant={validationResult?.valid ? 'outline' : 'default'}
                >
                  {validating ? (
                    <>
                      <Loader2 size={16} className="animate-spin mr-2" />
                      {t('providers.codexTokenImport.validatingToken')}
                    </>
                  ) : validationResult?.valid ? (
                    <>
                      <CheckCircle2 size={16} className="text-success mr-2" />
                      {t('providers.codexTokenImport.revalidate')}
                    </>
                  ) : (
                    t('providers.codexTokenImport.validateToken')
                  )}
                </Button>
              </div>

              {/* Validation Result */}
              {validationResult?.valid && (
                <div className="bg-success/5 border border-success/20 rounded-xl p-5 animate-in fade-in zoom-in-95">
                  <div className="flex items-start gap-4">
                    <div className="p-2 bg-success/10 rounded-full">
                      <CheckCircle2 size={24} className="text-success" />
                    </div>
                    <div className="flex-1 space-y-1">
                      <div className="font-semibold text-foreground">
                        {t('providers.codexTokenImport.tokenVerified')}
                      </div>
                      <div className="text-sm text-muted-foreground">
                        {t('providers.codexTokenImport.readyToConnectAs')}{' '}
                        <span className="font-medium text-foreground">
                          {validationResult.email || email || t('common.unknown')}
                        </span>
                      </div>

                      {validationResult.name && (
                        <div className="flex items-center gap-2 mt-3 pt-3 border-t border-success/10">
                          <span className="text-xs text-muted-foreground bg-card px-2 py-1 rounded border border-border/50">
                            {validationResult.name}
                          </span>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )}

              {/* Create Button */}
              {validationResult?.valid && (
                <div className="pt-4">
                  <Button
                    onClick={handleCreate}
                    disabled={creating}
                    size="lg"
                    className="w-full text-base shadow-lg shadow-accent/20 hover:shadow-accent/30 transition-all"
                  >
                    {creating ? (
                      <>
                        <Loader2 size={18} className="animate-spin mr-2" />
                        {t('providers.codexTokenImport.creatingProvider')}
                      </>
                    ) : (
                      t('providers.codexTokenImport.completeSetup')
                    )}
                  </Button>
                </div>
              )}
            </div>
          )}

          {/* Error Message */}
          {error && (
            <div className="bg-error/5 border border-error/20 rounded-xl p-4 flex items-start gap-3 animate-in fade-in zoom-in-95">
              <AlertCircle size={20} className="text-error shrink-0 mt-0.5" />
              <div>
                <p className="text-sm font-medium text-error">
                  {t('providers.codexTokenImport.error')}
                </p>
                <p className="text-xs text-error/80 mt-0.5">{error}</p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
