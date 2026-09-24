import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Activity,
  Bell,
  Copy,
  Eye,
  EyeOff,
  Gift,
  KeyRound,
  Loader2,
  LogOut,
  Server,
  Trophy,
  UserRound,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Input,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui';
import { LanguageToggle } from '@/components/language-toggle';
import { useAuth } from '@/lib/auth-context';
import {
  useCreateUserPanelAPIToken,
  useRegenerateUserPanelAPIToken,
  useRevealUserPanelAPIToken,
  useUserPanelAvailableModels,
  useUserPanelAnnouncement,
  useUserPanelAvailableModelRoutes,
  useUserPanelDailyCheckInStatus,
  useUserPanelDailyCheckIn,
  useCreateUserPanelRedemptionCodes,
  useRedeemUserPanelCode,
  useUserPanelAPIToken,
  useUserPanelConsumptionLeaderboard,
  useUserPanelModelHealth,
  usePublicSettings,
  useUserPanelUsageStats,
} from '@/hooks/queries';
import type {
  APIToken,
  UsageStatsFilter,
  UsageStats,
  UserPanelConsumptionLeaderboardRow,
  UserPanelModelHealthRow,
} from '@/lib/transport';
import { cn } from '@/lib/utils';
import {
  getUserPanelAnnouncementFingerprint,
  getUserPanelAnnouncementSeenStorageKey,
  isUserPanelAnnouncementUnread,
} from '@/lib/user-panel-announcement';
import { buildUserPanelEndpointHints } from '@/lib/user-panel-endpoints';
import { visibleUserPanelModelRouteGroups } from '@/lib/user-panel-model-routes';
import { MarkdownContent } from '@/lib/markdown';
import {
  getUserPanelTabStorageKey,
  resolveUserPanelTab,
  updateUserPanelTabSearch,
  type UserPanelTab,
} from '@/lib/user-panel-tabs';

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value || 0);
}

function formatQuotaBalance(value: number) {
  return `$${((value || 0) / 1_000_000_000).toFixed(6)}`;
}

function formatQuotaAmount(value: number) {
  const amount = (value || 0) / 1_000_000_000;
  return `$${Number.isInteger(amount) ? amount.toFixed(0) : amount.toFixed(2)}`;
}

function formatCostAmount(value: number) {
  return `$${((value || 0) / 1_000_000_000).toFixed(4)}`;
}

function getLocalDayBounds(now = new Date()) {
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  return {
    start: start.toISOString(),
    end: now.toISOString(),
  };
}

function totalTokens(stats?: UsageStats[]) {
  return (stats ?? []).reduce(
    (sum, item) => sum + item.inputTokens + item.outputTokens + item.cacheRead + item.cacheWrite,
    0,
  );
}

function parseDailyCheckInAmountSetting(value?: string) {
  const amount = Number(value);
  if (!Number.isFinite(amount) || amount <= 0) return undefined;
  return Math.round(amount * 1_000_000_000);
}

function getTokenStatus(token: APIToken) {
  if (!token.isEnabled) return 'disabled';
  if (token.expiresAt && new Date(token.expiresAt).getTime() <= Date.now()) return 'expired';
  return 'active';
}

function ConsumptionLeaderboardCard({
  title,
  rows,
  isLoading,
  isError,
  currentUserID,
}: {
  title: string;
  rows: UserPanelConsumptionLeaderboardRow[];
  isLoading: boolean;
  isError: boolean;
  currentUserID?: number;
}) {
  const { t } = useTranslation();

  return (
    <Card className="border-border bg-card shadow-sm">
      <CardHeader className="border-b border-border">
        <CardTitle className="flex items-center gap-2 text-base font-medium">
          <Trophy className="size-4 text-amber-500" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-5">
        {isLoading ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('common.loading')}</p>
        ) : isError ? (
          <p className="py-8 text-center text-sm text-destructive">
            {t('userPanel.consumptionLeaderboardLoadFailed')}
          </p>
        ) : rows.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('common.noData')}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('common.name')}</TableHead>
                <TableHead className="text-right">{t('userPanel.amount')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => {
                const isCurrentUser = currentUserID === row.userID;
                return (
                  <TableRow
                    key={row.tokenID || row.userID}
                    data-state={isCurrentUser ? 'selected' : undefined}
                    className={cn(
                      isCurrentUser &&
                        'relative bg-primary/5 shadow-[inset_3px_0_0_hsl(var(--primary))] hover:bg-primary/10 data-[state=selected]:bg-primary/5',
                    )}
                  >
                    <TableCell className="font-medium text-foreground">
                      <span className="flex min-w-0 items-center">
                        <span className="inline-flex min-w-0 items-center gap-1.5">
                          <span className="truncate">
                            {row.tokenName || row.username || `#${row.tokenID || row.userID}`}
                          </span>
                          {row.active ? (
                            <span
                              aria-label="active"
                              className="size-2 shrink-0 animate-pulse rounded-full bg-emerald-500 shadow-[0_0_0_4px_rgba(16,185,129,0.12)]"
                            />
                          ) : null}
                        </span>
                      </span>
                    </TableCell>
                    <TableCell className="text-right font-mono font-semibold tabular-nums">
                      {formatCostAmount(row.cost)}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

const MODEL_HEALTH_PAGE_SIZE = 50;

function ModelHealthCard({
  rows,
  isLoading,
  isError,
  hasUserPanelToken,
}: {
  rows: UserPanelModelHealthRow[];
  isLoading: boolean;
  isError: boolean;
  hasUserPanelToken: boolean;
}) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const normalizedSearch = search.trim().toLowerCase();
  const filteredRows = useMemo(() => {
    if (!normalizedSearch) return rows;
    return rows.filter((row) => {
      const routeType = formatModelHealthClientType(
        t(`externalModels.routeTypes.${row.clientType}`),
      );
      return [row.model, routeType, row.providerName]
        .filter(Boolean)
        .some((value) => value.toLowerCase().includes(normalizedSearch));
    });
  }, [normalizedSearch, rows, t]);
  const totalPages = Math.max(1, Math.ceil(filteredRows.length / MODEL_HEALTH_PAGE_SIZE));
  const currentPage = Math.min(page, totalPages);
  const visibleRows = filteredRows.slice(
    (currentPage - 1) * MODEL_HEALTH_PAGE_SIZE,
    currentPage * MODEL_HEALTH_PAGE_SIZE,
  );

  useEffect(() => {
    setPage(1);
  }, [normalizedSearch, rows.length]);

  return (
    <Card className="border-border bg-card shadow-sm">
      <CardHeader className="border-b border-border">
        <CardTitle className="flex items-center gap-2 text-base font-medium">
          <Activity className="size-4 text-primary" />
          {t('userPanel.modelHealthTitle')}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-5">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1">
              <HealthDot status="ok" /> {t('userPanel.modelHealthOk')}
            </span>
            <span className="inline-flex items-center gap-1">
              <HealthDot status="error" /> {t('userPanel.modelHealthError')}
            </span>
            <span className="inline-flex items-center gap-1">
              <HealthDot status="unknown" /> {t('userPanel.modelHealthUnknown')}
            </span>
          </div>
          {rows.length > MODEL_HEALTH_PAGE_SIZE && (
            <Input
              className="h-8 w-full max-w-64"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t('userPanel.modelHealthSearchPlaceholder')}
            />
          )}
        </div>
        {isLoading ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('common.loading')}</p>
        ) : isError ? (
          <p className="py-8 text-center text-sm text-destructive">
            {t('userPanel.modelStatusLoadFailed')}
          </p>
        ) : !hasUserPanelToken ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            {t('userPanel.modelHealthNoToken')}
          </p>
        ) : rows.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            {t('userPanel.modelHealthNoCheckableRoutes')}
          </p>
        ) : filteredRows.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            {t('userPanel.modelHealthNoMatches')}
          </p>
        ) : (
          <div className="space-y-3">
            <div className="text-xs text-muted-foreground">
              {t('userPanel.modelHealthShowing', {
                start: (currentPage - 1) * MODEL_HEALTH_PAGE_SIZE + 1,
                end: Math.min(currentPage * MODEL_HEALTH_PAGE_SIZE, filteredRows.length),
                total: filteredRows.length,
                overall: rows.length,
              })}
            </div>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('userPanel.modelHealthTarget')}</TableHead>
                    <TableHead>{t('userPanel.modelHealth24h')}</TableHead>
                    <TableHead className="text-right">
                      {t('userPanel.modelHealthCurrent')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visibleRows.map((row) => (
                    <TableRow key={`${row.model}:${row.routeID}:${row.providerID}`}>
                      <TableCell className="min-w-[14rem] max-w-[24rem]">
                        <div className="truncate font-medium text-foreground">{row.model}</div>
                        <div className="truncate text-xs text-muted-foreground">
                          {formatModelHealthClientType(
                            t(`externalModels.routeTypes.${row.clientType}`),
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex min-w-[16rem] items-center gap-1">
                          {row.points.map((point, index) => (
                            <HealthDot
                              key={index}
                              status={point.status}
                              title={formatHealthPointTitle(point)}
                            />
                          ))}
                        </div>
                      </TableCell>
                      <TableCell className="text-right">
                        <span className="inline-flex justify-end">
                          <HealthDot
                            status={row.current.status}
                            title={formatHealthPointTitle(row.current)}
                          />
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            {totalPages > 1 && (
              <div className="flex items-center justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((value) => Math.max(1, value - 1))}
                  disabled={currentPage <= 1}
                >
                  {t('userPanel.modelHealthPrevious')}
                </Button>
                <span className="text-xs text-muted-foreground">
                  {t('userPanel.modelHealthPage', { page: currentPage, total: totalPages })}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((value) => Math.min(totalPages, value + 1))}
                  disabled={currentPage >= totalPages}
                >
                  {t('userPanel.modelHealthNext')}
                </Button>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function formatModelHealthClientType(label: string) {
  return label.replace(/(?:\s*路由|\s+route)$/i, '');
}

function HealthDot({
  status,
  title,
}: {
  status: UserPanelModelHealthRow['current']['status'];
  title?: string;
}) {
  return (
    <span
      title={title}
      className={cn(
        'inline-block size-2.5 rounded-full border',
        status === 'ok' && 'border-emerald-500 bg-emerald-500',
        status === 'error' && 'border-red-500 bg-red-500',
        status === 'unknown' && 'border-muted-foreground/35 bg-muted-foreground/20',
      )}
    />
  );
}

function formatHealthPointTitle(point: UserPanelModelHealthRow['current']) {
  const parts: string[] = [point.status];
  if (point.checkedAt) parts.push(new Date(point.checkedAt).toLocaleString());
  if (point.latencyMs) parts.push(`${point.latencyMs}ms`);
  if (point.error) parts.push(point.error);
  return parts.join(' · ');
}

export function UserPanelPage() {
  const { t } = useTranslation();
  const { logout, user } = useAuth();
  const { data: userPanelTokenResponse, isLoading: tokenLoading } = useUserPanelAPIToken();
  const [usageClock, setUsageClock] = useState(() => new Date());
  const todayBounds = useMemo(() => getLocalDayBounds(usageClock), [usageClock]);
  const todayUsageFilter = useMemo<UsageStatsFilter>(
    () => ({
      granularity: 'day',
      start: todayBounds.start,
      end: todayBounds.end,
    }),
    [todayBounds.end, todayBounds.start],
  );
  const totalUsageFilter = useMemo<UsageStatsFilter>(
    () => ({
      granularity: 'month',
      start: '2020-01-01T00:00:00.000Z',
      end: todayBounds.end,
    }),
    [todayBounds.end],
  );
  const {
    data: todayUsageStats,
    isLoading: todayUsageLoading,
    isError: todayUsageError,
  } = useUserPanelUsageStats(todayUsageFilter, {
    enabled: Boolean(user),
  });
  const {
    data: totalUsageStats,
    isLoading: totalUsageLoading,
    isError: totalUsageError,
  } = useUserPanelUsageStats(totalUsageFilter, {
    enabled: Boolean(user),
  });
  const { data: publicSettings } = usePublicSettings();
  const externalModelListEnabled = publicSettings?.external_model_list_enabled === 'true';
  const {
    data: availableModels,
    isLoading: availableModelsLoading,
    isError: availableModelsError,
  } = useUserPanelAvailableModels(Boolean(user));
  const {
    data: availableModelRoutes,
    isLoading: availableModelRoutesLoading,
    isError: availableModelRoutesError,
  } = useUserPanelAvailableModelRoutes(Boolean(user) && externalModelListEnabled);
  const {
    data: consumptionLeaderboard,
    isLoading: consumptionLeaderboardLoading,
    isError: consumptionLeaderboardError,
  } = useUserPanelConsumptionLeaderboard(Boolean(user));
  const { data: userPanelAnnouncement } = useUserPanelAnnouncement(Boolean(user));
  const localUserPanelDayKey = useMemo(() => todayBounds.start.slice(0, 10), [todayBounds.start]);
  const dailyCheckInEnabled = publicSettings?.user_panel_daily_checkin_enabled === 'true';
  const { data: dailyCheckInStatus } = useUserPanelDailyCheckInStatus(
    dailyCheckInEnabled,
    localUserPanelDayKey,
  );
  const createUserPanelToken = useCreateUserPanelAPIToken();
  const regenerateUserPanelToken = useRegenerateUserPanelAPIToken();
  const revealUserPanelToken = useRevealUserPanelAPIToken();
  const dailyCheckIn = useUserPanelDailyCheckIn();
  const createUserPanelRedemptionCodes = useCreateUserPanelRedemptionCodes();
  const redeemUserPanelCode = useRedeemUserPanelCode();
  const { mutateAsync: runDailyCheckIn } = dailyCheckIn;
  const [copiedEndpointId, setCopiedEndpointId] = useState('');
  const [keyCopied, setKeyCopied] = useState(false);
  const [oneTimeToken, setOneTimeToken] = useState('');
  const [revealedUserPanelToken, setRevealedUserPanelToken] = useState('');
  const [revealKeyError, setRevealKeyError] = useState('');
  const [dailyCheckInMessage, setDailyCheckInMessage] = useState('');
  const [dailyCheckInDone, setDailyCheckInDone] = useState(false);
  const [redemptionCode, setRedemptionCode] = useState('');
  const [redemptionMessage, setRedemptionMessage] = useState('');
  const [selfRedemptionCount, setSelfRedemptionCount] = useState('1');
  const [selfRedemptionAmount, setSelfRedemptionAmount] = useState('1');
  const [selfRedemptionNote, setSelfRedemptionNote] = useState('');
  const [selfRedemptionMessage, setSelfRedemptionMessage] = useState('');
  const [createdSelfRedemptionCodes, setCreatedSelfRedemptionCodes] = useState<string[]>([]);
  const [announcementOpen, setAnnouncementOpen] = useState(false);
  const [announcementSeenFingerprint, setAnnouncementSeenFingerprint] = useState('');
  const autoDailyCheckInStartedRef = useRef(false);
  const tabStorageKey = getUserPanelTabStorageKey(user?.id);
  const [activeTab, setActiveTab] = useState<UserPanelTab>(() => {
    if (typeof window === 'undefined') return 'main';
    const params = new URLSearchParams(window.location.search);
    const navigationEntry = window.performance.getEntriesByType('navigation')[0] as
      | PerformanceNavigationTiming
      | undefined;
    const allowStoredTab = navigationEntry?.type === 'reload';
    return resolveUserPanelTab({
      urlTab: params.get('tab'),
      storedTab: window.localStorage.getItem(getUserPanelTabStorageKey(user?.id)),
      allowStoredTab,
    });
  });

  const {
    data: modelHealthRows,
    isLoading: modelHealthLoading,
    isError: modelHealthError,
  } = useUserPanelModelHealth(Boolean(user));

  const userPanelToken = userPanelTokenResponse?.apiToken ?? undefined;
  const todayTokenUsage = totalTokens(todayUsageStats);
  const totalTokenUsage = totalTokens(totalUsageStats);
  const hasCheckedInToday = dailyCheckInStatus?.alreadyCheckedIn || dailyCheckInDone;
  const dailyCheckInRewardAmount =
    dailyCheckInStatus?.rewardAmount ??
    parseDailyCheckInAmountSetting(publicSettings?.user_panel_daily_checkin_amount);
  const selfRedemptionCountNumber = Math.max(
    1,
    Math.min(100, Number.parseInt(selfRedemptionCount, 10) || 1),
  );
  const selfRedemptionAmountUSD = Number(selfRedemptionAmount);
  const selfRedemptionAmountValue = Number.isFinite(selfRedemptionAmountUSD)
    ? Math.round(selfRedemptionAmountUSD * 1_000_000_000)
    : 0;
  const selfRedemptionTotalValue = selfRedemptionAmountValue * selfRedemptionCountNumber;
  const canCreateSelfRedemptionCodes =
    selfRedemptionAmountValue > 0 &&
    Boolean(userPanelToken) &&
    (userPanelToken?.quotaBalance ?? 0) >= selfRedemptionTotalValue;
  const maskedUserPanelToken = userPanelToken?.tokenPrefix || 'maxx_••••';
  const userPanelTokenValue = revealedUserPanelToken || maskedUserPanelToken;
  const userPanelTokenRevealed = Boolean(revealedUserPanelToken);
  const origin = typeof window === 'undefined' ? '' : window.location.origin;
  const endpointHints = buildUserPanelEndpointHints(origin, publicSettings).map((endpoint) => ({
    ...endpoint,
    label:
      endpoint.id === 'openai'
        ? t('userPanel.routeOpenAI')
        : endpoint.id === 'codex'
          ? t('userPanel.routeCodex')
          : endpoint.id === 'claude'
            ? t('userPanel.routeClaude')
            : t('userPanel.routeGemini'),
  }));
  const modelRouteGroups = externalModelListEnabled
    ? visibleUserPanelModelRouteGroups(availableModelRoutes ?? [])
    : [];
  const availableModelCount = availableModels?.length ?? 0;
  const announcementMarkdown = userPanelAnnouncement?.enabled
    ? userPanelAnnouncement.markdown.trim()
    : '';
  const announcementFingerprint = getUserPanelAnnouncementFingerprint(announcementMarkdown);
  const announcementSeenStorageKey = getUserPanelAnnouncementSeenStorageKey(user?.id);
  const announcementUnread = isUserPanelAnnouncementUnread(
    announcementMarkdown,
    announcementSeenFingerprint,
  );
  const dailyCheckInVisible =
    dailyCheckInEnabled && Boolean(dailyCheckInStatus) && !dailyCheckInStatus?.blacklisted;

  useEffect(() => {
    if (typeof window === 'undefined') {
      setAnnouncementSeenFingerprint('');
      return;
    }
    setAnnouncementSeenFingerprint(window.localStorage.getItem(announcementSeenStorageKey) ?? '');
  }, [announcementSeenStorageKey]);

  useEffect(() => {
    setRevealedUserPanelToken('');
    setRevealKeyError('');
  }, [userPanelToken?.id]);

  useEffect(() => {
    autoDailyCheckInStartedRef.current = false;
    setDailyCheckInDone(false);
    setDailyCheckInMessage('');
  }, [localUserPanelDayKey]);

  useEffect(() => {
    if (!dailyCheckInEnabled) {
      autoDailyCheckInStartedRef.current = false;
      setDailyCheckInDone(false);
      setDailyCheckInMessage('');
      return;
    }

    if (dailyCheckInStatus?.blacklisted) {
      setDailyCheckInDone(false);
      setDailyCheckInMessage('');
      return;
    }

    if (dailyCheckInStatus?.alreadyCheckedIn) {
      setDailyCheckInDone(true);
      setDailyCheckInMessage(t('userPanel.dailyCheckInAlreadyDone'));
      return;
    }

    if (!dailyCheckInStatus || autoDailyCheckInStartedRef.current) {
      return;
    }

    autoDailyCheckInStartedRef.current = true;
    setDailyCheckInMessage(t('userPanel.dailyCheckInAutoRunning'));
    runDailyCheckIn()
      .then((result) => {
        setDailyCheckInDone(result.alreadyCheckedIn || result.checkedIn);
        setDailyCheckInMessage(
          result.alreadyCheckedIn
            ? t('userPanel.dailyCheckInAlreadyDone')
            : t('userPanel.dailyCheckInSuccess', {
                amount: formatQuotaAmount(result.rewardAmount),
              }),
        );
      })
      .catch(() => {
        setDailyCheckInMessage(t('userPanel.dailyCheckInError'));
      });
  }, [dailyCheckInEnabled, dailyCheckInStatus, runDailyCheckIn, t]);

  useEffect(() => {
    const interval = window.setInterval(() => setUsageClock(new Date()), 60_000);
    return () => window.clearInterval(interval);
  }, []);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const urlTab = new URLSearchParams(window.location.search).get('tab');
    const storedTab = window.localStorage.getItem(tabStorageKey);
    const navigationEntry = window.performance.getEntriesByType('navigation')[0] as
      | PerformanceNavigationTiming
      | undefined;
    const allowStoredTab = navigationEntry?.type === 'reload';
    setActiveTab(resolveUserPanelTab({ urlTab, storedTab, allowStoredTab }));
  }, [tabStorageKey]);

  const handleTabChange = (value: string | null) => {
    const nextTab = resolveUserPanelTab({ urlTab: value });
    setActiveTab(nextTab);
    if (typeof window === 'undefined') {
      return;
    }
    window.localStorage.setItem(tabStorageKey, nextTab);
    const nextSearch = updateUserPanelTabSearch(window.location.search, nextTab);
    window.history.replaceState(null, '', `${window.location.pathname}${nextSearch}`);
  };

  const handleCopyEndpoint = async (endpointId: string, url: string) => {
    if (!url || typeof navigator === 'undefined' || !navigator.clipboard) return;
    await navigator.clipboard.writeText(url);
    setCopiedEndpointId(endpointId);
    window.setTimeout(() => setCopiedEndpointId(''), 1600);
  };

  const handleCopyOneTimeToken = async () => {
    if (!oneTimeToken || typeof navigator === 'undefined' || !navigator.clipboard) return;
    await navigator.clipboard.writeText(oneTimeToken);
    setKeyCopied(true);
    window.setTimeout(() => setKeyCopied(false), 1600);
  };

  const handleToggleUserPanelTokenReveal = async () => {
    if (revealedUserPanelToken) {
      setRevealedUserPanelToken('');
      setRevealKeyError('');
      return;
    }

    setRevealKeyError('');
    try {
      const result = await revealUserPanelToken.mutateAsync();
      setRevealedUserPanelToken(result.token);
    } catch {
      setRevealKeyError(t('userPanel.revealKeyError'));
    }
  };

  const handleCreateUserPanelToken = async () => {
    const result = await createUserPanelToken.mutateAsync();
    setOneTimeToken(result.token);
    setRevealedUserPanelToken(result.token);
    setRevealKeyError('');
  };

  const handleRegenerateUserPanelToken = async () => {
    if (typeof window !== 'undefined' && !window.confirm(t('userPanel.regenerateConfirm'))) return;
    const result = await regenerateUserPanelToken.mutateAsync();
    setOneTimeToken(result.token);
    setRevealedUserPanelToken(result.token);
    setRevealKeyError('');
    setKeyCopied(false);
  };

  const handleCreateSelfRedemptionCodes = async () => {
    if (!canCreateSelfRedemptionCodes) return;
    setSelfRedemptionMessage('');
    try {
      const result = await createUserPanelRedemptionCodes.mutateAsync({
        count: selfRedemptionCountNumber,
        amount: selfRedemptionAmountValue,
        note: selfRedemptionNote.trim(),
      });
      setCreatedSelfRedemptionCodes(result.items.map((item) => item.code));
      setSelfRedemptionMessage(
        t('userPanel.selfRedemptionCreateSuccess', {
          count: result.items.length,
          amount: formatQuotaAmount(selfRedemptionTotalValue),
        }),
      );
    } catch {
      setSelfRedemptionMessage(t('userPanel.selfRedemptionCreateError'));
    }
  };

  const handleRedeemCode = async () => {
    const trimmed = redemptionCode.trim();
    if (!trimmed) return;
    setRedemptionMessage('');
    try {
      const result = await redeemUserPanelCode.mutateAsync(trimmed);
      setRedemptionCode('');
      setRedemptionMessage(
        t('userPanel.redemptionSuccess', { amount: formatQuotaAmount(result.amount) }),
      );
    } catch {
      setRedemptionMessage(t('userPanel.redemptionError'));
    }
  };

  const handleAnnouncementOpen = () => {
    setAnnouncementOpen(true);
    if (!announcementFingerprint || typeof window === 'undefined') return;
    window.localStorage.setItem(announcementSeenStorageKey, announcementFingerprint);
    setAnnouncementSeenFingerprint(announcementFingerprint);
  };

  const tokenActionPending = createUserPanelToken.isPending || regenerateUserPanelToken.isPending;
  const revealActionPending = revealUserPanelToken.isPending;

  return (
    <main className="min-h-svh bg-muted/30 px-4 py-6 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-5">
        <header className="flex flex-col gap-4 rounded-2xl border border-border bg-card p-5 shadow-sm sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-11 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm">
              <UserRound className="size-5" />
            </div>
            <h1 className="text-xl font-semibold tracking-tight">{t('userPanel.title')}</h1>
          </div>
          <div className="flex items-center gap-2 self-start sm:self-center">
            {announcementMarkdown ? (
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="relative size-9 text-muted-foreground transition-colors hover:text-foreground"
                onClick={handleAnnouncementOpen}
                aria-label={t('userPanel.announcementOpen')}
                title={t('userPanel.announcementOpen')}
              >
                <Bell className="size-4" />
                {announcementUnread ? (
                  <span className="absolute right-1.5 top-1.5 size-2 rounded-full bg-red-500 ring-2 ring-card" />
                ) : null}
              </Button>
            ) : null}
            <LanguageToggle />
            <Button
              variant="outline"
              className="gap-2 border-destructive/30 text-destructive hover:bg-destructive/10"
              onClick={logout}
            >
              <LogOut className="size-4" />
              {t('nav.logout')}
            </Button>
          </div>
        </header>

        <Tabs value={activeTab} onValueChange={handleTabChange} className="space-y-5">
          <TabsList className="grid w-full grid-cols-4 rounded-xl p-1">
            <TabsTrigger value="main">{t('userPanel.mainTab')}</TabsTrigger>
            <TabsTrigger value="consumption">{t('userPanel.consumptionTab')}</TabsTrigger>
            <TabsTrigger value="redemption">{t('userPanel.redemptionTab')}</TabsTrigger>
            <TabsTrigger value="model-status">{t('userPanel.modelStatusTab')}</TabsTrigger>
          </TabsList>

          <TabsContent value="main" className="space-y-5">
            {dailyCheckInVisible && (
              <Card className="border-border bg-card shadow-sm">
                <CardContent className="flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex items-center gap-3">
                    <div className="flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
                      <Gift className="size-5" />
                    </div>
                    <div>
                      <p className="text-sm font-medium text-foreground">
                        {t('userPanel.dailyCheckInTitle')}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {dailyCheckInRewardAmount
                          ? t('userPanel.dailyCheckInReward', {
                              amount: formatQuotaAmount(dailyCheckInRewardAmount),
                            })
                          : t('common.loading')}
                      </p>
                      {dailyCheckInMessage ? (
                        <p className="mt-1 text-xs text-muted-foreground">{dailyCheckInMessage}</p>
                      ) : null}
                    </div>
                  </div>
                  <Badge variant={hasCheckedInToday ? 'success' : 'secondary'} className="shrink-0">
                    {hasCheckedInToday
                      ? t('userPanel.dailyCheckInDone')
                      : dailyCheckIn.isPending
                        ? t('userPanel.dailyCheckInAutoRunning')
                        : t('userPanel.dailyCheckInAutoPending')}
                  </Badge>
                </CardContent>
              </Card>
            )}

            <Card className="border-border bg-card shadow-sm">
              <CardContent className="space-y-3 p-4">
                {oneTimeToken ? (
                  <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3">
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                        {t('userPanel.oneTimeKeyTitle')}
                      </p>
                      <Button size="sm" className="h-8 gap-2" onClick={handleCopyOneTimeToken}>
                        <Copy className="size-3.5" />
                        {keyCopied ? t('common.copied') : t('userPanel.copyKey')}
                      </Button>
                    </div>
                    <p className="mt-2 break-all rounded-md bg-background px-3 py-2 font-mono text-xs">
                      {oneTimeToken}
                    </p>
                  </div>
                ) : null}

                {tokenLoading ? (
                  <div className="text-sm text-muted-foreground">{t('common.loading')}</div>
                ) : userPanelToken ? (
                  <div className="space-y-3">
                    <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                      <div className="flex min-w-0 items-center gap-2">
                        <KeyRound className="size-4 shrink-0 text-muted-foreground" />
                        <p className="truncate text-sm font-medium">{userPanelToken.name}</p>
                        <Badge
                          variant={
                            getTokenStatus(userPanelToken) === 'active'
                              ? 'success'
                              : getTokenStatus(userPanelToken) === 'expired'
                                ? 'warning'
                                : 'danger'
                          }
                        >
                          {t(`userPanel.keyStatus.${getTokenStatus(userPanelToken)}`)}
                        </Badge>
                      </div>
                      <Button
                        size="sm"
                        variant="outline"
                        className="h-8 gap-2 border-destructive/30 text-destructive hover:bg-destructive/10"
                        disabled={tokenActionPending}
                        onClick={handleRegenerateUserPanelToken}
                      >
                        <KeyRound className="size-3.5" />
                        {tokenActionPending ? t('common.loading') : t('userPanel.regenerateKey')}
                      </Button>
                    </div>

                    <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_460px] lg:items-center">
                      <div className="space-y-1">
                        <div className="grid grid-cols-[minmax(0,1fr)_2.25rem] gap-1">
                          <Input
                            readOnly
                            type={userPanelTokenRevealed ? 'text' : 'password'}
                            value={userPanelTokenValue}
                            className="h-9 font-mono text-xs transition-colors"
                            aria-label={t('userPanel.myKey')}
                          />
                          <Button
                            type="button"
                            variant="outline"
                            size="icon"
                            className="size-9 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                            disabled={tokenActionPending || revealActionPending}
                            aria-label={t(
                              userPanelTokenRevealed ? 'userPanel.hideKey' : 'userPanel.showKey',
                            )}
                            aria-pressed={userPanelTokenRevealed}
                            title={t(
                              userPanelTokenRevealed ? 'userPanel.hideKey' : 'userPanel.showKey',
                            )}
                            onClick={handleToggleUserPanelTokenReveal}
                          >
                            {revealActionPending ? (
                              <Loader2 className="size-3.5 animate-spin" />
                            ) : userPanelTokenRevealed ? (
                              <EyeOff className="size-3.5" />
                            ) : (
                              <Eye className="size-3.5" />
                            )}
                          </Button>
                        </div>
                        {revealKeyError ? (
                          <p className="text-xs text-destructive">{revealKeyError}</p>
                        ) : userPanelTokenRevealed ? (
                          <p className="text-xs text-muted-foreground">
                            {t('userPanel.fullKeyVisible')}
                          </p>
                        ) : null}
                      </div>
                      <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                        <div className="rounded-md border border-border bg-muted/25 px-3 py-2">
                          <p className="text-[11px] text-muted-foreground">
                            {t('userPanel.quotaBalance')}
                          </p>
                          <p className="mt-1 truncate font-mono text-xs font-semibold tabular-nums text-foreground">
                            {formatQuotaBalance(userPanelToken.quotaBalance)}
                          </p>
                        </div>
                        <div className="rounded-md border border-border bg-muted/25 px-3 py-2">
                          <p className="text-[11px] text-muted-foreground">
                            {t('userPanel.todayTokenUsage')}
                          </p>
                          <p className="mt-1 truncate font-mono text-xs font-semibold tabular-nums text-foreground">
                            {todayUsageLoading || (todayUsageError && !todayUsageStats)
                              ? '—'
                              : formatNumber(todayTokenUsage)}
                          </p>
                        </div>
                        <div className="rounded-md border border-border bg-muted/25 px-3 py-2">
                          <p className="text-[11px] text-muted-foreground">
                            {t('userPanel.totalTokenUsage')}
                          </p>
                          <p className="mt-1 truncate font-mono text-xs font-semibold tabular-nums text-foreground">
                            {totalUsageLoading || (totalUsageError && !totalUsageStats)
                              ? '—'
                              : formatNumber(totalTokenUsage)}
                          </p>
                        </div>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="flex flex-col gap-3 rounded-lg border border-dashed border-border bg-muted/20 p-4 sm:flex-row sm:items-center sm:justify-between">
                    <p className="text-sm font-medium">{t('userPanel.noDedicatedKey')}</p>
                    <Button
                      size="sm"
                      className="h-8 gap-2"
                      disabled={tokenActionPending}
                      onClick={handleCreateUserPanelToken}
                    >
                      <KeyRound className="size-3.5" />
                      {tokenActionPending ? t('common.loading') : t('userPanel.createKey')}
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card className="border-border bg-card shadow-sm">
              <CardHeader className="border-b border-border">
                <CardTitle className="flex items-center gap-2 text-base font-medium">
                  <Server className="size-4 text-muted-foreground" />
                  {t('userPanel.apiAccess')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 p-5">
                <div className="rounded-xl border border-border bg-muted/25 p-4">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      {t('userPanel.availableModels')}
                    </p>
                    {availableModels && availableModels.length > 0 ? (
                      <Badge variant="secondary">
                        {t('userPanel.availableModelsCount', { count: availableModels.length })}
                      </Badge>
                    ) : null}
                  </div>
                  {availableModelsLoading ||
                  (externalModelListEnabled && availableModelRoutesLoading) ? (
                    <p className="mt-3 text-sm text-muted-foreground">{t('common.loading')}</p>
                  ) : availableModelsError ||
                    (externalModelListEnabled && availableModelRoutesError && !availableModels) ? (
                    <p className="mt-3 text-sm text-destructive">
                      {t('userPanel.availableModelsLoadFailed')}
                    </p>
                  ) : externalModelListEnabled && modelRouteGroups.length > 0 ? (
                    <div className="mt-3 space-y-3">
                      {modelRouteGroups.map((group) => (
                        <div
                          key={`${group.clientType}-${group.routeID}`}
                          className="rounded-lg border border-border bg-background p-3"
                        >
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <p className="truncate text-sm font-medium text-foreground">
                              {t(`externalModels.routeTypes.${group.clientType}`)}
                            </p>
                            <Badge variant="secondary">
                              {t('userPanel.availableModelsCount', { count: group.models.length })}
                            </Badge>
                          </div>
                          <div className="mt-3 flex flex-wrap gap-2">
                            {group.visibleModels.map((model) => (
                              <Badge
                                key={model}
                                variant="outline"
                                className="max-w-full truncate font-mono"
                              >
                                {model}
                              </Badge>
                            ))}
                            {group.hiddenModelCount > 0 ? (
                              <Badge variant="secondary">
                                {t('userPanel.moreAvailableModels', {
                                  count: group.hiddenModelCount,
                                })}
                              </Badge>
                            ) : null}
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : availableModelCount > 0 ? (
                    <div className="mt-3 flex flex-wrap gap-2">
                      {(availableModels ?? []).slice(0, 12).map((model) => (
                        <Badge
                          key={model}
                          variant="outline"
                          className="max-w-full truncate font-mono"
                        >
                          {model}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <p className="mt-3 text-sm text-muted-foreground">
                      {t('userPanel.noAvailableModels')}
                    </p>
                  )}
                </div>
                <div className="rounded-xl border border-border bg-muted/25 p-4">
                  <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t('userPanel.baseURL')}
                  </p>
                  <div className="mt-3 space-y-2">
                    {endpointHints.map((endpoint) => (
                      <div
                        key={endpoint.id}
                        className="flex items-center gap-3 rounded-lg bg-background px-3 py-2 text-xs"
                      >
                        <span className="w-20 shrink-0 font-medium text-foreground">
                          {endpoint.label}
                        </span>
                        <code className="min-w-0 flex-1 break-all text-muted-foreground">
                          {endpoint.url || '—'}
                        </code>
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-8 shrink-0 gap-2"
                          aria-label={`${t('common.copy')} ${endpoint.label}`}
                          onClick={() => handleCopyEndpoint(endpoint.id, endpoint.url)}
                        >
                          <Copy className="size-3.5" />
                          {copiedEndpointId === endpoint.id ? t('common.copied') : t('common.copy')}
                        </Button>
                      </div>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="redemption" className="space-y-5">
            <Card className="border-border bg-card shadow-sm">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <Gift className="size-4 text-primary" />
                  {t('userPanel.selfRedemptionCreateTitle')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <p className="text-sm text-muted-foreground">
                  {t('userPanel.selfRedemptionCreateDesc')}
                </p>
                <div className="grid gap-3 md:grid-cols-[120px_160px_1fr_auto] md:items-end">
                  <div className="space-y-2">
                    <label className="text-sm font-medium" htmlFor="self-redemption-count">
                      {t('userPanel.selfRedemptionCount')}
                    </label>
                    <Input
                      id="self-redemption-count"
                      type="number"
                      min="1"
                      max="100"
                      value={selfRedemptionCount}
                      onChange={(event) => setSelfRedemptionCount(event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium" htmlFor="self-redemption-amount">
                      {t('userPanel.selfRedemptionAmount')}
                    </label>
                    <Input
                      id="self-redemption-amount"
                      type="number"
                      min="0.000001"
                      step="0.01"
                      value={selfRedemptionAmount}
                      onChange={(event) => setSelfRedemptionAmount(event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium" htmlFor="self-redemption-note">
                      {t('userPanel.selfRedemptionNote')}
                    </label>
                    <Input
                      id="self-redemption-note"
                      value={selfRedemptionNote}
                      onChange={(event) => setSelfRedemptionNote(event.target.value)}
                    />
                  </div>
                  <Button
                    type="button"
                    className="shrink-0"
                    onClick={handleCreateSelfRedemptionCodes}
                    disabled={
                      createUserPanelRedemptionCodes.isPending || !canCreateSelfRedemptionCodes
                    }
                  >
                    {createUserPanelRedemptionCodes.isPending
                      ? t('common.loading')
                      : t('userPanel.selfRedemptionCreateSubmit')}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t('userPanel.selfRedemptionTotal', {
                    amount: formatQuotaAmount(selfRedemptionTotalValue),
                    balance: formatQuotaBalance(userPanelToken?.quotaBalance ?? 0),
                  })}
                </p>
                {selfRedemptionMessage && (
                  <p className="text-sm text-muted-foreground">{selfRedemptionMessage}</p>
                )}
                {createdSelfRedemptionCodes.length > 0 && (
                  <div className="rounded-md border border-border bg-background p-3">
                    <div className="mb-2 text-xs font-medium text-muted-foreground">
                      {t('userPanel.selfRedemptionCreated')}
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {createdSelfRedemptionCodes.map((code) => (
                        <code key={code} className="rounded bg-muted px-2 py-1 text-xs">
                          {code}
                        </code>
                      ))}
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card className="border-border bg-card shadow-sm">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <Gift className="size-4 text-primary" />
                  {t('userPanel.redemptionTitle')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <p className="text-sm text-muted-foreground">{t('userPanel.redemptionDesc')}</p>
                <div className="flex flex-col gap-3 sm:flex-row">
                  <Input
                    value={redemptionCode}
                    onChange={(event) => setRedemptionCode(event.target.value)}
                    placeholder={t('userPanel.redemptionPlaceholder')}
                    className="font-mono"
                  />
                  <Button
                    type="button"
                    className="shrink-0"
                    onClick={handleRedeemCode}
                    disabled={redeemUserPanelCode.isPending || !redemptionCode.trim()}
                  >
                    {redeemUserPanelCode.isPending
                      ? t('common.loading')
                      : t('userPanel.redemptionSubmit')}
                  </Button>
                </div>
                {redemptionMessage && (
                  <p className="text-sm text-muted-foreground">{redemptionMessage}</p>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="consumption" className="space-y-5">
            <div className="grid gap-4 lg:grid-cols-2">
              <ConsumptionLeaderboardCard
                title={t('userPanel.todayConsumptionLeaderboard')}
                rows={consumptionLeaderboard?.today ?? []}
                isLoading={consumptionLeaderboardLoading}
                isError={consumptionLeaderboardError}
                currentUserID={user?.id}
              />
              <ConsumptionLeaderboardCard
                title={t('userPanel.allConsumptionLeaderboard')}
                rows={consumptionLeaderboard?.all ?? []}
                isLoading={consumptionLeaderboardLoading}
                isError={consumptionLeaderboardError}
                currentUserID={user?.id}
              />
            </div>
          </TabsContent>

          <TabsContent value="model-status" className="space-y-5">
            <ModelHealthCard
              rows={modelHealthRows ?? []}
              isLoading={tokenLoading || modelHealthLoading}
              isError={modelHealthError}
              hasUserPanelToken={Boolean(userPanelToken?.isEnabled)}
            />
          </TabsContent>
        </Tabs>
      </div>
      <Dialog open={announcementOpen} onOpenChange={setAnnouncementOpen}>
        <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Bell className="size-4 text-primary" />
              {t('userPanel.announcementTitle')}
            </DialogTitle>
          </DialogHeader>
          <div className="rounded-lg border border-border bg-muted/20 p-4">
            <MarkdownContent markdown={announcementMarkdown} />
          </div>
        </DialogContent>
      </Dialog>
    </main>
  );
}
