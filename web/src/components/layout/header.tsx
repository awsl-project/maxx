import { Moon, Sun } from 'lucide-react';
import { Button } from '@/components/ui';
import { useTheme } from '@/hooks/use-theme';

export function Header() {
  const { theme, toggleTheme } = useTheme();

  return (
    <header className="flex h-14 items-center justify-between border-b border-gray-200 bg-[#faf8f5] px-6 dark:border-gray-800 dark:bg-gray-900">
      <h1 className="text-lg font-semibold">Maxx Next</h1>
      <Button variant="ghost" size="sm" onClick={toggleTheme}>
        {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
      </Button>
    </header>
  );
}
