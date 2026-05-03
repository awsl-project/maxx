import { useState, useEffect, useRef, Fragment, useId, useMemo } from 'react';
import {
  Settings,
  Monitor,
  FolderOpen,
  Database,
  Braces,
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
  Plus,
  Trash2,
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
import { Textarea } from '@/components/ui/textarea';
import { PageHeader } from '@/components/layout/page-header';
import { useSettings, useUpdateSetting, useDeleteSetting } from '@/hooks/queries';
import { useAuth } from '@/lib/auth-context';
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

const PAYLOAD_OVERRIDE_SETTING_KEY = 'payload_override_rules';
const PAYLOAD_OVERRIDE_RESERVED_ROOTS = new Set(['model', 'stream']);

type PayloadOverrideProtocol = 'codex';

interface PayloadOverrideFormRule {
  id: string;
  model: string;
  protocol: PayloadOverrideProtocol;
  paramsText: string;
}

interface StoredPayloadOverrideSelector {
  name?: string;
  protocol?: string;
}

interface StoredPayloadOverrideRule {
  models?: StoredPayloadOverrideSelector[];
  params?: Record<string, unknown>;
}

interface ParsedPayloadOverrideSetting {
  rules: PayloadOverrideFormRule[];
  parseError: string;
}

function createPayloadOverrideFormRule(
  overrides: Partial<PayloadOverrideFormRule> = {},
): PayloadOverrideFormRule {
  return {
    id: overrides.id ?? `payload-override-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    model: overrides.model ?? '',
    protocol: overrides.protocol ?? 'codex',
    paramsText: overrides.paramsText ?? '{}',
  };
}

function stringifyPayloadOverrideParams(value: Record<string, unknown> | undefined): string {
  if (!value || Object.keys(value).length === 0) {
    return '{}';
  }
  return JSON.stringify(value, null, 2);
}

function getPayloadOverridePathRoot(path: string): string {
  const trimmed = path.trim();
  if (!trimmed) {
    return '';
  }

  const match = /^[^.[\]]+/.exec(trimmed);
  return match ? match[0].toLowerCase() : trimmed.toLowerCase();
}

function getReservedPayloadOverridePath(path: string): string {
  const trimmed = path.trim();
  if (!trimmed) {
    return '';
  }
  return PAYLOAD_OVERRIDE_RESERVED_ROOTS.has(getPayloadOverridePathRoot(trimmed)) ? trimmed : '';
}

function normalizePayloadOverrideParamsText(paramsText: string): string {
  const trimmed = paramsText.trim();
  if (!trimmed) {
    return '';
  }

  try {
    const parsed = JSON.parse(trimmed);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return trimmed;
    }
    return JSON.stringify(parsed);
  } catch {
    return trimmed;
  }
}

function getPayloadOverrideRuleSnapshot(rules: PayloadOverrideFormRule[]): string {
  return JSON.stringify(
    rules.map((rule) => ({
      model: rule.model.trim(),
      protocol: rule.protocol,
      paramsText: normalizePayloadOverrideParamsText(rule.paramsText),
    })),
  );
}

function parsePayloadOverrideRulesSetting(raw: string): ParsedPayloadOverrideSetting {
  const trimmed = raw.trim();
  if (!trimmed) {
    return { rules: [], parseError: '' };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return {
      rules: [],
      parseError: 'settings.payloadOverrides.errors.loadInvalidJson',
    };
  }

  if (!Array.isArray(parsed)) {
    return {
      rules: [],
      parseError: 'settings.payloadOverrides.errors.loadInvalidArray',
    };
  }

  const rules: PayloadOverrideFormRule[] = [];
  for (const [ruleIndex, entry] of parsed.entries()) {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry)) {
      return {
        rules: [],
        parseError: 'settings.payloadOverrides.errors.loadInvalidRule',
      };
    }

    const typedEntry = entry as StoredPayloadOverrideRule;
    if (
      !typedEntry.params ||
      typeof typedEntry.params !== 'object' ||
      Array.isArray(typedEntry.params)
    ) {
      return {
        rules: [],
        parseError: 'settings.payloadOverrides.errors.loadInvalidParams',
      };
    }
    const paramPaths = Object.keys(typedEntry.params);
    if (paramPaths.length === 0) {
      return {
        rules: [],
        parseError: 'settings.payloadOverrides.errors.loadEmptyParams',
      };
    }
    for (const path of paramPaths) {
      if (!path.trim()) {
        return {
          rules: [],
          parseError: 'settings.payloadOverrides.errors.loadInvalidParamsPath',
        };
      }
      if (getReservedPayloadOverridePath(path)) {
        return {
          rules: [],
          parseError: 'settings.payloadOverrides.errors.loadReservedPath',
        };
      }
    }

    if (!Array.isArray(typedEntry.models) || typedEntry.models.length === 0) {
      return {
        rules: [],
        parseError: 'settings.payloadOverrides.errors.loadInvalidModels',
      };
    }

    for (const [selectorIndex, selector] of typedEntry.models.entries()) {
      const model = typeof selector?.name === 'string' ? selector.name.trim() : '';
      const protocol =
        typeof selector?.protocol === 'string' && selector.protocol.trim()
          ? selector.protocol.trim().toLowerCase()
          : 'codex';

      if (!model) {
        return {
          rules: [],
          parseError: 'settings.payloadOverrides.errors.loadInvalidModels',
        };
      }

      if (protocol !== 'codex') {
        return {
          rules: [],
          parseError: 'settings.payloadOverrides.errors.loadUnsupportedProtocol',
        };
      }

      rules.push(
        createPayloadOverrideFormRule({
          id: `payload-override-${ruleIndex}-${selectorIndex}`,
          model,
          protocol: 'codex',
          paramsText: stringifyPayloadOverrideParams(typedEntry.params),
        }),
      );
    }
  }

  return { rules, parseError: '' };
}

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
          {isAdmin && (
            <>
              <MultiTenantUISection />
              <TimezoneSection />
              <DataRetentionSection />
              <ForceProjectSection />
              <PayloadOverrideSection />
              <APITokenConcurrencySection />
              <AntigravitySection />
              <PprofSection />
              <BackupSection />
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

function DataRetentionSection() {
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

function PayloadOverrideSection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const deleteSetting = useDeleteSetting();
  const { t } = useTranslation();

  const rawSetting = settings?.[PAYLOAD_OVERRIDE_SETTING_KEY] ?? '';
  const parsedSetting = useMemo(() => parsePayloadOverrideRulesSetting(rawSetting), [rawSetting]);
  const serverSnapshot = parsedSetting.parseError
    ? '__invalid_payload_override_rules__'
    : getPayloadOverrideRuleSnapshot(parsedSetting.rules);

  const [rules, setRules] = useState<PayloadOverrideFormRule[]>([]);
  const [initialized, setInitialized] = useState(false);
  const [isDirty, setIsDirty] = useState(false);

  useEffect(() => {
    if (!isLoading && !initialized) {
      setRules(parsedSetting.rules);
      setInitialized(true);
      setIsDirty(false);
    }
  }, [initialized, isLoading, parsedSetting.rules]);

  const hasChanges = initialized && isDirty;

  useEffect(() => {
    if (initialized && !isDirty) {
      setRules(parsedSetting.rules);
    }
  }, [initialized, isDirty, parsedSetting.rules, serverSnapshot]);

  const validationError = (() => {
    const seen = new Set<string>();

    for (let i = 0; i < rules.length; i++) {
      const rule = rules[i];
      const model = rule.model.trim();
      if (!model) {
        return t('settings.payloadOverrides.errors.modelRequired', { index: i + 1 });
      }

      const dedupeKey = `${rule.protocol}:${model.toLowerCase()}`;
      if (seen.has(dedupeKey)) {
        return t('settings.payloadOverrides.errors.duplicateRule', { index: i + 1 });
      }
      seen.add(dedupeKey);

      if (!rule.paramsText.trim()) {
        return t('settings.payloadOverrides.errors.paramsRequired', { index: i + 1 });
      }

      let parsedParams: unknown;
      try {
        parsedParams = JSON.parse(rule.paramsText);
      } catch {
        return t('settings.payloadOverrides.errors.paramsInvalidJson', { index: i + 1 });
      }

      if (!parsedParams || typeof parsedParams !== 'object' || Array.isArray(parsedParams)) {
        return t('settings.payloadOverrides.errors.paramsObjectRequired', { index: i + 1 });
      }
      const paramPaths = Object.keys(parsedParams as Record<string, unknown>);
      if (paramPaths.length === 0) {
        return t('settings.payloadOverrides.errors.paramsPathsRequired', { index: i + 1 });
      }

      for (const path of paramPaths) {
        if (!path.trim()) {
          return t('settings.payloadOverrides.errors.paramsPathRequired', { index: i + 1 });
        }
        const reservedPath = getReservedPayloadOverridePath(path);
        if (reservedPath) {
          return t('settings.payloadOverrides.errors.reservedPath', {
            index: i + 1,
            path: reservedPath,
          });
        }
      }
    }

    return '';
  })();

  const isPending = updateSetting.isPending || deleteSetting.isPending;

  const updateRule = (id: string, updates: Partial<PayloadOverrideFormRule>) => {
    setRules((prev) => prev.map((rule) => (rule.id === id ? { ...rule, ...updates } : rule)));
    setIsDirty(true);
  };

  const handleAddRule = () => {
    setRules((prev) => [...prev, createPayloadOverrideFormRule()]);
    setIsDirty(true);
  };

  const handleRemoveRule = (id: string) => {
    setRules((prev) => prev.filter((rule) => rule.id !== id));
    setIsDirty(true);
  };

  const handleSave = async () => {
    if (validationError) {
      return;
    }

    if (rules.length === 0) {
      if (rawSetting.trim()) {
        await deleteSetting.mutateAsync(PAYLOAD_OVERRIDE_SETTING_KEY);
      }
      setIsDirty(false);
      return;
    }

    const payload = rules.map((rule) => ({
      models: [{ name: rule.model.trim(), protocol: rule.protocol }],
      params: JSON.parse(rule.paramsText) as Record<string, unknown>,
    }));

    await updateSetting.mutateAsync({
      key: PAYLOAD_OVERRIDE_SETTING_KEY,
      value: JSON.stringify(payload),
    });
    setIsDirty(false);
  };

  if (isLoading || !initialized) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <CardTitle className="text-base font-medium flex items-center gap-2">
              <Braces className="h-4 w-4 text-muted-foreground" />
              {t('settings.payloadOverrides.title')}
            </CardTitle>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.payloadOverrides.desc')}
            </p>
          </div>
          <Button
            onClick={handleSave}
            disabled={!hasChanges || !!validationError || isPending}
            size="sm"
          >
            {isPending ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        <div className="flex items-start gap-2 p-3 rounded-md bg-blue-500/10 border border-blue-500/20">
          <AlertTriangle className="h-4 w-4 text-blue-500 mt-0.5 shrink-0" />
          <p className="text-xs text-blue-600 dark:text-blue-400">
            {t('settings.payloadOverrides.precedenceHint')}
          </p>
        </div>

        {parsedSetting.parseError && (
          <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {t(parsedSetting.parseError)}
          </div>
        )}

        {validationError && (
          <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {validationError}
          </div>
        )}

        {rules.length === 0 ? (
          <div className="rounded-lg border border-dashed border-border px-4 py-8 text-center text-sm text-muted-foreground">
            {t('settings.payloadOverrides.empty')}
          </div>
        ) : (
          <div className="space-y-4">
            {rules.map((rule, index) => (
              <div key={rule.id} className="rounded-lg border border-border p-4 space-y-4">
                <div className="flex items-center justify-between gap-3">
                  <div className="text-sm font-medium text-foreground">
                    {t('settings.payloadOverrides.ruleLabel', { index: index + 1 })}
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={() => handleRemoveRule(rule.id)}
                    disabled={isPending}
                    aria-label={t('settings.payloadOverrides.removeRule', { index: index + 1 })}
                    className="shrink-0 text-muted-foreground hover:text-error"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>

                <div className="grid gap-4 md:grid-cols-[minmax(0,1fr),140px]">
                  <div className="space-y-2">
                    <Label className="text-sm font-medium text-muted-foreground">
                      {t('settings.payloadOverrides.modelPattern')}
                    </Label>
                    <Input
                      value={rule.model}
                      onChange={(e) => updateRule(rule.id, { model: e.target.value })}
                      placeholder={t('settings.payloadOverrides.modelPlaceholder')}
                      disabled={isPending}
                      className="w-full"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label className="text-sm font-medium text-muted-foreground">
                      {t('settings.payloadOverrides.protocol')}
                    </Label>
                    <Select
                      value={rule.protocol}
                      onValueChange={(value) => {
                        if (value === 'codex') {
                          updateRule(rule.id, { protocol: value });
                        }
                      }}
                      disabled={isPending}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="codex">
                          {t('settings.payloadOverrides.protocolCodex')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="space-y-2">
                  <Label className="text-sm font-medium text-muted-foreground">
                    {t('settings.payloadOverrides.paramsJson')}
                  </Label>
                  <Textarea
                    value={rule.paramsText}
                    onChange={(e) => updateRule(rule.id, { paramsText: e.target.value })}
                    placeholder={t('settings.payloadOverrides.paramsPlaceholder')}
                    rows={6}
                    disabled={isPending}
                    className="font-mono text-xs"
                  />
                </div>
              </div>
            ))}
          </div>
        )}

        <div className="flex flex-col gap-3 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs text-muted-foreground">
            {t('settings.payloadOverrides.paramsHint')}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleAddRule}
            disabled={isPending}
          >
            <Plus className="mr-2 h-4 w-4" />
            {t('settings.payloadOverrides.addRule')}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function APITokenConcurrencySection() {
  const { data: settings, isLoading } = useSettings();
  const updateSetting = useUpdateSetting();
  const { t } = useTranslation();

  const currentLimit = settings?.api_token_concurrent_limit || '5';
  const [limitDraft, setLimitDraft] = useState('');
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (!isLoading && !initialized) {
      setLimitDraft(currentLimit);
      setInitialized(true);
    }
  }, [isLoading, initialized, currentLimit]);

  const hasChanges = initialized && limitDraft !== currentLimit;

  useEffect(() => {
    if (initialized && !hasChanges) {
      setLimitDraft(currentLimit);
    }
  }, [currentLimit, initialized, hasChanges]);

  const parsedLimit = parseInt(limitDraft, 10);
  const isValid = !isNaN(parsedLimit) && parsedLimit >= 1;

  const handleSaveLimit = async () => {
    if (!isValid || !hasChanges) return;
    await updateSetting.mutateAsync({
      key: 'api_token_concurrent_limit',
      value: limitDraft,
    });
  };

  if (isLoading || !initialized) return null;

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base font-medium flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              {t('settings.apiTokenConcurrency')}
            </CardTitle>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.apiTokenConcurrencyDesc')}
            </p>
          </div>
          <Button
            onClick={handleSaveLimit}
            disabled={!hasChanges || !isValid || updateSetting.isPending}
            size="sm"
          >
            {updateSetting.isPending ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
          <Label className="text-sm font-medium text-muted-foreground shrink-0">
            {t('settings.apiTokenConcurrencyLimit')}
          </Label>
          <Input
            type="number"
            value={limitDraft}
            onChange={(e) => setLimitDraft(e.target.value)}
            className="w-24"
            min={1}
            disabled={updateSetting.isPending}
          />
          <span className="text-xs text-muted-foreground">
            {t('settings.concurrentRequestsUnit')}
          </span>
          <span className="text-xs text-muted-foreground">
            ({t('settings.defaultValue', { value: 5 })})
          </span>
        </div>
        <p className="text-xs text-muted-foreground">{t('settings.apiTokenConcurrencyHint')}</p>
      </CardContent>
    </Card>
  );
}

function AntigravitySection() {
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

function PprofSection() {
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
                    href={`http://localhost:${portDraft}/debug/pprof/`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="underline hover:text-blue-700 dark:hover:text-blue-300 font-medium"
                  >
                    http://localhost:{portDraft}/debug/pprof/
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

  const enabled = settings?.ui_multitenant_enabled === 'true';

  const handleToggle = async (checked: boolean) => {
    await updateSetting.mutateAsync({
      key: 'ui_multitenant_enabled',
      value: checked ? 'true' : 'false',
    });
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
        <div className="flex items-center justify-between">
          <div>
            <div className="text-sm font-medium text-foreground">{t('settings.enableMultiTenantUI')}</div>
            <p className="text-xs text-muted-foreground mt-1">{t('settings.enableMultiTenantUIDesc')}</p>
          </div>
          <Switch checked={enabled} onCheckedChange={handleToggle} disabled={updateSetting.isPending} />
        </div>
      </CardContent>
    </Card>
  );
}
