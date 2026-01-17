import { Settings, Moon, Sun, Monitor, Laptop, FolderOpen, Languages } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useTheme } from '@/components/theme-provider'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Switch,
} from '@/components/ui'
import { PageHeader } from '@/components/layout/page-header'
import { useSettings, useUpdateSetting } from '@/hooks/queries'

type Theme = 'light' | 'dark' | 'system'

export function SettingsPage() {
  const { t } = useTranslation()

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Settings}
        iconClassName="text-zinc-500"
        title={t('settings.title')}
        description={t('settings.description')}
      />

      <div className="flex-1 overflow-y-auto p-6">
        <div className="space-y-6">
          <AppearanceSection />
          <LanguageSection />
          <ForceProjectSection />
        </div>
      </div>
    </div>
  )
}

function AppearanceSection() {
  const { theme, setTheme } = useTheme()
  const { t } = useTranslation()

  const themes: { value: Theme; label: string; icon: typeof Sun }[] = [
    { value: 'light', label: t('settings.theme.light'), icon: Sun },
    { value: 'dark', label: t('settings.theme.dark'), icon: Moon },
    { value: 'system', label: t('settings.theme.system'), icon: Laptop },
  ]

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Monitor className="h-4 w-4 text-muted-foreground" />
          {t('settings.appearance')}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6">
        <div className="flex items-center gap-6">
          <label className="text-sm font-medium text-muted-foreground w-40 shrink-0">
            {t('settings.themePreference')}
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

function LanguageSection() {
  const { t, i18n } = useTranslation()

  const languages = [
    { value: 'en', label: t('settings.languages.en') },
    { value: 'zh', label: t('settings.languages.zh') },
  ]

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Languages className="h-4 w-4 text-muted-foreground" />
          {t('settings.language')}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6">
        <div className="flex items-center gap-6">
          <label className="text-sm font-medium text-muted-foreground w-40 shrink-0">
            {t('settings.languagePreference')}
          </label>
          <div className="flex flex-wrap gap-3">
            {languages.map(({ value, label }) => (
              <Button
                key={value}
                onClick={() => i18n.changeLanguage(value)}
                variant={i18n.language === value ? 'default' : 'outline'}
              >
                <span className="text-sm font-medium">{label}</span>
              </Button>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function ForceProjectSection() {
  const { data: settings, isLoading } = useSettings()
  const updateSetting = useUpdateSetting()
  const { t } = useTranslation()

  const forceProjectEnabled = settings?.force_project_binding === 'true'
  const timeout = settings?.force_project_timeout || '30'

  const handleToggle = async (checked: boolean) => {
    await updateSetting.mutateAsync({
      key: 'force_project_binding',
      value: checked ? 'true' : 'false',
    })
  }

  const handleTimeoutChange = async (value: string) => {
    const numValue = parseInt(value, 10)
    if (numValue >= 5 && numValue <= 300) {
      await updateSetting.mutateAsync({
        key: 'force_project_timeout',
        value: value,
      })
    }
  }

  if (isLoading) return null

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <FolderOpen className="h-4 w-4 text-muted-foreground" />
          {t('settings.forceProjectBinding')}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <label className="text-sm font-medium text-foreground">
              {t('settings.enableForceProjectBinding')}
            </label>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.forceProjectBindingDesc')}
            </p>
          </div>
          <Switch
            checked={forceProjectEnabled}
            onCheckedChange={handleToggle}
            disabled={updateSetting.isPending}
          />
        </div>

        {forceProjectEnabled && (
          <div className="flex items-center gap-6 pt-4 border-t border-border">
            <label className="text-sm font-medium text-muted-foreground w-32 shrink-0">
              {t('settings.waitTimeout')}
            </label>
            <Input
              type="number"
              value={timeout}
              onChange={e => handleTimeoutChange(e.target.value)}
              className="w-24"
              min={5}
              max={300}
              disabled={updateSetting.isPending}
            />
            <span className="text-xs text-muted-foreground">{t('settings.waitTimeoutRange')}</span>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export default SettingsPage
