import { useState } from 'react';
import {
  ChevronLeft,
  Loader2,
  CheckCircle2,
  AlertCircle,
  Key,
  Mail,
  ShieldCheck,
  Zap,
  Ban,
} from 'lucide-react';
import { getTransport } from '@/lib/transport';
import type { KiroTokenValidationResult, CreateProviderData } from '@/lib/transport';
import { KIRO_COLOR } from '../types';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useTranslation } from 'react-i18next';
import { useProviderNavigation } from '../hooks/use-provider-navigation';
import { useCreateProvider } from '@/hooks/queries';

export function KiroTokenImport() {
  const { t } = useTranslation();
  const { goToSelectType, goToProviders } = useProviderNavigation();
  const createProvider = useCreateProvider();
  const [email, setEmail] = useState('');
  const [token, setToken] = useState('');
  const [validating, setValidating] = useState(false);
  const [creating, setCreating] = useState(false);
  const [validationResult, setValidationResult] = useState<KiroTokenValidationResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  // 验证 token
  const handleValidate = async () => {
    if (token.trim() === '') {
      setError(t('providers.kiroTokenImport.errors.missingRefreshToken'));
      return;
    }

    setValidating(true);
    setError(null);
    setValidationResult(null);

    try {
      const result = await getTransport().validateKiroSocialToken(token.trim());
      setValidationResult(result);
      if (!result.valid) {
        setError(result.error || t('provider.tokenValidationFailed'));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('provider.validationFailed'));
    } finally {
      setValidating(false);
    }
  };

  // 创建 provider
  const handleCreate = async () => {
    if (!validationResult?.valid) {
      setError(t('providers.kiroTokenImport.errors.validateFirst'));
      return;
    }

    // 不允许创建被封禁的账号
    if (validationResult.isBanned) {
      setError(t('providers.kiroTokenImport.errors.bannedAccount'));
      return;
    }

    setCreating(true);
    setError(null);

    try {
      // 优先使用验证返回的邮箱，其次使用用户输入的邮箱
      const finalEmail = validationResult.email || email.trim() || '';
      const providerData: CreateProviderData = {
        type: 'kiro',
        name: finalEmail || 'Kiro Account',
        config: {
          kiro: {
            authMethod: 'social',
            email: finalEmail,
            refreshToken: validationResult.refreshToken || token.trim(),
            region: 'us-east-1',
          },
        },
      };
      await createProvider.mutateAsync(providerData);
      goToProviders();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t('providers.kiroTokenImport.errors.createFailed'),
      );
    } finally {
      setCreating(false);
    }
  };

  const isTokenValid = token.trim().length > 0;

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
              style={{ backgroundColor: KIRO_COLOR }}
            />
            {t('providers.kiroTokenImport.title')}
          </h2>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        <div className="container max-w-2xl mx-auto py-8 px-6 space-y-8">
          {/* Hero Section */}
          <div className="text-center space-y-2 mb-8">
            <h1 className="text-2xl font-bold text-foreground">
              {t('providers.kiroTokenImport.importTitle')}
            </h1>
            <p className="text-muted-foreground mx-auto">
              {t('providers.kiroTokenImport.importDescription')}
            </p>
          </div>

          {/* Token Input Form */}
          <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
            <div className="bg-muted rounded-2xl p-6 border border-border space-y-6 shadow-sm">
              <div className="flex items-center gap-3 pb-4 border-b border-border/50">
                <div className="p-2 rounded-lg bg-accent">
                  <ShieldCheck size={18} className="text-foreground" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-foreground">
                    {t('providers.kiroTokenImport.credentials')}
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    {t('providers.kiroTokenImport.credentialsDesc')}
                  </p>
                </div>
              </div>

              {/* Email Input */}
              <div className="space-y-2">
                <label className="text-sm font-medium text-foreground flex items-center justify-between">
                  <span className="flex items-center gap-2">
                    <Mail size={14} /> {t('providers.kiroTokenImport.emailAddress')}
                  </span>
                  <span className="text-[10px] text-muted-foreground bg-accent px-2 py-0.5 rounded-full">
                    {t('providers.kiroTokenImport.optional')}
                  </span>
                </label>
                <Input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder={t('providers.kiroTokenImport.emailPlaceholder')}
                  className="bg-card"
                  disabled={validating || creating}
                />
                <p className="text-[11px] text-muted-foreground pl-1">
                  {t('providers.kiroTokenImport.displayOnlyNote')}
                </p>
              </div>

              {/* Token Input */}
              <div className="space-y-2">
                <label className="text-sm font-medium text-foreground flex items-center gap-2">
                  <Key size={14} /> {t('providers.kiroTokenImport.refreshToken')}
                </label>
                <div className="relative">
                  <textarea
                    value={token}
                    onChange={(e) => {
                      setToken(e.target.value);
                      setValidationResult(null);
                    }}
                    placeholder={t('providers.kiroTokenImport.refreshTokenPlaceholder')}
                    className="w-full h-32 px-4 py-3 rounded-xl border border-border bg-card text-foreground placeholder:text-muted-foreground font-mono text-xs resize-none focus:outline-none focus:ring-2 focus:ring-accent/50 transition-all"
                    disabled={validating || creating}
                  />
                  {token && (
                    <div className="absolute bottom-3 right-3 text-[10px] text-muted-foreground font-mono bg-muted px-2 py-1 rounded border border-border">
                      {token.length} {t('providers.kiroTokenImport.chars')}
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
                    {t('providers.kiroTokenImport.validatingToken')}
                  </>
                ) : validationResult?.valid ? (
                  <>
                    <CheckCircle2 size={16} className="text-success mr-2" />
                    {t('providers.kiroTokenImport.revalidate')}
                  </>
                ) : (
                  t('providers.kiroTokenImport.validateToken')
                )}
              </Button>
            </div>

            {/* Error Message */}
            {error && (
              <div className="bg-error/5 border border-error/20 rounded-xl p-4 flex items-start gap-3 animate-in fade-in zoom-in-95">
                <AlertCircle size={20} className="text-error shrink-0 mt-0.5" />
                <div>
                  <p className="text-sm font-medium text-error">
                    {t('providers.kiroTokenImport.validationFailed')}
                  </p>
                  <p className="text-xs text-error/80 mt-0.5">{error}</p>
                </div>
              </div>
            )}

            {/* Banned Account Warning */}
            {validationResult?.valid && validationResult.isBanned && (
              <div className="bg-warning/5 border border-warning/20 rounded-xl p-5 animate-in fade-in zoom-in-95">
                <div className="flex items-start gap-4">
                  <div className="p-2 bg-warning/10 rounded-full">
                    <Ban size={24} className="text-warning" />
                  </div>
                  <div className="flex-1 space-y-1">
                    <div className="font-semibold text-foreground">
                      {t('providers.kiroTokenImport.accountBanned')}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {t('providers.kiroTokenImport.accountBannedDesc')}
                    </div>
                    {validationResult.banReason && (
                      <div className="text-xs text-warning mt-2 p-2 bg-warning/5 rounded border border-warning/10">
                        {t('providers.kiroTokenImport.reason')}: {validationResult.banReason}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            )}

            {/* Validation Result */}
            {validationResult?.valid && !validationResult.isBanned && (
              <div className="bg-success/5 border border-success/20 rounded-xl p-5 animate-in fade-in zoom-in-95">
                <div className="flex items-start gap-4">
                  <div className="p-2 bg-success/10 rounded-full">
                    <CheckCircle2 size={24} className="text-success" />
                  </div>
                  <div className="flex-1 space-y-1">
                    <div className="font-semibold text-foreground">
                      {t('providers.kiroTokenImport.tokenVerified')}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {t('providers.kiroTokenImport.readyToConnectAs')}{' '}
                      <span className="font-medium text-foreground">
                        {validationResult.email ||
                          email ||
                          t('providers.kiroTokenImport.defaultAccountName')}
                      </span>
                    </div>

                    <div className="flex flex-wrap items-center gap-2 mt-3 pt-3 border-t border-success/10">
                      {validationResult.subscriptionType && (
                        <div className="flex items-center gap-1.5 px-2 py-1 rounded bg-card border border-border/50">
                          <Zap
                            size={10}
                            className={
                              validationResult.subscriptionType === 'PRO'
                                ? 'text-purple-500'
                                : 'text-blue-500'
                            }
                          />
                          <span className="text-xs font-medium text-muted-foreground">
                            {validationResult.subscriptionType}
                          </span>
                        </div>
                      )}
                      {validationResult.usageLimit !== undefined && (
                        <div className="flex items-center gap-1.5 px-2 py-1 rounded bg-card border border-border/50">
                          <span className="text-xs text-muted-foreground">
                            {t('providers.kiroTokenImport.usage')}:{' '}
                            {validationResult.currentUsage ?? 0} / {validationResult.usageLimit}
                          </span>
                        </div>
                      )}
                      {validationResult.daysUntilReset !== undefined && (
                        <div className="flex items-center gap-1.5 px-2 py-1 rounded bg-card border border-border/50">
                          <span className="text-xs text-muted-foreground">
                            {t('providers.kiroTokenImport.resetsInDays', {
                              days: validationResult.daysUntilReset,
                            })}
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
                disabled={!validationResult?.valid || validationResult.isBanned || creating}
                size="lg"
                className="w-full text-base shadow-lg shadow-accent/20 hover:shadow-accent/30 transition-all"
              >
                {creating ? (
                  <>
                    <Loader2 size={18} className="animate-spin mr-2" />
                    {t('providers.kiroTokenImport.creatingProvider')}
                  </>
                ) : (
                  t('providers.kiroTokenImport.completeSetup')
                )}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
