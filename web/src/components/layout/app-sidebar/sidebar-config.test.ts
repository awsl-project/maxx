import { describe, expect, it } from 'vitest';
import { sidebarConfig } from './sidebar-config';
import type { MenuItem } from '@/types/sidebar';

function findItem(key: string): MenuItem | undefined {
  return sidebarConfig.sections
    .flatMap((section) => section.items)
    .find((item) => item.key === key);
}

describe('sidebarConfig', () => {
  it('keeps API tokens visible when admin auth is disabled', () => {
    const item = findItem('api-tokens');

    expect(item).toMatchObject({
      type: 'standard',
      to: '/api-tokens',
      adminOnly: true,
    });
    expect(item).not.toHaveProperty('authOnly', true);
  });

  it('exposes API token limits as an authenticated admin config page', () => {
    const item = findItem('api-token-limits');

    expect(item).toMatchObject({
      type: 'standard',
      to: '/api-token-limits',
      labelKey: 'nav.apiTokenLimits',
      adminOnly: true,
      authOnly: true,
    });
  });
});
