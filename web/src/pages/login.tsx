import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { browserSupportsWebAuthn, startAuthentication } from '@simplewebauthn/browser';
import {
  ChevronDownIcon,
  FingerprintIcon,
} from 'lucide-react';
import { PasswordRulesPopover } from '@/components/auth/password-rules-popover';
import { FieldError } from '@/components/field-error';
import { PasswordInput } from '@/components/password-input';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import type { AuthUser } from '@/lib/auth-context';
import { getManagedPasswordError, getManagedPasswordRuleState } from '@/lib/managed-password';
import { useTransport } from '@/lib/transport';

interface LoginPageProps {
  onSuccess: (token: string, user?: AuthUser) => void;
}

type AuthTab = 'login' | 'register';
type LoginField = 'username' | 'password';
type RegisterField = 'username' | 'password' | 'confirmPassword' | 'inviteCode';
type RegisterMappedError =
  | { field: RegisterField; message: string }
  | { formError: string };

function getRegisterPasswordError(password: string, t: (key: string) => string) {
  return getManagedPasswordError(password, t('login.passwordFormatInvalid'));
}

function mapRegisterError(
  error: string | undefined,
  t: (key: string) => string,
): RegisterMappedError {
  if (!error) {
    return { formError: t('login.registerFailed') };
  }

  switch (error) {
    case 'invite code required':
      return { field: 'inviteCode' as const, message: t('login.inviteCodeRequired') };
    case 'invite code invalid':
      return { field: 'inviteCode' as const, message: t('login.inviteCodeInvalid') };
    case 'invite code expired':
      return { field: 'inviteCode' as const, message: t('login.inviteCodeExpired') };
    case 'invite code exhausted':
      return { field: 'inviteCode' as const, message: t('login.inviteCodeExhausted') };
    case 'invite code disabled':
      return { field: 'inviteCode' as const, message: t('login.inviteCodeDisabled') };
    case 'username already exists':
      return { field: 'username' as const, message: t('login.usernameExists') };
    default:
      return { formError: error };
  }
}

export function LoginPage({ onSuccess }: LoginPageProps) {
  const { t } = useTranslation();
  const { transport } = useTransport();
  const [authTab, setAuthTab] = useState<AuthTab>('login');
  const [passkeyExpanded, setPasskeyExpanded] = useState(false);
  const [showRegisterPasswordRules, setShowRegisterPasswordRules] = useState(false);
  const [registerPasswordsVisible, setRegisterPasswordsVisible] = useState(false);
  const [forgotPasswordOpen, setForgotPasswordOpen] = useState(false);

  const [loginUsername, setLoginUsername] = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [loginFieldErrors, setLoginFieldErrors] = useState<Partial<Record<LoginField, string>>>({});
  const [loginFormError, setLoginFormError] = useState('');

  const [registerUsername, setRegisterUsername] = useState('');
  const [registerPassword, setRegisterPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [inviteCode, setInviteCode] = useState('');
  const [registerFieldErrors, setRegisterFieldErrors] = useState<
    Partial<Record<RegisterField, string>>
  >({});
  const [registerFormError, setRegisterFormError] = useState('');

  const [passkeyError, setPasskeyError] = useState('');
  const [successMessage, setSuccessMessage] = useState('');
  const [isLoginLoading, setIsLoginLoading] = useState(false);
  const [isRegisterLoading, setIsRegisterLoading] = useState(false);
  const [isPasskeyLoading, setIsPasskeyLoading] = useState(false);

  const passkeySupported = browserSupportsWebAuthn();
  const anyLoading = isLoginLoading || isRegisterLoading || isPasskeyLoading;
  const registerPasswordRuleState = getManagedPasswordRuleState(registerPassword);
  const registerPasswordFormatError = getRegisterPasswordError(registerPassword, t);
  const registerPasswordFieldError =
    registerFieldErrors.password === registerPasswordFormatError
      ? undefined
      : registerFieldErrors.password;
  const isRegisterSubmitDisabled =
    anyLoading ||
    !registerUsername.trim() ||
    !registerPassword.trim() ||
    !confirmPassword.trim() ||
    !inviteCode.trim() ||
    !!registerPasswordFormatError;

  const clearLoginMessages = () => {
    setLoginFormError('');
    setLoginFieldErrors({});
    setSuccessMessage('');
  };

  const clearRegisterMessages = () => {
    setRegisterFormError('');
    setRegisterFieldErrors({});
    setSuccessMessage('');
  };

  const clearPasskeyMessages = () => {
    setPasskeyError('');
    setSuccessMessage('');
  };

  const handleLogin = async (e: FormEvent) => {
    e.preventDefault();
    if (anyLoading) {
      return;
    }
    clearLoginMessages();
    clearPasskeyMessages();

    const nextErrors: Partial<Record<LoginField, string>> = {};
    if (!loginUsername.trim()) {
      nextErrors.username = t('login.usernameRequired');
    }
    if (!loginPassword.trim()) {
      nextErrors.password = t('login.passwordRequired');
    }

    if (Object.keys(nextErrors).length > 0) {
      setLoginFieldErrors(nextErrors);
      return;
    }

    setIsLoginLoading(true);

    try {
      const result = await transport.login(loginUsername.trim(), loginPassword);
      if (result.success && result.token) {
        onSuccess(result.token, result.user as AuthUser | undefined);
        return;
      }

      if (result.error === 'account pending approval') {
        setLoginFormError(t('login.pendingApproval'));
      } else if (result.error === 'invalid credentials') {
        setLoginFieldErrors({ password: t('login.invalidCredentials') });
      } else {
        setLoginFormError(result.error || t('login.invalidCredentials'));
      }
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string }, status?: number } };
      const errorMsg = axiosError?.response?.data?.error;

      if (errorMsg === 'account pending approval') {
        setLoginFormError(t('login.pendingApproval'));
      } else if (errorMsg === 'invalid credentials' || axiosError?.response?.status === 401) {
        setLoginFieldErrors({ password: t('login.invalidCredentials') });
      } else {
        setLoginFormError(errorMsg || t('login.invalidCredentials'));
      }
    } finally {
      setIsLoginLoading(false);
    }
  };

  const handleRegister = async (e: FormEvent) => {
    e.preventDefault();
    if (anyLoading) {
      return;
    }
    clearRegisterMessages();

    const nextErrors: Partial<Record<RegisterField, string>> = {};
    if (!registerUsername.trim()) {
      nextErrors.username = t('login.usernameRequired');
    }
    if (!registerPassword.trim()) {
      nextErrors.password = t('login.passwordRequired');
    }
    const passwordFormatError = getRegisterPasswordError(registerPassword, t);
    if (passwordFormatError) {
      setShowRegisterPasswordRules(true);
    }
    if (!confirmPassword.trim()) {
      nextErrors.confirmPassword = t('login.confirmPasswordRequired');
    }
    if (!inviteCode.trim()) {
      nextErrors.inviteCode = t('login.inviteCodeRequired');
    }
    if (registerPassword && confirmPassword && registerPassword !== confirmPassword) {
      nextErrors.confirmPassword = t('login.passwordMismatch');
    }

    if (Object.keys(nextErrors).length > 0) {
      setRegisterFieldErrors(nextErrors);
      return;
    }

    setIsRegisterLoading(true);

    try {
      const result = await transport.apply(registerUsername.trim(), registerPassword, inviteCode.trim());
      if (result.success) {
        setSuccessMessage(t('login.registerSuccess'));
        setLoginUsername(registerUsername.trim());
        setRegisterUsername('');
        setRegisterPassword('');
        setConfirmPassword('');
        setInviteCode('');
        setAuthTab('login');
        return;
      }

      const mappedError = mapRegisterError(result.error, t);
      if ('field' in mappedError) {
        setRegisterFieldErrors({ [mappedError.field]: mappedError.message });
      } else {
        setRegisterFormError(mappedError.formError);
      }
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string } } };
      const mappedError = mapRegisterError(axiosError?.response?.data?.error, t);
      if ('field' in mappedError) {
        setRegisterFieldErrors({ [mappedError.field]: mappedError.message });
      } else {
        setRegisterFormError(mappedError.formError);
      }
    } finally {
      setIsRegisterLoading(false);
    }
  };

  const handlePasskeyLogin = async () => {
    if (anyLoading) {
      return;
    }
    clearPasskeyMessages();
    setLoginFormError('');

    if (!passkeySupported) {
      setPasskeyError(t('login.passkeyNotSupported'));
      return;
    }

    setIsPasskeyLoading(true);
    try {
      const beginResult = await transport.startPasskeyLogin(loginUsername.trim() || undefined);
      if (!beginResult.success || !beginResult.sessionID || !beginResult.options) {
        setPasskeyError(beginResult.error || t('login.passkeyLoginFailed'));
        return;
      }

      const authentication = await startAuthentication({ optionsJSON: beginResult.options });
      const finishResult = await transport.finishPasskeyLogin(beginResult.sessionID, authentication);
      if (finishResult.success && finishResult.token) {
        onSuccess(finishResult.token, finishResult.user as AuthUser | undefined);
        return;
      }

      if (finishResult.error === 'account pending approval') {
        setPasskeyError(t('login.pendingApproval'));
      } else if (finishResult.error === 'invalid credentials') {
        setPasskeyError(t('login.invalidCredentials'));
      } else {
        setPasskeyError(finishResult.error || t('login.passkeyLoginFailed'));
      }
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string }, status?: number } };
      const errorMsg = axiosError?.response?.data?.error;

      if (errorMsg === 'account pending approval') {
        setPasskeyError(t('login.pendingApproval'));
      } else if (errorMsg === 'invalid credentials' || axiosError?.response?.status === 401) {
        setPasskeyError(t('login.invalidCredentials'));
      } else {
        setPasskeyError(errorMsg || t('login.passkeyLoginFailed'));
      }
    } finally {
      setIsPasskeyLoading(false);
    }
  };

  return (
    <>
      <div className="relative min-h-screen overflow-hidden bg-background px-4 py-8 sm:px-6 lg:px-8">
        <div className="pointer-events-none absolute inset-0">
          <div className="bg-primary/10 absolute left-[-12rem] top-[-10rem] h-80 w-80 rounded-full blur-3xl" />
          <div className="bg-amber-500/10 absolute left-1/2 top-1/3 h-64 w-64 -translate-x-1/2 rounded-full blur-3xl" />
          <div className="bg-emerald-500/10 absolute bottom-[-12rem] right-[-8rem] h-96 w-96 rounded-full blur-3xl" />
        </div>

        <div className="relative mx-auto flex min-h-[calc(100vh-4rem)] max-w-3xl items-center justify-center">
          <section className="w-full">
            <Card className="border-border/70 bg-card/95 overflow-hidden shadow-[0_24px_80px_rgba(15,23,42,0.12)] backdrop-blur">
              <CardContent className="space-y-6 px-6 py-6 sm:px-8 sm:py-8">
                <div className="space-y-1">
                  <h1 className="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
                    {t('login.title')}
                  </h1>
                </div>

                {successMessage && (
                  <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-700 dark:text-emerald-300">
                    {successMessage}
                  </div>
                )}
                {loginFormError && (
                  <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                    {loginFormError}
                  </div>
                )}

                <Tabs
                  value={authTab}
                  onValueChange={(value) => {
                    if (anyLoading) {
                      return;
                    }
                    setAuthTab(value as AuthTab);
                    if (value === 'register') {
                      setPasskeyExpanded(false);
                    } else {
                      setShowRegisterPasswordRules(false);
                    }
                    setRegisterPasswordsVisible(false);
                    clearLoginMessages();
                    clearRegisterMessages();
                  }}
                  className="w-full"
                >
                  <TabsList className="grid h-11 w-full grid-cols-2 rounded-xl p-1">
                    <TabsTrigger value="login" className="rounded-lg text-sm" disabled={anyLoading}>
                      {t('login.primaryTitle')}
                    </TabsTrigger>
                    <TabsTrigger value="register" className="rounded-lg text-sm" disabled={anyLoading}>
                      {t('login.registerSummaryTitle')}
                    </TabsTrigger>
                  </TabsList>

                  <TabsContent value="login" className="mt-6 space-y-5">
                    <form onSubmit={handleLogin} className="space-y-5">
                      <div className="space-y-2">
                        <Label htmlFor="login-username">{t('login.usernameLabel')}</Label>
                        <Input
                          id="login-username"
                          type="text"
                          value={loginUsername}
                          placeholder={t('login.usernamePlaceholder')}
                          autoComplete="username"
                          autoFocus
                          disabled={anyLoading}
                          className="h-11"
                          aria-invalid={loginFieldErrors.username ? 'true' : undefined}
                          onChange={(e) => {
                            setLoginUsername(e.target.value);
                            setLoginFieldErrors((current) => ({ ...current, username: undefined }));
                            setLoginFormError('');
                        setSuccessMessage('');
                      }}
                    />
                        <FieldError message={loginFieldErrors.username} />
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="login-password">{t('login.passwordLabel')}</Label>
                        <Input
                          id="login-password"
                          type="password"
                          value={loginPassword}
                          placeholder={t('login.passwordPlaceholder')}
                          autoComplete="current-password"
                          disabled={anyLoading}
                          className="h-11"
                          aria-invalid={loginFieldErrors.password ? 'true' : undefined}
                          onChange={(e) => {
                            setLoginPassword(e.target.value);
                            setLoginFieldErrors((current) => ({ ...current, password: undefined }));
                            setLoginFormError('');
                          }}
                        />
                        <div className="flex justify-end">
                          <Button
                            type="button"
                            variant="link"
                            className="text-muted-foreground h-auto px-0 py-0 text-xs"
                            onClick={() => setForgotPasswordOpen(true)}
                          >
                            {t('login.forgotPassword')}
                          </Button>
                        </div>
                        <FieldError message={loginFieldErrors.password} />
                      </div>

                      <Button
                        type="submit"
                        className="w-full"
                        size="lg"
                        disabled={anyLoading || !loginUsername.trim() || !loginPassword.trim()}
                      >
                        {isLoginLoading ? t('login.verifying') : t('login.submit')}
                      </Button>
                    </form>

                    <div className="space-y-3 border-t border-border/60 pt-4">
                      <button
                        type="button"
                        className="flex w-full items-start gap-3 rounded-2xl border border-border/70 bg-muted/25 px-4 py-4 text-left transition-colors hover:bg-muted/45"
                        onClick={() => setPasskeyExpanded((current) => !current)}
                        aria-expanded={passkeyExpanded}
                      >
                        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-background text-foreground ring-1 ring-border">
                          <FingerprintIcon className="size-4" />
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="block text-sm font-medium text-foreground">
                            {t('login.passkeySummaryTitle')}
                          </span>
                          <span className="text-muted-foreground mt-1 block text-xs leading-5">
                            {t('login.passkeySummaryDescription')}
                          </span>
                        </span>
                        <ChevronDownIcon
                          className={`mt-1 size-4 shrink-0 transition-transform ${passkeyExpanded ? 'rotate-180' : ''}`}
                        />
                      </button>

                      {passkeyExpanded && (
                        <div className="space-y-4 rounded-2xl border border-border/70 bg-muted/25 p-4">
                          <div className="space-y-1">
                            <p className="text-sm font-medium">{t('login.passkeySummaryTitle')}</p>
                            <p className="text-muted-foreground text-sm leading-6">
                              {t('login.passkeyHint')}
                            </p>
                          </div>

                          <div className="rounded-lg border border-border/70 bg-background/80 p-3">
                            <p className="text-sm leading-6 text-foreground">
                              {t('login.passkeyManageHint')}
                            </p>
                          </div>

                          {!passkeySupported && (
                            <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
                              {t('login.passkeyUnsupportedHint')}
                            </div>
                          )}

                          {passkeyError && (
                            <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                              {passkeyError}
                            </div>
                          )}

                          <Button
                            type="button"
                            className="w-full"
                            variant="secondary"
                            size="lg"
                            onClick={handlePasskeyLogin}
                            disabled={anyLoading}
                          >
                            {isPasskeyLoading ? t('login.verifying') : t('login.passkeyLogin')}
                          </Button>
                        </div>
                      )}
                    </div>
                  </TabsContent>

                  <TabsContent value="register" className="mt-6 space-y-5">
                    <div className="rounded-2xl border border-border/70 bg-muted/25 p-4">
                      <p className="text-sm font-medium">{t('login.registerSummaryTitle')}</p>
                      <p className="text-muted-foreground mt-1 text-sm leading-6">
                        {t('login.registerSummaryDescription')}
                      </p>
                    </div>

                    <form onSubmit={handleRegister} className="space-y-5">
                      {registerFormError && (
                        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                          {registerFormError}
                        </div>
                      )}

                      <div className="space-y-2">
                        <Label htmlFor="register-username">{t('login.usernameLabel')}</Label>
                        <Input
                          id="register-username"
                          type="text"
                          value={registerUsername}
                          placeholder={t('login.usernamePlaceholder')}
                          autoComplete="username"
                          disabled={anyLoading}
                          className="h-11"
                          aria-invalid={registerFieldErrors.username ? 'true' : undefined}
                          onChange={(e) => {
                            setRegisterUsername(e.target.value);
                            setRegisterFieldErrors((current) => ({ ...current, username: undefined }));
                            setRegisterFormError('');
                            setSuccessMessage('');
                          }}
                        />
                        <FieldError message={registerFieldErrors.username} />
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="register-password">{t('login.passwordLabel')}</Label>
                        <div className="relative">
                          <PasswordInput
                            id="register-password"
                            value={registerPassword}
                            placeholder={t('login.passwordPlaceholder')}
                            autoComplete="new-password"
                            disabled={anyLoading}
                            className="h-11"
                            aria-invalid={registerFieldErrors.password ? 'true' : undefined}
                            onFocus={() => setShowRegisterPasswordRules(true)}
                            onBlur={() => setShowRegisterPasswordRules(false)}
                            onChange={(e) => {
                              const nextPassword = e.target.value;
                              const nextPasswordError = getRegisterPasswordError(nextPassword, t);
                              setRegisterPassword(nextPassword);
                              setShowRegisterPasswordRules(true);
                              setRegisterFieldErrors((current) => ({
                                ...current,
                                password: nextPasswordError,
                                confirmPassword:
                                  confirmPassword && nextPassword !== confirmPassword
                                    ? t('login.passwordMismatch')
                                    : undefined,
                              }));
                              setRegisterFormError('');
                            }}
                            visible={registerPasswordsVisible}
                            onVisibleChange={setRegisterPasswordsVisible}
                          />
                          <PasswordRulesPopover
                            open={showRegisterPasswordRules}
                            ruleState={registerPasswordRuleState}
                            title={t('login.passwordChecklistTitle')}
                            progressLabel={t('login.passwordCategoryProgress', {
                              count: registerPasswordRuleState.categoryCount,
                            })}
                            minLengthLabel={t('login.passwordRuleMinLength')}
                            numberLabel={t('login.passwordRuleNumber')}
                            letterLabel={t('login.passwordRuleLetter')}
                            punctuationLabel={t('login.passwordRulePunctuation')}
                            className="sm:left-auto sm:right-0"
                          />
                        </div>
                        <FieldError message={registerPasswordFieldError} />
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="register-confirm-password">{t('login.confirmPasswordLabel')}</Label>
                        <PasswordInput
                          id="register-confirm-password"
                          value={confirmPassword}
                          placeholder={t('login.confirmPasswordPlaceholder')}
                          autoComplete="new-password"
                          disabled={anyLoading}
                          className="h-11"
                          aria-invalid={registerFieldErrors.confirmPassword ? 'true' : undefined}
                          onChange={(e) => {
                            const nextConfirmPassword = e.target.value;
                            setConfirmPassword(nextConfirmPassword);
                            setRegisterFieldErrors((current) => ({
                              ...current,
                              confirmPassword:
                                nextConfirmPassword && registerPassword !== nextConfirmPassword
                                  ? t('login.passwordMismatch')
                                  : undefined,
                            }));
                            setRegisterFormError('');
                          }}
                          visible={registerPasswordsVisible}
                          onVisibleChange={setRegisterPasswordsVisible}
                        />
                        <FieldError message={registerFieldErrors.confirmPassword} />
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="register-invite-code">{t('login.inviteCodeLabel')}</Label>
                        <Input
                          id="register-invite-code"
                          type="text"
                          value={inviteCode}
                          placeholder={t('login.inviteCodePlaceholder')}
                          autoCapitalize="off"
                          autoCorrect="off"
                          autoComplete="off"
                          spellCheck={false}
                          disabled={anyLoading}
                          className="h-11"
                          aria-invalid={registerFieldErrors.inviteCode ? 'true' : undefined}
                          onChange={(e) => {
                            setInviteCode(e.target.value);
                            setRegisterFieldErrors((current) => ({ ...current, inviteCode: undefined }));
                            setRegisterFormError('');
                          }}
                        />
                        <FieldError message={registerFieldErrors.inviteCode} />
                      </div>

                      <Button
                        type="submit"
                        className="w-full"
                        size="lg"
                        disabled={isRegisterSubmitDisabled}
                      >
                        {isRegisterLoading ? t('login.registering') : t('login.register')}
                      </Button>
                    </form>
                  </TabsContent>
                </Tabs>
              </CardContent>
            </Card>
          </section>
        </div>
      </div>

      <Dialog open={forgotPasswordOpen} onOpenChange={setForgotPasswordOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('login.forgotPasswordTitle')}</DialogTitle>
            <DialogDescription>{t('login.forgotPasswordDescription')}</DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            <div className="rounded-lg border border-border/70 bg-muted/30 p-3">
              <p className="text-sm font-medium">{t('login.forgotPasswordTip')}</p>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setForgotPasswordOpen(false)}>
              {t('common.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
