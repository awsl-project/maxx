import type { Route, UsageStats } from '@/lib/transport';

const MIN_SUCCESSFUL_REQUESTS_FOR_TTFT_SORT = 3;

type RouteLike = Pick<Route, 'id' | 'position'>;

type UsageStatsLike = Pick<UsageStats, 'routeID' | 'successfulRequests' | 'totalTtftMs'>;

export interface RouteTtftSummary {
  routeID: number;
  successfulRequests: number;
  avgTtftMs: number;
}

export interface TtftRoutePositionUpdate {
  id: number;
  position: number;
}

export function summarizeRouteTtft(stats: UsageStatsLike[] | undefined): Map<number, RouteTtftSummary> {
  const summaries = new Map<number, RouteTtftSummary>();

  for (const stat of stats ?? []) {
    if (stat.routeID === 0 || stat.successfulRequests <= 0 || stat.totalTtftMs <= 0) continue;

    const existing = summaries.get(stat.routeID);
    if (existing) {
      const totalTtftMs = existing.avgTtftMs * existing.successfulRequests + stat.totalTtftMs;
      const successfulRequests = existing.successfulRequests + stat.successfulRequests;
      summaries.set(stat.routeID, {
        routeID: stat.routeID,
        successfulRequests,
        avgTtftMs: totalTtftMs / successfulRequests,
      });
      continue;
    }

    summaries.set(stat.routeID, {
      routeID: stat.routeID,
      successfulRequests: stat.successfulRequests,
      avgTtftMs: stat.totalTtftMs / stat.successfulRequests,
    });
  }

  return summaries;
}

export function buildTtftRoutePositionUpdates(
  routes: RouteLike[],
  stats: UsageStatsLike[] | undefined,
  minSuccessfulRequests = MIN_SUCCESSFUL_REQUESTS_FOR_TTFT_SORT,
): TtftRoutePositionUpdate[] {
  const summaries = summarizeRouteTtft(stats);
  const originalOrder = routes.slice().sort((a, b) => {
    const posDiff = (a.position ?? 0) - (b.position ?? 0);
    if (posDiff !== 0) return posDiff;
    return a.id - b.id;
  });
  const originalIndexByRouteId = new Map(originalOrder.map((route, index) => [route.id, index]));

  const sortableRoutes = originalOrder.filter((route) => {
    const summary = summaries.get(route.id);
    return summary && summary.successfulRequests >= minSuccessfulRequests;
  });

  const unsortedRoutes = originalOrder.filter((route) => !sortableRoutes.includes(route));

  sortableRoutes.sort((a, b) => {
    const aSummary = summaries.get(a.id)!;
    const bSummary = summaries.get(b.id)!;
    const ttftDiff = aSummary.avgTtftMs - bSummary.avgTtftMs;
    if (ttftDiff !== 0) return ttftDiff;
    const requestDiff = bSummary.successfulRequests - aSummary.successfulRequests;
    if (requestDiff !== 0) return requestDiff;
    return (originalIndexByRouteId.get(a.id) ?? 0) - (originalIndexByRouteId.get(b.id) ?? 0);
  });

  const sortedQueue = [...sortableRoutes];
  const stableQueue = [...unsortedRoutes];
  const reordered = originalOrder.map((route) => {
    const summary = summaries.get(route.id);
    if (summary && summary.successfulRequests >= minSuccessfulRequests) {
      return sortedQueue.shift()!;
    }
    return stableQueue.shift()!;
  });

  return reordered
    .map((route, index) => ({ id: route.id, position: index + 1 }))
    .filter((update) => {
      const route = routes.find((candidate) => candidate.id === update.id);
      return route?.position !== update.position;
    });
}
