import { useState, useEffect, useRef } from 'react';
import {
  Wand2,
  ChevronLeft,
  Loader2,
  CheckCircle2,
  AlertCircle,
  Key,
  ExternalLink,
  Mail,
  ShieldCheck,
  Zap,
} from 'lucide-react';
import { getTransport } from '@/lib/transport';
import type {
  AntigravityTokenValidationResult,
  CreateProviderData,
  AntigravityOAuthResult,
} from '@/lib/transport';
import { ANTIGRAVITY_COLOR } from '../types';
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

export function AntigravityTokenImport() {
  const { t } = useTranslation();
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();
  const [mode, setMode] = useState<ImportMode>('token');
  const [email, setEmail] = useState('');
  const [token, setToken] = useState('');
  const [validating, setValidating] = useState(false);
  const [creating, setCreating] = useState(false);
  const [validationResult, setValidationResult] = useState<AntigravityTokenValidationResult | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);

  // CLIProxyAPI 开关
  const [useCLIProxyAPI, setUseCLIProxyAPI] = useState(false);

  // OAuth state
  const [oauthStatus, setOAuthStatus] = useState<OAuthStatus>('idle');
  const [oauthState, setOAuthState] = useState<string | null>(null);
  const [oauthResult, setOAuthResult] = useState<AntigravityOAuthResult | null>(null);
  const oauthWindowRef = useRef<Window | null>(null);

  // Subscribe to OAuth result messages via WebSocket
  useEffect(() => {
    const transport = getTransport();
    const unsubscribe = transport.subscribe<AntigravityOAuthResult>(
      'antigravity_oauth_result',
      (result) => {
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
            setError(result.error || t('providers.antigravityTokenImport.errors.oauthFailed'));
          }
        }
      },
    );

    return () => unsubscribe();
  }, [oauthState, t]);

  // Handle OAuth flow
  const handleOAuth = async () => {
    setOAuthStatus('waiting');
    setError(null);

    try {
      // Request OAuth URL from backend
      const { authURL, state } = await getTransport().startAntigravityOAuth();
      setOAuthState(state);

      // Open OAuth window
      const width = 600;
      const height = 700;
      const left = window.screenX + (window.outerWidth - width) / 2;
      const top = window.screenY + (window.outerHeight - height) / 2;

      oauthWindowRef.current = window.open(
        authURL,
        'Antigravity OAuth',
        `width=${width},height=${height},left=${left},top=${top},resizable=yes,scrollbars=yes`,
      );

      // Monitor window closure
      const checkWindowClosed = setInterval(() => {
        if (oauthWindowRef.current?.closed) {
          clearInterval(checkWindowClosed);
          // If still waiting when window closes, assume user cancelled
          setOAuthStatus((current) => {
            if (current === 'waiting') {
              setOAuthState(null);
              return 'idle';
            }
            return current;
          });
        }
      }, 500);
    } catch (err) {
      setOAuthStatus('error');
      setError(
        err instanceof Error
          ? err.message
          : t('providers.antigravityTokenImport.errors.startOAuthFailed'),
      );
    }
  };

  // 验证 token
  const handleValidate = async () => {
    if (token.trim() === '' || !token.startsWith('1//')) {
      setError(t('providers.antigravityTokenImport.errors.invalidRefreshToken'));
      return;
    }

    setValidating(true);
    setError(null);
    setValidationResult(null);

    try {
      const result = await getTransport().validateAntigravityToken(token.trim());
      setValidationResult(result);
      if (!result.valid) {
        setError(
          result.error || t('providers.antigravityTokenImport.errors.tokenValidationFailed'),
        );
      }
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t('providers.antigravityTokenImport.errors.validationFailed'),
      );
    } finally {
      setValidating(false);
    }
  };

  // 创建 provider
  const handleCreate = async () => {
    if (!validationResult?.valid) {
      setError(t('providers.antigravityTokenImport.errors.validateFirst'));
      return;
    }

    setCreating(true);
    setError(null);

    try {
      // 优先使用验证返回的邮箱，其次使用用户输入的邮箱
      const finalEmail = validationResult.userInfo?.email || email.trim() || '';
      const providerData: CreateProviderData = {
        type: 'antigravity',
        name: finalEmail || 'Antigravity Account',
        config: {
          antigravity: {
            email: finalEmail,
            refreshToken: token.trim(),
            projectID: validationResult.projectID || '',
            endpoint: validationResult.projectID
              ? `https://us-central1-aiplatform.googleapis.com/v1/projects/${validationResult.projectID}/locations/us-central1`
              : '',
            useCLIProxyAPI,
          },
        },
      };
      await createProvider.mutateAsync(providerData);
      goToProviders();
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t('providers.antigravityTokenImport.errors.createFailed'),
      );
    } finally {
      setCreating(false);
    }
  };

  // 创建 OAuth provider
  const handleOAuthCreate = async () => {
    if (!oauthResult?.refreshToken) {
      setError(t('providers.antigravityTokenImport.errors.oauthResultMissing'));
      return;
    }

    setCreating(true);
    setError(null);

    try {
      const providerData: CreateProviderData = {
        type: 'antigravity',
        name: oauthResult.email || 'Antigravity Account',
        config: {
          antigravity: {
            email: oauthResult.email || '',
            refreshToken: oauthResult.refreshToken,
            projectID: oauthResult.projectID || '',
            endpoint: oauthResult.projectID
              ? `https://us-central1-aiplatform.googleapis.com/v1/projects/${oauthResult.projectID}/locations/us-central1`
              : '',
            useCLIProxyAPI,
          },
        },
      };
      await createProvider.mutateAsync(providerData);
      goToProviders();
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t('providers.antigravityTokenImport.errors.createFailed'),
      );
    } finally {
      setCreating(false);
    }
  };

  const isTokenValid = token.trim().startsWith('1//');

  return (
    <div className="flex flex-col h-full bg-card">
      <PageHeader
        icon={<ChevronLeft className="cursor-pointer" onClick={goToSelectType} />}
        title={t('providers.antigravityTokenImport.title')}
      />

      <div className="flex-1 overflow-y-auto">
        <div className="container max-w-2xl mx-auto py-8 px-6 space-y-8">
          {/* Hero Section */}
          <div className="text-center space-y-2 mb-8">
            <h1 className="text-2xl font-bold text-foreground">
              {t('providers.antigravityTokenImport.chooseMethodTitle')}
            </h1>
            <p className="text-muted-foreground mx-auto">
              {t('providers.antigravityTokenImport.chooseMethodDesc')}
            </p>
          </div>

          {/* CLIProxyAPI Switch */}
          <CLIProxyAPISwitch checked={useCLIProxyAPI} onChange={setUseCLIProxyAPI} />

          {/* Mode Selector */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <button
              onClick={() => setMode('oauth')}
              className={cn(
                'relative group p-4 rounded-xl border-2 transition-all duration-200 text-left',
                mode === 'oauth'
                  ? 'border-primary bg-primary/10 shadow-md'
                  : 'border-border hover:border-primary/30 bg-secondary/50 hover:bg-secondary',
              )}
            >
              <div className="flex items-start gap-4">
                <div
                  className={cn(
                    'size-10 rounded-md flex items-center justify-center transition-colors',
                    mode === 'oauth'
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-secondary text-secondary-foreground group-hover:bg-primary/20',
                  )}
                >
                  <ExternalLink size={20} />
                </div>
                <div className="flex-1">
                  <div
                    className={cn(
                      'font-semibold mb-1',
                      mode === 'oauth' ? 'text-primary' : 'text-foreground',
                    )}
                  >
                    {t('providers.antigravityTokenImport.oauthConnect')}
                  </div>
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    {t('providers.antigravityTokenImport.oauthConnectDesc')}
                  </p>
                </div>
              </div>
              {mode === 'oauth' && (
                <div className="absolute top-3 right-3 w-2 h-2 rounded-full bg-primary" />
              )}
            </button>

            <button
              onClick={() => setMode('token')}
              className={cn(
                'relative group p-4 rounded-xl border-2 transition-all duration-200 text-left',
                mode === 'token'
                  ? 'border-primary bg-primary/10 shadow-md'
                  : 'border-border hover:border-primary/30 bg-secondary/50 hover:bg-secondary',
              )}
            >
              <div className="flex items-start gap-4">
                <div
                  className={cn(
                    'size-10 rounded-md flex items-center justify-center transition-colors',
                    mode === 'token'
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-secondary text-secondary-foreground group-hover:bg-primary/20',
                  )}
                >
                  <Key size={20} />
                </div>
                <div className="flex-1">
                  <div
                    className={cn(
                      'font-semibold mb-1',
                      mode === 'token' ? 'text-primary' : 'text-foreground',
                    )}
                  >
                    {t('providers.antigravityTokenImport.manualToken')}
                  </div>
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    {t('providers.antigravityTokenImport.manualTokenDesc')}
                  </p>
                </div>
              </div>
              {mode === 'token' && (
                <div className="absolute top-3 right-3 w-2 h-2 rounded-full bg-primary" />
              )}
            </button>
          </div>

          <div className="w-full h-px bg-border/50" />

          {/* Content Area */}
          <div className="min-h-[400px]">
            {mode === 'oauth' ? (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <div className="bg-muted/50 rounded-2xl p-10 border border-dashed border-border text-center flex flex-col items-center justify-center">
                  <div
                    className="w-16 h-16 rounded-2xl flex items-center justify-center mb-6 shadow-inner"
                    style={{ backgroundColor: `${ANTIGRAVITY_COLOR}15` }}
                  >
                    <Wand2 size={32} style={{ color: ANTIGRAVITY_COLOR }} />
                  </div>
                  <h3 className="text-lg font-semibold text-foreground mb-2">
                    {t('providers.antigravityTokenImport.oauthAuthorization')}
                  </h3>
                  <p className="text-sm text-muted-foreground mb-8">
                    {t('providers.antigravityTokenImport.oauthAuthorizationDesc')}
                  </p>

                  {oauthStatus === 'idle' && (
                    <Button onClick={handleOAuth} variant="default" size="lg" className="gap-2">
                      <ExternalLink size={16} />
                      {t('providers.antigravityTokenImport.connectWithGoogle')}
                    </Button>
                  )}

                  {oauthStatus === 'waiting' && (
                    <div className="space-y-4">
                      <Button disabled variant="secondary" size="lg" className="gap-2">
                        <Loader2 size={16} className="animate-spin" />
                        {t('providers.antigravityTokenImport.waitingAuth')}
                      </Button>
                      <p className="text-xs text-muted-foreground">
                        {t('providers.antigravityTokenImport.completeAuthInPopup')}
                      </p>
                    </div>
                  )}
                </div>

                {/* OAuth Success Result */}
                {oauthStatus === 'success' && oauthResult && (
                  <div className="bg-success/5 border border-success/20 rounded-xl p-5 animate-in fade-in zoom-in-95">
                    <div className="flex items-start gap-4">
                      <div className="p-2 bg-success/10 rounded-full">
                        <CheckCircle2 size={24} className="text-success" />
                      </div>
                      <div className="flex-1 space-y-1">
                        <div className="font-semibold text-foreground">
                          {t('providers.antigravityTokenImport.authorizationSuccessful')}
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {t('providers.antigravityTokenImport.readyToConnectAs')}{' '}
                          <span className="font-medium text-foreground">{oauthResult.email}</span>
                        </div>

                        <div className="flex items-center gap-2 mt-3 pt-3 border-t border-success/10">
                          {oauthResult.userInfo?.name && (
                            <span className="text-xs text-muted-foreground bg-card px-2 py-1 rounded border border-border/50">
                              {oauthResult.userInfo.name}
                            </span>
                          )}
                          {oauthResult.quota?.subscriptionTier && (
                            <div className="flex items-center gap-1.5 px-2 py-1 rounded bg-card border border-border/50">
                              <Zap
                                size={10}
                                className={
                                  oauthResult.quota.subscriptionTier === 'ULTRA'
                                    ? 'text-purple-500'
                                    : 'text-blue-500'
                                }
                              />
                              <span className="text-xs font-medium text-muted-foreground">
                                {oauthResult.quota.subscriptionTier}{' '}
                                {t('providers.antigravityTokenImport.tier')}
                              </span>
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {/* Error Message */}
                {error && oauthStatus === 'error' && (
                  <div className="bg-error/5 border border-error/20 rounded-xl p-4 flex items-start gap-3 animate-in fade-in zoom-in-95">
                    <AlertCircle size={20} className="text-error shrink-0 mt-0.5" />
                    <div>
                      <p className="text-sm font-medium text-error">
                        {t('providers.antigravityTokenImport.oauthFailed')}
                      </p>
                      <p className="text-xs text-error/80 mt-0.5">{error}</p>
                    </div>
                  </div>
                )}

                {/* Action Buttons */}
                {oauthStatus === 'error' && (
                  <div className="text-center">
                    <Button
                      onClick={() => {
                        setOAuthStatus('idle');
                        setError(null);
                      }}
                      variant="outline"
                    >
                      {t('providers.antigravityTokenImport.tryAgain')}
                    </Button>
                  </div>
                )}

                {oauthStatus === 'success' && oauthResult && (
                  <div className="pt-4">
                    <Button
                      onClick={handleOAuthCreate}
                      disabled={creating}
                      size="lg"
                      className="w-full text-base shadow-lg shadow-accent/20 hover:shadow-accent/30 transition-all"
                    >
                      {creating ? (
                        <>
                          <Loader2 size={18} className="animate-spin mr-2" />
                          {t('providers.antigravityTokenImport.creatingProvider')}
                        </>
                      ) : (
                        t('providers.antigravityTokenImport.completeSetup')
                      )}
                    </Button>
                  </div>
                )}
              </div>
            ) : (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <div className="bg-muted rounded-2xl p-6 border border-border space-y-6 shadow-sm">
                  <div className="flex items-center gap-3 pb-4 border-b border-border/50">
                    <div className="p-2 rounded-lg bg-accent">
                      <ShieldCheck size={18} className="text-foreground" />
                    </div>
                    <div>
                      <h3 className="text-base font-semibold text-foreground">
                        {t('providers.antigravityTokenImport.credentials')}
                      </h3>
                      <p className="text-xs text-muted-foreground">
                        {t('providers.antigravityTokenImport.enterAccountDetails')}
                      </p>
                    </div>
                  </div>

                  {/* Email Input */}
                  <div className="space-y-2">
                    <label className="text-sm font-medium text-foreground flex items-center justify-between">
                      <span className="flex items-center gap-2">
                        <Mail size={14} /> {t('providers.antigravityTokenImport.emailAddress')}
                      </span>
                      <span className="text-[10px] text-muted-foreground bg-accent px-2 py-0.5 rounded-full">
                        {t('providers.antigravityTokenImport.optional')}
                      </span>
                    </label>
                    <Input
                      type="email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder={t('providers.antigravityTokenImport.emailPlaceholder')}
                      className="bg-card"
                      disabled={validating || creating}
                    />
                    <p className="text-[11px] text-muted-foreground pl-1">
                      {t('providers.antigravityTokenImport.displayOnlyNote')}
                    </p>
                  </div>

                  {/* Token Input */}
                  <div className="space-y-2">
                    <label className="text-sm font-medium text-foreground flex items-center gap-2">
                      <Key size={14} /> {t('providers.antigravityTokenImport.refreshToken')}
                    </label>
                    <div className="relative">
                      <textarea
                        value={token}
                        onChange={(e) => {
                          setToken(e.target.value);
                          setValidationResult(null);
                        }}
                        placeholder={t('providers.antigravityTokenImport.refreshTokenPlaceholder')}
                        className="w-full h-32 px-4 py-3 rounded-xl border border-border bg-card text-foreground placeholder:text-muted-foreground font-mono text-xs resize-none focus:outline-none focus:ring-2 focus:ring-accent/50 transition-all"
                        disabled={validating || creating}
                      />
                      {token && (
                        <div className="absolute bottom-3 right-3 text-[10px] text-muted-foreground font-mono bg-muted px-2 py-1 rounded border border-border">
                          {token.length} {t('providers.antigravityTokenImport.chars')}
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Validate Button */}
                  <Button
                    onClick={handleValidate}
                    disabled={!isTokenValid || validating || creating}
                    className="w-full font-medium"
                    variant={validationResult?.valid ? 'outline' : 'default'}
                  >
                    {validating ? (
                      <>
                        <Loader2 size={16} className="animate-spin mr-2" />
                        {t('providers.antigravityTokenImport.validatingToken')}
                      </>
                    ) : validationResult?.valid ? (
                      <>
                        <CheckCircle2 size={16} className="text-success mr-2" />
                        {t('providers.antigravityTokenImport.revalidate')}
                      </>
                    ) : (
                      t('providers.antigravityTokenImport.validateToken')
                    )}
                  </Button>
                </div>

                {/* Error Message */}
                {error && (
                  <div className="bg-error/5 border border-error/20 rounded-xl p-4 flex items-start gap-3 animate-in fade-in zoom-in-95">
                    <AlertCircle size={20} className="text-error shrink-0 mt-0.5" />
                    <div>
                      <p className="text-sm font-medium text-error">
                        {t('providers.antigravityTokenImport.validationFailed')}
                      </p>
                      <p className="text-xs text-error/80 mt-0.5">{error}</p>
                    </div>
                  </div>
                )}

                {/* Validation Result */}
                {validationResult?.valid && (
                  <div className="bg-success/5 border border-success/20 rounded-xl p-5 animate-in fade-in zoom-in-95">
                    <div className="flex items-start gap-4">
                      <div className="p-2 bg-success/10 rounded-full">
                        <CheckCircle2 size={24} className="text-success" />
                      </div>
                      <div className="flex-1 space-y-1">
                        <div className="font-semibold text-foreground">
                          {t('providers.antigravityTokenImport.tokenVerified')}
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {t('providers.antigravityTokenImport.readyToConnectAs')}{' '}
                          <span className="font-medium text-foreground">
                            {validationResult.userInfo?.email || email}
                          </span>
                        </div>

                        <div className="flex items-center gap-2 mt-3 pt-3 border-t border-success/10">
                          {validationResult.userInfo?.name && (
                            <span className="text-xs text-muted-foreground bg-card px-2 py-1 rounded border border-border/50">
                              {validationResult.userInfo.name}
                            </span>
                          )}
                          {validationResult.quota?.subscriptionTier && (
                            <div className="flex items-center gap-1.5 px-2 py-1 rounded bg-card border border-border/50">
                              <Zap
                                size={10}
                                className={
                                  validationResult.quota.subscriptionTier === 'ULTRA'
                                    ? 'text-purple-500'
                                    : 'text-blue-500'
                                }
                              />
                              <span className="text-xs font-medium text-muted-foreground">
                                {validationResult.quota.subscriptionTier}{' '}
                                {t('providers.antigravityTokenImport.tier')}
                              </span>
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {/* Create Button */}
                <div className="pt-4">
                  <Button
                    onClick={handleCreate}
                    disabled={!validationResult?.valid || creating}
                    size="lg"
                    className="w-full text-base shadow-lg shadow-accent/20 hover:shadow-accent/30 transition-all"
                  >
                    {creating ? (
                      <>
                        <Loader2 size={18} className="animate-spin mr-2" />
                        {t('providers.antigravityTokenImport.creatingProvider')}
                      </>
                    ) : (
                      t('providers.antigravityTokenImport.completeSetup')
                    )}
                  </Button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
