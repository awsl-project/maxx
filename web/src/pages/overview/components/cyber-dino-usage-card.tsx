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
  const flameScale = 0.62 + intensity * 0.12;
  const alive = motion === 'alive';
  const calm = motion === 'calm';
  const flameOpacity = 0.55 + intensity * 0.1;

  return (
    <div
      className="relative mx-auto h-56 w-full max-w-[22rem] overflow-hidden rounded-3xl border border-cyan-400/15 bg-[radial-gradient(circle_at_50%_18%,rgba(34,211,238,0.16),transparent_34%),linear-gradient(180deg,rgba(15,23,42,0.42),rgba(2,6,23,0.72))]"
      style={{
        boxShadow: `0 0 ${14 + intensity * 6}px ${flame.primary}22, inset 0 0 42px ${dino.secondary}1f`,
      }}
      aria-hidden="true"
    >
      <div
        className="absolute left-1/2 top-8 h-36 w-36 -translate-x-1/2 rounded-full blur-3xl"
        style={{ background: `radial-gradient(circle, ${dino.primary}24, transparent 72%)` }}
      />
      <div
        className="absolute inset-x-12 bottom-8 h-16 rounded-full blur-2xl"
        style={{
          background: `radial-gradient(circle, ${flame.primary}55, ${flame.secondary}22 46%, transparent 76%)`,
        }}
      />

      <svg viewBox="0 0 420 260" className="absolute inset-0 h-full w-full">
        <defs>
          <linearGradient
            id="dinoShell"
            x1="95"
            x2="330"
            y1="58"
            y2="196"
            gradientUnits="userSpaceOnUse"
          >
            <stop offset="0%" stopColor={dino.primary} />
            <stop offset="58%" stopColor={dino.secondary} />
            <stop offset="100%" stopColor={dino.accent} />
          </linearGradient>
          <linearGradient
            id="dinoBelly"
            x1="125"
            x2="270"
            y1="118"
            y2="195"
            gradientUnits="userSpaceOnUse"
          >
            <stop offset="0%" stopColor="#ffffff" stopOpacity="0.28" />
            <stop offset="100%" stopColor="#020617" stopOpacity="0.16" />
          </linearGradient>
          <linearGradient id="flameBody" x1="0" x2="0" y1="1" y2="0">
            <stop offset="0%" stopColor={flame.secondary} />
            <stop offset="62%" stopColor={flame.primary} />
            <stop offset="100%" stopColor={flame.accent} />
          </linearGradient>
          <filter id="mascotGlow" x="-35%" y="-35%" width="170%" height="170%">
            <feGaussianBlur stdDeviation="3.5" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>

        <g opacity="0.18">
          <path d="M68 198 H352" stroke={dino.primary} strokeWidth="1" strokeDasharray="9 10" />
          <path
            d="M98 72 H322 M86 112 H338"
            stroke={dino.accent}
            strokeWidth="1"
            strokeDasharray="3 13"
          />
          <path
            d="M118 42 V224 M210 32 V232 M302 48 V220"
            stroke={dino.primary}
            strokeWidth="1"
            strokeDasharray="4 12"
          />
        </g>

        <ellipse cx="214" cy="218" rx="132" ry="16" fill="#020617" opacity="0.42" />

        <g
          filter="url(#mascotGlow)"
          opacity={flameOpacity}
          transform={`translate(0 ${calm ? 3 : 0}) scale(1 ${flameScale}) translate(0 ${134 - 134 / flameScale})`}
          style={{ transformOrigin: '210px 218px' }}
          className={alive ? 'motion-safe:animate-pulse' : undefined}
        >
          <path
            d="M155 220 C139 182 170 176 166 143 C198 166 187 195 207 220 Z"
            fill="url(#flameBody)"
          />
          <path
            d="M202 222 C179 174 222 155 213 111 C268 158 236 190 260 222 Z"
            fill="url(#flameBody)"
          />
          <path
            d="M260 220 C243 184 282 170 272 137 C313 166 296 194 318 220 Z"
            fill="url(#flameBody)"
          />
          <path
            d="M211 222 C199 192 225 181 222 151 C249 181 236 202 248 222 Z"
            fill={flame.accent}
            opacity="0.86"
          />
        </g>

        <g filter="url(#mascotGlow)">
          <path
            d="M113 139 C84 133 61 117 49 95 C82 98 112 111 135 132 Z"
            fill={dino.secondary}
            stroke={dino.primary}
            strokeWidth="3"
            strokeLinejoin="round"
          />
          <path
            d="M108 134 C116 87 157 60 213 63 C270 66 318 96 331 143 C345 194 304 216 222 215 C142 214 98 190 108 134 Z"
            fill="url(#dinoShell)"
            stroke={dino.primary}
            strokeWidth="4"
            strokeLinejoin="round"
          />
          <path
            d="M142 147 C160 122 209 114 257 127 C288 136 305 158 298 179 C289 207 244 213 191 204 C148 197 123 174 142 147 Z"
            fill="url(#dinoBelly)"
            opacity="0.9"
          />
          <path
            d="M270 73 C291 52 330 55 352 80 C371 101 369 132 348 149 C322 170 280 157 265 127 C255 107 255 88 270 73 Z"
            fill="url(#dinoShell)"
            stroke={dino.primary}
            strokeWidth="4"
            strokeLinejoin="round"
          />
          <path
            d="M340 105 C362 100 384 106 397 121 C382 134 359 139 340 132 Z"
            fill={dino.secondary}
            stroke={dino.primary}
            strokeWidth="3"
            strokeLinejoin="round"
          />
          <path
            d="M291 92 H340 C347 92 353 98 353 105 V110 C353 117 347 123 340 123 H291 C284 123 278 117 278 110 V105 C278 98 284 92 291 92 Z"
            fill="#020617"
            opacity="0.78"
          />
          <path
            d="M289 108 H343"
            stroke={flame.accent}
            strokeWidth="3"
            strokeLinecap="round"
            opacity="0.94"
          />
          <circle cx="333" cy="105" r="4" fill={flame.primary} />

          <path
            d="M154 72 L169 38 L191 76 Z M207 63 L224 30 L248 68 Z M281 66 L300 35 L322 76 Z"
            fill={dino.accent}
            stroke={dino.primary}
            strokeWidth="2"
            strokeLinejoin="round"
          />
          <path
            d="M153 200 L187 200 L183 234 H158 Z M245 201 L281 199 L276 234 H250 Z"
            fill={dino.secondary}
            stroke={dino.primary}
            strokeWidth="3"
            strokeLinejoin="round"
          />
          <path
            d="M145 232 H194 C198 232 201 235 201 239 V242 H139 V239 C139 235 142 232 145 232 Z M239 232 H289 C293 232 296 235 296 239 V242 H233 V239 C233 235 236 232 239 232 Z"
            fill="#020617"
            stroke={dino.primary}
            strokeWidth="3"
          />
          <path
            d="M131 146 C159 156 181 156 207 145 M161 184 C195 194 245 193 276 178 M206 94 C223 88 242 89 260 98"
            stroke="#020617"
            strokeWidth="6"
            strokeLinecap="round"
            opacity="0.18"
          />
          <path
            d="M131 146 C159 156 181 156 207 145 M161 184 C195 194 245 193 276 178 M206 94 C223 88 242 89 260 98"
            stroke={dino.primary}
            strokeWidth="1.6"
            strokeLinecap="round"
            opacity="0.7"
          />
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
      title={t('dashboard.cyberDino.badgeTooltip', { tokens: formatNumber(totals.totalTokens) })}
    >
      <span className="relative inline-flex h-5 w-8 items-center justify-center" aria-hidden="true">
        <span
          className="absolute bottom-0 h-3 w-3 rounded-full blur-[3px]"
          style={{ background: flame.primary, opacity: 0.45 + level.level * 0.1 }}
        />
        <span
          className="absolute bottom-0 h-4 w-2 rounded-t-full"
          style={{
            background: `linear-gradient(${flame.accent}, ${flame.primary}, ${flame.secondary})`,
          }}
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
                        tokens: formatNumber(
                          Math.max(0, burnLevel.nextThreshold - totals.totalTokens),
                        ),
                      })
                    : t('dashboard.cyberDino.maxLevelHint')}
                </div>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-4">
              <Metric
                label={t('dashboard.cyberDino.inputTokens')}
                value={formatNumber(totals.inputTokens)}
              />
              <Metric
                label={t('dashboard.cyberDino.outputTokens')}
                value={formatNumber(totals.outputTokens)}
              />
              <Metric
                label={t('dashboard.cyberDino.cacheTokens')}
                value={formatNumber(totals.cacheTokens)}
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
