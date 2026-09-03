import type { ProxyRequest } from '@/lib/transport';

export type UserPanelModelHealth = 'healthy' | 'degraded' | 'no-data' | 'unavailable';

export interface UserPanelModelStatusSummary {
  model: string;
  health: UserPanelModelHealth;
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  consecutiveFailures: number;
  lastRequestedAt?: string;
  lastError?: string;
}

const RECENT_WINDOW_MS = 24 * 60 * 60 * 1000;

function modelNameFromRequest(request: ProxyRequest): string {
  return request.requestModel || request.responseModel || request.mappedModel || '';
}

function isRequestFailure(request: ProxyRequest): boolean {
  return request.status === 'FAILED' || request.status === 'REJECTED' || request.statusCode >= 400;
}

function isRequestSuccess(request: ProxyRequest): boolean {
  return request.status === 'COMPLETED' && request.statusCode > 0 && request.statusCode < 400;
}

function requestTime(request: ProxyRequest): number {
  const value = new Date(request.createdAt).getTime();
  return Number.isFinite(value) ? value : 0;
}

function trimError(value: string): string | undefined {
  const error = value.trim();
  if (!error) return undefined;
  return error.length > 160 ? `${error.slice(0, 157)}...` : error;
}

function resolveHealth(params: {
  totalRequests: number;
  failedRequests: number;
  consecutiveFailures: number;
}): UserPanelModelHealth {
  if (params.totalRequests === 0) return 'no-data';
  if (params.consecutiveFailures >= 3) return 'unavailable';
  if (params.failedRequests > 0) return 'degraded';
  return 'healthy';
}

export function buildUserPanelModelStatuses(params: {
  availableModels?: string[];
  requests?: ProxyRequest[];
  now?: number;
}): UserPanelModelStatusSummary[] {
  const now = params.now ?? Date.now();
  const since = now - RECENT_WINDOW_MS;
  const summaries = new Map<string, UserPanelModelStatusSummary>();

  for (const model of params.availableModels ?? []) {
    const name = model.trim();
    if (!name) continue;
    summaries.set(name, {
      model: name,
      health: 'no-data',
      totalRequests: 0,
      successfulRequests: 0,
      failedRequests: 0,
      consecutiveFailures: 0,
    });
  }

  const recentRequests = (params.requests ?? [])
    .filter((request) => requestTime(request) >= since)
    .filter((request) => modelNameFromRequest(request).trim() !== '')
    .sort((a, b) => requestTime(b) - requestTime(a));

  for (const request of recentRequests) {
    const model = modelNameFromRequest(request).trim();
    const existing = summaries.get(model) ?? {
      model,
      health: 'no-data' as UserPanelModelHealth,
      totalRequests: 0,
      successfulRequests: 0,
      failedRequests: 0,
      consecutiveFailures: 0,
    };

    const failed = isRequestFailure(request);
    const successful = isRequestSuccess(request);
    existing.totalRequests += 1;
    if (successful) existing.successfulRequests += 1;
    if (failed) existing.failedRequests += 1;
    if (!existing.lastRequestedAt) existing.lastRequestedAt = request.createdAt;
    if (failed && !existing.lastError) existing.lastError = trimError(request.error);
    summaries.set(model, existing);
  }

  for (const summary of summaries.values()) {
    const modelRequests = recentRequests.filter(
      (request) => modelNameFromRequest(request).trim() === summary.model,
    );
    let consecutiveFailures = 0;
    for (const request of modelRequests) {
      if (!isRequestFailure(request)) break;
      consecutiveFailures += 1;
    }
    summary.consecutiveFailures = consecutiveFailures;
    summary.health = resolveHealth(summary);
  }

  return [...summaries.values()].sort((a, b) => {
    const order: Record<UserPanelModelHealth, number> = {
      unavailable: 0,
      degraded: 1,
      healthy: 2,
      'no-data': 3,
    };
    const byHealth = order[a.health] - order[b.health];
    if (byHealth !== 0) return byHealth;
    return a.model.localeCompare(b.model);
  });
}
