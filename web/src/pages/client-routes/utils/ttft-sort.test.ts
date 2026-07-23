import { describe, expect, it } from 'vitest';

import { buildTtftRoutePositionUpdates, summarizeTtftProbeResults } from './ttft-sort';

describe('TTFT route sorting helpers', () => {
  it('sorts successful real probe results by measured TTFT and moves failed probes after successes', () => {
    const updates = buildTtftRoutePositionUpdates(
      [
        { id: 1, position: 1, isEnabled: true, providerID: 1 },
        { id: 2, position: 2, isEnabled: true, providerID: 2 },
        { id: 3, position: 3, isEnabled: true, providerID: 3 },
        { id: 4, position: 4, isEnabled: false, providerID: 4 },
      ],
      [
        {
          routeID: 1,
          providerID: 1,
          providerName: 'slow',
          ok: true,
          status: 'success',
          metric: 'ttft',
          ttftMs: 300,
          durationMs: 350,
        },
        {
          routeID: 2,
          providerID: 2,
          providerName: 'fast',
          ok: true,
          status: 'success',
          metric: 'ttft',
          ttftMs: 80,
          durationMs: 120,
        },
        {
          routeID: 3,
          providerID: 3,
          providerName: 'dead',
          ok: false,
          status: 'timeout',
          metric: 'none',
          durationMs: 30000,
        },
      ],
    );

    expect(updates).toEqual([
      { id: 2, position: 1 },
      { id: 1, position: 2 },
    ]);
  });

  it('keeps unprobed routes in their original slots while sorting probed routes', () => {
    const updates = buildTtftRoutePositionUpdates(
      [
        { id: 1, position: 1, isEnabled: true, providerID: 1 },
        { id: 2, position: 2, isEnabled: false, providerID: 2 },
        { id: 3, position: 3, isEnabled: true, providerID: 3 },
      ],
      [
        {
          routeID: 1,
          providerID: 1,
          providerName: 'slow',
          ok: true,
          status: 'success',
          metric: 'ttft',
          ttftMs: 200,
          durationMs: 230,
        },
        {
          routeID: 3,
          providerID: 3,
          providerName: 'fast',
          ok: true,
          status: 'success',
          metric: 'ttft',
          ttftMs: 50,
          durationMs: 80,
        },
      ],
    );

    expect(updates).toEqual([
      { id: 3, position: 1 },
      { id: 1, position: 3 },
    ]);
  });

  it('summarizes probe outcomes for UI feedback', () => {
    const summary = summarizeTtftProbeResults([
      {
        routeID: 1,
        providerID: 1,
        providerName: 'a',
        ok: true,
        status: 'success',
        metric: 'ttft',
        ttftMs: 90,
        durationMs: 110,
      },
      {
        routeID: 2,
        providerID: 2,
        providerName: 'b',
        ok: false,
        status: 'http_error',
        metric: 'none',
        durationMs: 30,
        httpStatus: 500,
      },
      {
        routeID: 3,
        providerID: 3,
        providerName: 'c',
        ok: true,
        status: 'success',
        metric: 'ttft',
        ttftMs: 40,
        durationMs: 60,
      },
    ]);

    expect(summary).toMatchObject({ total: 3, successful: 2, failed: 1 });
    expect(summary.fastest?.providerName).toBe('c');
  });
});
