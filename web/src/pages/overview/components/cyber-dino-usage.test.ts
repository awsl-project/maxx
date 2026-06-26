import { describe, expect, it } from 'vitest';
import type { UsageStats } from '@/lib/transport';
import {
  aggregateMonthlyUsage,
  getBurnLevel,
  getBurnProgress,
  getCurrentMonthUsageWindow,
  normalizeCyberDinoUsagePreferences,
} from './cyber-dino-usage';

function usage(partial: Partial<UsageStats>): UsageStats {
  return {
    id: 1,
    createdAt: '2099-06-01T00:00:00.000Z',
    timeBucket: '2099-06-01T00:00:00.000Z',
    granularity: 'day',
    routeID: 0,
    providerID: 0,
    projectID: 0,
    apiTokenID: 0,
    clientType: 'claude',
    model: 'test-model',
    totalRequests: 0,
    successfulRequests: 0,
    failedRequests: 0,
    totalDurationMs: 0,
    totalTtftMs: 0,
    inputTokens: 0,
    outputTokens: 0,
    cacheRead: 0,
    cacheWrite: 0,
    cost: 0,
    ...partial,
  };
}

describe('cyber dino monthly usage helpers', () => {
  it('builds a stable current-month local time window', () => {
    const { start, end } = getCurrentMonthUsageWindow(new Date(2099, 5, 26, 19, 30));

    expect(start).toEqual(new Date(2099, 5, 1));
    expect(end).toEqual(new Date(2099, 6, 1));
  });

  it('aggregates input, output, cache read and cache write tokens into the token burn total', () => {
    const totals = aggregateMonthlyUsage([
      usage({
        inputTokens: 100,
        outputTokens: 50,
        cacheRead: 25,
        cacheWrite: 10,
        totalRequests: 2,
        cost: 123,
      }),
      usage({
        inputTokens: 200,
        outputTokens: 75,
        cacheRead: 5,
        cacheWrite: 15,
        totalRequests: 3,
        cost: 456,
      }),
    ]);

    expect(totals).toEqual({
      totalTokens: 480,
      inputTokens: 300,
      outputTokens: 125,
      cacheReadTokens: 30,
      cacheWriteTokens: 25,
      requests: 5,
      cost: 579,
    });
  });

  it('maps token totals to stable burn levels', () => {
    expect(getBurnLevel(0)).toMatchObject({ level: 0, tone: 'idle', nextThreshold: 1_000_000 });
    expect(getBurnLevel(999_999)).toMatchObject({ level: 1, tone: 'warm' });
    expect(getBurnLevel(1_000_000)).toMatchObject({ level: 2, tone: 'steady' });
    expect(getBurnLevel(10_000_000)).toMatchObject({ level: 3, tone: 'hot' });
    expect(getBurnLevel(50_000_000)).toMatchObject({ level: 4, tone: 'inferno' });
  });

  it('calculates progress within the current burn band', () => {
    expect(getBurnProgress(5_500_000, getBurnLevel(5_500_000))).toBe(50);
    expect(getBurnProgress(80_000_000, getBurnLevel(80_000_000))).toBe(100);
  });

  it('normalizes persisted material preferences and rejects unknown values', () => {
    expect(
      normalizeCyberDinoUsagePreferences({
        dinoMaterial: 'pixel-neon',
        flameMaterial: 'glitch-violet',
        motion: 'off',
      }),
    ).toEqual({
      dinoMaterial: 'pixel-neon',
      flameMaterial: 'glitch-violet',
      motion: 'off',
    });

    expect(
      normalizeCyberDinoUsagePreferences({
        dinoMaterial: 'unknown-dino',
        flameMaterial: 'unknown-flame',
        motion: 'chaos',
      }),
    ).toEqual({
      dinoMaterial: 'cyber-metal',
      flameMaterial: 'blue-plasma',
      motion: 'alive',
    });
  });
});
