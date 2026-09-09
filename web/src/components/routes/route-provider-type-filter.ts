export type RouteProviderTypeFilter =
  | 'all'
  | 'custom'
  | 'codex'
  | 'claude'
  | 'openai'
  | 'gemini'
  | 'antigravity'
  | 'other';

export const ROUTE_PROVIDER_TYPE_FILTERS: RouteProviderTypeFilter[] = [
  'all',
  'custom',
  'codex',
  'claude',
  'openai',
  'gemini',
  'antigravity',
  'other',
];

export const ROUTE_PROVIDER_TYPE_FILTER_LABELS: Record<
  Exclude<RouteProviderTypeFilter, 'all'>,
  string
> = {
  custom: 'Custom',
  codex: 'Codex',
  claude: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  other: 'Other',
};

export function getRouteProviderTypeFilter(
  providerType: string,
): Exclude<RouteProviderTypeFilter, 'all'> {
  switch (providerType.toLowerCase()) {
    case 'custom':
    case 'codex':
    case 'claude':
    case 'openai':
    case 'gemini':
    case 'antigravity':
      return providerType.toLowerCase() as Exclude<RouteProviderTypeFilter, 'all' | 'other'>;
    default:
      return 'other';
  }
}

export function filterRouteProvidersByType<T extends { provider: { type: string } }>(
  items: readonly T[],
  filter: RouteProviderTypeFilter,
): T[] {
  if (filter === 'all') return [...items];
  return items.filter((item) => getRouteProviderTypeFilter(item.provider.type) === filter);
}

export function countRouteProvidersByType<T extends { provider: { type: string } }>(
  items: readonly T[],
): Map<Exclude<RouteProviderTypeFilter, 'all'>, number> {
  const counts = new Map<Exclude<RouteProviderTypeFilter, 'all'>, number>();
  for (const item of items) {
    const type = getRouteProviderTypeFilter(item.provider.type);
    counts.set(type, (counts.get(type) ?? 0) + 1);
  }
  return counts;
}
