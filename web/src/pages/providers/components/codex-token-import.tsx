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
import { cn } from '@/lib/utils';
import { useProviderNavigation } from '../hooks/use-provider-navigation';
import { useCreateProvider } from '@/hooks/queries';

type ImportMode = 'oauth' | 'token';
type OAuthStatus = 'idle' | 'waiting' | 'success' | 'error';

export function CodexTokenImport() {
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();
  const [mode, setMode] = useState<ImportMode>('oauth');
  const [email, setEmail] = useState('');
  const [token, setToken] = useState('');
  const [validating, setValidating] = useState(false);
  const [creating, setCreating] = useState(false);
  const [validationResult, setValidationResult] = useState<CodexTokenValidationResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  // OAuth state
  const [oauthStatus, setOAuthStatus] = useState<OAuthStatus>('idle');
  const [oauthState, setOAuthState] = useState<string | null>(null);
  const [oauthResult, setOAuthResult] = useState<CodexOAuthResult | null>(null);
  const oauthWindowRef = useRef<Window | null>(null);

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
          setError(result.error || 'OAuth authorization failed');
        }
      }
    });

    return () => unsubscribe();
  }, [oauthState]);

  // Handle OAuth flow
  const handleOAuth = async () => {
    setOAuthStatus('waiting');
    setError(null);

    try {
      // Request OAuth URL from backend
      const { authURL, state } = await getTransport().startCodexOAuth();
      setOAuthState(state);

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
      setError(err instanceof Error ? err.message : 'Failed to start OAuth flow');
    }
  };

  // Validate token
  const handleValidate = async () => {
    if (token.trim() === '') {
      setError('Please enter a valid refresh token');
      return;
    }

    setValidating(true);
    setError(null);
    setValidationResult(null);

    try {
      const result = await getTransport().validateCodexToken(token.trim());
      setValidationResult(result);
      if (!result.valid) {
        setError(result.error || 'Token validation failed');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Validation failed');
    } finally {
      setValidating(false);
    }
  };

  // Create provider from OAuth result
  const handleCreateFromOAuth = async () => {
    if (!oauthResult?.refreshToken) {
      setError('No valid OAuth result');
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
          },
        },
      };
      await createProvider.mutateAsync(providerData);
      goToProviders();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create provider');
    } finally {
      setCreating(false);
    }
  };

  // Create provider from token validation
  const handleCreate = async () => {
    if (!validationResult?.valid) {
      setError('Please validate the token first');
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
          },
        },
      };
      await createProvider.mutateAsync(providerData);
      goToProviders();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create provider');
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="flex flex-col h-full bg-card">
      {/* Header */}
      <div className="h-16 flex items-center gap-4 px-6 border-b border-border bg-card/80 backdrop-blur-sm sticky top-0 z-10">
        <Button
          variant="ghost"
          size="icon"
          onClick={goToSelectType}
          className="rounded-full hover:bg-accent -ml-2"
        >
          <ChevronLeft size={20} className="text-muted-foreground" />
        </Button>
        <div>
          <h2 className="text-lg font-semibold text-foreground flex items-center gap-2">
            <span
              className="w-2 h-2 rounded-full inline-block"
              style={{ backgroundColor: CODEX_COLOR }}
            />
            Add Codex Account
          </h2>
        </div>
      </div>

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
            <h1 className="text-2xl font-bold text-foreground">Connect Codex Account</h1>
            <p className="text-muted-foreground mx-auto max-w-md">
              Sign in with your OpenAI account or import a refresh token manually.
            </p>
          </div>

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
              OAuth Login
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
              Token Import
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
                    <h3 className="text-base font-semibold text-foreground">OpenAI OAuth</h3>
                    <p className="text-xs text-muted-foreground">
                      Sign in with your OpenAI account
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
                    Sign in with OpenAI
                  </Button>
                )}

                {oauthStatus === 'waiting' && (
                  <div className="text-center py-6 space-y-4">
                    <Loader2 size={32} className="animate-spin mx-auto" style={{ color: CODEX_COLOR }} />
                    <div>
                      <p className="text-sm font-medium text-foreground">Waiting for authorization...</p>
                      <p className="text-xs text-muted-foreground mt-1">
                        Complete the sign-in in the popup window
                      </p>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setOAuthStatus('idle');
                        setOAuthState(null);
                        if (oauthWindowRef.current && !oauthWindowRef.current.closed) {
                          oauthWindowRef.current.close();
                        }
                      }}
                    >
                      Cancel
                    </Button>
                  </div>
                )}

                {oauthStatus === 'success' && oauthResult && (
                  <div className="space-y-4">
                    <div className="bg-success/5 border border-success/20 rounded-xl p-4 flex items-start gap-3">
                      <CheckCircle2 size={20} className="text-success shrink-0 mt-0.5" />
                      <div>
                        <p className="text-sm font-medium text-foreground">Authorization Successful</p>
                        <p className="text-xs text-muted-foreground mt-0.5">
                          Signed in as {oauthResult.email || oauthResult.name || 'Unknown'}
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
                          Creating Provider...
                        </>
                      ) : (
                        'Complete Setup'
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
                    <h3 className="text-base font-semibold text-foreground">Credentials</h3>
                    <p className="text-xs text-muted-foreground">
                      Import token from{' '}
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
                      <Mail size={14} /> Email Address
                    </span>
                    <span className="text-[10px] text-muted-foreground bg-accent px-2 py-0.5 rounded-full">
                      Optional
                    </span>
                  </label>
                  <Input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="e.g. user@example.com"
                    className="bg-card"
                    disabled={validating || creating}
                  />
                  <p className="text-[11px] text-muted-foreground pl-1">
                    Used for display purposes only. Auto-detected if valid token provided.
                  </p>
                </div>

                {/* Token Input */}
                <div className="space-y-2">
                  <label className="text-sm font-medium text-foreground flex items-center gap-2">
                    <Key size={14} /> Refresh Token
                  </label>
                  <div className="relative">
                    <textarea
                      value={token}
                      onChange={(e) => {
                        setToken(e.target.value);
                        setValidationResult(null);
                      }}
                      placeholder="Paste your refresh token here..."
                      className="w-full h-32 px-4 py-3 rounded-xl border border-border bg-card text-foreground placeholder:text-muted-foreground font-mono text-xs resize-none focus:outline-none focus:ring-2 focus:ring-accent/50 transition-all"
                      disabled={validating || creating}
                    />
                    {token && (
                      <div className="absolute bottom-3 right-3 text-[10px] text-muted-foreground font-mono bg-muted px-2 py-1 rounded border border-border">
                        {token.length} chars
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
                      Validating Token...
                    </>
                  ) : validationResult?.valid ? (
                    <>
                      <CheckCircle2 size={16} className="text-success mr-2" />
                      Re-validate
                    </>
                  ) : (
                    'Validate Token'
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
                      <div className="font-semibold text-foreground">Token Verified Successfully</div>
                      <div className="text-sm text-muted-foreground">
                        Ready to connect as{' '}
                        <span className="font-medium text-foreground">
                          {validationResult.email || email || 'Unknown'}
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
                        Creating Provider...
                      </>
                    ) : (
                      'Complete Setup'
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
                <p className="text-sm font-medium text-error">Error</p>
                <p className="text-xs text-error/80 mt-0.5">{error}</p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
