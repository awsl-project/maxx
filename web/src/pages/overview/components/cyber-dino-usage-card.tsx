import { useEffect, useMemo, useState } from 'react';
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

function formatNumber(num: number): string {
  if (num >= 1_000_000_000) return `${(num / 1_000_000_000).toFixed(1)}B`;
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`;
  return num.toLocaleString();
}

function formatResetDate(date: Date): string {
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
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

function useCyberDinoUsagePreferences() {
  const [preferences, setPreferences] = useState<CyberDinoUsagePreferences>(loadPreferences);

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(preferences));
    } catch {
      // Cosmetic preferences should never break the dashboard.
    }
  }, [preferences]);

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
  const flameScale = 0.45 + intensity * 0.16;
  const alive = motion === 'alive';
  const calm = motion === 'calm';

  return (
    <div
      className={cn(
        'relative mx-auto h-56 w-full max-w-[22rem] overflow-hidden rounded-3xl border border-border/50 bg-gradient-to-b from-background/20 to-background/70',
        alive && 'animate-pulse',
      )}
      style={{
        boxShadow: `0 0 ${18 + intensity * 8}px ${flame.primary}30, inset 0 0 42px ${dino.secondary}24`,
      }}
      aria-hidden="true"
    >
      <div
        className="absolute inset-x-8 bottom-4 h-14 rounded-full blur-2xl"
        style={{ background: `radial-gradient(circle, ${flame.primary}66, transparent 70%)` }}
      />
      <svg viewBox="0 0 420 250" className="absolute inset-0 h-full w-full">
        <defs>
          <linearGradient id="dinoBody" x1="0" x2="1" y1="0" y2="1">
            <stop offset="0%" stopColor={dino.primary} />
            <stop offset="55%" stopColor={dino.secondary} />
            <stop offset="100%" stopColor={dino.accent} />
          </linearGradient>
          <linearGradient id="flameBody" x1="0" x2="0" y1="1" y2="0">
            <stop offset="0%" stopColor={flame.secondary} />
            <stop offset="58%" stopColor={flame.primary} />
            <stop offset="100%" stopColor={flame.accent} />
          </linearGradient>
          <filter id="softGlow" x="-35%" y="-35%" width="170%" height="170%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          <pattern id="circuitPattern" width="24" height="24" patternUnits="userSpaceOnUse">
            <path d="M 0 12 H 24 M 12 0 V 24" stroke={dino.accent} strokeOpacity="0.28" strokeWidth="1" />
          </pattern>
        </defs>

        <g opacity="0.28">
          <path d="M30 205 H390" stroke={dino.primary} strokeWidth="1" strokeDasharray="8 8" />
          <path d="M60 170 H360" stroke={dino.secondary} strokeWidth="1" strokeDasharray="5 10" />
          <path d="M105 38 V220 M205 28 V225 M305 42 V220" stroke={dino.primary} strokeWidth="1" strokeDasharray="4 12" />
        </g>

        <g
          filter="url(#softGlow)"
          transform={`translate(0 ${calm ? 2 : 0}) scale(1 ${flameScale}) translate(0 ${138 - 138 / flameScale})`}
        >
          <path d="M154 212 C140 175 172 171 165 134 C202 164 185 184 202 212 Z" fill="url(#flameBody)" opacity="0.78" />
          <path d="M205 218 C184 169 222 155 214 108 C266 155 236 181 262 218 Z" fill="url(#flameBody)" opacity="0.92" />
          <path d="M265 215 C247 178 282 166 270 130 C315 163 293 186 316 215 Z" fill="url(#flameBody)" opacity="0.82" />
          <path d="M214 218 C202 188 224 178 221 146 C247 178 237 196 247 218 Z" fill={flame.accent} opacity="0.8" />
        </g>

        <g transform="translate(22 10)">
          <path d="M135 124 L50 100 L118 155 Z" fill={dino.secondary} stroke={dino.primary} strokeWidth="3" />
          <path d="M118 92 L235 72 L324 125 L304 184 L154 190 L92 143 Z" fill="url(#dinoBody)" stroke={dino.primary} strokeWidth="3" />
          <path d="M118 92 L235 72 L204 134 L92 143 Z" fill={dino.secondary} opacity="0.76" />
          <path d="M204 134 L324 125 L304 184 L154 190 L92 143 Z" fill="url(#circuitPattern)" opacity="0.7" />
          <path d="M292 46 L367 64 L392 103 L359 136 L295 126 L260 86 Z" fill="url(#dinoBody)" stroke={dino.primary} strokeWidth="3" />
          <path d="M360 92 L410 102 L384 126 L350 122 Z" fill={dino.secondary} stroke={dino.primary} strokeWidth="3" />
          <circle cx="337" cy="78" r="7" fill="#020617" />
          <circle cx="339" cy="76" r="2.5" fill={flame.accent} />
          <path d="M142 90 L160 48 L186 94 Z M202 76 L222 32 L250 80 Z M294 47 L314 12 L343 57 Z" fill={dino.accent} stroke={dino.primary} strokeWidth="2" />
          <path d="M151 183 L193 181 L185 232 L157 232 Z M250 181 L292 178 L284 232 L256 232 Z" fill={dino.secondary} stroke={dino.primary} strokeWidth="3" />
          <path d="M148 230 H210 L190 246 H132 Z M248 230 H310 L291 246 H234 Z" fill="#0f172a" stroke={dino.primary} strokeWidth="3" />
          <path d="M305 144 L360 168 L339 190 L291 158 Z" fill={dino.secondary} stroke={dino.primary} strokeWidth="3" />
          <path d="M142 123 L205 112 M211 154 L286 151 M302 68 L350 82" stroke="#031c24" strokeWidth="6" strokeLinecap="round" opacity="0.45" />
          <path d="M142 123 L205 112 M211 154 L286 151 M302 68 L350 82" stroke={dino.primary} strokeWidth="1.5" strokeLinecap="round" opacity="0.85" />
        </g>
      </svg>
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
      <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{title}</div>
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
                  style={{ background: `linear-gradient(135deg, ${option.primary}, ${option.secondary})` }}
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
      title={t('dashboard.cyberDino.badgeTooltip', { tokens: formatNumber(totals.totalTokens) })}
    >
      <span className="relative inline-flex h-5 w-8 items-center justify-center" aria-hidden="true">
        <span
          className="absolute bottom-0 h-3 w-3 rounded-full blur-[3px]"
          style={{ background: flame.primary, opacity: 0.45 + level.level * 0.1 }}
        />
        <span
          className="absolute bottom-0 h-4 w-2 rounded-t-full"
          style={{ background: `linear-gradient(${flame.accent}, ${flame.primary}, ${flame.secondary})` }}
        />
        <span
          className="absolute top-0 h-3 w-6 rounded-[55%_45%_45%_55%] border border-white/30"
          style={{ background: `linear-gradient(135deg, ${dino.primary}, ${dino.secondary})` }}
        />
      </span>
      <span className="font-mono">{formatNumber(totals.totalTokens)}</span>
    </div>
  );
}

export function CyberDinoUsageCard() {
  const { t } = useTranslation();
  const [preferences, setPreferences] = useCyberDinoUsagePreferences();
  const { totals, isLoading } = useMonthlyUsageTotals();
  const burnLevel = getBurnLevel(totals.totalTokens);
  const burnProgress = getBurnProgress(totals.totalTokens, burnLevel);
  const dino = materialByID(DINO_MATERIALS, preferences.dinoMaterial);
  const flame = materialByID(FLAME_MATERIALS, preferences.flameMaterial);
  const resetDate = formatResetDate(nextMonthStart());

  return (
    <Card className="overflow-hidden border-cyan-500/20 bg-card/50 backdrop-blur-sm shadow-[0_0_35px_rgba(34,211,238,0.08)]">
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between gap-4">
          <div>
            <CardTitle className="flex items-center gap-2 text-base font-semibold">
              <Sparkles className="h-4 w-4 text-cyan-400" />
              {t('dashboard.cyberDino.title')}
            </CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">{t('dashboard.cyberDino.description')}</p>
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
                  <span className="pb-1 text-sm font-medium text-muted-foreground">tokens</span>
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
                        tokens: formatNumber(Math.max(0, burnLevel.nextThreshold - totals.totalTokens)),
                      })
                    : t('dashboard.cyberDino.maxLevelHint')}
                </div>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-4">
              <Metric label={t('dashboard.cyberDino.inputTokens')} value={formatNumber(totals.inputTokens)} />
              <Metric label={t('dashboard.cyberDino.outputTokens')} value={formatNumber(totals.outputTokens)} />
              <Metric label={t('dashboard.cyberDino.cacheTokens')} value={formatNumber(totals.cacheTokens)} />
              <Metric label={t('dashboard.cyberDino.requests')} value={formatNumber(totals.requests)} />
            </div>

            <div className="grid gap-4 md:grid-cols-2">
              <MaterialSelector
                title={t('dashboard.cyberDino.dinoMaterial')}
                options={DINO_MATERIALS}
                value={preferences.dinoMaterial}
                onChange={(dinoMaterial) => setPreferences((current) => ({ ...current, dinoMaterial }))}
              />
              <MaterialSelector
                title={t('dashboard.cyberDino.flameMaterial')}
                options={FLAME_MATERIALS}
                value={preferences.flameMaterial}
                onChange={(flameMaterial) => setPreferences((current) => ({ ...current, flameMaterial }))}
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
            <CyberDinoSvg dino={dino} flame={flame} intensity={burnLevel.level} motion={preferences.motion} />
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
