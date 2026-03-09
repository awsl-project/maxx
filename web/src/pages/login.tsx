import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { browserSupportsWebAuthn, startAuthentication } from '@simplewebauthn/browser';
import { ArrowRight, Fingerprint, KeyRound, LifeBuoy, ShieldCheck, UserPlus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
import { useTransport } from '@/lib/transport';
import type { AuthUser } from '@/lib/auth-context';

interface LoginPageProps {
  onSuccess: (token: string, user?: AuthUser) => void;
}

type LoginErrors = {
  username?: string;
  password?: string;
  form?: string;
};

type RegisterErrors = {
  username?: string;
  password?: string;
  confirmPassword?: string;
  form?: string;
};

type AuthAction = 'login' | 'passkey' | 'register' | null;

export function LoginPage({ onSuccess }: LoginPageProps) {
  const { t } = useTranslation();
  const { transport } = useTransport();
  const passkeySupported = browserSupportsWebAuthn();

  const [loginUsername, setLoginUsername] = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [registerUsername, setRegisterUsername] = useState('');
  const [registerPassword, setRegisterPassword] = useState('');
  const [registerConfirmPassword, setRegisterConfirmPassword] = useState('');
  const [loginErrors, setLoginErrors] = useState<LoginErrors>({});
  const [registerErrors, setRegisterErrors] = useState<RegisterErrors>({});
  const [successMessage, setSuccessMessage] = useState('');
  const [activeAction, setActiveAction] = useState<AuthAction>(null);
  const [isRegisterOpen, setIsRegisterOpen] = useState(false);
  const [isForgotPasswordOpen, setIsForgotPasswordOpen] = useState(false);

  const handleLogin = async (e: FormEvent) => {
    e.preventDefault();
    setSuccessMessage('');

    const nextErrors: LoginErrors = {};
    const trimmedUsername = loginUsername.trim();

    if (!trimmedUsername) {
      nextErrors.username = t('login.usernameRequired');
    }
    if (!loginPassword) {
      nextErrors.password = t('login.passwordRequired');
    }

    if (Object.keys(nextErrors).length > 0) {
      setLoginErrors(nextErrors);
      return;
    }

    setLoginErrors({});
    setActiveAction('login');

    try {
      const result = await transport.login(trimmedUsername, loginPassword);
      if (result.success && result.token) {
        const user: AuthUser | undefined = result.user
          ? {
              id: result.user.id,
              username: result.user.username,
              tenantID: result.user.tenantID,
              tenantName: result.user.tenantName,
              role: result.user.role,
            }
          : undefined;
        onSuccess(result.token, user);
        return;
      }

      setLoginErrors({
        password: result.error || t('login.invalidCredentials'),
      });
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string }; status?: number } };
      const errorMsg = axiosError?.response?.data?.error;

      if (errorMsg === 'account pending approval') {
        setLoginErrors({ form: t('login.pendingApproval') });
      } else if (axiosError?.response?.status === 401 || errorMsg === 'invalid credentials') {
        setLoginErrors({ password: t('login.invalidCredentials') });
      } else {
        setLoginErrors({ form: errorMsg || t('login.invalidCredentials') });
      }
    } finally {
      setActiveAction(null);
    }
  };

  const handleRegister = async (e: FormEvent) => {
    e.preventDefault();

    const nextErrors: RegisterErrors = {};
    const trimmedUsername = registerUsername.trim();

    if (!trimmedUsername) {
      nextErrors.username = t('login.usernameRequired');
    }
    if (!registerPassword) {
      nextErrors.password = t('login.passwordRequired');
    }
    if (!registerConfirmPassword) {
      nextErrors.confirmPassword = t('login.confirmPasswordRequired');
    } else if (registerPassword !== registerConfirmPassword) {
      nextErrors.confirmPassword = t('login.passwordMismatch');
    }

    if (Object.keys(nextErrors).length > 0) {
      setRegisterErrors(nextErrors);
      return;
    }

    setRegisterErrors({});
    setActiveAction('register');

    try {
      const result = await transport.apply(trimmedUsername, registerPassword);
      if (result.success) {
        setSuccessMessage(t('login.registerSuccess'));
        setLoginUsername(trimmedUsername);
        setLoginPassword('');
        setRegisterUsername('');
        setRegisterPassword('');
        setRegisterConfirmPassword('');
        setIsRegisterOpen(false);
        return;
      }

      setRegisterErrors({ form: result.error || t('login.registerFailed') });
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string } } };
      const errorMsg = axiosError?.response?.data?.error;

      if (errorMsg === 'username already exists') {
        setRegisterErrors({ username: t('login.usernameTaken') });
      } else {
        setRegisterErrors({ form: errorMsg || t('login.registerFailed') });
      }
    } finally {
      setActiveAction(null);
    }
  };

  const handlePasskeyLogin = async () => {
    setSuccessMessage('');

    if (!passkeySupported) {
      setLoginErrors({ form: t('login.passkeyNotSupported') });
      return;
    }

    setLoginErrors({});
    setActiveAction('passkey');

    try {
      const beginResult = await transport.startPasskeyLogin(loginUsername.trim());
      if (!beginResult.success || !beginResult.sessionID || !beginResult.options) {
        setLoginErrors({ form: beginResult.error || t('login.passkeyLoginFailed') });
        return;
      }

      const asseResp = await startAuthentication({ optionsJSON: beginResult.options });
      const finishResult = await transport.finishPasskeyLogin(beginResult.sessionID, asseResp);

      if (finishResult.success && finishResult.token) {
        const user: AuthUser | undefined = finishResult.user
          ? {
              id: finishResult.user.id,
              username: finishResult.user.username,
              tenantID: finishResult.user.tenantID,
              tenantName: finishResult.user.tenantName,
              role: finishResult.user.role,
            }
          : undefined;
        onSuccess(finishResult.token, user);
        return;
      }

      setLoginErrors({ form: finishResult.error || t('login.passkeyLoginFailed') });
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string }; status?: number } };
      const errorMsg = axiosError?.response?.data?.error;

      if (errorMsg === 'account pending approval') {
        setLoginErrors({ form: t('login.pendingApproval') });
      } else if (axiosError?.response?.status === 401) {
        setLoginErrors({ form: t('login.passkeyLoginFailed') });
      } else {
        setLoginErrors({ form: errorMsg || t('login.passkeyLoginFailed') });
      }
    } finally {
      setActiveAction(null);
    }
  };

  return (
    <>
      <div className="min-h-screen bg-[linear-gradient(180deg,rgba(14,165,233,0.08),transparent_24%),linear-gradient(135deg,rgba(15,23,42,0.03),transparent_40%)]">
        <div className="mx-auto flex min-h-screen w-full max-w-6xl items-center px-4 py-8 sm:px-6 lg:px-8">
          <div className="grid w-full gap-6 lg:grid-cols-[minmax(0,1.15fr)_minmax(22rem,28rem)]">
            <Card className="border-border/70 bg-background/92 backdrop-blur-sm">
              <CardHeader className="space-y-4">
                <div className="inline-flex w-fit items-center gap-2 rounded-full border border-sky-200/70 bg-sky-50 px-3 py-1 text-xs font-medium text-sky-700">
                  <ShieldCheck className="h-3.5 w-3.5" />
                  {t('login.layoutBadge')}
                </div>
                <div className="space-y-3">
                  <CardTitle className="text-3xl font-semibold tracking-tight sm:text-4xl">
                    {t('login.title')}
                  </CardTitle>
                  <CardDescription className="max-w-2xl text-sm leading-6 sm:text-base">
                    {t('login.descriptionMultiUser')}
                  </CardDescription>
                </div>
              </CardHeader>

              <CardContent className="grid gap-4 md:grid-cols-2">
                <div className="rounded-2xl border border-border/70 bg-muted/40 p-5">
                  <div className="mb-4 flex items-start gap-3">
                    <div className="rounded-xl bg-background p-2 text-sky-600 shadow-sm">
                      <Fingerprint className="h-5 w-5" />
                    </div>
                    <div className="space-y-1">
                      <h2 className="font-medium">{t('login.passkeyPanelTitle')}</h2>
                      <p className="text-muted-foreground text-sm leading-6">
                        {t('login.passkeyPanelDescription')}
                      </p>
                    </div>
                  </div>
                  <div className="space-y-3">
                    <p className="text-muted-foreground text-sm leading-6">
                      {t('login.passkeyPanelHint')}
                    </p>
                    <p className="rounded-xl bg-background px-3 py-2 text-sm leading-6 shadow-sm">
                      {t('login.passkeyDiscoverableHint')}
                    </p>
                    <Button
                      type="button"
                      variant="secondary"
                      size="lg"
                      className="w-full justify-between"
                      onClick={handlePasskeyLogin}
                      disabled={!passkeySupported || activeAction !== null}
                    >
                      <span>
                        {activeAction === 'passkey'
                          ? t('login.verifying')
                          : t('login.passkeyLogin')}
                      </span>
                      <ArrowRight className="h-4 w-4" />
                    </Button>
                    {!passkeySupported && (
                      <p className="text-destructive text-sm">{t('login.passkeyNotSupported')}</p>
                    )}
                  </div>
                </div>

                <div className="space-y-4">
                  <div className="rounded-2xl border border-border/70 bg-muted/40 p-5">
                    <div className="mb-4 flex items-start gap-3">
                      <div className="rounded-xl bg-background p-2 text-emerald-600 shadow-sm">
                        <UserPlus className="h-5 w-5" />
                      </div>
                      <div className="space-y-1">
                        <h2 className="font-medium">{t('login.registerTitle')}</h2>
                        <p className="text-muted-foreground text-sm leading-6">
                          {t('login.registerDescription')}
                        </p>
                      </div>
                    </div>
                    <p className="text-muted-foreground mb-4 text-sm leading-6">
                      {t('login.registerApprovalHint')}
                    </p>
                    <Button
                      type="button"
                      variant="outline"
                      size="lg"
                      className="w-full justify-between"
                      onClick={() => {
                        setRegisterErrors({});
                        setIsRegisterOpen(true);
                      }}
                      disabled={activeAction !== null}
                    >
                      <span>{t('login.openRegister')}</span>
                      <ArrowRight className="h-4 w-4" />
                    </Button>
                  </div>

                  <div className="rounded-2xl border border-border/70 bg-muted/40 p-5">
                    <div className="mb-4 flex items-start gap-3">
                      <div className="rounded-xl bg-background p-2 text-amber-600 shadow-sm">
                        <LifeBuoy className="h-5 w-5" />
                      </div>
                      <div className="space-y-1">
                        <h2 className="font-medium">{t('login.forgotPasswordTitle')}</h2>
                        <p className="text-muted-foreground text-sm leading-6">
                          {t('login.forgotPasswordDescription')}
                        </p>
                      </div>
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      className="px-0 text-sm"
                      onClick={() => setIsForgotPasswordOpen(true)}
                    >
                      {t('login.forgotPassword')}
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card className="border-border/70 bg-background/96 shadow-lg backdrop-blur-sm">
              <CardHeader className="space-y-3">
                <div className="inline-flex w-fit items-center gap-2 rounded-full border border-border/70 bg-muted/50 px-3 py-1 text-xs font-medium">
                  <KeyRound className="h-3.5 w-3.5" />
                  {t('login.accountLoginTitle')}
                </div>
                <div className="space-y-1">
                  <CardTitle className="text-2xl font-semibold">
                    {t('login.accountLoginTitle')}
                  </CardTitle>
                  <CardDescription className="text-sm leading-6">
                    {t('login.accountLoginDescription')}
                  </CardDescription>
                </div>
              </CardHeader>

              <CardContent>
                <form onSubmit={handleLogin} className="space-y-5">
                  <div className="space-y-2">
                    <Label htmlFor="login-username">{t('login.usernameLabel')}</Label>
                    <Input
                      id="login-username"
                      type="text"
                      value={loginUsername}
                      onChange={(e) => {
                        setLoginUsername(e.target.value);
                        setLoginErrors({});
                        setSuccessMessage('');
                      }}
                      autoFocus
                      disabled={activeAction !== null}
                      aria-invalid={Boolean(loginErrors.username)}
                      aria-describedby={loginErrors.username ? 'login-username-error' : undefined}
                      placeholder={t('login.usernamePlaceholder')}
                      className="h-11"
                    />
                    {loginErrors.username && (
                      <p id="login-username-error" className="text-destructive text-sm">
                        {loginErrors.username}
                      </p>
                    )}
                  </div>

                  <div className="space-y-2">
                    <div className="flex items-center justify-between gap-3">
                      <Label htmlFor="login-password">{t('login.passwordLabel')}</Label>
                      <Button
                        type="button"
                        variant="link"
                        className="h-auto p-0 text-sm"
                        onClick={() => setIsForgotPasswordOpen(true)}
                      >
                        {t('login.forgotPassword')}
                      </Button>
                    </div>
                    <Input
                      id="login-password"
                      type="password"
                      value={loginPassword}
                      onChange={(e) => {
                        setLoginPassword(e.target.value);
                        setLoginErrors({});
                        setSuccessMessage('');
                      }}
                      disabled={activeAction !== null}
                      aria-invalid={Boolean(loginErrors.password)}
                      aria-describedby={
                        loginErrors.password
                          ? 'login-password-error login-password-hint'
                          : 'login-password-hint'
                      }
                      placeholder={t('login.passwordPlaceholder')}
                      className="h-11"
                    />
                    <p id="login-password-hint" className="text-muted-foreground text-sm">
                      {t('login.passwordCaseHint')}
                    </p>
                    {loginErrors.password && (
                      <p id="login-password-error" className="text-destructive text-sm">
                        {loginErrors.password}
                      </p>
                    )}
                  </div>

                  {loginErrors.form && (
                    <div className="rounded-xl border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                      {loginErrors.form}
                    </div>
                  )}

                  {successMessage && (
                    <div className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
                      {successMessage}
                    </div>
                  )}

                  <Button
                    type="submit"
                    size="lg"
                    className="w-full"
                    disabled={activeAction !== null}
                  >
                    {activeAction === 'login' ? t('login.verifying') : t('login.submit')}
                  </Button>
                </form>
              </CardContent>
            </Card>
          </div>
        </div>
      </div>

      <Dialog
        open={isRegisterOpen}
        onOpenChange={(open) => {
          setIsRegisterOpen(open);
          if (!open) {
            setRegisterErrors({});
          }
        }}
      >
        <DialogContent className="w-[min(32rem,calc(100vw-1.5rem))] max-w-[min(32rem,calc(100vw-1.5rem))]">
          <DialogHeader>
            <DialogTitle>{t('login.registerTitle')}</DialogTitle>
            <DialogDescription>{t('login.registerDescription')}</DialogDescription>
          </DialogHeader>

          <form onSubmit={handleRegister} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="register-username">{t('login.usernameLabel')}</Label>
              <Input
                id="register-username"
                type="text"
                value={registerUsername}
                onChange={(e) => {
                  setRegisterUsername(e.target.value);
                  setRegisterErrors((current) => ({
                    ...current,
                    username: undefined,
                    form: undefined,
                  }));
                }}
                disabled={activeAction !== null}
                aria-invalid={Boolean(registerErrors.username)}
                placeholder={t('login.usernamePlaceholder')}
              />
              {registerErrors.username && (
                <p className="text-destructive text-sm">{registerErrors.username}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="register-password">{t('login.passwordLabel')}</Label>
              <Input
                id="register-password"
                type="password"
                value={registerPassword}
                onChange={(e) => {
                  setRegisterPassword(e.target.value);
                  setRegisterErrors((current) => ({
                    ...current,
                    password: undefined,
                    form: undefined,
                  }));
                }}
                disabled={activeAction !== null}
                aria-invalid={Boolean(registerErrors.password)}
                placeholder={t('login.passwordPlaceholder')}
              />
              <p className="text-muted-foreground text-sm">{t('login.passwordRuleHint')}</p>
              {registerErrors.password && (
                <p className="text-destructive text-sm">{registerErrors.password}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="register-confirm-password">{t('login.confirmPasswordLabel')}</Label>
              <Input
                id="register-confirm-password"
                type="password"
                value={registerConfirmPassword}
                onChange={(e) => {
                  setRegisterConfirmPassword(e.target.value);
                  setRegisterErrors((current) => ({
                    ...current,
                    confirmPassword: undefined,
                    form: undefined,
                  }));
                }}
                disabled={activeAction !== null}
                aria-invalid={Boolean(registerErrors.confirmPassword)}
                placeholder={t('login.confirmPasswordPlaceholder')}
              />
              {registerErrors.confirmPassword && (
                <p className="text-destructive text-sm">{registerErrors.confirmPassword}</p>
              )}
            </div>

            <div className="rounded-xl border border-border/70 bg-muted/40 px-3 py-2 text-sm leading-6">
              {t('login.registerApprovalHint')}
            </div>

            {registerErrors.form && (
              <div className="rounded-xl border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                {registerErrors.form}
              </div>
            )}

            <DialogFooter className="pt-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setIsRegisterOpen(false)}
                disabled={activeAction !== null}
              >
                {t('login.backToLogin')}
              </Button>
              <Button type="submit" disabled={activeAction !== null}>
                {activeAction === 'register' ? t('login.registering') : t('login.register')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={isForgotPasswordOpen} onOpenChange={setIsForgotPasswordOpen}>
        <DialogContent className="w-[min(30rem,calc(100vw-1.5rem))] max-w-[min(30rem,calc(100vw-1.5rem))]">
          <DialogHeader>
            <DialogTitle>{t('login.forgotPasswordTitle')}</DialogTitle>
            <DialogDescription>{t('login.forgotPasswordDialogDescription')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 text-sm leading-6">
            <p className="rounded-xl border border-border/70 bg-muted/40 px-3 py-2">
              {t('login.forgotPasswordDialogHint')}
            </p>
            <p className="text-muted-foreground">{t('login.forgotPasswordDialogSupport')}</p>
          </div>
          <DialogFooter>
            <Button type="button" onClick={() => setIsForgotPasswordOpen(false)}>
              {t('login.backToLogin')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
