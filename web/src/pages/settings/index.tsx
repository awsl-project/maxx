import { useState, useEffect, useRef, Fragment, useId } from 'react';
import { useQueryClient } from '@tanstack/react-query';
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
  Zap,
  Activity,
  Eye,
  EyeOff,
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
  Label,
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
import { ProxyKillSwitchCard } from '@/components/settings/proxy-kill-switch-card';
import { BackendAddressControl } from '@/components/backend-address-control';
import { settingsKeys, useSettings, useUpdateSetting, useDeleteSetting } from '@/hooks/queries';
import { useAuth } from '@/lib/auth-context';
import { buildPprofUrl } from '@/lib/backend-config';
import { useTransport } from '@/lib/transport/context';
import type { BackupFile, BackupImportResult } from '@/lib/transport/types';
import { getDefaultThemes, getLuxuryThemes, isLuxuryTheme } from '@/lib/theme';
import { cn } from '@/lib/utils';

function parseRetentionInteger(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed.length === 0) {
    return null;
  }

  if (!/^-?\d+$/.test(trimmed)) {
    return null;
  }

  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || !Number.isInteger(parsed)) {
    return null;
  }

  return parsed;
}

const REQUEST_FAILURE_DETAILS_SETTING_KEY = 'request_failure_details_enabled';
const OPENAI_CHAT_STREAM_TIMEOUTS_ENABLED_SETTING_KEY = 'openai_chat_stream_timeouts_enabled';
const OPENAI_CHAT_STREAM_FIRST_EVENT_TIMEOUT_SETTING_KEY = 'openai_chat_stream_first_event_timeout_ms';
const OPENAI_CHAT_STREAM_IDLE_TIMEOUT_SETTING_KEY = 'openai_chat_stream_idle_timeout_ms';
const DEFAULT_STREAM_FIRST_EVENT_TIMEOUT_MS = '20000';
const DEFAULT_STREAM_IDLE_TIMEOUT_MS = '45000';
const MULTITENANT_UI_LAYOUT_SETTING_KEY = 'ui_multitenant_layout';
type MultiTenantUILayout = 'current' | 'user_panel';

interface ProxyRouteExposureSetting {
  key: string;
  titleKey: string;
  descKey: string;
  paths: string[];
  defaultEnabled: boolean;
}

const PROXY_ROUTE_EXPOSURE_SETTINGS: ProxyRouteExposureSetting[] = [
  {
    key: 'proxy_route_claude_messages_enabled',
    titleKey: 'settings.proxyRouteClaudeMessages',
    descKey: 'settings.proxyRouteClaudeMessagesDesc',
    paths: ['/v1/messages'],
    defaultEnabled: true,
  },
  {
    key: 'proxy_route_openai_chat_enabled',
    titleKey: 'settings.proxyRouteOpenAIChat',
    descKey: 'settings.proxyRouteOpenAIChatDesc',
    paths: ['/v1/chat/completions', '/chat/completions'],
    defaultEnabled: true,
  },
  {
    key: 'proxy_route_responses_enabled',
    titleKey: 'settings.proxyRouteResponses',
    descKey: 'settings.proxyRouteResponsesDesc',
    paths: ['/v1/responses', '/responses'],
    defaultEnabled: true,
  },
  {
    key: 'proxy_route_gemini_enabled',
    titleKey: 'settings.proxyRouteGemini',
    descKey: 'settings.proxyRouteGeminiDesc',
    paths: ['/v1beta/models/*'],
    defaultEnabled: false,
  },
];

export function SettingsPage() {
  const { t } = useTranslation();
  const { user } = useAuth();
  const isAdmin = user?.role === 'admin';

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Settings}
        iconClassName="text-zinc-500"
        title={t('settings.title')}
        description={t('settings.description')}
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="space-y-6">
          <GeneralSection />
          <BackendAddressSection />
          {isAdmin && (
            <>
              <MultiTenantUISection />
              <TimezoneSection />
            </>
          )}
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
  const currentThemeCategoryTab = isLuxuryTheme(theme) ? 'luxury' : 'default';
  const [themeCategoryTab, setThemeCategoryTab] = useState<'default' | 'luxury'>(
    currentThemeCategoryTab,
  );

  useEffect(() => {
    setThemeCategoryTab(currentThemeCategoryTab);
  }, [currentThemeCategoryTab]);

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
          <Tabs
            value={themeCategoryTab}
            onValueChange={(value) => setThemeCategoryTab(value as 'default' | 'luxury')}
            className="w-full"
          >
            <div className="flex items-center justify-between mb-3 ">
              <div className="text-sm font-medium text-muted-foreground">
                {t('settings.themePreference')}
              </div>
              <TabsList className="inline-flex">
                <TabsTrigger value="default">{t('settings.themeDefault')}</TabsTrigger>
                <TabsTrigger value="luxury">{t('settings.themeLuxury')}</TabsTrigger>
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
          <div className="text-sm font-medium text-muted-foreground w-full sm:w-40 shrink-0">
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

function BackendAddressSection() {
  const { t } = useTranslation();

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Globe className="h-4 w-4 text-muted-foreground" />
          {t('backendAddress.title')}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <BackendAddressControl alwaysOpen />
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

const getBrowserTimezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

const getTimezoneOptions = (currentTimezone: string) => {
  if (COMMON_TIMEZONES.includes(currentTimezone)) {
    return COMMON_TIMEZONES;
  }

  return [currentTimezone, ...COMMON_TIMEZONES];
};

function TimezoneSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const currentTimezone = settings?.timezone || getBrowserTimezone();
  const timezoneOptions = getTimezoneOptions(currentTimezone);

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
          <SelectTrigger className="w-full max-w-64">
            <SelectValue>{currentTimezone}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            {timezoneOptions.map((tz) => (
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

export function DataRetentionSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();
  const requestRetentionInputId = useId();
  const sessionRetentionInputId = useId();
  const requestDetailRetentionInputId = useId();
  const requestDetailRetentionSuccessInputId = useId();
  const requestDetailRetentionFailedInputId = useId();
  const detailSplitToggleId = useId();

  const requestRetentionHours = settings?.request_retention_hours ?? '168';
  const sessionRetentionHours = settings?.session_retention_hours ?? '168';
  const requestDetailRetentionSeconds = settings?.request_detail_retention_seconds ?? '-1';
  const requestDetailRetentionSplitEnabled =
    settings?.request_detail_retention_split_enabled === 'true';
  const requestDetailRetentionSecondsSuccess =
    settings?.request_detail_retention_seconds_success ?? requestDetailRetentionSeconds;
  const requestDetailRetentionSecondsFailed =
    settings?.request_detail_retention_seconds_failed ?? requestDetailRetentionSeconds;

  const [requestDraft, setRequestDraft] = useState('');
  const [sessionDraft, setSessionDraft] = useState('');
  const [detailDraft, setDetailDraft] = useState('');
  const [splitDraft, setSplitDraft] = useState(false);
  const [detailSuccessDraft, setDetailSuccessDraft] = useState('');
  const [detailFailedDraft, setDetailFailedDraft] = useState('');
  const [validationError, setValidationError] = useState('');
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (!isLoading && !initialized) {
      setRequestDraft(requestRetentionHours);
      setSessionDraft(sessionRetentionHours);
      setDetailDraft(requestDetailRetentionSeconds);
      setSplitDraft(requestDetailRetentionSplitEnabled);
      setDetailSuccessDraft(requestDetailRetentionSecondsSuccess);
      setDetailFailedDraft(requestDetailRetentionSecondsFailed);
      setInitialized(true);
    }
  }, [
    isLoading,
    initialized,
    requestRetentionHours,
    sessionRetentionHours,
    requestDetailRetentionSeconds,
    requestDetailRetentionSplitEnabled,
    requestDetailRetentionSecondsSuccess,
    requestDetailRetentionSecondsFailed,
  ]);

  // 与 handleSave 的提交规则保持对称：split-only 字段仅在 splitDraft=true
  // 时纳入 dirty 判断。否则，统一键被独立修改后，由统一值派生出来的 split
  // 草稿会与服务端最新值持续不一致，导致表单永远是 dirty，并且下一次保存
  // 会把陈旧的派生值写回成显式的 split 键，覆盖新的统一保留时间
  // 与 handleSave 的提交规则保持对称：
  //   - splitDraft=false 时，仅统一键参与 dirty 比较
  //   - splitDraft=true 时，仅 split-only 字段参与 dirty 比较
  // 这样切到任一模式后，另一模式的草稿与服务端派生值之间的常驻偏差不会
  // 让表单永远 dirty，也不会让 Save 把陈旧值意外覆盖回去
  const hasChanges =
    initialized &&
    (requestDraft !== requestRetentionHours ||
      sessionDraft !== sessionRetentionHours ||
      splitDraft !== requestDetailRetentionSplitEnabled ||
      (splitDraft
        ? detailSuccessDraft !== requestDetailRetentionSecondsSuccess ||
          detailFailedDraft !== requestDetailRetentionSecondsFailed
        : detailDraft !== requestDetailRetentionSeconds));

  useEffect(() => {
    // 仅在本地没有未保存修改时，才用服务端最新值回填表单
    if (initialized && !hasChanges) {
      setRequestDraft(requestRetentionHours);
      setSessionDraft(sessionRetentionHours);
      setDetailDraft(requestDetailRetentionSeconds);
      setSplitDraft(requestDetailRetentionSplitEnabled);
      setDetailSuccessDraft(requestDetailRetentionSecondsSuccess);
      setDetailFailedDraft(requestDetailRetentionSecondsFailed);
    }
  }, [
    requestRetentionHours,
    sessionRetentionHours,
    requestDetailRetentionSeconds,
    requestDetailRetentionSplitEnabled,
    requestDetailRetentionSecondsSuccess,
    requestDetailRetentionSecondsFailed,
    initialized,
    hasChanges,
  ]);

  useEffect(() => {
    setValidationError((current) => (current ? '' : current));
  }, [requestDraft, sessionDraft, detailDraft, detailSuccessDraft, detailFailedDraft, splitDraft]);

  const handleSave = async () => {
    const requestNum = parseRetentionInteger(requestDraft);
    const sessionNum = parseRetentionInteger(sessionDraft);
    const detailNum = parseRetentionInteger(detailDraft);
    const detailSuccessNum = parseRetentionInteger(detailSuccessDraft);
    const detailFailedNum = parseRetentionInteger(detailFailedDraft);
    const updates: Array<{ key: string; value: string }> = [];

    if (requestDraft !== requestRetentionHours) {
      if (requestNum === null || requestNum < 0) {
        setValidationError(t('settings.retentionValidationError'));
        return;
      }
      updates.push({ key: 'request_retention_hours', value: String(requestNum) });
    }

    if (sessionDraft !== sessionRetentionHours) {
      if (sessionNum === null || sessionNum < 0) {
        setValidationError(t('settings.retentionValidationError'));
        return;
      }
      updates.push({ key: 'session_retention_hours', value: String(sessionNum) });
    }

    // 统一键仅在 split 关闭时参与校验与提交——开启 split 后该输入隐藏，
    // 残留的草稿（可能尚未保存或临时无效）不应阻塞保存
    if (!splitDraft && detailDraft !== requestDetailRetentionSeconds) {
      if (detailNum === null || detailNum < -1) {
        setValidationError(t('settings.retentionValidationError'));
        return;
      }
      updates.push({ key: 'request_detail_retention_seconds', value: String(detailNum) });
    }

    if (splitDraft !== requestDetailRetentionSplitEnabled) {
      updates.push({
        key: 'request_detail_retention_split_enabled',
        value: splitDraft ? 'true' : 'false',
      });
    }

    // 仅在 split 开启时校验/提交 split-only 字段——如果用户编辑了 split 输入
    // 后又关闭 split，那些隐藏字段不应阻塞保存或被持久化
    if (splitDraft && detailSuccessDraft !== requestDetailRetentionSecondsSuccess) {
      if (detailSuccessNum === null || detailSuccessNum < -1) {
        setValidationError(t('settings.retentionValidationError'));
        return;
      }
      updates.push({
        key: 'request_detail_retention_seconds_success',
        value: String(detailSuccessNum),
      });
    }

    if (splitDraft && detailFailedDraft !== requestDetailRetentionSecondsFailed) {
      if (detailFailedNum === null || detailFailedNum < -1) {
        setValidationError(t('settings.retentionValidationError'));
        return;
      }
      updates.push({
        key: 'request_detail_retention_seconds_failed',
        value: String(detailFailedNum),
      });
    }

    setValidationError('');
    for (const update of updates) {
      await updateSetting.mutateAsync(update);
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
      <CardContent className="space-y-4">
        {validationError && <p className="text-xs text-destructive">{validationError}</p>}
        <div className="space-y-1.5">
          <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
            <Label
              htmlFor={requestRetentionInputId}
              className="text-sm font-medium text-muted-foreground shrink-0"
            >
              {t('settings.requestRetentionHours')}
            </Label>
            <Input
              id={requestRetentionInputId}
              type="number"
              value={requestDraft}
              onChange={(e) => setRequestDraft(e.target.value)}
              className="w-24"
              min={0}
              step={1}
              disabled={updateSetting.isPending}
            />
            <span className="text-xs text-muted-foreground">{t('common.hours')}</span>
          </div>
          <p className="text-xs text-muted-foreground">{t('settings.requestRetentionHoursDesc')}</p>
        </div>

        <div className="space-y-1.5 pt-4 border-t border-border">
          <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
            <Label
              htmlFor={sessionRetentionInputId}
              className="text-sm font-medium text-muted-foreground shrink-0"
            >
              {t('settings.sessionRetentionHours')}
            </Label>
            <Input
              id={sessionRetentionInputId}
              type="number"
              value={sessionDraft}
              onChange={(e) => setSessionDraft(e.target.value)}
              className="w-24"
              min={0}
              step={1}
              disabled={updateSetting.isPending}
            />
            <span className="text-xs text-muted-foreground">{t('common.hours')}</span>
          </div>
          <p className="text-xs text-muted-foreground">{t('settings.sessionRetentionHoursDesc')}</p>
        </div>

        <div className="space-y-1.5 pt-4 border-t border-border">
          {!splitDraft && (
            <>
              <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
                <Label
                  htmlFor={requestDetailRetentionInputId}
                  className="text-sm font-medium text-muted-foreground shrink-0"
                >
                  {t('settings.requestDetailRetention')}
                </Label>
                <Input
                  id={requestDetailRetentionInputId}
                  type="number"
                  value={detailDraft}
                  onChange={(e) => setDetailDraft(e.target.value)}
                  className="w-24"
                  min={-1}
                  step={1}
                  disabled={updateSetting.isPending}
                />
                <span className="text-xs text-muted-foreground">{t('common.seconds')}</span>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('settings.requestDetailRetentionDesc')}
              </p>
            </>
          )}

          <div className="flex items-center justify-between gap-3 pt-2">
            <Label
              htmlFor={detailSplitToggleId}
              className="text-sm font-medium text-muted-foreground"
            >
              {t('settings.requestDetailRetentionSplit')}
            </Label>
            <Switch
              id={detailSplitToggleId}
              checked={splitDraft}
              onCheckedChange={(next) => {
                // 切换到 split 时，把正在编辑（未保存）的 unified 草稿同步到
                // 两个 split 输入，避免显示陈旧的服务端派生值，并保证保存后
                // 实际写入的值与用户当下看到的一致
                if (next && !splitDraft) {
                  setDetailSuccessDraft(detailDraft);
                  setDetailFailedDraft(detailDraft);
                }
                setSplitDraft(next);
              }}
              disabled={updateSetting.isPending}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {t('settings.requestDetailRetentionSplitDesc')}
          </p>

          {splitDraft && (
            <div className="space-y-3 pt-2">
              <div className="space-y-1.5">
                <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
                  <Label
                    htmlFor={requestDetailRetentionSuccessInputId}
                    className="text-sm font-medium text-muted-foreground shrink-0"
                  >
                    {t('settings.requestDetailRetentionSuccess')}
                  </Label>
                  <Input
                    id={requestDetailRetentionSuccessInputId}
                    type="number"
                    value={detailSuccessDraft}
                    onChange={(e) => setDetailSuccessDraft(e.target.value)}
                    className="w-24"
                    min={-1}
                    step={1}
                    disabled={updateSetting.isPending}
                  />
                  <span className="text-xs text-muted-foreground">{t('common.seconds')}</span>
                </div>
              </div>
              <div className="space-y-1.5">
                <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
                  <Label
                    htmlFor={requestDetailRetentionFailedInputId}
                    className="text-sm font-medium text-muted-foreground shrink-0"
                  >
                    {t('settings.requestDetailRetentionFailed')}
                  </Label>
                  <Input
                    id={requestDetailRetentionFailedInputId}
                    type="number"
                    value={detailFailedDraft}
                    onChange={(e) => setDetailFailedDraft(e.target.value)}
                    className="w-24"
                    min={-1}
                    step={1}
                    disabled={updateSetting.isPending}
                  />
                  <span className="text-xs text-muted-foreground">{t('common.seconds')}</span>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('settings.requestDetailRetentionDesc')}
              </p>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export function ForceProjectSection() {
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
          <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-6 pt-4 border-t border-border">
            <div className="text-sm font-medium text-muted-foreground w-full sm:w-32 shrink-0">
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

export function OpenAIChatStreamTimeoutSection() {
  const { data: settings, isLoading } = useSettings();
  const { transport } = useTransport();
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const streamTimeoutsEnabled = settings?.[OPENAI_CHAT_STREAM_TIMEOUTS_ENABLED_SETTING_KEY] === 'true';
  const firstEventTimeout =
    settings?.[OPENAI_CHAT_STREAM_FIRST_EVENT_TIMEOUT_SETTING_KEY] || DEFAULT_STREAM_FIRST_EVENT_TIMEOUT_MS;
  const idleTimeout = settings?.[OPENAI_CHAT_STREAM_IDLE_TIMEOUT_SETTING_KEY] || DEFAULT_STREAM_IDLE_TIMEOUT_MS;

  const [enabledDraft, setEnabledDraft] = useState(false);
  const [firstEventDraft, setFirstEventDraft] = useState('');
  const [idleDraft, setIdleDraft] = useState('');
  const [initialized, setInitialized] = useState(false);
  const [error, setError] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const firstEventInputId = useId();
  const idleInputId = useId();

  useEffect(() => {
    if (!isLoading && !initialized) {
      setEnabledDraft(streamTimeoutsEnabled);
      setFirstEventDraft(firstEventTimeout);
      setIdleDraft(idleTimeout);
      setInitialized(true);
    }
  }, [firstEventTimeout, idleTimeout, initialized, isLoading, streamTimeoutsEnabled]);

  useEffect(() => {
    if (initialized) {
      setEnabledDraft(streamTimeoutsEnabled);
      setFirstEventDraft(firstEventTimeout);
      setIdleDraft(idleTimeout);
    }
  }, [firstEventTimeout, idleTimeout, initialized, streamTimeoutsEnabled]);

  const hasChanges =
    initialized &&
    (enabledDraft !== streamTimeoutsEnabled ||
      firstEventDraft !== firstEventTimeout ||
      idleDraft !== idleTimeout);

  const validateTimeout = (value: string) => {
    const parsed = parseInt(value, 10);
    return Number.isFinite(parsed) && parsed >= 1000 && parsed <= 600000;
  };

  const handleSave = async () => {
    if (!validateTimeout(firstEventDraft) || !validateTimeout(idleDraft)) {
      setError(t('settings.openAIChatStreamTimeoutInvalid'));
      return;
    }

    setError('');
    setIsSaving(true);
    try {
      await transport.updateSetting(
        OPENAI_CHAT_STREAM_TIMEOUTS_ENABLED_SETTING_KEY,
        enabledDraft ? 'true' : 'false',
      );
      await transport.updateSetting(OPENAI_CHAT_STREAM_FIRST_EVENT_TIMEOUT_SETTING_KEY, firstEventDraft);
      await transport.updateSetting(OPENAI_CHAT_STREAM_IDLE_TIMEOUT_SETTING_KEY, idleDraft);
      queryClient.setQueryData<Record<string, string>>(settingsKeys.all, {
        ...(settings || {}),
        [OPENAI_CHAT_STREAM_TIMEOUTS_ENABLED_SETTING_KEY]: enabledDraft ? 'true' : 'false',
        [OPENAI_CHAT_STREAM_FIRST_EVENT_TIMEOUT_SETTING_KEY]: firstEventDraft,
        [OPENAI_CHAT_STREAM_IDLE_TIMEOUT_SETTING_KEY]: idleDraft,
      });
      await queryClient.invalidateQueries({ queryKey: settingsKeys.all });
      await queryClient.invalidateQueries({ queryKey: settingsKeys.public });
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : String(saveError));
    } finally {
      setIsSaving(false);
    }
  };

  if (isLoading || !initialized) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <CardTitle className="text-base font-medium flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              {t('settings.openAIChatStreamTimeouts')}
            </CardTitle>
            <p className="text-xs text-muted-foreground mt-1">{t('settings.openAIChatStreamTimeoutsDesc')}</p>
          </div>
          <Button onClick={handleSave} disabled={!hasChanges || isSaving} size="sm">
            {isSaving ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        <div className="flex items-center justify-between gap-4 rounded-lg border border-border bg-muted/20 p-4">
          <div>
            <Label className="text-sm font-medium text-foreground">
              {t('settings.enableOpenAIChatStreamTimeouts')}
            </Label>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.enableOpenAIChatStreamTimeoutsDesc')}
            </p>
          </div>
          <Switch
            aria-label={t('settings.enableOpenAIChatStreamTimeouts')}
            checked={enabledDraft}
            onCheckedChange={setEnabledDraft}
            disabled={isSaving}
          />
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor={firstEventInputId} className="text-sm font-medium text-foreground">
              {t('settings.openAIChatStreamFirstEventTimeout')}
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id={firstEventInputId}
                type="number"
                value={firstEventDraft}
                onChange={(event) => setFirstEventDraft(event.target.value)}
                min={1000}
                max={600000}
                step={1000}
                disabled={isSaving || !enabledDraft}
              />
              <span className="text-xs text-muted-foreground">ms</span>
            </div>
            <p className="text-xs text-muted-foreground">
              {t('settings.openAIChatStreamFirstEventTimeoutDesc')}
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor={idleInputId} className="text-sm font-medium text-foreground">
              {t('settings.openAIChatStreamIdleTimeout')}
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id={idleInputId}
                type="number"
                value={idleDraft}
                onChange={(event) => setIdleDraft(event.target.value)}
                min={1000}
                max={600000}
                step={1000}
                disabled={isSaving || !enabledDraft}
              />
              <span className="text-xs text-muted-foreground">ms</span>
            </div>
            <p className="text-xs text-muted-foreground">{t('settings.openAIChatStreamIdleTimeoutDesc')}</p>
          </div>
        </div>

        {error && <p className="text-xs text-destructive">{error}</p>}
        <div className="flex items-start gap-2 p-3 rounded-md bg-blue-500/10 border border-blue-500/20">
          <AlertTriangle className="h-4 w-4 text-blue-500 mt-0.5 shrink-0" />
          <p className="text-xs text-blue-600 dark:text-blue-400">
            {t('settings.openAIChatStreamTimeoutsHint')}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

export function RequestDiagnosticsSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const enhancedFailureDetailsEnabled = settings?.[REQUEST_FAILURE_DETAILS_SETTING_KEY] === 'true';

  const handleToggle = async (checked: boolean) => {
    await updateSetting.mutateAsync({
      key: REQUEST_FAILURE_DETAILS_SETTING_KEY,
      value: checked ? 'true' : 'false',
    });
  };

  if (isLoading) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <CardTitle className="text-base font-medium flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              {t('settings.requestDiagnostics')}
            </CardTitle>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.requestDiagnosticsDesc')}
            </p>
          </div>
          <Switch
            aria-label={t('settings.enhancedFailureDetails')}
            checked={enhancedFailureDetailsEnabled}
            onCheckedChange={handleToggle}
            disabled={updateSetting.isPending}
          />
        </div>
      </CardHeader>
      <CardContent className="p-6 space-y-2">
        <Label className="text-sm font-medium text-foreground">
          {t('settings.enhancedFailureDetails')}
        </Label>
        <p className="text-xs text-muted-foreground">{t('settings.enhancedFailureDetailsDesc')}</p>
        <p className="text-xs text-muted-foreground">{t('settings.defaultOff')}</p>
      </CardContent>
    </Card>
  );
}

export function ProxyKillSwitchSection() {
  return <ProxyKillSwitchCard />;
}

export function ProxyRouteExposureSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const enabledCount = PROXY_ROUTE_EXPOSURE_SETTINGS.filter((route) => {
    const value = settings?.[route.key];
    return value === undefined ? route.defaultEnabled : value !== 'false';
  }).length;

  const handleToggle = async (key: string, checked: boolean) => {
    if (!checked && enabledCount <= 1) return;

    await updateSetting.mutateAsync({
      key,
      value: checked ? 'true' : 'false',
    });
  };

  if (isLoading) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div>
          <CardTitle className="text-base font-medium flex items-center gap-2">
            <Globe className="h-4 w-4 text-muted-foreground" />
            {t('settings.proxyRouteExposure')}
          </CardTitle>
          <p className="text-xs text-muted-foreground mt-1">
            {t('settings.proxyRouteExposureDesc')}
          </p>
        </div>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        {PROXY_ROUTE_EXPOSURE_SETTINGS.map((route) => {
          const value = settings?.[route.key];
          const enabled = value === undefined ? route.defaultEnabled : value !== 'false';
          const disableLastEnabledRoute = enabled && enabledCount <= 1;

          return (
            <div
              key={route.key}
              className="flex flex-col gap-3 rounded-lg border border-border bg-muted/20 p-4 sm:flex-row sm:items-start sm:justify-between"
            >
              <div className="min-w-0 space-y-2">
                <div>
                  <Label className="text-sm font-medium text-foreground">{t(route.titleKey)}</Label>
                  <p className="text-xs text-muted-foreground mt-1">{t(route.descKey)}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                  {route.paths.map((path) => (
                    <code
                      key={path}
                      className="rounded border border-border bg-background px-2 py-1 text-xs text-muted-foreground"
                    >
                      {path}
                    </code>
                  ))}
                </div>
              </div>
              <div className="flex flex-col items-end gap-1">
                <Switch
                  aria-label={t(route.titleKey)}
                  checked={enabled}
                  onCheckedChange={(checked) => handleToggle(route.key, checked)}
                  disabled={updateSetting.isPending || disableLastEnabledRoute}
                />
                {disableLastEnabledRoute && (
                  <span className="max-w-40 text-right text-[11px] text-muted-foreground">
                    {t('settings.proxyRouteExposureAtLeastOne')}
                  </span>
                )}
              </div>
            </div>
          );
        })}
        <div className="flex items-start gap-2 p-3 rounded-md bg-blue-500/10 border border-blue-500/20">
          <AlertTriangle className="h-4 w-4 text-blue-500 mt-0.5 shrink-0" />
          <p className="text-xs text-blue-600 dark:text-blue-400">
            {t('settings.proxyRouteExposureHint')}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

export function AntigravitySection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

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
              {t('settings.quotaSettings')}
            </CardTitle>
            <p className="text-xs text-muted-foreground mt-1">{t('settings.quotaSettingsDesc')}</p>
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
        <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
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

        <div className="flex items-start gap-2 p-3 rounded-md bg-blue-500/10 border border-blue-500/20">
          <AlertTriangle className="h-4 w-4 text-blue-500 mt-0.5 shrink-0" />
          <p className="text-xs text-blue-600 dark:text-blue-400">{t('settings.routesSortNote')}</p>
        </div>
      </CardContent>
    </Card>
  );
}

export function PprofSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const deleteSetting = useDeleteSetting();
  const { t } = useTranslation();

  const pprofEnabled = settings?.enable_pprof === 'true';
  const pprofPort = settings?.pprof_port || '6060';
  const pprofPassword = settings?.pprof_password || '';

  const [enabledDraft, setEnabledDraft] = useState(false);
  const [portDraft, setPortDraft] = useState('');
  const [usePasswordDraft, setUsePasswordDraft] = useState(false);
  const [passwordDraft, setPasswordDraft] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [initialized, setInitialized] = useState(false);
  const [passwordError, setPasswordError] = useState('');
  const [portError, setPortError] = useState('');
  const pprofUrl = buildPprofUrl(portDraft);

  useEffect(() => {
    if (!isLoading && !initialized) {
      setEnabledDraft(pprofEnabled);
      setPortDraft(pprofPort);
      setUsePasswordDraft(pprofPassword !== '');
      setPasswordDraft(pprofPassword);
      setInitialized(true);
    }
  }, [isLoading, initialized, pprofEnabled, pprofPort, pprofPassword]);

  useEffect(() => {
    // 仅在用户无未保存更改时同步外部配置更新
    if (initialized) {
      const currentHasChanges =
        enabledDraft !== pprofEnabled ||
        (enabledDraft && portDraft !== pprofPort) ||
        (enabledDraft && usePasswordDraft !== (pprofPassword !== '')) ||
        (enabledDraft && usePasswordDraft && passwordDraft !== pprofPassword);

      // 只在没有未保存的更改时更新 draft 状态
      if (!currentHasChanges) {
        setEnabledDraft(pprofEnabled);
        setPortDraft(pprofPort);
        setUsePasswordDraft(pprofPassword !== '');
        setPasswordDraft(pprofPassword);
      }
    }
  }, [
    pprofEnabled,
    pprofPort,
    pprofPassword,
    initialized,
    enabledDraft,
    portDraft,
    usePasswordDraft,
    passwordDraft,
  ]);

  // Clear password error when password changes
  useEffect(() => {
    if (passwordDraft) {
      setPasswordError('');
    }
  }, [passwordDraft]);

  // Clear port error when port changes
  useEffect(() => {
    if (portDraft) {
      setPortError('');
    }
  }, [portDraft]);

  // Clear port error when disabling pprof
  useEffect(() => {
    if (!enabledDraft) {
      setPortError('');
    }
  }, [enabledDraft]);

  const isPasswordInvalid = usePasswordDraft && !passwordDraft.trim();
  const portNum = parseInt(portDraft, 10);
  // 只在启用 pprof 时才验证端口
  const isPortInvalid = enabledDraft && (isNaN(portNum) || portNum < 1 || portNum > 65535);

  const hasChanges =
    initialized &&
    !isPasswordInvalid &&
    !isPortInvalid &&
    (enabledDraft !== pprofEnabled ||
      (enabledDraft && portDraft !== pprofPort) || // 只在启用时检查端口变化
      usePasswordDraft !== (pprofPassword !== '') ||
      (usePasswordDraft && passwordDraft !== pprofPassword));

  const handleSave = async () => {
    // Validate password if protection is enabled
    if (usePasswordDraft && !passwordDraft.trim()) {
      setPasswordError(t('settings.pprofPasswordRequired'));
      return;
    }

    // 只在启用 pprof 时验证端口
    if (enabledDraft) {
      const portNum = parseInt(portDraft, 10);
      if (isNaN(portNum) || portNum < 1 || portNum > 65535) {
        setPortError(t('settings.pprofPortInvalid'));
        return;
      }
    }

    try {
      // Save enabled state
      if (enabledDraft !== pprofEnabled) {
        await updateSetting.mutateAsync({
          key: 'enable_pprof',
          value: enabledDraft ? 'true' : 'false',
        });
      }

      // 只在启用 pprof 时保存端口
      if (enabledDraft) {
        const portNum = parseInt(portDraft, 10);
        if (portNum >= 1 && portNum <= 65535 && portDraft !== pprofPort) {
          await updateSetting.mutateAsync({
            key: 'pprof_port',
            value: portDraft,
          });
        }
      }

      // Handle password: delete from database if disabled, otherwise save
      if (!usePasswordDraft && pprofPassword !== '') {
        // Password protection disabled and old password exists - delete it
        await deleteSetting.mutateAsync('pprof_password');
      } else if (usePasswordDraft && passwordDraft && passwordDraft !== pprofPassword) {
        // Password protection enabled and password changed - save new password (only if not empty)
        await updateSetting.mutateAsync({
          key: 'pprof_password',
          value: passwordDraft,
        });
      }
    } catch (error) {
      console.error('Failed to save pprof settings:', error);
    }
  };

  if (isLoading || !initialized) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base font-medium flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              {t('settings.pprof')}
            </CardTitle>
            <p className="text-xs text-muted-foreground mt-1">{t('settings.pprofDesc')}</p>
          </div>
          <Button
            onClick={handleSave}
            disabled={!hasChanges || updateSetting.isPending || deleteSetting.isPending}
            size="sm"
          >
            {updateSetting.isPending || deleteSetting.isPending
              ? t('common.saving')
              : t('common.save')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        {/* Enable pprof */}
        <div className="flex items-center justify-between">
          <div>
            <label className="text-sm font-medium text-foreground">
              {t('settings.enablePprof')}
            </label>
            <p className="text-xs text-muted-foreground mt-1">{t('settings.enablePprofDesc')}</p>
          </div>
          <Switch
            checked={enabledDraft}
            onCheckedChange={setEnabledDraft}
            disabled={updateSetting.isPending}
          />
        </div>

        {enabledDraft && (
          <>
            {/* Port */}
            <div className="space-y-2 pt-4 border-t border-border">
              <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
                <label className="text-sm font-medium text-muted-foreground shrink-0 w-full sm:w-20">
                  {t('settings.pprofPort')}
                </label>
                <Input
                  type="number"
                  value={portDraft}
                  onChange={(e) => setPortDraft(e.target.value)}
                  className={`w-32 ${portError ? 'border-red-500 focus-visible:ring-red-500' : ''}`}
                  min={1}
                  max={65535}
                  disabled={updateSetting.isPending}
                />
                <span className="text-xs text-muted-foreground">{t('settings.pprofPortDesc')}</span>
              </div>
              {portError && (
                <div className="flex items-center gap-3">
                  <div className="w-20 shrink-0"></div>
                  <p className="text-xs text-red-500">{portError}</p>
                </div>
              )}
            </div>

            {/* Password protection toggle */}
            <div className="flex items-center justify-between pt-4 border-t border-border">
              <div>
                <label className="text-sm font-medium text-foreground">
                  {t('settings.enablePasswordProtection')}
                </label>
                <p className="text-xs text-muted-foreground mt-1">
                  {t('settings.enablePasswordProtectionDesc')}
                </p>
              </div>
              <Switch
                checked={usePasswordDraft}
                onCheckedChange={setUsePasswordDraft}
                disabled={updateSetting.isPending || deleteSetting.isPending}
              />
            </div>

            {/* Password input (only shown when password protection is enabled) */}
            {usePasswordDraft && (
              <div className="space-y-2">
                <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
                  <label className="text-sm font-medium text-muted-foreground shrink-0 w-full sm:w-20">
                    {t('settings.pprofPassword')}
                  </label>
                  <div className="flex-1 max-w-md relative">
                    <Input
                      type={showPassword ? 'text' : 'password'}
                      value={passwordDraft}
                      onChange={(e) => setPasswordDraft(e.target.value)}
                      placeholder={t('settings.pprofPasswordPlaceholder')}
                      disabled={updateSetting.isPending}
                      className={`pr-10 ${passwordError ? 'border-red-500 focus-visible:ring-red-500' : ''}`}
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      tabIndex={-1}
                    >
                      {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  </div>
                </div>
                {passwordError && (
                  <div className="flex items-center gap-3">
                    <div className="w-20 shrink-0"></div>
                    <p className="text-xs text-red-500 flex-1 max-w-md">{passwordError}</p>
                  </div>
                )}
              </div>
            )}

            {/* Access hint */}
            <div className="flex items-start gap-2 p-3 rounded-md bg-blue-500/10 border border-blue-500/20">
              <AlertTriangle className="h-4 w-4 text-blue-500 mt-0.5 shrink-0" />
              <div className="text-xs text-blue-600 dark:text-blue-400 space-y-1 flex-1">
                <p className="flex items-center gap-2">
                  <span>{t('settings.pprofAccessHint')}:</span>
                  <a
                    href={pprofUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="underline hover:text-blue-700 dark:hover:text-blue-300 font-medium"
                  >
                    {pprofUrl}
                  </a>
                </p>
                {usePasswordDraft && (
                  <p>
                    {t('settings.pprofAuthHint')}: {t('settings.pprofUsername')}: pprof /{' '}
                    {t('settings.pprofPassword')}: ***
                  </p>
                )}
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

export function BackupSection() {
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
                  <div className="capitalize">{key}</div>
                  <div className="text-center text-green-600">{summary.imported}</div>
                  <div className="text-center text-muted-foreground">{summary.skipped}</div>
                  <div className="text-center text-blue-600">{summary.updated}</div>
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

function MultiTenantUISection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const settingsEnabled = settings?.ui_multitenant_enabled === 'true';
  const settingsLayout: MultiTenantUILayout =
    settings?.[MULTITENANT_UI_LAYOUT_SETTING_KEY] === 'user_panel' ? 'user_panel' : 'current';
  const [localEnabled, setLocalEnabled] = useState(settingsEnabled);
  const [localLayout, setLocalLayout] = useState<MultiTenantUILayout>(settingsLayout);

  useEffect(() => {
    setLocalEnabled(settingsEnabled);
    setLocalLayout(settingsLayout);
  }, [settingsEnabled, settingsLayout]);

  const handleToggle = async (checked: boolean) => {
    const previous = localEnabled;
    setLocalEnabled(checked);
    try {
      await updateSetting.mutateAsync({
        key: 'ui_multitenant_enabled',
        value: checked ? 'true' : 'false',
      });
    } catch (error) {
      setLocalEnabled(previous);
      throw error;
    }
  };

  const handleLayoutChange = async (value: string) => {
    const nextLayout: MultiTenantUILayout = value === 'user_panel' ? 'user_panel' : 'current';
    const previous = localLayout;
    setLocalLayout(nextLayout);
    try {
      await updateSetting.mutateAsync({
        key: MULTITENANT_UI_LAYOUT_SETTING_KEY,
        value: nextLayout,
      });
    } catch (error) {
      setLocalLayout(previous);
      throw error;
    }
  };

  if (isLoading) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Monitor className="h-4 w-4 text-muted-foreground" />
          {t('settings.ui')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-sm font-medium text-foreground">
              {t('settings.enableMultiTenantUI')}
            </div>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.enableMultiTenantUIDesc')}
            </p>
          </div>
          <Switch
            aria-label={t('settings.enableMultiTenantUI')}
            checked={localEnabled}
            onCheckedChange={handleToggle}
            disabled={updateSetting.isPending}
          />
        </div>

        {localEnabled && (
          <div className="flex flex-col gap-3 rounded-lg border border-border bg-muted/20 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <Label className="text-sm font-medium text-foreground">
                {t('settings.multiTenantUILayout')}
              </Label>
              <p className="text-xs text-muted-foreground mt-1">
                {t('settings.multiTenantUILayoutDesc')}
              </p>
            </div>
            <Select
              value={localLayout}
              onValueChange={(value) => value && handleLayoutChange(value)}
              disabled={updateSetting.isPending}
            >
              <SelectTrigger className="w-full sm:w-56">
                <SelectValue>
                  {localLayout === 'user_panel'
                    ? t('settings.multiTenantUILayoutUserPanel')
                    : t('settings.multiTenantUILayoutCurrent')}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="current">{t('settings.multiTenantUILayoutCurrent')}</SelectItem>
                <SelectItem value="user_panel">
                  {t('settings.multiTenantUILayoutUserPanel')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
