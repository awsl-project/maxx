import { describe, expect, it } from 'vitest';
import { normalizeProviderArrayField, normalizeProviderList } from './provider-normalize';
import type { Provider } from '@/lib/transport';

describe('provider-normalize', () => {
  it('normalizes null provider lists to an empty array', () => {
    expect(normalizeProviderList(null)).toEqual([]);
    expect(normalizeProviderList(undefined)).toEqual([]);
  });

  it('preserves provider arrays', () => {
    const providers = [{ id: 1, name: 'custom', type: 'custom' }] as Provider[];
    expect(normalizeProviderList(providers)).toBe(providers);
  });

  it('normalizes nullable provider array fields', () => {
    expect(normalizeProviderArrayField<string>(null)).toEqual([]);
    expect(normalizeProviderArrayField<string>(undefined)).toEqual([]);
    expect(normalizeProviderArrayField(['gpt-5'])).toEqual(['gpt-5']);
  });
});
