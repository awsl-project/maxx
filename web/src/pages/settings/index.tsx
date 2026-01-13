import { Settings, Moon, Sun, Monitor, Laptop } from 'lucide-react'
import { useTheme } from '@/components/theme-provider'
import { Card, CardContent, CardHeader, CardTitle, Button } from '@/components/ui'
import { PageHeader } from '@/components/layout/page-header'

type Theme = 'light' | 'dark' | 'system'

export function SettingsPage() {
  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Settings}
        iconClassName="text-zinc-500"
        title="Settings"
        description="Configure your maxx-next instance"
      />

      <div className="flex-1 overflow-y-auto p-6">
        <div className="space-y-6">
          <AppearanceSection />
        </div>
      </div>
    </div>
  )
}

function AppearanceSection() {
  const { theme, setTheme } = useTheme()

  const themes: { value: Theme; label: string; icon: typeof Sun }[] = [
    { value: 'light', label: 'Light', icon: Sun },
    { value: 'dark', label: 'Dark', icon: Moon },
    { value: 'system', label: 'System', icon: Laptop },
  ]

  return (
    <Card className="border-border bg-surface-primary">
      <CardHeader className="border-b border-border py-4">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Monitor className="h-4 w-4 text-text-muted" />
          Appearance
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6">
        <div className="flex items-center gap-6">
          <label className="text-sm font-medium text-text-secondary w-40 shrink-0">
            Theme Preference
          </label>
          <div className="flex flex-wrap gap-3">
            {themes.map(({ value, label, icon: Icon }) => (
              <Button
                key={value}
                onClick={() => setTheme(value)}
                variant={theme === value ? 'default' : 'outline'}
              >
                <Icon size={16} />
                <span className="text-sm font-medium">{label}</span>
              </Button>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default SettingsPage
