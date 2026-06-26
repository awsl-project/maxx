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
  const burnOpacity = 0.52 + intensity * 0.1;
  const scanlineOpacity = 0.1 + intensity * 0.035;

  return (
    <div
      className="relative mx-auto h-56 w-full max-w-[22rem] overflow-hidden rounded-[1.75rem] border border-white/10 bg-[linear-gradient(180deg,rgba(15,23,42,0.86),rgba(2,6,23,0.92))] shadow-2xl"
      style={{
        boxShadow: `0 18px 60px ${flame.secondary}20, inset 0 1px 0 rgba(255,255,255,0.08), inset 0 0 54px ${dino.secondary}1c`,
      }}
      aria-hidden="true"
    >
      <div
        className="absolute -left-12 top-4 h-40 w-40 rounded-full blur-3xl"
        style={{ background: `${dino.primary}20` }}
      />
      <div
        className="absolute -right-14 bottom-0 h-44 w-44 rounded-full blur-3xl"
        style={{ background: `${flame.primary}22` }}
      />
      <div className="absolute inset-x-8 top-6 flex items-center justify-between text-[10px] font-semibold uppercase tracking-[0.28em] text-white/35">
        <span>Token Burn</span>
        <span>Lv.{intensity}</span>
      </div>

      <svg viewBox="0 0 420 260" className="absolute inset-0 h-full w-full">
        <defs>
          <linearGradient
            id="emblemFrame"
            x1="90"
            x2="330"
            y1="42"
            y2="222"
            gradientUnits="userSpaceOnUse"
          >
            <stop offset="0%" stopColor="#ffffff" stopOpacity="0.24" />
            <stop offset="52%" stopColor={dino.primary} stopOpacity="0.16" />
            <stop offset="100%" stopColor={flame.secondary} stopOpacity="0.18" />
          </linearGradient>
          <linearGradient
            id="mascotFill"
            x1="120"
            x2="306"
            y1="76"
            y2="185"
            gradientUnits="userSpaceOnUse"
          >
            <stop offset="0%" stopColor={dino.primary} />
            <stop offset="62%" stopColor={dino.secondary} />
            <stop offset="100%" stopColor={dino.accent} />
          </linearGradient>
          <linearGradient
            id="mascotPlate"
            x1="160"
            x2="276"
            y1="112"
            y2="174"
            gradientUnits="userSpaceOnUse"
          >
            <stop offset="0%" stopColor="#f8fafc" stopOpacity="0.3" />
            <stop offset="100%" stopColor="#020617" stopOpacity="0.1" />
          </linearGradient>
          <linearGradient id="flameGlyph" x1="0" x2="0" y1="1" y2="0">
            <stop offset="0%" stopColor={flame.secondary} />
            <stop offset="58%" stopColor={flame.primary} />
            <stop offset="100%" stopColor={flame.accent} />
          </linearGradient>
          <filter id="premiumGlow" x="-30%" y="-30%" width="160%" height="160%">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          <pattern id="microGrid" width="18" height="18" patternUnits="userSpaceOnUse">
            <path
              d="M18 0H0V18"
              fill="none"
              stroke="#94a3b8"
              strokeOpacity="0.13"
              strokeWidth="1"
            />
          </pattern>
        </defs>

        <rect x="34" y="36" width="352" height="188" rx="34" fill="url(#microGrid)" opacity="0.8" />
        <path
          d="M126 50 H294 C330 50 358 78 358 114 V146 C358 192 322 222 266 222 H154 C98 222 62 192 62 146 V114 C62 78 90 50 126 50 Z"
          fill="url(#emblemFrame)"
          stroke="#ffffff"
          strokeOpacity="0.13"
          strokeWidth="1.5"
        />
        <path
          d="M90 195 H330"
          stroke={flame.primary}
          strokeOpacity="0.28"
          strokeWidth="1"
          strokeDasharray="8 10"
        />
        <path
          d="M96 82 H150 M270 82 H324"
          stroke={dino.primary}
          strokeOpacity="0.46"
          strokeWidth="2"
          strokeLinecap="round"
        />
        <path
          d="M104 98 H132 M288 98 H316"
          stroke={dino.accent}
          strokeOpacity="0.34"
          strokeWidth="2"
          strokeLinecap="round"
        />

        <g
          opacity={burnOpacity}
          filter="url(#premiumGlow)"
          transform={`translate(0 ${calm ? 2 : 0}) scale(1 ${flameScale}) translate(0 ${166 - 166 / flameScale})`}
          style={{ transformOrigin: '210px 213px' }}
          className={alive ? 'motion-safe:animate-pulse' : undefined}
        >
          <ellipse cx="210" cy="215" rx="72" ry="13" fill={flame.secondary} opacity="0.34" />
          <path
            d="M171 216 C158 186 182 178 178 151 C204 172 194 195 211 216 Z"
            fill="url(#flameGlyph)"
          />
          <path
            d="M207 218 C189 181 219 165 214 130 C256 166 231 195 252 218 Z"
            fill="url(#flameGlyph)"
          />
          <path
            d="M248 216 C236 188 261 180 254 154 C286 176 273 197 289 216 Z"
            fill="url(#flameGlyph)"
          />
          <path
            d="M211 217 C202 196 220 188 218 166 C239 190 229 203 238 217 Z"
            fill={flame.accent}
            opacity="0.82"
          />
        </g>

        <g filter="url(#premiumGlow)">
          <path
            d="M126 135 C103 131 82 120 69 101 C100 100 129 110 150 129 Z"
            fill={dino.secondary}
            stroke={dino.primary}
            strokeOpacity="0.9"
            strokeWidth="3"
            strokeLinejoin="round"
          />
          <path
            d="M126 130 C134 93 169 73 216 75 C264 77 302 102 314 138 C327 178 294 198 226 198 C161 198 118 174 126 130 Z"
            fill="url(#mascotFill)"
            stroke={dino.primary}
            strokeWidth="3.5"
            strokeLinejoin="round"
          />
          <path
            d="M160 142 C177 123 218 118 256 129 C282 136 295 153 289 169 C282 190 245 196 199 188 C164 183 145 160 160 142 Z"
            fill="url(#mascotPlate)"
          />
          <path
            d="M272 85 C291 69 324 73 341 94 C356 113 352 139 333 151 C309 166 274 151 264 124 C258 108 260 95 272 85 Z"
            fill="url(#mascotFill)"
            stroke={dino.primary}
            strokeWidth="3.5"
            strokeLinejoin="round"
          />
          <path
            d="M326 111 C347 107 367 113 379 126 C365 138 344 142 326 136 Z"
            fill={dino.secondary}
            stroke={dino.primary}
            strokeOpacity="0.9"
            strokeWidth="3"
            strokeLinejoin="round"
          />
          <path
            d="M286 100 H329 C337 100 343 106 343 114 C343 121 337 127 329 127 H286 C278 127 272 121 272 114 C272 106 278 100 286 100 Z"
            fill="#020617"
            opacity="0.86"
          />
          <path
            d="M285 114 H333"
            stroke={flame.accent}
            strokeWidth="3"
            strokeLinecap="round"
            opacity="0.95"
          />
          <circle cx="323" cy="113" r="3.5" fill={flame.primary} />
          <path
            d="M163 78 L177 50 L197 82 Z M211 75 L226 48 L248 79 Z M276 83 L292 57 L313 88 Z"
            fill={dino.accent}
            stroke={dino.primary}
            strokeWidth="2"
            strokeLinejoin="round"
          />
          <path
            d="M169 190 L198 190 L193 220 H173 Z M239 191 L269 190 L264 220 H244 Z"
            fill={dino.secondary}
            stroke={dino.primary}
            strokeWidth="3"
            strokeLinejoin="round"
          />
          <path
            d="M160 219 H206 C211 219 215 223 215 228 V230 H154 V226 C154 222 157 219 160 219 Z M233 219 H279 C284 219 288 223 288 228 V230 H227 V226 C227 222 230 219 233 219 Z"
            fill="#020617"
            stroke={dino.primary}
            strokeOpacity="0.86"
            strokeWidth="2.5"
          />
          <path
            d="M155 143 C182 153 207 153 233 143 M183 176 C213 185 250 183 276 169 M205 104 C220 99 239 100 255 108"
            stroke="#020617"
            strokeWidth="6"
            strokeLinecap="round"
            opacity="0.16"
          />
          <path
            d="M155 143 C182 153 207 153 233 143 M183 176 C213 185 250 183 276 169 M205 104 C220 99 239 100 255 108"
            stroke={dino.primary}
            strokeWidth="1.5"
            strokeLinecap="round"
            opacity="0.68"
          />
        </g>

        <g opacity={scanlineOpacity}>
          <path d="M72 70 H348 M62 124 H358 M82 178 H338" stroke="#e2e8f0" strokeWidth="1" />
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
      title={t('dashboard.cyberDino.badgeTooltip', {
        tokens: formatNumber(totals.totalTokens),
        unit: t('dashboard.cyberDino.tokenUnit'),
      })}
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
