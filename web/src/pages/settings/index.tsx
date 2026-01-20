import { useState, useEffect, useRef, Fragment } from 'react';
import {
  Settings,
  Monitor,
  FolderOpen,
  Database,
  Globe,
  Archive,
  Download,
  Upload,
  AlertTriangle,
  CheckCircle,
  Check,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@/components/theme-provider';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Switch,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { useSettings, useUpdateSetting } from '@/hooks/queries';
import { useTransport } from '@/lib/transport/context';
import type { BackupFile, BackupImportResult } from '@/lib/transport/types';
import { getDefaultThemes, getLuxuryThemes } from '@/lib/theme';
import { cn } from '@/lib/utils';

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
          <AntigravitySection />
          <BackupSection />
        </div>
      </div>
    </div>
  );
}

function GeneralSection() {
  const { theme, setTheme } = useTheme();
  const { t, i18n } = useTranslation();

  const defaultThemes = getDefaultThemes();
  const luxuryThemes = getLuxuryThemes();

  const languages = [
    { value: 'en', label: t('settings.languages.en') },
    { value: 'zh', label: t('settings.languages.zh') },
  ];

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Monitor className="h-4 w-4 text-muted-foreground" />
          {t('settings.general')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Theme Selection */}
        <div className="space-y-3">
          <Tabs defaultValue="default" className="w-full">
            <div className="flex items-center justify-between mb-3 ">
              <div className="text-sm font-medium text-muted-foreground">
                {t('settings.themePreference')}
              </div>
              <TabsList className="inline-flex">
                <TabsTrigger value="default">Default</TabsTrigger>
                <TabsTrigger value="luxury">Luxury</TabsTrigger>
              </TabsList>
            </div>

            <TabsContent value="default" className="mt-0">
              <div className="flex flex-wrap gap-2">
                {defaultThemes.map((themeOption) => {
                  const displayColor =
                    themeOption.id === 'light'
                      ? 'oklch(0.95 0 0)'
                      : themeOption.id === 'dark'
                        ? 'oklch(0.25 0 0)'
                        : themeOption.accentColor;

                  return (
                    <button
                      key={themeOption.id}
                      type="button"
                      onClick={() => setTheme(themeOption.id)}
                      className={cn(
                        'flex items-center gap-2 px-3 py-2 rounded-md transition-all',
                        'border',
                        theme === themeOption.id
                          ? 'border-primary bg-primary/5'
                          : 'border-border hover:border-primary/50 hover:bg-muted/50',
                      )}
                      aria-label={`Select ${themeOption.name} theme`}
                    >
                      {/* Color indicator */}
                      <div className="flex gap-0.5">
                        <div
                          className="w-3 h-3 rounded-full ring-1 ring-black/10"
                          style={{ background: displayColor }}
                        />
                      </div>

                      {/* Theme name */}
                      <span className="text-sm font-medium">{themeOption.name}</span>
                    </button>
                  );
                })}
              </div>
            </TabsContent>

            <TabsContent value="luxury" className="mt-0">
              <div className="flex flex-wrap gap-2">
                {luxuryThemes.map((themeOption) => (
                  <button
                    key={themeOption.id}
                    type="button"
                    onClick={() => setTheme(themeOption.id)}
                    className={cn(
                      'flex items-center gap-2 px-3 py-2 rounded-md transition-all',
                      'border',
                      theme === themeOption.id
                        ? 'border-primary bg-primary/5'
                        : 'border-border hover:border-primary/50 hover:bg-muted/50',
                    )}
                    aria-label={`Select ${themeOption.name} theme`}
                  >
                    {/* Color indicator */}
                    <div className="flex gap-0.5">
                      <div
                        className="w-3 h-3 rounded-full ring-1 ring-black/10"
                        style={{ background: themeOption.primaryColor }}
                      />
                    </div>

                    {/* Theme name */}
                    <span className="text-sm font-medium">{themeOption.name}</span>
                  </button>
                ))}
              </div>
            </TabsContent>
          </Tabs>
        </div>

        {/* Language Selection */}
        <div className="flex gap-6 pt-4 border-t border-border flex-col">
          <div className="text-sm font-medium text-muted-foreground w-40 shrink-0">
            {t('settings.languagePreference')}
          </div>
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
      <CardHeader className="border-b border-border">
        <div>
          <CardTitle className="text-base font-medium flex items-center gap-2">
            <Globe className="h-4 w-4 text-muted-foreground" />
            {t('settings.timezone')}
          </CardTitle>
          <p className="text-xs text-muted-foreground mt-1">{t('settings.timezoneDesc')}</p>
        </div>
      </CardHeader>
      <CardContent>
        <Select
          value={currentTimezone}
          onValueChange={(v) => v && handleTimezoneChange(v)}
          disabled={updateSetting.isPending}
        >
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
      <CardHeader className="border-b border-border">
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
      <CardContent>
        <div className="flex items-center gap-3">
          <div className="text-sm font-medium text-muted-foreground shrink-0">
            {t('settings.requestRetentionHours')}
          </div>
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
      <CardHeader className="border-b border-border">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <FolderOpen className="h-4 w-4 text-muted-foreground" />
          {t('settings.forceProjectBinding')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <div className="text-sm font-medium text-foreground">
              {t('settings.enableForceProjectBinding')}
            </div>
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
            <div className="text-sm font-medium text-muted-foreground w-32 shrink-0">
              {t('settings.waitTimeout')}
            </div>
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

function AntigravitySection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const autoSortEnabled = settings?.auto_sort_antigravity === 'true';
  const refreshInterval = settings?.quota_refresh_interval || '0';

  const [intervalDraft, setIntervalDraft] = useState('');
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (!isLoading && !initialized) {
      setIntervalDraft(refreshInterval);
      setInitialized(true);
    }
  }, [isLoading, initialized, refreshInterval]);

  useEffect(() => {
    if (initialized) {
      setIntervalDraft(refreshInterval);
    }
  }, [refreshInterval, initialized]);

  const hasChanges = initialized && intervalDraft !== refreshInterval;

  const handleAutoSortToggle = async (checked: boolean) => {
    await updateSetting.mutateAsync({
      key: 'auto_sort_antigravity',
      value: checked ? 'true' : 'false',
    });
  };

  const handleSaveInterval = async () => {
    const intervalNum = parseInt(intervalDraft, 10);
    if (!isNaN(intervalNum) && intervalNum >= 0 && intervalDraft !== refreshInterval) {
      await updateSetting.mutateAsync({
        key: 'quota_refresh_interval',
        value: intervalDraft,
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
              <Zap className="h-4 w-4 text-muted-foreground" />
              {t('settings.antigravity')}
            </CardTitle>
            <p className="text-xs text-muted-foreground mt-1">{t('settings.antigravityDesc')}</p>
          </div>
          <Button
            onClick={handleSaveInterval}
            disabled={!hasChanges || updateSetting.isPending}
            size="sm"
          >
            {updateSetting.isPending ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        <div className="flex items-center gap-3">
          <label className="text-sm font-medium text-muted-foreground shrink-0">
            {t('settings.quotaRefreshInterval')}
          </label>
          <Input
            type="number"
            value={intervalDraft}
            onChange={(e) => setIntervalDraft(e.target.value)}
            className="w-24"
            min={0}
            disabled={updateSetting.isPending}
          />
          <span className="text-xs text-muted-foreground">{t('settings.minutes')}</span>
          <span className="text-xs text-muted-foreground">
            ({t('settings.quotaRefreshIntervalDesc')})
          </span>
        </div>

        <div className="flex items-center justify-between pt-4 border-t border-border">
          <div>
            <label className="text-sm font-medium text-foreground">
              {t('settings.autoSortAntigravity')}
            </label>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.autoSortAntigravityDesc')}
            </p>
          </div>
          <Switch
            checked={autoSortEnabled}
            onCheckedChange={handleAutoSortToggle}
            disabled={updateSetting.isPending}
          />
        </div>
      </CardContent>
    </Card>
  );
}

function BackupSection() {
  const { t } = useTranslation();
  const { transport } = useTransport();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [isExporting, setIsExporting] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const [importResult, setImportResult] = useState<BackupImportResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleExport = async () => {
    setIsExporting(true);
    setError(null);
    try {
      const backup = await transport.exportBackup();
      // Download as JSON file
      const blob = new Blob([JSON.stringify(backup, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `maxx-backup-${new Date().toISOString().split('T')[0]}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(t('settings.exportFailed'));
      console.error('Export failed:', err);
    } finally {
      setIsExporting(false);
    }
  };

  const handleImport = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    setIsImporting(true);
    setError(null);
    setImportResult(null);

    try {
      const text = await file.text();
      const backup: BackupFile = JSON.parse(text);
      const result = await transport.importBackup(backup, { conflictStrategy: 'skip' });
      setImportResult(result);
    } catch (err) {
      setError(t('settings.importFailed'));
      console.error('Import failed:', err);
    } finally {
      setIsImporting(false);
      // Reset file input
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    }
  };

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border">
        <div>
          <CardTitle className="text-base font-medium flex items-center gap-2">
            <Archive className="h-4 w-4 text-muted-foreground" />
            {t('settings.backup')}
          </CardTitle>
          <p className="text-xs text-muted-foreground mt-1">{t('settings.backupDesc')}</p>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Warning about sensitive data */}
        <div className="flex items-start gap-2 p-3 rounded-md bg-amber-500/10 border border-amber-500/20">
          <AlertTriangle className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" />
          <p className="text-xs text-amber-600 dark:text-amber-400">
            {t('settings.backupContainsSensitive')}
          </p>
        </div>

        {/* Export/Import buttons */}
        <div className="flex flex-wrap gap-3">
          <div className="flex-1 min-w-[200px]">
            <p className="text-sm font-medium mb-2">{t('settings.exportBackup')}</p>
            <p className="text-xs text-muted-foreground mb-3">{t('settings.exportBackupDesc')}</p>
            <Button onClick={handleExport} disabled={isExporting} variant="outline" size="sm">
              <Download className="h-4 w-4 mr-2" />
              {isExporting ? t('settings.exporting') : t('settings.exportBackup')}
            </Button>
          </div>

          <div className="flex-1 min-w-[200px]">
            <p className="text-sm font-medium mb-2">{t('settings.importBackup')}</p>
            <p className="text-xs text-muted-foreground mb-3">{t('settings.importBackupDesc')}</p>
            <input
              ref={fileInputRef}
              type="file"
              accept=".json"
              onChange={handleImport}
              className="hidden"
              id="backup-file-input"
            />
            <Button
              onClick={() => fileInputRef.current?.click()}
              disabled={isImporting}
              variant="outline"
              size="sm"
            >
              <Upload className="h-4 w-4 mr-2" />
              {isImporting ? t('settings.importing') : t('settings.selectBackupFile')}
            </Button>
          </div>
        </div>

        {/* Error message */}
        {error && (
          <div className="flex items-center gap-2 p-3 rounded-md bg-destructive/10 border border-destructive/20">
            <AlertTriangle className="h-4 w-4 text-destructive" />
            <p className="text-sm text-destructive">{error}</p>
          </div>
        )}

        {/* Import result */}
        {importResult && (
          <div className="space-y-3 p-4 rounded-md border border-border bg-muted/30">
            <div className="flex items-center gap-2">
              <CheckCircle className="h-4 w-4 text-green-500" />
              <p className="text-sm font-medium">{t('settings.importSummary')}</p>
            </div>

            {/* Summary table */}
            <div className="grid grid-cols-4 gap-2 text-xs">
              <div className="font-medium text-muted-foreground"></div>
              <div className="font-medium text-muted-foreground text-center">
                {t('settings.imported')}
              </div>
              <div className="font-medium text-muted-foreground text-center">
                {t('settings.skipped')}
              </div>
              <div className="font-medium text-muted-foreground text-center">
                {t('settings.updated')}
              </div>
              {Object.entries(importResult.summary).map(([key, summary]) => (
                <Fragment key={key}>
                  <div className="capitalize">
                    {key}
                  </div>
                  <div className="text-center text-green-600">
                    {summary.imported}
                  </div>
                  <div className="text-center text-muted-foreground">
                    {summary.skipped}
                  </div>
                  <div className="text-center text-blue-600">
                    {summary.updated}
                  </div>
                </Fragment>
              ))}
            </div>

            {/* Warnings */}
            {importResult.warnings && importResult.warnings.length > 0 && (
              <div className="space-y-1">
                <p className="text-xs font-medium text-amber-600">
                  {t('settings.importWarnings')}:
                </p>
                <div className="max-h-32 overflow-y-auto space-y-1">
                  {importResult.warnings.map((warning, i) => (
                    <p
                      key={i}
                      className="text-xs text-amber-600 dark:text-amber-400 pl-2 border-l-2 border-amber-500/30"
                    >
                      {warning}
                    </p>
                  ))}
                </div>
              </div>
            )}

            {/* Errors */}
            {importResult.errors && importResult.errors.length > 0 && (
              <div className="space-y-1">
                <p className="text-xs font-medium text-destructive">
                  {t('settings.importErrors')}:
                </p>
                <div className="max-h-32 overflow-y-auto space-y-1">
                  {importResult.errors.map((err, i) => (
                    <p
                      key={i}
                      className="text-xs text-destructive pl-2 border-l-2 border-destructive/30"
                    >
                      {err}
                    </p>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export default SettingsPage;
