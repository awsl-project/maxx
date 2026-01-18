import { useState, useEffect } from 'react';
import { Settings, Moon, Sun, Monitor, Laptop, FolderOpen, Database, Globe } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@/components/theme-provider';
import { Card, CardContent, CardHeader, CardTitle, Button, Input, Switch, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { useSettings, useUpdateSetting } from '@/hooks/queries';

type Theme = 'light' | 'dark' | 'system';

export function SettingsPage() {
  const { t } = useTranslation();

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
          <GeneralSection />
          <TimezoneSection />
          <DataRetentionSection />
          <ForceProjectSection />
        </div>
      </div>
    </div>
  );
}

function GeneralSection() {
  const { theme, setTheme } = useTheme();
  const { t, i18n } = useTranslation();

  const themes: { value: Theme; label: string; icon: typeof Sun }[] = [
    { value: 'light', label: t('settings.theme.light'), icon: Sun },
    { value: 'dark', label: t('settings.theme.dark'), icon: Moon },
    { value: 'system', label: t('settings.theme.system'), icon: Laptop },
  ];

  const languages = [
    { value: 'en', label: t('settings.languages.en') },
    { value: 'zh', label: t('settings.languages.zh') },
  ];

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Monitor className="h-4 w-4 text-muted-foreground" />
          {t('settings.general')}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
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
  );
}

// 常用时区列表
const COMMON_TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Moscow',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Bangkok',
  'Asia/Singapore',
  'Asia/Hong_Kong',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Australia/Sydney',
  'Pacific/Auckland',
];

function TimezoneSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const currentTimezone = settings?.timezone || 'Asia/Shanghai';

  const handleTimezoneChange = async (value: string) => {
    await updateSetting.mutateAsync({
      key: 'timezone',
      value: value,
    });
  };

  if (isLoading) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div>
          <CardTitle className="text-base font-medium flex items-center gap-2">
            <Globe className="h-4 w-4 text-muted-foreground" />
            {t('settings.timezone')}
          </CardTitle>
          <p className="text-xs text-muted-foreground mt-1">{t('settings.timezoneDesc')}</p>
        </div>
      </CardHeader>
      <CardContent className="p-6">
        <Select value={currentTimezone} onValueChange={(v) => v && handleTimezoneChange(v)} disabled={updateSetting.isPending}>
          <SelectTrigger className="w-64">
            <SelectValue>{currentTimezone}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            {COMMON_TIMEZONES.map((tz) => (
              <SelectItem key={tz} value={tz}>
                {tz}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </CardContent>
    </Card>
  );
}

function DataRetentionSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const requestRetentionHours = settings?.request_retention_hours ?? '168';

  const [requestDraft, setRequestDraft] = useState('');
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (!isLoading && !initialized) {
      setRequestDraft(requestRetentionHours);
      setInitialized(true);
    }
  }, [isLoading, initialized, requestRetentionHours]);

  useEffect(() => {
    if (initialized) {
      setRequestDraft(requestRetentionHours);
    }
  }, [requestRetentionHours, initialized]);

  const hasChanges = initialized && requestDraft !== requestRetentionHours;

  const handleSave = async () => {
    const requestNum = parseInt(requestDraft, 10);

    if (!isNaN(requestNum) && requestNum >= 0 && requestDraft !== requestRetentionHours) {
      await updateSetting.mutateAsync({
        key: 'request_retention_hours',
        value: requestDraft,
      });
    }
  };

  if (isLoading || !initialized) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base font-medium flex items-center gap-2">
              <Database className="h-4 w-4 text-muted-foreground" />
              {t('settings.dataRetention')}
            </CardTitle>
            <p className="text-xs text-muted-foreground mt-1">{t('settings.retentionHoursHint')}</p>
          </div>
          <Button onClick={handleSave} disabled={!hasChanges || updateSetting.isPending} size="sm">
            {updateSetting.isPending ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-6">
        <div className="flex items-center gap-3">
          <label className="text-sm font-medium text-muted-foreground shrink-0">
            {t('settings.requestRetentionHours')}
          </label>
          <Input
            type="number"
            value={requestDraft}
            onChange={(e) => setRequestDraft(e.target.value)}
            className="w-24"
            min={0}
            disabled={updateSetting.isPending}
          />
          <span className="text-xs text-muted-foreground">{t('common.hours')}</span>
        </div>
      </CardContent>
    </Card>
  );
}

function ForceProjectSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const forceProjectEnabled = settings?.force_project_binding === 'true';
  const timeout = settings?.force_project_timeout || '30';

  const handleToggle = async (checked: boolean) => {
    await updateSetting.mutateAsync({
      key: 'force_project_binding',
      value: checked ? 'true' : 'false',
    });
  };

  const handleTimeoutChange = async (value: string) => {
    const numValue = parseInt(value, 10);
    if (numValue >= 5 && numValue <= 300) {
      await updateSetting.mutateAsync({
        key: 'force_project_timeout',
        value: value,
      });
    }
  };

  if (isLoading) return null;

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
              onChange={(e) => handleTimeoutChange(e.target.value)}
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
  );
}

export default SettingsPage;
