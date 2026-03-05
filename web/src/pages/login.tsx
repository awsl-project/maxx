import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useTransport } from '@/lib/transport';
import type { AuthUser } from '@/lib/auth-context';

interface LoginPageProps {
  onSuccess: (token: string, user?: AuthUser) => void;
}

function decodeBase64URL(value: string): ArrayBuffer {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4);
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

function encodeBase64URL(buffer: ArrayBuffer | null): string | undefined {
  if (!buffer) {
    return undefined;
  }
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function normalizePasskeyRegisterOptions(rawOptions: Record<string, unknown>): PublicKeyCredentialCreationOptions {
  const options = JSON.parse(JSON.stringify(rawOptions)) as Record<string, any>;
  options.challenge = decodeBase64URL(options.challenge);
  options.user.id = decodeBase64URL(options.user.id);
  if (Array.isArray(options.excludeCredentials)) {
    options.excludeCredentials = options.excludeCredentials.map((item: Record<string, any>) => ({
      ...item,
      id: decodeBase64URL(item.id),
    }));
  }
  return options as unknown as PublicKeyCredentialCreationOptions;
}

function normalizePasskeyLoginOptions(rawOptions: Record<string, unknown>): PublicKeyCredentialRequestOptions {
  const options = JSON.parse(JSON.stringify(rawOptions)) as Record<string, any>;
  options.challenge = decodeBase64URL(options.challenge);
  if (Array.isArray(options.allowCredentials)) {
    options.allowCredentials = options.allowCredentials.map((item: Record<string, any>) => ({
      ...item,
      id: decodeBase64URL(item.id),
    }));
  }
  return options as unknown as PublicKeyCredentialRequestOptions;
}

function publicKeyCredentialToJSON(credential: PublicKeyCredential): Record<string, unknown> {
  const response = credential.response as AuthenticatorAttestationResponse | AuthenticatorAssertionResponse;
  const payload: Record<string, unknown> = {
    id: credential.id,
    type: credential.type,
    rawId: encodeBase64URL(credential.rawId),
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: encodeBase64URL(response.clientDataJSON),
    },
  };

  const responsePayload = payload.response as Record<string, unknown>;

  if ('attestationObject' in response) {
    responsePayload.attestationObject = encodeBase64URL(response.attestationObject);
    if (typeof response.getTransports === 'function') {
      responsePayload.transports = response.getTransports();
    }
  }

  if ('authenticatorData' in response) {
    responsePayload.authenticatorData = encodeBase64URL(response.authenticatorData);
  }
  if ('signature' in response) {
    responsePayload.signature = encodeBase64URL(response.signature);
  }
  if ('userHandle' in response && response.userHandle) {
    responsePayload.userHandle = encodeBase64URL(response.userHandle);
  }

  return payload;
}

function isPasskeySupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.PublicKeyCredential !== 'undefined' &&
    typeof navigator !== 'undefined' &&
    typeof navigator.credentials?.create === 'function' &&
    typeof navigator.credentials?.get === 'function'
  );
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
  const passkeySupported = isPasskeySupported();

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

  const handlePasskeyLogin = async () => {
    setError('');
    setSuccessMessage('');

    if (!passkeySupported) {
      setError(t('login.passkeyNotSupported'));
      return;
    }
    if (!username) {
      setError(t('login.passkeyUsernameRequired'));
      return;
    }

    setIsLoading(true);
    try {
      const beginResult = await transport.startPasskeyLogin(username);
      if (!beginResult.success || !beginResult.sessionID || !beginResult.options) {
        setError(beginResult.error || t('login.passkeyLoginFailed'));
        return;
      }

      const publicKey = normalizePasskeyLoginOptions(beginResult.options);
      const credential = await navigator.credentials.get({ publicKey });
      if (!credential) {
        setError(t('login.passkeyLoginFailed'));
        return;
      }

      const finishResult = await transport.finishPasskeyLogin(
        beginResult.sessionID,
        publicKeyCredentialToJSON(credential as PublicKeyCredential),
      );
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
      setError(finishResult.error || t('login.passkeyLoginFailed'));
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string }, status?: number } };
      const errorMsg = axiosError?.response?.data?.error;
      if (errorMsg === 'account pending approval') {
        setError(t('login.pendingApproval'));
      } else if (axiosError?.response?.status === 401) {
        setError(t('login.invalidCredentials'));
      } else {
        setError(errorMsg || t('login.passkeyLoginFailed'));
      }
    } finally {
      setIsLoading(false);
    }
  };

  const handlePasskeyRegister = async () => {
    setError('');
    setSuccessMessage('');

    if (!passkeySupported) {
      setError(t('login.passkeyNotSupported'));
      return;
    }
    if (!username || !password) {
      setError(t('login.passkeyRegistrationNeedsPassword'));
      return;
    }

    setIsLoading(true);
    try {
      const beginResult = await transport.startPasskeyRegistration(username, password);
      if (!beginResult.success || !beginResult.sessionID || !beginResult.options) {
        setError(beginResult.error || t('login.passkeyRegisterFailed'));
        return;
      }

      const publicKey = normalizePasskeyRegisterOptions(beginResult.options);
      const credential = await navigator.credentials.create({ publicKey });
      if (!credential) {
        setError(t('login.passkeyRegisterFailed'));
        return;
      }

      const finishResult = await transport.finishPasskeyRegistration(
        beginResult.sessionID,
        publicKeyCredentialToJSON(credential as PublicKeyCredential),
      );
      if (finishResult.success) {
        setSuccessMessage(t('login.passkeyRegisterSuccess'));
      } else {
        setError(finishResult.error || t('login.passkeyRegisterFailed'));
      }
    } catch (err: unknown) {
      const axiosError = err as { response?: { data?: { error?: string } } };
      setError(axiosError?.response?.data?.error || t('login.passkeyRegisterFailed'));
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
  const isPasskeyLoginDisabled = isLoading || !username;
  const isPasskeyRegisterDisabled = isLoading || !username || !password;

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

          {passkeySupported && (
            <Button
              type="button"
              variant="secondary"
              className="w-full"
              onClick={handlePasskeyLogin}
              disabled={isPasskeyLoginDisabled}
            >
              {isLoading ? t('login.verifying') : t('login.passkeyLogin')}
            </Button>
          )}

          {passkeySupported && (
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={handlePasskeyRegister}
              disabled={isPasskeyRegisterDisabled}
            >
              {isLoading ? t('login.verifying') : t('login.passkeyRegister')}
            </Button>
          )}

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
