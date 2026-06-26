import type { UsageStats } from '@/lib/transport';

export type DinoMaterialID = 'cyber-metal' | 'hologram' | 'carbon-fiber' | 'pixel-neon';
export type FlameMaterialID = 'blue-plasma' | 'lava-core' | 'toxic-neon' | 'glitch-violet';

export type CyberDinoUsagePreferences = {
  dinoMaterial: DinoMaterialID;
  flameMaterial: FlameMaterialID;
  motion: 'calm' | 'alive' | 'off';
};

export type MonthlyUsageTotals = {
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  requests: number;
  cost: number;
};

export type BurnLevel = {
  level: 0 | 1 | 2 | 3 | 4;
  threshold: number;
  nextThreshold?: number;
  tone: 'idle' | 'warm' | 'steady' | 'hot' | 'inferno';
};

export type MaterialOption<ID extends string> = {
  id: ID;
  labelKey: string;
  descriptionKey: string;
  primary: string;
  secondary: string;
  accent: string;
};

export const DINO_MATERIALS: MaterialOption<DinoMaterialID>[] = [
  {
    id: 'cyber-metal',
    labelKey: 'dashboard.cyberDino.dinoMaterials.cyberMetal.label',
    descriptionKey: 'dashboard.cyberDino.dinoMaterials.cyberMetal.description',
    primary: '#31e6d5',
    secondary: '#155e75',
    accent: '#a855f7',
  },
  {
    id: 'hologram',
    labelKey: 'dashboard.cyberDino.dinoMaterials.hologram.label',
    descriptionKey: 'dashboard.cyberDino.dinoMaterials.hologram.description',
    primary: '#a5f3fc',
    secondary: '#6366f1',
    accent: '#f0abfc',
  },
  {
    id: 'carbon-fiber',
    labelKey: 'dashboard.cyberDino.dinoMaterials.carbonFiber.label',
    descriptionKey: 'dashboard.cyberDino.dinoMaterials.carbonFiber.description',
    primary: '#94a3b8',
    secondary: '#0f172a',
    accent: '#22d3ee',
  },
  {
    id: 'pixel-neon',
    labelKey: 'dashboard.cyberDino.dinoMaterials.pixelNeon.label',
    descriptionKey: 'dashboard.cyberDino.dinoMaterials.pixelNeon.description',
    primary: '#bef264',
    secondary: '#166534',
    accent: '#f472b6',
  },
];

export const FLAME_MATERIALS: MaterialOption<FlameMaterialID>[] = [
  {
    id: 'blue-plasma',
    labelKey: 'dashboard.cyberDino.flameMaterials.bluePlasma.label',
    descriptionKey: 'dashboard.cyberDino.flameMaterials.bluePlasma.description',
    primary: '#38bdf8',
    secondary: '#7c3aed',
    accent: '#fef08a',
  },
  {
    id: 'lava-core',
    labelKey: 'dashboard.cyberDino.flameMaterials.lavaCore.label',
    descriptionKey: 'dashboard.cyberDino.flameMaterials.lavaCore.description',
    primary: '#fb923c',
    secondary: '#dc2626',
    accent: '#fde68a',
  },
  {
    id: 'toxic-neon',
    labelKey: 'dashboard.cyberDino.flameMaterials.toxicNeon.label',
    descriptionKey: 'dashboard.cyberDino.flameMaterials.toxicNeon.description',
    primary: '#a3e635',
    secondary: '#14b8a6',
    accent: '#ecfccb',
  },
  {
    id: 'glitch-violet',
    labelKey: 'dashboard.cyberDino.flameMaterials.glitchViolet.label',
    descriptionKey: 'dashboard.cyberDino.flameMaterials.glitchViolet.description',
    primary: '#c084fc',
    secondary: '#db2777',
    accent: '#f0abfc',
  },
];

export const DEFAULT_CYBER_DINO_USAGE_PREFERENCES: CyberDinoUsagePreferences = {
  dinoMaterial: 'cyber-metal',
  flameMaterial: 'blue-plasma',
  motion: 'alive',
};

export function getCurrentMonthUsageWindow(now = new Date()): { start: Date; end: Date } {
  return {
    start: new Date(now.getFullYear(), now.getMonth(), 1),
    end: new Date(now.getFullYear(), now.getMonth() + 1, 1),
  };
}

export function aggregateMonthlyUsage(stats: UsageStats[] | undefined): MonthlyUsageTotals {
  return (stats ?? []).reduce<MonthlyUsageTotals>(
    (totals, item) => {
      totals.inputTokens += item.inputTokens;
      totals.outputTokens += item.outputTokens;
      totals.cacheReadTokens += item.cacheRead;
      totals.cacheWriteTokens += item.cacheWrite;
      totals.totalTokens += item.inputTokens + item.outputTokens + item.cacheRead + item.cacheWrite;
      totals.requests += item.totalRequests;
      totals.cost += item.cost;
      return totals;
    },
    {
      totalTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
      cacheReadTokens: 0,
      cacheWriteTokens: 0,
      requests: 0,
      cost: 0,
    },
  );
}

export function getBurnLevel(totalTokens: number): BurnLevel {
  if (totalTokens <= 0) {
    return { level: 0, threshold: 0, nextThreshold: 1_000_000, tone: 'idle' };
  }
  if (totalTokens < 1_000_000) {
    return { level: 1, threshold: 1, nextThreshold: 1_000_000, tone: 'warm' };
  }
  if (totalTokens < 10_000_000) {
    return { level: 2, threshold: 1_000_000, nextThreshold: 10_000_000, tone: 'steady' };
  }
  if (totalTokens < 50_000_000) {
    return { level: 3, threshold: 10_000_000, nextThreshold: 50_000_000, tone: 'hot' };
  }
  return { level: 4, threshold: 50_000_000, tone: 'inferno' };
}

export function getBurnProgress(totalTokens: number, level: BurnLevel): number {
  if (level.nextThreshold === undefined) return 100;
  const span = level.nextThreshold - level.threshold;
  if (span <= 0) return 0;
  return Math.min(100, Math.max(0, ((totalTokens - level.threshold) / span) * 100));
}

function isDinoMaterialID(value: unknown): value is DinoMaterialID {
  return DINO_MATERIALS.some((material) => material.id === value);
}

function isFlameMaterialID(value: unknown): value is FlameMaterialID {
  return FLAME_MATERIALS.some((material) => material.id === value);
}

function isMotionPreference(value: unknown): value is CyberDinoUsagePreferences['motion'] {
  return value === 'calm' || value === 'alive' || value === 'off';
}

export function normalizeCyberDinoUsagePreferences(value: unknown): CyberDinoUsagePreferences {
  if (!value || typeof value !== 'object') {
    return DEFAULT_CYBER_DINO_USAGE_PREFERENCES;
  }

  const candidate = value as Partial<CyberDinoUsagePreferences>;
  return {
    dinoMaterial: isDinoMaterialID(candidate.dinoMaterial)
      ? candidate.dinoMaterial
      : DEFAULT_CYBER_DINO_USAGE_PREFERENCES.dinoMaterial,
    flameMaterial: isFlameMaterialID(candidate.flameMaterial)
      ? candidate.flameMaterial
      : DEFAULT_CYBER_DINO_USAGE_PREFERENCES.flameMaterial,
    motion: isMotionPreference(candidate.motion)
      ? candidate.motion
      : DEFAULT_CYBER_DINO_USAGE_PREFERENCES.motion,
  };
}

export function materialByID<ID extends string>(
  materials: MaterialOption<ID>[],
  id: ID,
): MaterialOption<ID> {
  return materials.find((material) => material.id === id) ?? materials[0];
}
