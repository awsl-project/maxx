import { useState, type FormEvent } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useTransport } from '@/lib/transport';

interface LoginPageProps {
  onSuccess: (token: string) => void;
}

export function LoginPage({ onSuccess }: LoginPageProps) {
  const { transport } = useTransport();
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setIsLoading(true);

    try {
      const result = await transport.verifyPassword(password);
      if (result.success && result.token) {
        onSuccess(result.token);
      } else {
        setError(result.error || 'Invalid password');
      }
    } catch {
      setError('Failed to verify password');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-sm space-y-6 p-6">
        <div className="space-y-2 text-center">
          <h1 className="text-2xl font-bold">Maxx Admin</h1>
          <p className="text-muted-foreground text-sm">
            Enter password to access the admin panel
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Input
              type="password"
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus
              disabled={isLoading}
            />
            {error && (
              <p className="text-destructive text-sm">{error}</p>
            )}
          </div>

          <Button
            type="submit"
            className="w-full"
            disabled={isLoading || !password}
          >
            {isLoading ? 'Verifying...' : 'Login'}
          </Button>
        </form>
      </div>
    </div>
  );
}
