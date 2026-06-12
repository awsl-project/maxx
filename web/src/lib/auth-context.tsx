import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useTransport } from '@/lib/transport';
import type { UserRole } from '@/lib/transport/types';

const AUTH_TOKEN_KEY = 'maxx-admin-token';
const AUTH_INIT_TIMEOUT_MS = 8000;

export interface AuthUser {
  id: number;
  username: string;
  tenantID: number;
  tenantName?: string;
  role: UserRole;
}

interface AuthContextValue {
  isAuthenticated: boolean;
  isLoading: boolean;
  authEnabled: boolean;
  user: AuthUser | null;
  login: (token: string, user?: AuthUser) => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const { transport } = useTransport();
  const queryClient = useQueryClient();
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [authEnabled, setAuthEnabled] = useState(false);
  const [user, setUser] = useState<AuthUser | null>(null);

  const resetClientState = useCallback(() => {
    queryClient.clear();
  }, [queryClient]);

  useEffect(() => {
    let cancelled = false;
    let timedOut = false;
    let timeoutId: ReturnType<typeof setTimeout> | null = null;

    const shouldSkip = () => cancelled || timedOut;

    const checkAuth = async () => {
      try {
        const savedToken = localStorage.getItem(AUTH_TOKEN_KEY);
        if (savedToken) {
          transport.setAuthToken(savedToken);
        }

        const status = await transport.getAuthStatus();
        if (shouldSkip()) {
          return;
        }
        setAuthEnabled(status.authEnabled);

        // If auth is disabled, auto-authenticate as admin
        if (!status.authEnabled) {
          setIsAuthenticated(true);
          setUser({
            id: 1,
            username: 'admin',
            tenantID: 1,
            role: 'admin',
          });
          return;
        }

        if (savedToken) {
          if (!status.user) {
            if (shouldSkip()) {
              return;
            }
            console.error(
              '[AuthProvider] Saved token verification failed: auth status returned no user',
            );
            localStorage.removeItem(AUTH_TOKEN_KEY);
            transport.clearAuthToken();
            resetClientState();
            return;
          }

          setIsAuthenticated(true);
          setUser({
            id: status.user.id,
            username: status.user.username ?? '',
            tenantID: status.user.tenantID,
            tenantName: status.user.tenantName,
            role: status.user.role,
          });
        }
      } catch (error) {
        if (shouldSkip()) {
          return;
        }
        console.error('[AuthProvider] Auth check failed:', error);
        // Fail closed: treat unknown state as auth-required so admin routes stay guarded
        setAuthEnabled(true);
        setIsAuthenticated(false);
        setUser(null);
      }
    };

    const runAuthBootstrap = async () => {
      console.log('[AuthProvider] Starting auth bootstrap...');

      try {
        await Promise.race([
          checkAuth(),
          new Promise<never>((_, reject) => {
            timeoutId = setTimeout(() => {
              reject(
                new Error(`[AuthProvider] Auth bootstrap timeout after ${AUTH_INIT_TIMEOUT_MS}ms`),
              );
            }, AUTH_INIT_TIMEOUT_MS);
          }),
        ]);
      } catch (error) {
        if (cancelled) {
          return;
        }

        timedOut = true;
        console.error('[AuthProvider] Auth bootstrap failed or timed out:', error);
        // Fail closed: treat unknown state as auth-required so admin routes stay guarded
        setAuthEnabled(true);
        setIsAuthenticated(false);
        setUser(null);
      } finally {
        if (timeoutId) {
          clearTimeout(timeoutId);
        }
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    };

    runAuthBootstrap();

    return () => {
      cancelled = true;
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
    };
  }, [resetClientState, transport]);

  const login = useCallback(
    (token: string, userInfo?: AuthUser) => {
      resetClientState();
      localStorage.setItem(AUTH_TOKEN_KEY, token);
      transport.setAuthToken(token);
      if (userInfo) {
        setUser(userInfo);
      }
      setIsAuthenticated(true);
    },
    [resetClientState, transport],
  );

  const logout = useCallback(() => {
    resetClientState();
    localStorage.removeItem(AUTH_TOKEN_KEY);
    transport.clearAuthToken();
    setUser(null);
    setIsAuthenticated(false);
  }, [resetClientState, transport]);

  return (
    <AuthContext.Provider value={{ isAuthenticated, isLoading, authEnabled, user, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return context;
}
