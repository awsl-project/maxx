import type { Route } from '@/lib/transport';

export interface RouteTtftProbeResult {
  routeID: number;
  providerID: number;
  providerName: string;
  ok: boolean;
  status: 'success' | 'timeout' | 'http_error' | 'network_error' | 'unsupported' | 'cancelled';
  metric: 'ttft' | 'first_byte' | 'none';
  ttftMs?: number;
  durationMs: number;
  httpStatus?: number;
  error?: string;
}

export interface TtftRoutePositionUpdate {
  id: number;
  position: number;
}

type RouteLike = Pick<Route, 'id' | 'position' | 'isEnabled' | 'providerID'>;

export function buildTtftRoutePositionUpdates(
  routes: RouteLike[],
  results: RouteTtftProbeResult[],
): TtftRoutePositionUpdate[] {
  const originalOrder = routes.slice().sort((a, b) => {
    const posDiff = (a.position ?? 0) - (b.position ?? 0);
    if (posDiff !== 0) return posDiff;
    return a.id - b.id;
  });
  const originalIndexByRouteId = new Map(originalOrder.map((route, index) => [route.id, index]));
  const resultByRouteId = new Map(results.map((result) => [result.routeID, result]));

  const probedRoutes = originalOrder.filter((route) => resultByRouteId.has(route.id));
  if (probedRoutes.length <= 1) return [];

  const sortedProbedRoutes = probedRoutes.slice().sort((a, b) => {
    const aResult = resultByRouteId.get(a.id)!;
    const bResult = resultByRouteId.get(b.id)!;

    if (aResult.ok !== bResult.ok) return aResult.ok ? -1 : 1;
    if (aResult.ok && bResult.ok) {
      const ttftDiff =
        (aResult.ttftMs ?? Number.MAX_SAFE_INTEGER) - (bResult.ttftMs ?? Number.MAX_SAFE_INTEGER);
      if (ttftDiff !== 0) return ttftDiff;
    }

    const statusDiff = aResult.status.localeCompare(bResult.status);
    if (!aResult.ok && !bResult.ok && statusDiff !== 0) return statusDiff;
    return (originalIndexByRouteId.get(a.id) ?? 0) - (originalIndexByRouteId.get(b.id) ?? 0);
  });

  const sortedQueue = [...sortedProbedRoutes];
  const reordered = originalOrder.map((route) => {
    if (resultByRouteId.has(route.id)) {
      return sortedQueue.shift()!;
    }
    return route;
  });

  return reordered
    .map((route, index) => ({ id: route.id, position: index + 1 }))
    .filter((update) => {
      const route = routes.find((candidate) => candidate.id === update.id);
      return route?.position !== update.position;
    });
}

export function summarizeTtftProbeResults(results: RouteTtftProbeResult[]) {
  const successful = results.filter((result) => result.ok);
  const failed = results.length - successful.length;
  const fastest = successful.reduce<RouteTtftProbeResult | undefined>((best, result) => {
    if (!best) return result;
    return (result.ttftMs ?? Number.MAX_SAFE_INTEGER) < (best.ttftMs ?? Number.MAX_SAFE_INTEGER)
      ? result
      : best;
  }, undefined);

  return {
    total: results.length,
    successful: successful.length,
    failed,
    fastest,
  };
}
