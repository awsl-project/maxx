import { describe, expect, it } from 'vitest';

import { buildTtftRoutePositionUpdates, summarizeRouteTtft } from './ttft-sort';

describe('TTFT route sorting helpers', () => {
  it('aggregates average TTFT by route using successful requests', () => {
    const summaries = summarizeRouteTtft([
      { routeID: 1, successfulRequests: 2, totalTtftMs: 400 },
      { routeID: 1, successfulRequests: 2, totalTtftMs: 200 },
      { routeID: 2, successfulRequests: 0, totalTtftMs: 100 },
    ]);

    expect(summaries.get(1)).toEqual({
      routeID: 1,
      successfulRequests: 4,
      avgTtftMs: 150,
    });
    expect(summaries.has(2)).toBe(false);
  });

  it('moves sufficiently sampled faster routes earlier while leaving low-sample routes in place', () => {
    const updates = buildTtftRoutePositionUpdates(
      [
        { id: 1, position: 1 },
        { id: 2, position: 2 },
        { id: 3, position: 3 },
        { id: 4, position: 4 },
      ],
      [
        { routeID: 1, successfulRequests: 10, totalTtftMs: 10000 },
        { routeID: 2, successfulRequests: 2, totalTtftMs: 20 },
        { routeID: 3, successfulRequests: 10, totalTtftMs: 1000 },
        { routeID: 4, successfulRequests: 10, totalTtftMs: 5000 },
      ],
    );

    expect(updates).toEqual([
      { id: 3, position: 1 },
      { id: 4, position: 3 },
      { id: 1, position: 4 },
    ]);
  });

  it('keeps the current order when no route has enough TTFT samples', () => {
    expect(
      buildTtftRoutePositionUpdates(
        [
          { id: 1, position: 1 },
          { id: 2, position: 2 },
        ],
        [
          { routeID: 1, successfulRequests: 1, totalTtftMs: 20 },
          { routeID: 2, successfulRequests: 0, totalTtftMs: 0 },
        ],
      ),
    ).toEqual([]);
  });
});
