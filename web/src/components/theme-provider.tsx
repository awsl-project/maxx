import { createContext, useContext, useEffect, useState } from 'react';
import { type Theme, getThemeBaseMode, isLuxuryTheme, THEME_REGISTRY } from '@/lib/theme';

type ThemeProviderProps = {
  children: React.ReactNode;
  defaultTheme?: Theme;
  storageKey?: string;
};

type ThemeProviderState = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
  effectiveTheme: 'light' | 'dark';
};

const initialState: ThemeProviderState = {
  theme: 'system',
  setTheme: () => null,
  toggleTheme: () => null,
  effectiveTheme: 'light',
};

const ThemeProviderContext = createContext<ThemeProviderState>(initialState);

export function ThemeProvider({
  children,
  defaultTheme = 'system',
  storageKey = 'maxx-ui-theme',
  ...props
}: ThemeProviderProps) {
  const [theme, setTheme] = useState<Theme>(() => {
    const storedTheme = localStorage.getItem(storageKey) as Theme;

    // Validate stored theme exists in registry
    if (storedTheme && storedTheme in THEME_REGISTRY) {
      return storedTheme;
    }

    // If invalid theme found, clean up localStorage and use default
    if (storedTheme) {
      console.warn(`Invalid theme "${storedTheme}" found in localStorage. Resetting to default.`);
      localStorage.removeItem(storageKey);
    }

    return defaultTheme;
  });

  const [effectiveTheme, setEffectiveTheme] = useState<'light' | 'dark'>('light');

  useEffect(() => {
    const root = window.document.documentElement;

    // Remove all theme classes
    root.classList.remove('light', 'dark');
    const luxuryClasses = [
      'theme-hermes',
      'theme-tiffany',
      'theme-chanel',
      'theme-cartier',
      'theme-burberry',
      'theme-gucci',
      'theme-dior',
    ];
    luxuryClasses.forEach((cls) => root.classList.remove(cls));

    // Handle system theme
    if (theme === 'system') {
      const systemTheme = window.matchMedia('(prefers-color-scheme: dark)').matches
        ? 'dark'
        : 'light';
      root.classList.add(systemTheme);
      setEffectiveTheme(systemTheme);
      return;
    }

    // Handle luxury themes
    if (isLuxuryTheme(theme)) {
      root.classList.add(`theme-${theme}`);
      const baseMode = getThemeBaseMode(theme);
      setEffectiveTheme(baseMode);
      // Also add dark class for Tailwind dark mode utilities
      if (baseMode === 'dark') {
        root.classList.add('dark');
      }
      return;
    }

    // Handle default light/dark
    root.classList.add(theme);
    setEffectiveTheme(theme as 'light' | 'dark');
  }, [theme]);

  // Listen for system theme changes
  useEffect(() => {
    if (theme !== 'system') return;

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handleChange = () => {
      const root = window.document.documentElement;
      root.classList.remove('light', 'dark');
      const systemTheme = mediaQuery.matches ? 'dark' : 'light';
      root.classList.add(systemTheme);
      setEffectiveTheme(systemTheme);
    };

    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, [theme]);

  const value = {
    theme,
    effectiveTheme,
    setTheme: (theme: Theme) => {
      localStorage.setItem(storageKey, theme);
      setTheme(theme);
    },
    toggleTheme: () => {
      // Toggle between light and dark for default themes
      if (theme === 'dark') {
        const newTheme = 'light';
        localStorage.setItem(storageKey, newTheme);
        setTheme(newTheme);
      } else if (theme === 'light') {
        const newTheme = 'dark';
        localStorage.setItem(storageKey, newTheme);
        setTheme(newTheme);
      }
      // For luxury themes, toggle to opposite base mode default theme
      else if (isLuxuryTheme(theme)) {
        const baseMode = getThemeBaseMode(theme);
        const newTheme = baseMode === 'dark' ? 'light' : 'dark';
        localStorage.setItem(storageKey, newTheme);
        setTheme(newTheme);
      }
    },
  };

  return (
    <ThemeProviderContext.Provider {...props} value={value}>
      {children}
    </ThemeProviderContext.Provider>
  );
}

export const useTheme = () => {
  const context = useContext(ThemeProviderContext);

  if (context === undefined) throw new Error('useTheme must be used within a ThemeProvider');

  return context;
};
