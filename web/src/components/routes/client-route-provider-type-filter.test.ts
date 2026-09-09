import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const source = readFileSync(
  join(process.cwd(), 'src/components/routes/ClientTypeRoutesContent.tsx'),
  'utf8',
);

describe('client route provider type filter wiring', () => {
  it('keeps the provider type filter as local tab state', () => {
    expect(source).toContain("useState<RouteProviderTypeFilter>('all')");
    expect(source).toContain('setRouteProviderTypeFilter(value as RouteProviderTypeFilter)');
    expect(source).not.toContain('localStorage.setItem');
    expect(source).not.toContain('searchParams.set');
  });

  it('stacks text search with provider type filtering before sorting', () => {
    const searchIndex = source.indexOf('const searchFilteredRouteItems = useMemo');
    const typeIndex = source.indexOf('filterRouteProvidersByType(\n      searchFilteredRouteItems,');
    const sortIndex = source.indexOf('return filteredItems.slice().sort');

    expect(searchIndex).toBeGreaterThan(-1);
    expect(typeIndex).toBeGreaterThan(searchIndex);
    expect(sortIndex).toBeGreaterThan(typeIndex);
  });

  it('keeps bulk selection scoped to filtered visible route rows', () => {
    expect(source).toContain('const visibleRouteIds = useMemo(\n    () => items.flatMap');
    expect(source).toContain('setSelectedRouteIds(checked ? new Set(visibleRouteIds) : new Set())');
    expect(source).toContain('if (visibleRouteIdSet.has(id))');
  });

  it('shows the active filter count next to the selector', () => {
    expect(source).toContain("t('routes.providerTypeFilterCount'");
    expect(source).toContain('count: routeProviderTypeMatchCount');
    expect(source).toContain('total: routeProviderTypeTotalCount');
    expect(source).toContain("aria-label={t('routes.providerTypeFilterLabel')}");
  });
});
