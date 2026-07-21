export type RequestColumnId =
  | 'time'
  | 'client'
  | 'model'
  | 'protocol'
  | 'reasoningEffort'
  | 'project'
  | 'token'
  | 'provider'
  | 'status'
  | 'code'
  | 'ttft'
  | 'duration'
  | 'attempts'
  | 'inputTokens'
  | 'outputTokens'
  | 'cacheRead'
  | 'cacheWrite'
  | 'cost';

export const REQUEST_COLUMN_IDS: readonly RequestColumnId[] = [
  'time',
  'client',
  'model',
  'protocol',
  'reasoningEffort',
  'project',
  'token',
  'provider',
  'status',
  'code',
  'ttft',
  'duration',
  'attempts',
  'inputTokens',
  'outputTokens',
  'cacheRead',
  'cacheWrite',
  'cost',
] as const;

/** Default column order (project/token may be filtered out when unavailable). */
export const DEFAULT_COLUMN_ORDER: readonly RequestColumnId[] = [...REQUEST_COLUMN_IDS];

/** Default widths in pixels. Model is intentionally narrower than the old fixed 200px. */
export const DEFAULT_COLUMN_WIDTHS: Readonly<Record<RequestColumnId, number>> = {
  time: 180,
  client: 120,
  model: 140,
  protocol: 88,
  reasoningEffort: 90,
  project: 100,
  token: 100,
  provider: 110,
  status: 100,
  code: 60,
  ttft: 60,
  duration: 80,
  attempts: 45,
  inputTokens: 65,
  outputTokens: 65,
  cacheRead: 65,
  cacheWrite: 65,
  cost: 80,
};

export const MIN_COLUMN_WIDTHS: Readonly<Record<RequestColumnId, number>> = {
  time: 120,
  client: 80,
  model: 90,
  protocol: 64,
  reasoningEffort: 60,
  project: 70,
  token: 70,
  provider: 80,
  status: 70,
  code: 44,
  ttft: 44,
  duration: 56,
  attempts: 36,
  inputTokens: 44,
  outputTokens: 44,
  cacheRead: 44,
  cacheWrite: 44,
  cost: 56,
};

export const MAX_COLUMN_WIDTH = 480;

/** Columns visible by default. */
export const DEFAULT_COLUMN_VISIBILITY: Readonly<Record<RequestColumnId, boolean>> = {
  time: true,
  client: true,
  model: true,
  protocol: true,
  reasoningEffort: true,
  project: true,
  token: true,
  provider: true,
  status: true,
  code: true,
  ttft: true,
  duration: true,
  attempts: true,
  inputTokens: true,
  outputTokens: true,
  cacheRead: true,
  cacheWrite: true,
  cost: true,
};

export const REQUEST_COLUMNS_STORAGE_KEY = 'maxx-requests-table-columns';

export type RequestColumnPrefs = {
  order: RequestColumnId[];
  widths: Record<RequestColumnId, number>;
  visibility: Record<RequestColumnId, boolean>;
};

export function createDefaultColumnPrefs(): RequestColumnPrefs {
  return {
    order: [...DEFAULT_COLUMN_ORDER],
    widths: { ...DEFAULT_COLUMN_WIDTHS },
    visibility: { ...DEFAULT_COLUMN_VISIBILITY },
  };
}

function isRequestColumnId(value: unknown): value is RequestColumnId {
  return typeof value === 'string' && (REQUEST_COLUMN_IDS as readonly string[]).includes(value);
}

function clampWidth(id: RequestColumnId, width: number): number {
  const min = MIN_COLUMN_WIDTHS[id];
  if (!Number.isFinite(width)) {
    return DEFAULT_COLUMN_WIDTHS[id];
  }
  return Math.min(MAX_COLUMN_WIDTH, Math.max(min, Math.round(width)));
}

/** Merge stored prefs with defaults; unknown ids dropped, missing ids appended. */
export function normalizeColumnPrefs(raw: unknown): RequestColumnPrefs {
  const defaults = createDefaultColumnPrefs();
  if (!raw || typeof raw !== 'object') {
    return defaults;
  }

  const input = raw as Partial<RequestColumnPrefs>;

  const seen = new Set<RequestColumnId>();
  const order: RequestColumnId[] = [];
  if (Array.isArray(input.order)) {
    for (const id of input.order) {
      if (isRequestColumnId(id) && !seen.has(id)) {
        order.push(id);
        seen.add(id);
      }
    }
  }
  for (const id of DEFAULT_COLUMN_ORDER) {
    if (!seen.has(id)) {
      order.push(id);
      seen.add(id);
    }
  }

  const widths = { ...DEFAULT_COLUMN_WIDTHS };
  if (input.widths && typeof input.widths === 'object') {
    for (const id of REQUEST_COLUMN_IDS) {
      const value = (input.widths as Record<string, unknown>)[id];
      if (typeof value === 'number') {
        widths[id] = clampWidth(id, value);
      }
    }
  }

  const visibility = { ...DEFAULT_COLUMN_VISIBILITY };
  if (input.visibility && typeof input.visibility === 'object') {
    for (const id of REQUEST_COLUMN_IDS) {
      const value = (input.visibility as Record<string, unknown>)[id];
      if (typeof value === 'boolean') {
        visibility[id] = value;
      }
    }
  }

  // Always keep at least one column visible.
  if (!REQUEST_COLUMN_IDS.some((id) => visibility[id])) {
    visibility.time = true;
  }

  return { order, widths, visibility };
}

export function readColumnPrefs(storageKey: string): RequestColumnPrefs {
  if (typeof window === 'undefined') {
    return createDefaultColumnPrefs();
  }
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (!raw) {
      return createDefaultColumnPrefs();
    }
    return normalizeColumnPrefs(JSON.parse(raw));
  } catch {
    return createDefaultColumnPrefs();
  }
}

export function writeColumnPrefs(storageKey: string, prefs: RequestColumnPrefs): void {
  if (typeof window === 'undefined') {
    return;
  }
  window.localStorage.setItem(storageKey, JSON.stringify(normalizeColumnPrefs(prefs)));
}

export type ColumnAvailability = {
  hasProjects: boolean;
  apiTokenAuthEnabled: boolean;
};

/** Columns that can appear in the table given feature flags. */
export function isColumnAvailable(id: RequestColumnId, availability: ColumnAvailability): boolean {
  if (id === 'project') {
    return availability.hasProjects;
  }
  if (id === 'token') {
    return availability.apiTokenAuthEnabled;
  }
  return true;
}

/** Ordered visible columns after applying prefs + feature availability. */
export function resolveVisibleColumns(
  prefs: RequestColumnPrefs,
  availability: ColumnAvailability,
): RequestColumnId[] {
  return prefs.order.filter(
    (id) => prefs.visibility[id] !== false && isColumnAvailable(id, availability),
  );
}

export function columnLabelKey(id: RequestColumnId): string {
  switch (id) {
    case 'time':
      return 'requests.time';
    case 'client':
      return 'requests.client';
    case 'model':
      return 'requests.model';
    case 'protocol':
      return 'requests.protocol';
    case 'reasoningEffort':
      return 'requests.reasoningEffort';
    case 'project':
      return 'requests.project';
    case 'token':
      return 'requests.token';
    case 'provider':
      return 'requests.provider';
    case 'status':
      return 'common.status';
    case 'code':
      return 'requests.code';
    case 'ttft':
      return 'requests.ttft';
    case 'duration':
      return 'requests.duration';
    case 'attempts':
      return 'requests.attempts';
    case 'inputTokens':
      return 'requests.inputTokens';
    case 'outputTokens':
      return 'requests.outputTokens';
    case 'cacheRead':
      return 'requests.cacheRead';
    case 'cacheWrite':
      return 'requests.cacheWrite';
    case 'cost':
      return 'requests.cost';
  }
}

export function columnShortLabelKey(id: RequestColumnId): string | null {
  switch (id) {
    case 'ttft':
      return null; // display literal "TTFT"
    case 'attempts':
      return 'requests.attShort';
    case 'inputTokens':
      return 'requests.inShort';
    case 'outputTokens':
      return 'requests.outShort';
    case 'cacheRead':
      return 'requests.cacheRShort';
    case 'cacheWrite':
      return 'requests.cacheWShort';
    default:
      return null;
  }
}

export function isCenteredColumn(id: RequestColumnId): boolean {
  return (
    id === 'protocol' ||
    id === 'reasoningEffort' ||
    id === 'code' ||
    id === 'ttft' ||
    id === 'duration' ||
    id === 'attempts' ||
    id === 'inputTokens' ||
    id === 'outputTokens' ||
    id === 'cacheRead' ||
    id === 'cacheWrite' ||
    id === 'cost'
  );
}
