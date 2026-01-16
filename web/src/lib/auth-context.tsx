import { createContext, useContext, useState, useEffect, type ReactNode } from 'react';
import { useTransport } from '@/lib/transport';

const AUTH_TOKEN_KEY = 'maxx-admin-token';

interface AuthContextValue {
  isAuthenticated: boolean;
  isLoading: boolean;
  authEnabled: boolean;
  login: (token: string) => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const { transport } = useTransport();
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [authEnabled, setAuthEnabled] = useState(false);

  useEffect(() => {
    const checkAuth = async () => {
      try {
        const status = await transport.getAuthStatus();
        setAuthEnabled(status.authEnabled);

        if (!status.authEnabled) {
          setIsAuthenticated(true);
          setIsLoading(false);
          return;
        }

        const savedToken = localStorage.getItem(AUTH_TOKEN_KEY);
        if (savedToken) {
          transport.setAuthToken(savedToken);
          try {
            await transport.getProxyStatus();
            setIsAuthenticated(true);
          } catch {
            localStorage.removeItem(AUTH_TOKEN_KEY);
            transport.clearAuthToken();
          }
        }
      } catch {
        // Auth check failed, assume no auth required
        setIsAuthenticated(true);
      } finally {
        setIsLoading(false);
      }
    };

    checkAuth();
  }, [transport]);

  const login = (token: string) => {
    localStorage.setItem(AUTH_TOKEN_KEY, token);
    transport.setAuthToken(token);
    setIsAuthenticated(true);
  };

  const logout = () => {
    localStorage.removeItem(AUTH_TOKEN_KEY);
    transport.clearAuthToken();
    setIsAuthenticated(false);
  };

  return (
    <AuthContext.Provider value={{ isAuthenticated, isLoading, authEnabled, login, logout }}>
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
