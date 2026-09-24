import type { ProxyRequest } from '@/lib/transport/types';

const ACTIVE_STATUSES = new Set(['PENDING', 'IN_PROGRESS']);

export function isActiveProxyRequest(request: Pick<ProxyRequest, 'status'>): boolean {
  return ACTIVE_STATUSES.has(request.status);
}

function parsePositiveTimestamp(value?: string | null): number | null {
  if (!value) return null;
  const timestamp = new Date(value).getTime();
  return Number.isFinite(timestamp) && timestamp > 0 ? timestamp : null;
}

export function getRequestStartTimestampMs(
  request: Pick<ProxyRequest, 'startTime' | 'createdAt'>,
): number | null {
  return parsePositiveTimestamp(request.startTime) ?? parsePositiveTimestamp(request.createdAt);
}

export function getActiveRequestDurationMs(
  request: Pick<ProxyRequest, 'status' | 'startTime' | 'createdAt'>,
  nowMs: number,
): number | null {
  if (!isActiveProxyRequest(request)) return null;
  const startMs = getRequestStartTimestampMs(request);
  if (startMs === null || !Number.isFinite(nowMs)) return null;
  return Math.max(0, nowMs - startMs);
}

export function formatDurationNs(ns?: number | null): string {
  if (ns === undefined || ns === null || ns <= 0) return '-';
  return `${(ns / 1_000_000_000).toFixed(2)}s`;
}

export function formatDurationMs(ms?: number | null): string {
  if (ms === undefined || ms === null) return '-';
  return `${(ms / 1000).toFixed(2)}s`;
}

export function formatRequestDuration(
  request: Pick<ProxyRequest, 'status' | 'startTime' | 'createdAt' | 'duration'>,
  nowMs: number,
): string {
  const liveDurationMs = getActiveRequestDurationMs(request, nowMs);
  if (liveDurationMs !== null) return formatDurationMs(liveDurationMs);
  return formatDurationNs(request.duration);
}
