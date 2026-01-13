import { Moon, Sun } from 'lucide-react'
import { Button } from '@/components/ui'
import { useTheme } from '@/components/theme-provider'

export function Header() {
  const { theme, toggleTheme } = useTheme()

  return (
    <header className="flex h-14 items-center justify-between border-b border-border bg-surface-primary px-6">
      <h1 className="text-lg font-semibold">Maxx Next</h1>
      <Button variant="ghost" size="sm" onClick={toggleTheme}>
        {theme === 'dark' ? (
          <Sun className="h-4 w-4" />
        ) : (
          <Moon className="h-4 w-4" />
        )}
      </Button>
    </header>
  )
}
