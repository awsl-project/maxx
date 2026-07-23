import { describe, expect, it } from 'vitest';
import { getProviderCreatePath, PROVIDER_CREATE_PATHS } from './use-provider-navigation';
import { PROVIDER_TYPE_CONFIGS, PROVIDER_TYPE_ORDER } from '../types';

describe('provider create navigation', () => {
  it('routes every configured provider type to a create step', () => {
    for (const type of PROVIDER_TYPE_ORDER) {
      expect(getProviderCreatePath(type)).toBe(PROVIDER_CREATE_PATHS[type]);
      expect(getProviderCreatePath(type)).toMatch(/^\/providers\/create\//);
    }
  });

  it('routes the visible Custom Provider card to the custom configuration step', () => {
    expect(PROVIDER_TYPE_CONFIGS.custom.hidden).toBeFalsy();
    expect(getProviderCreatePath('custom')).toBe('/providers/create/custom');
  });
});
