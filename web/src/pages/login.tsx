import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useTransport } from '@/lib/transport';
import type { AuthUser } from '@/lib/auth-context';

interface LoginPageProps {
  onSuccess: (token: string, user?: AuthUser) => void;
}

export function LoginPage({ onSuccess }: LoginPageProps) {
  const { t } = useTranslation();
  const { transport } = useTransport();
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [successMessage, setSuccessMessage] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleLogin = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccessMessage('');
    setIsLoading(true);

    try {
      const result = await transport.login(username, password);
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
      } else {
        if (result.error === 'account pending approval') {
          setError(t('login.pendingApproval'));
        } else {
          setError(result.error || t('login.invalidCredentials'));
        }
      }
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string }, status?: number } };
      const errorMsg = axiosError?.response?.data?.error;
      if (errorMsg === 'account pending approval') {
        setError(t('login.pendingApproval'));
      } else if (axiosError?.response?.status === 401) {
        setError(t('login.invalidCredentials'));
      } else {
        setError(errorMsg || t('login.invalidCredentials'));
      }
    } finally {
      setIsLoading(false);
    }
  };

  const handleRegister = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccessMessage('');

    if (password !== confirmPassword) {
      setError(t('login.passwordMismatch'));
      return;
    }

    setIsLoading(true);

    try {
      const result = await transport.apply(username, password);
      if (result.success) {
        setSuccessMessage(t('login.registerSuccess'));
        setMode('login');
        setPassword('');
        setConfirmPassword('');
      } else {
        setError(result.error || t('login.registerFailed'));
      }
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string } } };
      setError(axiosError?.response?.data?.error || t('login.registerFailed'));
    } finally {
      setIsLoading(false);
    }
  };

  if (mode === 'register') {
    const isRegisterDisabled = isLoading || !username || !password || !confirmPassword;

    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="w-full max-w-sm space-y-6 p-6">
          <div className="space-y-2 text-center">
            <h1 className="text-2xl font-bold">{t('login.registerTitle')}</h1>
            <p className="text-muted-foreground text-sm">{t('login.registerDescription')}</p>
          </div>

          <form onSubmit={handleRegister} className="space-y-4">
            <div className="space-y-2">
              <Input
                type="text"
                placeholder={t('login.usernamePlaceholder')}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoFocus
                disabled={isLoading}
              />
              <Input
                type="password"
                placeholder={t('login.passwordPlaceholder')}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={isLoading}
              />
              <Input
                type="password"
                placeholder={t('login.confirmPasswordPlaceholder')}
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                disabled={isLoading}
              />
              {error && <p className="text-destructive text-sm">{error}</p>}
            </div>

            <Button type="submit" className="w-full" disabled={isRegisterDisabled}>
              {isLoading ? t('login.registering') : t('login.register')}
            </Button>

            <Button
              type="button"
              variant="ghost"
              className="w-full"
              onClick={() => { setMode('login'); setError(''); }}
            >
              {t('login.backToLogin')}
            </Button>
          </form>
        </div>
      </div>
    );
  }

  const isSubmitDisabled = isLoading || !username || !password;

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-sm space-y-6 p-6">
        <div className="space-y-2 text-center">
          <h1 className="text-2xl font-bold">{t('login.title')}</h1>
          <p className="text-muted-foreground text-sm">
            {t('login.descriptionMultiUser')}
          </p>
        </div>

        <form onSubmit={handleLogin} className="space-y-4">
          <div className="space-y-2">
            <Input
              type="text"
              placeholder={t('login.usernamePlaceholder')}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoFocus
              disabled={isLoading}
            />
            <Input
              type="password"
              placeholder={t('login.passwordPlaceholder')}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={isLoading}
            />
            {error && <p className="text-destructive text-sm">{error}</p>}
            {successMessage && <p className="text-green-600 dark:text-green-400 text-sm">{successMessage}</p>}
          </div>

          <Button type="submit" className="w-full" disabled={isSubmitDisabled}>
            {isLoading ? t('login.verifying') : t('login.submit')}
          </Button>

          <Button
            type="button"
            variant="ghost"
            className="w-full"
            onClick={() => { setMode('register'); setError(''); setSuccessMessage(''); }}
          >
            {t('login.register')}
          </Button>
        </form>
      </div>
    </div>
  );
}
