import { describe, expect, it } from 'vitest';
import {
  countRouteProvidersByType,
  filterRouteProvidersByType,
  getRouteProviderTypeFilter,
  ROUTE_PROVIDER_TYPE_FILTERS,
} from './route-provider-type-filter';

const routeItems = [
  { id: 1, provider: { name: 'Custom A', type: 'custom' } },
  { id: 2, provider: { name: 'Codex A', type: 'codex' } },
  { id: 3, provider: { name: 'Codex B', type: 'codex' } },
  { id: 4, provider: { name: 'Claude A', type: 'claude' } },
  { id: 5, provider: { name: 'OpenAI A', type: 'openai' } },
  { id: 6, provider: { name: 'Gemini A', type: 'gemini' } },
  { id: 7, provider: { name: 'Antigravity A', type: 'antigravity' } },
  { id: 8, provider: { name: 'Bedrock A', type: 'bedrock' } },
];

describe('route provider type filter helpers', () => {
  it('offers all requested filter buckets', () => {
    expect(ROUTE_PROVIDER_TYPE_FILTERS).toEqual([
      'all',
      'custom',
      'codex',
      'claude',
      'openai',
      'gemini',
      'antigravity',
      'other',
    ]);
  });

  it('maps known provider types and sends unlisted provider types to other', () => {
    expect(getRouteProviderTypeFilter('codex')).toBe('codex');
    expect(getRouteProviderTypeFilter('OpenAI')).toBe('openai');
    expect(getRouteProviderTypeFilter('bedrock')).toBe('other');
    expect(getRouteProviderTypeFilter('future-provider')).toBe('other');
  });

  it('filters route rows by provider type without mutating the source list', () => {
    const codex = filterRouteProvidersByType(routeItems, 'codex');
    const all = filterRouteProvidersByType(routeItems, 'all');

    expect(codex.map((item) => item.id)).toEqual([2, 3]);
    expect(all.map((item) => item.id)).toEqual(routeItems.map((item) => item.id));
    expect(all).not.toBe(routeItems);
  });

  it('counts filtered route rows by display bucket', () => {
    const counts = countRouteProvidersByType(routeItems);

    expect(counts.get('custom')).toBe(1);
    expect(counts.get('codex')).toBe(2);
    expect(counts.get('claude')).toBe(1);
    expect(counts.get('openai')).toBe(1);
    expect(counts.get('gemini')).toBe(1);
    expect(counts.get('antigravity')).toBe(1);
    expect(counts.get('other')).toBe(1);
  });
});
