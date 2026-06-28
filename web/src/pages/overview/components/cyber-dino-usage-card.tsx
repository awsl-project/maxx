import { type SetStateAction, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Flame, Gauge, Sparkles } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui';
import { useUsageStats } from '@/hooks/queries';
import { cn } from '@/lib/utils';
import {
  aggregateMonthlyUsage,
  DEFAULT_CYBER_DINO_USAGE_PREFERENCES,
  DINO_MATERIALS,
  FLAME_MATERIALS,
  getBurnLevel,
  getBurnProgress,
  getCurrentMonthUsageWindow,
  materialByID,
  normalizeCyberDinoUsagePreferences,
  type CyberDinoUsagePreferences,
  type MaterialOption,
} from './cyber-dino-usage';

const STORAGE_KEY = 'maxx-cyber-dino-usage-preferences';
const PREFERENCES_EVENT = 'maxx-cyber-dino-usage-preferences-change';

function formatNumber(num: number): string {
  if (num >= 1_000_000_000) return `${(num / 1_000_000_000).toFixed(1)}B`;
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`;
  return num.toLocaleString();
}

function formatResetDate(date: Date, locale: string): string {
  return date.toLocaleDateString(locale, { month: 'short', day: 'numeric' });
}

function nextMonthStart(now = new Date()): Date {
  return new Date(now.getFullYear(), now.getMonth() + 1, 1);
}

function loadPreferences(): CyberDinoUsagePreferences {
  if (typeof window === 'undefined') return DEFAULT_CYBER_DINO_USAGE_PREFERENCES;

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    return normalizeCyberDinoUsagePreferences(raw ? JSON.parse(raw) : null);
  } catch {
    return DEFAULT_CYBER_DINO_USAGE_PREFERENCES;
  }
}

function savePreferences(preferences: CyberDinoUsagePreferences) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(preferences));
  } catch {
    // Cosmetic preferences should never break the dashboard.
  }
}

function useCyberDinoUsagePreferences() {
  const [preferences, setPreferencesState] = useState<CyberDinoUsagePreferences>(loadPreferences);

  useEffect(() => {
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== STORAGE_KEY) return;
      try {
        setPreferencesState(
          normalizeCyberDinoUsagePreferences(event.newValue ? JSON.parse(event.newValue) : null),
        );
      } catch {
        setPreferencesState(DEFAULT_CYBER_DINO_USAGE_PREFERENCES);
      }
    };
    const handleLocalChange = (event: Event) => {
      setPreferencesState(
        normalizeCyberDinoUsagePreferences(
          (event as CustomEvent<CyberDinoUsagePreferences>).detail,
        ),
      );
    };

    window.addEventListener('storage', handleStorage);
    window.addEventListener(PREFERENCES_EVENT, handleLocalChange);
    return () => {
      window.removeEventListener('storage', handleStorage);
      window.removeEventListener(PREFERENCES_EVENT, handleLocalChange);
    };
  }, []);

  const setPreferences = (next: SetStateAction<CyberDinoUsagePreferences>) => {
    setPreferencesState((current) => {
      const resolved = typeof next === 'function' ? next(current) : next;
      const normalized = normalizeCyberDinoUsagePreferences(resolved);
      savePreferences(normalized);
      window.dispatchEvent(new CustomEvent(PREFERENCES_EVENT, { detail: normalized }));
      return normalized;
    });
  };

  return [preferences, setPreferences] as const;
}

function useMonthlyUsageTotals() {
  const windowRange = useMemo(() => getCurrentMonthUsageWindow(), []);
  const { data: stats, isLoading } = useUsageStats({
    granularity: 'day',
    start: windowRange.start.toISOString(),
    end: windowRange.end.toISOString(),
  });

  const totals = useMemo(() => aggregateMonthlyUsage(stats), [stats]);

  return { totals, isLoading, windowRange };
}

function CyberDinoSvg({
  dino,
  flame,
  intensity,
  motion,
}: {
  dino: MaterialOption<string>;
  flame: MaterialOption<string>;
  intensity: number;
  motion: CyberDinoUsagePreferences['motion'];
}) {
  const flameScale = 0.74 + intensity * 0.1;
  const alive = motion === 'alive';
  const calm = motion === 'calm';
  const burnOpacity = 0.5 + intensity * 0.1;
  const scanlineOpacity = 0.12 + intensity * 0.035;

  return (
    <div
      className="relative mx-auto h-56 w-full max-w-[22rem] overflow-hidden rounded-[1.75rem] border border-white/10 bg-[linear-gradient(180deg,rgba(13,11,18,0.96),rgba(2,6,23,0.94))] shadow-2xl"
      style={{
        boxShadow: `0 18px 60px ${flame.secondary}22, inset 0 1px 0 rgba(255,255,255,0.08), inset 0 0 54px ${dino.secondary}1c`,
      }}
      aria-hidden="true"
    >
      <div
        className="absolute -left-12 top-4 h-40 w-40 rounded-full blur-3xl"
        style={{ background: `${dino.primary}22` }}
      />
      <div
        className="absolute -right-14 bottom-0 h-44 w-44 rounded-full blur-3xl"
        style={{ background: `${flame.primary}26` }}
      />
      <div className="absolute inset-x-8 top-6 z-10 flex items-center justify-between text-[10px] font-semibold uppercase tracking-[0.28em] text-white/40">
        <span>Routing Gremlin</span>
        <span>Lv.{intensity}</span>
      </div>

      <div className="absolute inset-8 top-12 rounded-[1.25rem] border border-white/10 bg-black/25" />
      <div
        className="absolute inset-x-10 bottom-8 h-10 origin-bottom rounded-[50%_50%_18%_18%] blur-md transition-transform"
        style={{
          background: `linear-gradient(90deg, ${flame.secondary}, ${flame.primary}, ${flame.accent})`,
          opacity: burnOpacity,
          transform: `scale(${1 + intensity * 0.08}, ${flameScale})`,
        }}
      />
      <div
        className="absolute bottom-11 left-1/2 h-24 w-28 -translate-x-1/2 origin-bottom rounded-[45%_55%_18%_18%] opacity-80 blur-sm"
        style={{
          background: `radial-gradient(circle at 50% 18%, ${flame.accent}, ${flame.primary} 48%, ${flame.secondary} 82%)`,
          transform: `translateX(-50%) scaleY(${flameScale})`,
        }}
      />

      <div
        className={cn(
          'absolute left-1/2 top-[4.1rem] z-20 flex h-32 w-32 -translate-x-1/2 items-center justify-center rounded-[1.25rem] border-2 bg-black/55 p-2 shadow-[10px_10px_0_rgba(34,211,238,0.65)]',
          alive && 'motion-safe:animate-pulse',
          calm && 'translate-y-1',
        )}
        style={{ borderColor: dino.primary }}
      >
        <img
          src="/logo.png"
          alt=""
          className="h-full w-full object-contain [image-rendering:pixelated]"
          draggable={false}
        />
      </div>

      <div className="absolute inset-x-10 bottom-5 z-30 flex items-center justify-between font-mono text-[10px] uppercase tracking-[0.22em] text-white/55">
        <span>logo-linked</span>
        <span>token fire</span>
      </div>
      <div
        className="pointer-events-none absolute inset-0 bg-[repeating-linear-gradient(0deg,rgba(255,255,255,0.08)_0,rgba(255,255,255,0.08)_1px,transparent_1px,transparent_5px)]"
        style={{ opacity: scanlineOpacity }}
      />
      <div className="absolute inset-x-0 bottom-0 flex h-4 overflow-hidden">
        {Array.from({ length: 12 }).map((_, index) => (
          <span
            key={index}
            className="h-8 w-10 -skew-x-12"
            style={{ background: index % 2 === 0 ? flame.accent : '#020617' }}
          />
        ))}
      </div>
    </div>
  );
}

function MaterialSelector<ID extends string>({
  title,
  options,
  value,
  onChange,
}: {
  title: string;
  options: MaterialOption<ID>[];
  value: ID;
  onChange: (value: ID) => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-2">
      <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </div>
      <div className="grid grid-cols-2 gap-2">
        {options.map((option) => {
          const selected = option.id === value;
          return (
            <button
              key={option.id}
              type="button"
              onClick={() => onChange(option.id)}
              className={cn(
                'rounded-xl border px-3 py-2 text-left transition-colors',
                selected
                  ? 'border-cyan-400/70 bg-cyan-500/10 text-foreground shadow-[0_0_16px_rgba(34,211,238,0.18)]'
                  : 'border-border/60 bg-background/30 text-muted-foreground hover:border-border hover:text-foreground',
              )}
              title={t(option.descriptionKey)}
              aria-pressed={selected}
            >
              <span className="flex items-center gap-2 text-xs font-medium">
                <span
                  className="h-3 w-3 rounded-full border border-white/30"
                  style={{
                    background: `linear-gradient(135deg, ${option.primary}, ${option.secondary})`,
                  }}
                />
                {t(option.labelKey)}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

export function CyberDinoUsageBadge() {
  const { t } = useTranslation();
  const [preferences] = useCyberDinoUsagePreferences();
  const { totals } = useMonthlyUsageTotals();
  const level = getBurnLevel(totals.totalTokens);
  const dino = materialByID(DINO_MATERIALS, preferences.dinoMaterial);
  const flame = materialByID(FLAME_MATERIALS, preferences.flameMaterial);

  return (
    <div
      className="hidden items-center gap-2 rounded-full border border-cyan-400/30 bg-secondary/40 px-3 py-1.5 text-xs font-medium text-foreground shadow-[0_0_18px_rgba(34,211,238,0.12)] sm:inline-flex"
      title={t('dashboard.cyberDino.badgeTooltip', {
        tokens: formatNumber(totals.totalTokens),
        unit: t('dashboard.cyberDino.tokenUnit'),
      })}
    >
      <span className="relative inline-flex h-6 w-6 items-center justify-center" aria-hidden="true">
        <span
          className="absolute inset-0 rounded-md blur-[3px]"
          style={{ background: flame.primary, opacity: 0.35 + level.level * 0.08 }}
        />
        <span
          className="absolute inset-0 rounded-md border"
          style={{ borderColor: dino.primary, background: '#020617' }}
        />
        <img
          src="/logo.png"
          alt=""
          className="relative h-5 w-5 object-contain [image-rendering:pixelated]"
          draggable={false}
        />
      </span>
      <span className="font-mono">{formatNumber(totals.totalTokens)}</span>
    </div>
  );
}

export function CyberDinoUsageCard() {
  const { t, i18n } = useTranslation();
  const [preferences, setPreferences] = useCyberDinoUsagePreferences();
  const { totals, isLoading } = useMonthlyUsageTotals();
  const burnLevel = getBurnLevel(totals.totalTokens);
  const burnProgress = getBurnProgress(totals.totalTokens, burnLevel);
  const dino = materialByID(DINO_MATERIALS, preferences.dinoMaterial);
  const flame = materialByID(FLAME_MATERIALS, preferences.flameMaterial);
  const resetDate = formatResetDate(nextMonthStart(), i18n.language);

  return (
    <Card className="overflow-hidden border-cyan-500/20 bg-card/50 backdrop-blur-sm shadow-[0_0_35px_rgba(34,211,238,0.08)]">
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between gap-4">
          <div>
            <CardTitle className="flex items-center gap-2 text-base font-semibold">
              <Sparkles className="h-4 w-4 text-cyan-400" />
              {t('dashboard.cyberDino.title')}
            </CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              {t('dashboard.cyberDino.description')}
            </p>
          </div>
          <div className="rounded-full border border-orange-400/30 bg-orange-500/10 px-3 py-1 text-xs font-medium text-orange-300">
            {t(`dashboard.cyberDino.levels.${burnLevel.tone}`)} · {burnLevel.level}/4
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
          <div className="space-y-5">
            <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_11rem]">
              <div>
                <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  {t('dashboard.cyberDino.monthlyBurn')}
                </div>
                <div className="mt-2 flex flex-wrap items-end gap-x-3 gap-y-1">
                  <span className="font-mono text-4xl font-bold tracking-tight text-cyan-300 md:text-5xl">
                    {isLoading ? '—' : formatNumber(totals.totalTokens)}
                  </span>
                  <span className="pb-1 text-sm font-medium text-muted-foreground">
                    {t('dashboard.cyberDino.tokenUnit')}
                  </span>
                </div>
              </div>
              <div className="rounded-2xl border border-border/60 bg-background/35 p-3">
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Gauge className="h-3.5 w-3.5" />
                  {t('dashboard.cyberDino.nextLevel')}
                </div>
                <div className="mt-3 h-2 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full rounded-full transition-all"
                    style={{
                      width: `${burnProgress}%`,
                      background: `linear-gradient(90deg, ${flame.primary}, ${flame.secondary})`,
                    }}
                  />
                </div>
                <div className="mt-2 text-xs text-muted-foreground">
                  {burnLevel.nextThreshold
                    ? t('dashboard.cyberDino.nextLevelHint', {
                        tokens: formatNumber(
                          Math.max(0, burnLevel.nextThreshold - totals.totalTokens),
                        ),
                        unit: t('dashboard.cyberDino.tokenUnit'),
                      })
                    : t('dashboard.cyberDino.maxLevelHint')}
                </div>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
              <Metric
                label={t('dashboard.cyberDino.inputTokens')}
                value={formatNumber(totals.inputTokens)}
              />
              <Metric
                label={t('dashboard.cyberDino.outputTokens')}
                value={formatNumber(totals.outputTokens)}
              />
              <Metric
                label={t('dashboard.cyberDino.cacheReadTokens')}
                value={formatNumber(totals.cacheReadTokens)}
              />
              <Metric
                label={t('dashboard.cyberDino.cacheWriteTokens')}
                value={formatNumber(totals.cacheWriteTokens)}
              />
              <Metric
                label={t('dashboard.cyberDino.requests')}
                value={formatNumber(totals.requests)}
              />
            </div>

            <div className="grid gap-4 md:grid-cols-2">
              <MaterialSelector
                title={t('dashboard.cyberDino.dinoMaterial')}
                options={DINO_MATERIALS}
                value={preferences.dinoMaterial}
                onChange={(dinoMaterial) =>
                  setPreferences((current) => ({ ...current, dinoMaterial }))
                }
              />
              <MaterialSelector
                title={t('dashboard.cyberDino.flameMaterial')}
                options={FLAME_MATERIALS}
                value={preferences.flameMaterial}
                onChange={(flameMaterial) =>
                  setPreferences((current) => ({ ...current, flameMaterial }))
                }
              />
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border/50 bg-background/30 px-3 py-2 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-2">
                <Flame className="h-3.5 w-3.5 text-orange-400" />
                {t('dashboard.cyberDino.mappingHint')}
              </span>
              <span>{t('dashboard.cyberDino.resetHint', { date: resetDate })}</span>
            </div>
          </div>

          <div className="space-y-3">
            <CyberDinoSvg
              dino={dino}
              flame={flame}
              intensity={burnLevel.level}
              motion={preferences.motion}
            />
            <div className="grid grid-cols-3 gap-2">
              {(['alive', 'calm', 'off'] as const).map((motion) => (
                <button
                  key={motion}
                  type="button"
                  onClick={() => setPreferences((current) => ({ ...current, motion }))}
                  className={cn(
                    'rounded-xl border px-3 py-2 text-xs font-medium transition-colors',
                    preferences.motion === motion
                      ? 'border-cyan-400/70 bg-cyan-500/10 text-foreground'
                      : 'border-border/60 bg-background/30 text-muted-foreground hover:text-foreground',
                  )}
                  aria-pressed={preferences.motion === motion}
                >
                  {t(`dashboard.cyberDino.motion.${motion}`)}
                </button>
              ))}
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-border/50 bg-background/30 p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 font-mono text-lg font-semibold text-foreground">{value}</div>
    </div>
  );
}
