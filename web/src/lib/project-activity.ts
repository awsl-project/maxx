import type { Project } from './transport';

const DAY_MS = 24 * 60 * 60 * 1000;

export type ProjectActivityStatus = 'active' | 'inactive' | 'never-used';

export interface ProjectActivityState {
  status: ProjectActivityStatus;
  inactive: boolean;
  unusedDays: number | null;
  lastUsedAt: string | null;
  lastSuccessfulRequestAt: string | null;
  requestCount30d: number;
  successfulRequestCount30d: number;
  totalRequestCount: number;
}

function parseTime(value?: string | null): number | null {
  if (!value) {
    return null;
  }
  const time = new Date(value).getTime();
  return Number.isFinite(time) ? time : null;
}

export function getProjectActivityState(
  project: Project,
  thresholdDays: number,
  nowMs = Date.now(),
): ProjectActivityState {
  const lastRequestMs = parseTime(project.lastRequestAt);
  const lastSuccessfulRequestMs = parseTime(project.lastSuccessfulRequestAt);
  const requestCount30d = project.requestCount30d ?? 0;
  const successfulRequestCount30d = project.successfulRequestCount30d ?? 0;
  const totalRequestCount = project.totalRequestCount ?? 0;

  if (lastRequestMs === null) {
    return {
      status: 'never-used',
      inactive: true,
      unusedDays: null,
      lastUsedAt: null,
      lastSuccessfulRequestAt: project.lastSuccessfulRequestAt ?? null,
      requestCount30d,
      successfulRequestCount30d,
      totalRequestCount,
    };
  }

  const unusedDays = Math.max(0, Math.floor((nowMs - lastRequestMs) / DAY_MS));
  const inactive = unusedDays >= thresholdDays;

  return {
    status: inactive ? 'inactive' : 'active',
    inactive,
    unusedDays,
    lastUsedAt: project.lastRequestAt ?? null,
    lastSuccessfulRequestAt:
      lastSuccessfulRequestMs === null ? null : (project.lastSuccessfulRequestAt ?? null),
    requestCount30d,
    successfulRequestCount30d,
    totalRequestCount,
  };
}
