import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useTransport } from '@/lib/transport';
import type { AuthUser } from '@/lib/auth-context';

interface LoginPageProps {
  onSuccess: (token: string, user?: AuthUser) => void;
  multiTenancyEnabled?: boolean;
}

export function LoginPage({ onSuccess, multiTenancyEnabled }: LoginPageProps) {
  const { t } = useTranslation();
  const { transport } = useTransport();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setIsLoading(true);

    try {
      if (multiTenancyEnabled) {
        // Multi-tenancy: username + password login
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
          setError(result.error || t('login.invalidCredentials'));
        }
      } else {
        // Legacy: password-only login
        const result = await transport.verifyPassword(password);
        if (result.success && result.token) {
          onSuccess(result.token);
        } else {
          setError(result.error || t('login.invalidPassword'));
        }
      }
    } catch {
      setError(t('login.verifyFailed'));
    } finally {
      setIsLoading(false);
    }
  };

  const isSubmitDisabled = multiTenancyEnabled
    ? isLoading || !username || !password
    : isLoading || !password;

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-sm space-y-6 p-6">
        <div className="space-y-2 text-center">
          <h1 className="text-2xl font-bold">{t('login.title')}</h1>
          <p className="text-muted-foreground text-sm">
            {multiTenancyEnabled ? t('login.descriptionMultiUser') : t('login.description')}
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            {multiTenancyEnabled && (
              <Input
                type="text"
                placeholder={t('login.usernamePlaceholder')}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoFocus
                disabled={isLoading}
              />
            )}
            <Input
              type="password"
              placeholder={t('login.passwordPlaceholder')}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus={!multiTenancyEnabled}
              disabled={isLoading}
            />
            {error && <p className="text-destructive text-sm">{error}</p>}
          </div>

          <Button type="submit" className="w-full" disabled={isSubmitDisabled}>
            {isLoading ? t('login.verifying') : t('login.submit')}
          </Button>
        </form>
      </div>
    </div>
  );
}
